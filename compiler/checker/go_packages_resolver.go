package checker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/token"
	gotypes "go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/tools/go/gcexportdata"
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
	loaded, err := loadGoPackages(cfg, pending)
	if needsWritableModuleRetry(loaded, err) {
		retryConfig, retryCleanup, available, retryErr := r.loadWritableConfigWithDependencies()
		if retryErr != nil {
			r.recordFailure(pending, retryErr)
			return nil
		}
		if available {
			defer retryCleanup()
			cfg = retryConfig
			loaded, err = loadGoPackages(cfg, pending)
		}
	}
	if err != nil {
		r.recordFailure(pending, err)
		return nil
	}
	exportErrors := loadPackageExportData(loaded)
	if len(exportErrors) > 0 {
		fallbackConfig := *cfg
		fallbackConfig.Mode = packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedImports | packages.NeedDeps | packages.NeedFiles
		loaded, err = packages.Load(&fallbackConfig, pending...)
		if err != nil {
			r.recordFailure(pending, err)
			return nil
		}
		exportErrors = nil
	}
	byPath := make(map[string]*packages.Package, len(loaded))
	for _, pkg := range loaded {
		byPath[pkg.PkgPath] = pkg
	}
	type conversionJob struct {
		index int
		path  string
		pkg   *packages.Package
	}
	results := make([]goPackageResolveResult, len(pending))
	jobs := make(chan conversionJob)
	workerCount := min(runtime.GOMAXPROCS(0), len(pending))
	var conversions sync.WaitGroup
	for range workerCount {
		conversions.Add(1)
		go func() {
			defer conversions.Done()
			for job := range jobs {
				goPkg, pkgErr := r.packageFromLoadResult(job.path, job.pkg)
				results[job.index] = goPackageResolveResult{pkg: goPkg, err: pkgErr}
			}
		}()
	}
	for index, path := range pending {
		pkg, ok := byPath[path]
		if !ok {
			results[index] = goPackageResolveResult{err: fmt.Errorf("package %q not found", path)}
			continue
		}
		if exportErr := exportErrors[path]; exportErr != nil {
			results[index] = goPackageResolveResult{err: exportErr}
			continue
		}
		jobs <- conversionJob{index: index, path: path, pkg: pkg}
	}
	close(jobs)
	conversions.Wait()
	for index, path := range pending {
		r.cache[path] = results[index]
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

func needsWritableModuleRetry(loaded []*packages.Package, loadErr error) bool {
	if loadErr != nil && isReadonlyModuleCompletionError(loadErr.Error()) {
		return true
	}
	for _, pkg := range loaded {
		for _, pkgErr := range pkg.Errors {
			if isReadonlyModuleCompletionError(pkgErr.Msg) {
				return true
			}
		}
	}
	return false
}

func isReadonlyModuleCompletionError(message string) bool {
	return strings.Contains(message, "missing go.sum entry") ||
		strings.Contains(message, "updates to go.mod needed") ||
		strings.Contains(message, "updates to go.sum needed") ||
		strings.Contains(message, "import lookup disabled by -mod=readonly")
}

type goListError struct {
	Pos string
	Err string
}

type goListPackage struct {
	Dir        string
	ImportPath string
	GoFiles    []string
	CgoFiles   []string
	Imports    []string
	Export     string
	Error      *goListError
	DepsErrors []goListError
}

func loadGoPackages(cfg *packages.Config, paths []string) ([]*packages.Package, error) {
	if externalGoPackagesDriverConfigured(cfg) {
		driverConfig := *cfg
		driverConfig.Mode = packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps | packages.NeedFiles
		return packages.Load(&driverConfig, paths...)
	}
	return loadGoListPackages(cfg, paths)
}

func externalGoPackagesDriverConfigured(cfg *packages.Config) bool {
	env := cfg.Env
	if env == nil {
		env = os.Environ()
	}
	driver, _ := environmentValue(env, "GOPACKAGESDRIVER")
	if driver == "off" {
		return false
	}
	if driver != "" {
		return true
	}
	// Match go/packages: implicit driver discovery uses the compiler process's
	// PATH even when Config.Env supplies a child-process environment.
	_, err := exec.LookPath("gopackagesdriver")
	return err == nil
}

func environmentValue(env []string, name string) (string, bool) {
	prefix := name + "="
	value := ""
	found := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			value = strings.TrimPrefix(entry, prefix)
			found = true
		}
	}
	return value, found
}

