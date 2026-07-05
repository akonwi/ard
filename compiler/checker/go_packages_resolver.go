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
	pkg, err := r.load(path)
	r.cache[path] = goPackageResolveResult{pkg: pkg, err: err}
	return pkg, err
}

func (r *GoPackagesResolver) load(path string) (*GoPackage, error) {
	if r.modulePathErr != nil {
		return nil, fmt.Errorf("read go.mod: %w", r.modulePathErr)
	}
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
	loaded, err := packages.Load(cfg, path)
	if err != nil {
		return nil, err
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("package not found")
	}
	pkg := loaded[0]
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
