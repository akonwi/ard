package checker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

type GoPackagesResolver struct {
	ProjectRoot   string
	BuildTags     []string
	modulePath    string
	modulePathErr error
	cache         map[string]goPackageResolveResult
	// primed marks that the whole-program import pre-scan has loaded every
	// Go package into one shared go/types universe (ADR 0044). After
	// priming, a cache miss means the pre-scan failed to collect a path,
	// which is a compiler bug, not a load trigger: issuing a fresh load
	// would silently create a second type universe.
	primed bool
}

type goPackageResolveResult struct {
	pkg *GoPackage
	err error
}

// JSONV2BuildTag enables encoding/json/v2, which generated Ard output
// imports for union marshalling. It is part of the generated output's
// contract, so both go/packages resolution and generated builds apply it
// unconditionally — the checker must see the same build configuration the
// backend compiles. Drop when json/v2 leaves its Go experiment.
const JSONV2BuildTag = "goexperiment.jsonv2"

// resolutionBuildTags prepends the compiler-owned jsonv2 tag to the
// project's configured tags, deduplicating.
func resolutionBuildTags(buildTags []string) []string {
	tags := []string{JSONV2BuildTag}
	for _, tag := range buildTags {
		if tag != JSONV2BuildTag {
			tags = append(tags, tag)
		}
	}
	return tags
}

func NewGoPackagesResolver(projectRoot string, buildTags []string) *GoPackagesResolver {
	if absRoot, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = absRoot
	}
	modulePath, modulePathErr := readGoModulePath(projectRoot)
	if os.IsNotExist(modulePathErr) {
		modulePathErr = nil
	}
	return &GoPackagesResolver{ProjectRoot: projectRoot, BuildTags: resolutionBuildTags(buildTags), modulePath: modulePath, modulePathErr: modulePathErr, cache: map[string]goPackageResolveResult{}}
}

func (r *GoPackagesResolver) ResolveGoPackage(path string) (*GoPackage, error) {
	if r.cache == nil {
		r.cache = map[string]goPackageResolveResult{}
	}
	if cached, ok := r.cache[path]; ok {
		return cached.pkg, cached.err
	}
	// Every resolution comes from the primed session (ADR 0044): a lazy
	// per-path load here would silently create a second go/types universe.
	return nil, fmt.Errorf("internal compiler bug: Go package %q was not collected by the import pre-scan; please report this", path)
}

// Prime loads every given Go import path in a single go/packages call so all
// resolved packages share one go/types universe (ADR 0044). Failures —
// including a load session that cannot run at all — are recorded per path
// and surface as diagnostics at the importing `use` statement.
//
// Priming is a one-shot operation. Once primed, paths outside the primed set
// indicate an incomplete pre-scan: loading them would silently create a
// second type universe, so Prime reports the internal error instead.
func (r *GoPackagesResolver) Prime(paths []string) error {
	if r.cache == nil {
		r.cache = map[string]goPackageResolveResult{}
	}
	pending := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		if _, cached := r.cache[path]; cached {
			continue
		}
		pending = append(pending, path)
	}
	if len(pending) == 0 {
		r.primed = true
		return nil
	}
	if r.primed {
		return fmt.Errorf("internal compiler bug: Go packages %v were not collected by the import pre-scan; please report this", pending)
	}
	defer func() { r.primed = true }()
	if r.modulePathErr != nil {
		r.recordFailure(pending, fmt.Errorf("read go.mod: %w", r.modulePathErr))
		return nil
	}
	loaded, err := packages.Load(r.loadConfig(), pending...)
	if err != nil {
		r.recordFailure(pending, err)
		return nil
	}
	byPath := make(map[string]*packages.Package, len(loaded))
	for _, pkg := range loaded {
		byPath[pkg.PkgPath] = pkg
	}
	for _, path := range pending {
		pkg, ok := byPath[path]
		if !ok {
			r.cache[path] = goPackageResolveResult{err: fmt.Errorf("package %q not found", path)}
			continue
		}
		goPkg, pkgErr := r.packageFromLoadResult(path, pkg)
		r.cache[path] = goPackageResolveResult{pkg: goPkg, err: pkgErr}
	}
	return nil
}

// recordFailure caches a session-level load failure for every pending path
// so it surfaces as a source-located diagnostic at each Go import.
func (r *GoPackagesResolver) recordFailure(paths []string, err error) {
	for _, path := range paths {
		r.cache[path] = goPackageResolveResult{err: err}
	}
}

func (r *GoPackagesResolver) loadConfig() *packages.Config {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedFiles,
		Dir:   r.ProjectRoot,
		Tests: false,
	}
	if len(r.BuildTags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(r.BuildTags, ",")}
	}
	return cfg
}

func (r *GoPackagesResolver) packageFromLoadResult(path string, pkg *packages.Package) (*GoPackage, error) {
	if err := r.validateLocalFFIBoundary(path, pkg); err != nil {
		return nil, err
	}
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("resolve Go package %q: %s", path, pkg.Errors[0].Msg)
	}
	if pkg.Types == nil {
		return nil, fmt.Errorf("package has no type information")
	}
	return goPackageFromTypesPackage(path, pkg.Types), nil
}

func readGoModulePath(projectRoot string) (string, error) {
	if projectRoot == "" {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Join(projectRoot, "go.mod"))
	if err != nil {
		return "", err
	}
	file, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return "", err
	}
	if file.Module == nil {
		return "", nil
	}
	return file.Module.Mod.Path, nil
}

func (r *GoPackagesResolver) validateLocalFFIBoundary(importPath string, pkg *packages.Package) error {
	if r.modulePath == "" || importPath != r.modulePath && !strings.HasPrefix(importPath, r.modulePath+"/") {
		return nil
	}
	if len(pkg.GoFiles) == 0 {
		return nil
	}
	pkgDir := filepath.Dir(pkg.GoFiles[0])
	ffiRoot := filepath.Join(r.ProjectRoot, "ffi")
	rel, err := filepath.Rel(ffiRoot, pkgDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("project-local Go package %s is outside the FFI boundary; move Ard-callable Go code under ./ffi", importPath)
	}
	return nil
}