func goListArgs(cfg *packages.Config, paths []string) []string {
	fields := "ImportPath,Error,DepsErrors,Dir,GoFiles,CgoFiles,Imports,Export"
	args := []string{"list", "-e", "-json=" + fields, "-compiled=false", "-test=false", "-export=true", "-deps=false", "-find=false", "-buildvcs=false", "-pgo=off"}
	args = append(args, cfg.BuildFlags...)
	args = append(args, "--")
	return append(args, paths...)
}

func loadGoListPackages(cfg *packages.Config, paths []string) ([]*packages.Package, error) {
	args := goListArgs(cfg, paths)
	hostProcs := runtime.GOMAXPROCS(0)
	effectiveEnv := cfg.Env
	if effectiveEnv == nil {
		effectiveEnv = os.Environ()
	}
	_, explicitProcs := environmentValue(effectiveEnv, "GOMAXPROCS")
	capRuntime := hostProcs > 4 && !explicitProcs
	ctx := cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = cfg.Dir
	cmd.Env = cfg.Env
	if capRuntime {
		// Warm export lookups are dominated by package-graph bookkeeping, where
		// excessive runtime parallelism adds more scheduling than useful work.
		// Deriving from cmd.Environ after setting Dir preserves os/exec's PWD fix.
		cmd.Env = append(cmd.Environ(), "GOMAXPROCS=4")
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("go list: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	loaded := make([]*packages.Package, 0, len(paths))
	importsByPath := make(map[string][]string, len(paths))
	seen := map[string]bool{}
	decoder := json.NewDecoder(&stdout)
	for {
		var listed goListPackage
		if err := decoder.Decode(&listed); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		if listed.ImportPath == "" {
			return nil, fmt.Errorf("go list returned a package without an import path")
		}
		if seen[listed.ImportPath] {
			return nil, fmt.Errorf("go list returned duplicate package %q", listed.ImportPath)
		}
		seen[listed.ImportPath] = true
		importsByPath[listed.ImportPath] = listed.Imports
		exportFile := listed.Export
		if exportFile != "" && !filepath.IsAbs(exportFile) {
			exportFile = filepath.Join(listed.Dir, exportFile)
		}
		pkg := &packages.Package{ID: listed.ImportPath, PkgPath: listed.ImportPath, ExportFile: exportFile}
		pkg.GoFiles = goListSourceFiles(listed)
		if listed.Error != nil {
			pkg.Errors = append(pkg.Errors, packages.Error{Pos: listed.Error.Pos, Msg: listed.Error.Err, Kind: packages.ListError})
		}
		for _, depErr := range listed.DepsErrors {
			pkg.Errors = append(pkg.Errors, packages.Error{Pos: depErr.Pos, Msg: depErr.Err, Kind: packages.ListError})
		}
		loaded = append(loaded, pkg)
	}
	return orderGoListRootsByDependency(loaded, importsByPath), nil
}

func orderGoListRootsByDependency(loaded []*packages.Package, importsByPath map[string][]string) []*packages.Package {
	byPath := make(map[string]*packages.Package, len(loaded))
	incoming := make(map[string]int, len(loaded))
	for _, pkg := range loaded {
		byPath[pkg.PkgPath] = pkg
	}
	for path, imports := range importsByPath {
		if byPath[path] == nil {
			continue
		}
		for _, importedPath := range imports {
			if importedPath != path && byPath[importedPath] != nil {
				incoming[importedPath]++
			}
		}
	}
	queue := make([]string, 0, len(loaded))
	for _, pkg := range loaded {
		if incoming[pkg.PkgPath] == 0 {
			queue = append(queue, pkg.PkgPath)
		}
	}
	ordered := make([]*packages.Package, 0, len(loaded))
	seen := make(map[string]bool, len(loaded))
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		if seen[path] {
			continue
		}
		seen[path] = true
		ordered = append(ordered, byPath[path])
		for _, importedPath := range importsByPath[path] {
			if byPath[importedPath] == nil {
				continue
			}
			incoming[importedPath]--
			if incoming[importedPath] == 0 {
				queue = append(queue, importedPath)
			}
		}
	}
	for _, pkg := range loaded {
		if !seen[pkg.PkgPath] {
			ordered = append(ordered, pkg)
		}
	}
	return ordered
}

func goListSourceFiles(listed goListPackage) []string {
	files := make([]string, 0, len(listed.GoFiles)+len(listed.CgoFiles))
	for _, file := range append(append([]string(nil), listed.GoFiles...), listed.CgoFiles...) {
		if !filepath.IsAbs(file) {
			file = filepath.Join(listed.Dir, file)
		}
		files = append(files, file)
	}
	return files
}

func loadPackageExportData(packagesToLoad []*packages.Package) map[string]error {
	imports := map[string]*gotypes.Package{}
	fset := token.NewFileSet()
	errorsByPath := map[string]error{}
	for _, pkg := range packagesToLoad {
		if pkg.PkgPath == "unsafe" {
			pkg.Types = gotypes.Unsafe
			continue
		}
		if pkg.Types != nil || len(pkg.Errors) > 0 {
			continue
		}
		if imported := imports[pkg.PkgPath]; imported != nil && imported.Complete() {
			pkg.Types = imported
			continue
		}
		if pkg.ExportFile == "" {
			errorsByPath[pkg.PkgPath] = fmt.Errorf("Go package %q has no compiler export data", pkg.PkgPath)
			continue
		}
		file, err := os.Open(pkg.ExportFile)
		if err != nil {
			errorsByPath[pkg.PkgPath] = fmt.Errorf("open Go export data for %q: %w", pkg.PkgPath, err)
			continue
		}
		reader, err := gcexportdata.NewReader(file)
		if err == nil {
			pkg.Types, err = gcexportdata.Read(reader, fset, imports, pkg.PkgPath)
		}
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			errorsByPath[pkg.PkgPath] = fmt.Errorf("read Go export data for %q: %w", pkg.PkgPath, err)
		}
	}
	return errorsByPath
}

