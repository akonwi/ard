package checker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

type GoPackagesResolver struct {
	ProjectRoot string
	BuildTags   []string
	// DependencyModuleRoots maps a dependency's Go module path to the local
	// directory backing its Ard source (a locked Git checkout or path root).
	// Injected into Go module resolution so dependency FFI is checked against
	// the same source used by Ard (#353, #437).
	DependencyModuleRoots map[string]string
	modulePath            string
	modulePathErr         error
	cache                 map[string]goPackageResolveResult
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

func NewGoPackagesResolver(projectRoot string, buildTags []string) *GoPackagesResolver {
	if absRoot, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = absRoot
	}
	modulePath, modulePathErr := readGoModulePath(projectRoot)
	if os.IsNotExist(modulePathErr) {
		modulePathErr = nil
	}
	return &GoPackagesResolver{ProjectRoot: projectRoot, BuildTags: append([]string(nil), buildTags...), modulePath: modulePath, modulePathErr: modulePathErr, cache: map[string]goPackageResolveResult{}}
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
type goPrimeCoverageError struct {
	Missing []string
}

func (e *goPrimeCoverageError) Error() string {
	return fmt.Sprintf("internal compiler bug: Go packages %v were not collected by the import pre-scan; please report this", e.Missing)
}

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
		return &goPrimeCoverageError{Missing: pending}
	}
	defer func() { r.primed = true }()
	if r.modulePathErr != nil {
		r.recordFailure(pending, fmt.Errorf("read go.mod: %w", r.modulePathErr))
		return nil
	}
	cfg, cleanup, err := r.loadConfigWithDependencies()
	if err != nil {
		r.recordFailure(pending, err)
		return nil
	}
	defer cleanup()
	loaded, err := packages.Load(cfg, pending...)
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

func (r *GoPackagesResolver) loadConfigWithDependencies() (*packages.Config, func(), error) {
	cfg := r.loadConfig()
	if overlay := r.dependencyReplaceOverlay(); overlay != nil {
		cfg.Overlay = overlay
		return cfg, func() {}, nil
	}
	if len(r.DependencyModuleRoots) == 0 || r.ProjectRoot == "" {
		return cfg, func() {}, nil
	}
	if _, err := os.Stat(filepath.Join(r.ProjectRoot, "go.mod")); err == nil || !os.IsNotExist(err) {
		return cfg, func() {}, nil
	}

	// Without a consumer go.mod, resolve dependency FFI through a temporary Go
	// workspace whose main modules are the dependency roots. Keeping Dir at the
	// Ard project root preserves package diagnostics while avoiding any writes
	// to the user's project (#437).
	workspaceDir, err := os.MkdirTemp("", "ard-go-work-")
	if err != nil {
		return cfg, func() {}, fmt.Errorf("create dependency Go workspace: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(workspaceDir) }
	roots := make([]string, 0, len(r.DependencyModuleRoots))
	seen := map[string]bool{}
	for _, root := range r.DependencyModuleRoots {
		absRoot, err := filepath.Abs(root)
		if err != nil || seen[absRoot] {
			continue
		}
		seen[absRoot] = true
		roots = append(roots, absRoot)
	}
	if len(roots) == 0 {
		cleanup()
		return cfg, func() {}, nil
	}
	sort.Strings(roots)
	var workFile strings.Builder
	workFile.WriteString("go 1.27.0\n\nuse (\n")
	for _, root := range roots {
		fmt.Fprintf(&workFile, "\t%q\n", filepath.ToSlash(root))
	}
	workFile.WriteString(")\n")
	workPath := filepath.Join(workspaceDir, "go.work")
	if err := os.WriteFile(workPath, []byte(workFile.String()), 0o644); err != nil {
		cleanup()
		return cfg, func() {}, fmt.Errorf("write dependency Go workspace: %w", err)
	}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GOWORK=") {
			cfg.Env = append(cfg.Env, entry)
		}
	}
	cfg.Env = append(cfg.Env, "GOWORK="+workPath)
	return cfg, cleanup, nil
}

// dependencyReplaceOverlay synthesizes a go.mod overlay that redirects each
// dependency's Go module to the root backing its Ard source. It returns nil
// when there is nothing to redirect or no project go.mod to overlay. The
// user's on-disk go.mod is never modified.
func (r *GoPackagesResolver) dependencyReplaceOverlay() map[string][]byte {
	if len(r.DependencyModuleRoots) == 0 || r.ProjectRoot == "" {
		return nil
	}
	goModPath := filepath.Join(r.ProjectRoot, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return nil
	}
	file, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil
	}
	changed := false
	for modulePath, dir := range r.DependencyModuleRoots {
		if modulePath == "" || dir == "" {
			continue
		}
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		// A local replace needs the module to be required to enter the build
		// graph; add a placeholder require when the consumer's go.mod omits it.
		if !moduleRequired(file, modulePath) {
			if err := file.AddRequire(modulePath, "v0.0.0"); err != nil {
				continue
			}
		}
		if err := file.AddReplace(modulePath, "", absDir, ""); err != nil {
			continue
		}
		changed = true
	}
	if !changed {
		return nil
	}
	file.Cleanup()
	formatted, err := file.Format()
	if err != nil {
		return nil
	}
	return map[string][]byte{goModPath: formatted}
}

func moduleRequired(file *modfile.File, modulePath string) bool {
	for _, require := range file.Require {
		if require.Mod.Path == modulePath {
			return true
		}
	}
	return false
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

// DependencyGoModuleRoots maps each dependency's Go module path to the local
// source root used for its Ard code. Git dependencies use their locked checkout
// (#353), while path dependencies use their declared local root (#437).
// Dependencies without a Go module (pure Ard, no FFI) are omitted.
func DependencyGoModuleRoots(info *ProjectInfo) map[string]string {
	if info == nil {
		return nil
	}
	roots := map[string]string{}
	add := func(root string) {
		if root == "" {
			return
		}
		modulePath, err := readGoModulePath(root)
		if err != nil || modulePath == "" {
			return
		}
		roots[modulePath] = root
	}
	// Add path dependencies first so a locked Git checkout retains precedence
	// if malformed dependency metadata names the same Go module from both.
	for _, dep := range info.Dependencies {
		if dep.Git == "" {
			add(dep.RootPath)
		}
	}
	for packageID, pkg := range info.Packages {
		if packageID != info.RootPackageID && pkg.Git == "" && pkg.Path != "" {
			add(pkg.RootPath)
		}
	}
	for _, dep := range info.Dependencies {
		if dep.Git != "" {
			add(dep.RootPath)
		}
	}
	for packageID, pkg := range info.Packages {
		if packageID != info.RootPackageID && pkg.Git != "" {
			add(pkg.RootPath)
		}
	}
	if len(roots) == 0 {
		return nil
	}
	return roots
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