func (r *GoPackagesResolver) loadConfig() *packages.Config {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedExportFile |
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
	modData, standalone, err := r.dependencyGoMod()
	if err != nil {
		return cfg, func() {}, err
	}
	if modData == nil {
		return cfg, func() {}, nil
	}
	sumData, err := r.dependencyGoSum(standalone)
	if err != nil {
		return cfg, func() {}, err
	}
	if standalone {
		moduleDir, cleanup, err := prepareDependencyGoModule(modData, sumData)
		if err != nil {
			return cfg, func() {}, err
		}
		cfg.Dir = moduleDir
		cfg.BuildFlags = append(cfg.BuildFlags, "-mod=readonly")
		return cfg, cleanup, nil
	}
	modPath, cleanup, err := prepareDependencyGoModfile(modData, sumData)
	if err != nil {
		return cfg, func() {}, err
	}
	cfg.BuildFlags = append(cfg.BuildFlags, "-modfile="+modPath, "-mod=readonly")
	return cfg, cleanup, nil
}

func (r *GoPackagesResolver) loadWritableConfigWithDependencies() (*packages.Config, func(), bool, error) {
	cfg := r.loadConfig()
	modData, standalone, err := r.dependencyGoMod()
	if err != nil {
		return cfg, func() {}, false, err
	}
	if modData == nil {
		return cfg, func() {}, false, nil
	}
	sumData, err := r.dependencyGoSum(standalone)
	if err != nil {
		return cfg, func() {}, false, err
	}
	if standalone {
		moduleDir, cleanup, err := prepareWritableDependencyGoModule(modData, sumData)
		if err != nil {
			return cfg, func() {}, false, err
		}
		cfg.Dir = moduleDir
		cfg.BuildFlags = append(cfg.BuildFlags, "-mod=mod")
		return cfg, cleanup, true, nil
	}
	modPath, cleanup, err := prepareWritableDependencyGoModfile(modData, sumData)
	if err != nil {
		return cfg, func() {}, false, err
	}
	cfg.BuildFlags = append(cfg.BuildFlags, "-modfile="+modPath, "-mod=mod")
	return cfg, cleanup, true, nil
}

// dependencyGoSum seeds private module files only with checksums trusted by an
// existing consumer module. A dependency's go.sum is not a trust root for the
// main module; missing entries are verified and added only in the private
// writable retry. A standalone resolver module starts without checksums.
func (r *GoPackagesResolver) dependencyGoSum(standalone bool) ([]byte, error) {
	if standalone {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(r.ProjectRoot, "go.sum"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read project Go checksums: %w", err)
	}
	return data, nil
}

func prepareWritableDependencyGoModfile(modData []byte, sumData []byte) (string, func(), error) {
	moduleDir, cleanup, err := writeTemporaryGoModule("ard-go-mod-retry-", "ard.mod", modData, sumData)
	if err != nil {
		return "", func() {}, err
	}
	return filepath.Join(moduleDir, "ard.mod"), cleanup, nil
}

func prepareDependencyGoModule(modData []byte, sumData []byte) (string, func(), error) {
	return writeTemporaryGoModule("ard-go-module-", "go.mod", modData, sumData)
}

func prepareWritableDependencyGoModule(modData []byte, sumData []byte) (string, func(), error) {
	return writeTemporaryGoModule("ard-go-module-retry-", "go.mod", modData, sumData)
}

func writeTemporaryGoModule(pattern string, modName string, modData []byte, sumData []byte) (string, func(), error) {
	moduleDir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", func() {}, fmt.Errorf("create dependency Go module files: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(moduleDir) }
	if err := os.WriteFile(filepath.Join(moduleDir, modName), modData, 0o600); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("write dependency Go module file: %w", err)
	}
	if len(sumData) > 0 {
		sumName := strings.TrimSuffix(modName, ".mod") + ".sum"
		if err := os.WriteFile(filepath.Join(moduleDir, sumName), sumData, 0o600); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("write dependency Go checksum file: %w", err)
		}
	}
	return moduleDir, cleanup, nil
}

// prepareDependencyGoModfile caches the read-only alternate module files used
// when the source project already has a go.mod.
func prepareDependencyGoModfile(modData []byte, sumData []byte) (string, func(), error) {
	hash := sha256.New()
	_, _ = hash.Write([]byte("ard-go-mod-v2\x00"))
	fmt.Fprintf(hash, "%d:", len(modData))
	_, _ = hash.Write(modData)
	fmt.Fprintf(hash, "%d:", len(sumData))
	_, _ = hash.Write(sumData)
	cacheKey := fmt.Sprintf("%x", hash.Sum(nil))
	if cacheRoot, err := os.UserCacheDir(); err == nil {
		moduleDir := filepath.Join(cacheRoot, "ard", "go-mod", cacheKey)
		if err := os.MkdirAll(moduleDir, 0o700); err == nil {
			modPath := filepath.Join(moduleDir, "ard.mod")
			modErr := writeCachedGoModuleFile(modPath, modData)
			sumErr := error(nil)
			if len(sumData) > 0 {
				sumErr = writeCachedGoModuleFile(filepath.Join(moduleDir, "ard.sum"), sumData)
			}
			if modErr == nil && sumErr == nil {
				return modPath, func() {}, nil
			}
		}
	}

	moduleDir, err := os.MkdirTemp("", "ard-go-mod-")
	if err != nil {
		return "", func() {}, fmt.Errorf("create dependency Go module files: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(moduleDir) }
	modPath := filepath.Join(moduleDir, "ard.mod")
	if err := os.WriteFile(modPath, modData, 0o600); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("write dependency Go module file: %w", err)
	}
	if len(sumData) > 0 {
		if err := os.WriteFile(filepath.Join(moduleDir, "ard.sum"), sumData, 0o600); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("write dependency Go checksum file: %w", err)
		}
	}
	return modPath, cleanup, nil
}

func writeCachedGoModuleFile(path string, data []byte) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".ard-go-mod-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		// Windows does not replace an existing destination atomically. Another
		// compiler process may have won the same content-addressed cache write.
		if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, data) {
			return nil
		}
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return err
		}
		if retryErr := os.Rename(tempPath, path); retryErr != nil {
			if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, data) {
				return nil
			}
			return retryErr
		}
	}
	return nil
}

type dependencyGoModuleSource struct {
	modulePath string
	root       string
	file       *modfile.File
}

// dependencyGoMod builds the private module definition used to redirect each
// dependency's Go module to the root backing its Ard source. Existing project
// modules remain the main module through -modfile. Projects without go.mod use
// a standalone private module rooted outside the source tree. The user's module
// files are never modified.
func (r *GoPackagesResolver) dependencyGoMod() ([]byte, bool, error) {
	dependencies, err := r.dependencyGoModuleSources()
	if err != nil || len(dependencies) == 0 {
		return nil, false, err
	}
	goModPath := filepath.Join(r.ProjectRoot, "go.mod")
	data, err := os.ReadFile(goModPath)
	standalone := false
	var file *modfile.File
	switch {
	case err == nil:
		file, err = modfile.Parse("go.mod", data, nil)
		if err != nil {
			return nil, false, fmt.Errorf("parse project go.mod: %w", err)
		}
	case os.IsNotExist(err):
		standalone = true
		file = new(modfile.File)
		if err := file.AddModuleStmt(privateResolverModulePath(dependencies)); err != nil {
			return nil, false, fmt.Errorf("create private Go module: %w", err)
		}
		if err := file.AddGoStmt("1.27.0"); err != nil {
			return nil, false, fmt.Errorf("create private Go module: %w", err)
		}
	default:
		return nil, false, fmt.Errorf("read project go.mod: %w", err)
	}

	required := make(map[string]bool, len(file.Require)+len(dependencies))
	for _, require := range file.Require {
		required[require.Mod.Path] = true
	}
	// Promote transitive requirements because generated modules do the same,
	// keeping checker and backend module graphs identical when dependencies use
	// local replacement modules.
	for _, dependency := range dependencies {
		for _, require := range dependency.file.Require {
			if required[require.Mod.Path] {
				continue
			}
			file.AddNewRequire(require.Mod.Path, require.Mod.Version, require.Indirect)
			required[require.Mod.Path] = true
		}
	}
	for _, dependency := range dependencies {
		if required[dependency.modulePath] {
			continue
		}
		if err := file.AddRequire(dependency.modulePath, DependencyGoModuleVersion(dependency.modulePath)); err != nil {
			return nil, false, fmt.Errorf("require dependency Go module %s: %w", dependency.modulePath, err)
		}
		required[dependency.modulePath] = true
	}

	// Locked/path roots override a stale consumer replacement for the same
	// module, so FFI always comes from the source selected by Ard (#353, #437).
	for _, dependency := range dependencies {
		for _, replace := range append([]*modfile.Replace(nil), file.Replace...) {
			if replace.Old.Path == dependency.modulePath {
				if err := file.DropReplace(replace.Old.Path, replace.Old.Version); err != nil {
					return nil, false, fmt.Errorf("replace dependency Go module %s: %w", dependency.modulePath, err)
				}
			}
		}
		if err := file.AddReplace(dependency.modulePath, "", dependency.root, ""); err != nil {
			return nil, false, fmt.Errorf("replace dependency Go module %s: %w", dependency.modulePath, err)
		}
	}

	selected := make(map[string]bool, len(dependencies))
	for _, dependency := range dependencies {
		selected[dependency.modulePath] = true
	}
	replaced := make(map[string]bool, len(file.Replace))
	wildcardReplaced := make(map[string]bool, len(file.Replace))
	for _, replace := range file.Replace {
		replaced[goModReplaceKey(replace.Old.Path, replace.Old.Version)] = true
		if replace.Old.Version == "" {
			wildcardReplaced[replace.Old.Path] = true
		}
	}
	for _, dependency := range dependencies {
		sourceWildcards := map[string]bool{}
		for _, replace := range dependency.file.Replace {
			key := goModReplaceKey(replace.Old.Path, replace.Old.Version)
			if selected[replace.Old.Path] || wildcardReplaced[replace.Old.Path] || replaced[key] {
				continue
			}
			newPath := replace.New.Path
			if replace.New.Version == "" && modfile.IsDirectoryPath(newPath) {
				newPath, err = ResolveLocalGoModulePath(dependency.root, newPath)
				if err != nil {
					return nil, false, fmt.Errorf("resolve replacement from dependency Go module %s: %w", dependency.modulePath, err)
				}
			}
			if err := file.AddReplace(replace.Old.Path, replace.Old.Version, newPath, replace.New.Version); err != nil {
				return nil, false, fmt.Errorf("merge replacement from dependency Go module %s: %w", dependency.modulePath, err)
			}
			replaced[key] = true
			if replace.Old.Version == "" {
				sourceWildcards[replace.Old.Path] = true
			}
		}
		for modulePath := range sourceWildcards {
			wildcardReplaced[modulePath] = true
		}
	}

	file.Cleanup()
	formatted, err := file.Format()
	if err != nil {
		return nil, false, fmt.Errorf("format dependency Go module: %w", err)
	}
	return formatted, standalone, nil
}

func (r *GoPackagesResolver) dependencyGoModuleSources() ([]dependencyGoModuleSource, error) {
	if len(r.DependencyModuleRoots) == 0 || r.ProjectRoot == "" {
		return nil, nil
	}
	modulePaths := make([]string, 0, len(r.DependencyModuleRoots))
	for modulePath := range r.DependencyModuleRoots {
		if modulePath != "" && r.DependencyModuleRoots[modulePath] != "" {
			modulePaths = append(modulePaths, modulePath)
		}
	}
	sort.Strings(modulePaths)
	dependencies := make([]dependencyGoModuleSource, 0, len(modulePaths))
	for _, modulePath := range modulePaths {
		root, err := filepath.Abs(r.DependencyModuleRoots[modulePath])
		if err != nil {
			return nil, fmt.Errorf("resolve dependency Go module %s: %w", modulePath, err)
		}
		data, err := os.ReadFile(filepath.Join(root, "go.mod"))
		if err != nil {
			return nil, fmt.Errorf("read dependency Go module %s: %w", modulePath, err)
		}
		file, err := modfile.Parse("go.mod", data, nil)
		if err != nil {
			return nil, fmt.Errorf("parse dependency Go module %s: %w", modulePath, err)
		}
		if file.Module == nil || file.Module.Mod.Path != modulePath {
			return nil, fmt.Errorf("dependency Go module root %s declares %q, want %q", root, readModfileModulePath(file), modulePath)
		}
		dependencies = append(dependencies, dependencyGoModuleSource{modulePath: modulePath, root: root, file: file})
	}
	return dependencies, nil
}

func readModfileModulePath(file *modfile.File) string {
	if file == nil || file.Module == nil {
		return ""
	}
	return file.Module.Mod.Path
}

func privateResolverModulePath(dependencies []dependencyGoModuleSource) string {
	forbidden := map[string]bool{}
	for _, dependency := range dependencies {
		forbidden[dependency.modulePath] = true
		if dependency.file != nil {
			for _, require := range dependency.file.Require {
				forbidden[require.Mod.Path] = true
			}
		}
	}
	modulePaths := make([]string, 0, len(forbidden))
	for modulePath := range forbidden {
		modulePaths = append(modulePaths, modulePath)
	}
	sort.Strings(modulePaths)
	hash := sha256.New()
	for _, modulePath := range modulePaths {
		_, _ = hash.Write([]byte(modulePath))
		_, _ = hash.Write([]byte{0})
	}
	digest := fmt.Sprintf("%x", hash.Sum(nil))[:12]
	for suffix := 0; ; suffix++ {
		root := "ard-resolver"
		if suffix > 0 {
			root += fmt.Sprintf("-%d", suffix)
		}
		candidate := fmt.Sprintf("%s.invalid/%s", root, digest)
		conflict := false
		for _, modulePath := range modulePaths {
			if GoModulePathsOverlap(candidate, modulePath) {
				conflict = true
				break
			}
		}
		if !conflict {
			return candidate
		}
	}
}

// GoModulePathsOverlap reports whether either module path owns the other's
// import-path namespace.
func GoModulePathsOverlap(left string, right string) bool {
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func goModReplaceKey(path string, version string) string {
	return path + "\x00" + version
}

// ResolveLocalGoModulePath resolves a Go module replacement directory against
// the go.mod that declares it, accepting both slash conventions.
func ResolveLocalGoModulePath(baseDir string, path string) (string, error) {
	if strings.HasPrefix(path, `.\`) || strings.HasPrefix(path, `..\`) {
		path = strings.ReplaceAll(path, `\`, "/")
	}
	path = filepath.FromSlash(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Abs(path)
}

// DependencyGoModuleVersion returns a valid placeholder version for a locally
// replaced module, including modules with semantic-import-version suffixes.
func DependencyGoModuleVersion(modulePath string) string {
	_, pathMajor, ok := module.SplitPathVersion(modulePath)
	if !ok || pathMajor == "" {
		return "v0.0.0"
	}
	major := strings.TrimPrefix(strings.TrimPrefix(pathMajor, "/v"), ".v")
	if major == "" {
		return "v0.0.0"
	}
	return "v" + major + ".0.0"
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
	dependencyAliases := make([]string, 0, len(info.Dependencies))
	for alias := range info.Dependencies {
		dependencyAliases = append(dependencyAliases, alias)
	}
	sort.Strings(dependencyAliases)
	packageIDs := make([]string, 0, len(info.Packages))
	for packageID := range info.Packages {
		packageIDs = append(packageIDs, packageID)
	}
	sort.Strings(packageIDs)

	// Add path dependencies first so a locked Git checkout retains precedence
	// if malformed dependency metadata names the same Go module from both.
	for _, alias := range dependencyAliases {
		dep := info.Dependencies[alias]
		if dep.Git == "" {
			add(dep.RootPath)
		}
	}
	for _, packageID := range packageIDs {
		pkg := info.Packages[packageID]
		if packageID != info.RootPackageID && pkg.Git == "" && pkg.Path != "" {
			add(pkg.RootPath)
		}
	}
	for _, alias := range dependencyAliases {
		dep := info.Dependencies[alias]
		if dep.Git != "" {
			add(dep.RootPath)
		}
	}
	for _, packageID := range packageIDs {
		pkg := info.Packages[packageID]
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
