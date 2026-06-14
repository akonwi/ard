package checker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"slices"

	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/version"
)

// ProjectInfo holds information about the current project
type ProjectInfo struct {
	RootPath      string                    // absolute path to project root
	ProjectName   string                    // project name from ard.toml or directory name
	Target        string                    // default build target from ard.toml
	Dependencies  map[string]DependencyInfo // dependency aliases from ard.toml
	RootPackageID string
	Packages      map[string]PackageInfo
}

type DependencyInfo struct {
	Alias      string
	SourcePath string // original local path for path dependencies
	Git        string
	Tag        string
	Commit     string
	VendorPath string // legacy project-local dependency path
	RootPath   string // source root used by resolver: path dependency or locked cache checkout
	PackageID  string
	Name       string
}

type PackageInfo struct {
	ID           string
	Name         string
	RootPath     string
	Dependencies map[string]string
	Git          string
	Commit       string
	Path         string
}

type LockFile struct {
	Version  int             `json:"version"`
	Root     string          `json:"root"`
	Packages []LockedPackage `json:"packages"`
}

type LockedPackage struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Path         string            `json:"path,omitempty"`
	Git          string            `json:"git,omitempty"`
	Commit       string            `json:"commit,omitempty"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
}

// ModuleResolver handles finding and loading user modules
type ModuleResolver struct {
	project      *ProjectInfo
	moduleCache  map[string]Module         // cache loaded modules by file path
	astCache     map[string]*parse.Program // cache parsed ASTs by file path
	overlays     map[string]string         // unsaved source text by resolved file path
	loadingChain []string                  // track import paths currently being loaded for circular dependency detection
}

// FindProjectRoot walks up the directory tree to find ard.toml or falls back to directory name
func FindProjectRoot(startPath string) (*ProjectInfo, error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	current := absPath
	for {
		// Check for ard.toml file
		tomlPath := filepath.Join(current, "ard.toml")
		if _, err := os.Stat(tomlPath); err == nil {
			// Found ard.toml, parse project name
			projectName, err := parseProjectName(tomlPath)
			if err != nil {
				return nil, fmt.Errorf("failed to parse ard.toml: %w", err)
			}

			target, err := parseProjectTarget(tomlPath)
			if err != nil {
				return nil, fmt.Errorf("failed to parse ard.toml: %w", err)
			}

			// Check ard version constraint (required in ard.toml)
			constraint, ok := parseArdVersion(tomlPath)
			if !ok {
				return nil, fmt.Errorf("ard.toml is missing required field: ard (e.g. ard = \">= 0.13.0\")")
			}
			if err := version.CheckVersion(constraint); err != nil {
				return nil, err
			}

			dependencies, err := parseProjectDependencies(tomlPath, current)
			if err != nil {
				return nil, fmt.Errorf("failed to parse ard.toml: %w", err)
			}
			rootPackageID := "root"
			packages := map[string]PackageInfo{
				rootPackageID: {
					ID:           rootPackageID,
					Name:         projectName,
					RootPath:     current,
					Path:         ".",
					Dependencies: map[string]string{},
				},
			}
			if lock, ok, err := ReadDependencyLock(current); err != nil {
				return nil, fmt.Errorf("failed to parse ard.lock: %w", err)
			} else if ok {
				rootPackageID, packages, dependencies = applyDependencyLock(current, projectName, dependencies, lock)
			}

			return &ProjectInfo{
				RootPath:      current,
				ProjectName:   projectName,
				Target:        target,
				Dependencies:  dependencies,
				RootPackageID: rootPackageID,
				Packages:      packages,
			}, nil
		}

		// Move up one directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root, use directory name as fallback
			dirName := filepath.Base(absPath)
			return &ProjectInfo{
				RootPath:      absPath,
				ProjectName:   dirName,
				Target:        backend.DefaultTarget,
				Dependencies:  map[string]DependencyInfo{},
				RootPackageID: "root",
				Packages: map[string]PackageInfo{
					"root": {ID: "root", Name: dirName, RootPath: absPath, Path: ".", Dependencies: map[string]string{}},
				},
			}, nil
		}
		current = parent
	}
}

// parseProjectName extracts the project name from ard.toml
// For now, use simple regex parsing. Format: name = "project_name"
func parseProjectName(tomlPath string) (string, error) {
	content, err := os.ReadFile(tomlPath)
	if err != nil {
		return "", err
	}

	// Simple regex to match: name = "project_name" or name = 'project_name'
	re := regexp.MustCompile(`(?m)^\s*name\s*=\s*["']([^"']+)["']`)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) < 2 {
		return "", fmt.Errorf("no project name found in ard.toml")
	}

	return matches[1], nil
}

// parseArdVersion extracts the ard constraint from ard.toml if present.
// Format: ard = ">= 0.13.0" or ard = "0.13.0"
func parseArdVersion(tomlPath string) (string, bool) {
	content, err := os.ReadFile(tomlPath)
	if err != nil {
		return "", false
	}

	re := regexp.MustCompile(`(?m)^\s*ard\s*=\s*["']([^"']+)["']`)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) < 2 {
		return "", false
	}

	return matches[1], true
}

func parseProjectDependencies(tomlPath string, projectRoot string) (map[string]DependencyInfo, error) {
	content, err := os.ReadFile(tomlPath)
	if err != nil {
		return nil, err
	}
	deps := map[string]DependencyInfo{}
	inDependencies := false
	depRe := regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*=\s*\{([^}]*)\}`)
	pathRe := regexp.MustCompile(`\bpath\s*=\s*["']([^"']+)["']`)
	gitRe := regexp.MustCompile(`\bgit\s*=\s*["']([^"']+)["']`)
	tagRe := regexp.MustCompile(`\btag\s*=\s*["']([^"']+)["']`)
	commitRe := regexp.MustCompile(`\bcommit\s*=\s*["']([^"']+)["']`)
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inDependencies = trimmed == "[dependencies]"
			continue
		}
		if !inDependencies || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		matches := depRe.FindStringSubmatch(line)
		if len(matches) < 3 {
			continue
		}
		alias := matches[1]
		body := matches[2]
		dep := DependencyInfo{Alias: alias, VendorPath: filepath.Join(projectRoot, ".ard", "vendor", alias)}
		if pathMatches := pathRe.FindStringSubmatch(body); len(pathMatches) >= 2 {
			dep.SourcePath = pathMatches[1]
			if !filepath.IsAbs(dep.SourcePath) {
				dep.SourcePath = filepath.Clean(filepath.Join(projectRoot, dep.SourcePath))
			}
			dep.RootPath = dep.SourcePath
			dep.PackageID = "path:" + dep.SourcePath
		}
		if gitMatches := gitRe.FindStringSubmatch(body); len(gitMatches) >= 2 {
			dep.Git = gitMatches[1]
		}
		if tagMatches := tagRe.FindStringSubmatch(body); len(tagMatches) >= 2 {
			dep.Tag = tagMatches[1]
		}
		if commitMatches := commitRe.FindStringSubmatch(body); len(commitMatches) >= 2 {
			dep.Commit = commitMatches[1]
		}
		if dep.SourcePath == "" && dep.Git == "" {
			continue
		}
		if dep.RootPath == "" && dep.Git != "" {
			if _, err := os.Stat(dep.VendorPath); err == nil {
				dep.RootPath = dep.VendorPath
			}
		}
		deps[alias] = dep
	}
	return deps, nil
}

func ReadDependencyLock(projectRoot string) (LockFile, bool, error) {
	path := filepath.Join(projectRoot, "ard.lock")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return LockFile{}, false, nil
	}
	if err != nil {
		return LockFile{}, false, err
	}
	var lock LockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		return LockFile{}, false, err
	}
	if lock.Version == 0 {
		lock.Version = 1
	}
	if lock.Root == "" {
		lock.Root = "root"
	}
	return lock, true, nil
}

func WriteDependencyLock(projectRoot string, lock LockFile) error {
	if lock.Version == 0 {
		lock.Version = 1
	}
	if lock.Root == "" {
		lock.Root = "root"
	}
	sort.Slice(lock.Packages, func(i, j int) bool {
		return lock.Packages[i].ID < lock.Packages[j].ID
	})
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(projectRoot, "ard.lock"), data, 0o644)
}

func applyDependencyLock(projectRoot string, projectName string, deps map[string]DependencyInfo, lock LockFile) (string, map[string]PackageInfo, map[string]DependencyInfo) {
	packages := map[string]PackageInfo{}
	for _, locked := range lock.Packages {
		rootPath := locked.Path
		if rootPath != "" && !filepath.IsAbs(rootPath) {
			rootPath = filepath.Clean(filepath.Join(projectRoot, rootPath))
		}
		if locked.Git != "" && locked.Commit != "" {
			rootPath = DependencyCachePath(locked.Git, locked.Commit)
		}
		packages[locked.ID] = PackageInfo{
			ID:           locked.ID,
			Name:         locked.Name,
			RootPath:     rootPath,
			Dependencies: copyStringMap(locked.Dependencies),
			Git:          locked.Git,
			Commit:       locked.Commit,
			Path:         locked.Path,
		}
	}
	rootPackageID := lock.Root
	if rootPackageID == "" {
		rootPackageID = "root"
	}
	rootPkg, ok := packages[rootPackageID]
	if !ok {
		rootPkg = PackageInfo{ID: rootPackageID, Name: projectName, RootPath: projectRoot, Path: ".", Dependencies: map[string]string{}}
		packages[rootPackageID] = rootPkg
	}
	if rootPkg.Dependencies == nil {
		rootPkg.Dependencies = map[string]string{}
	}
	for alias, dep := range deps {
		dep.Name = alias
		if dep.SourcePath != "" {
			dep.RootPath = dep.SourcePath
			if dep.PackageID == "" {
				dep.PackageID = "path:" + dep.SourcePath
			}
			deps[alias] = dep
			continue
		}
		packageID := rootPkg.Dependencies[alias]
		if packageID == "" {
			deps[alias] = dep
			continue
		}
		pkg, ok := packages[packageID]
		if !ok {
			deps[alias] = dep
			continue
		}
		dep.PackageID = packageID
		dep.Name = pkg.Name
		dep.RootPath = pkg.RootPath
		if pkg.Git != "" {
			dep.Git = pkg.Git
		}
		if pkg.Commit != "" {
			dep.Commit = pkg.Commit
		}
		deps[alias] = dep
	}
	return rootPackageID, packages, deps
}

func copyStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func GitPackageID(git string, commit string) string {
	return "git:" + cacheSourceHash(git) + ":" + commit
}

func DependencyCachePath(git string, commit string) string {
	return filepath.Join(ArdCacheDir(), "git", cacheSourceHash(git), commit)
}

func ArdCacheDir() string {
	if dir := strings.TrimSpace(os.Getenv("ARD_CACHE_DIR")); dir != "" {
		return filepath.Clean(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Clean(filepath.Join(".", ".ard", "cache"))
	}
	return filepath.Join(home, ".ard", "cache")
}

func cacheSourceHash(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])[:16]
}

func LockWithDependency(projectRoot string, projectName string, alias string, dep DependencyInfo, packageName string, commit string) (LockFile, error) {
	lock, ok, err := ReadDependencyLock(projectRoot)
	if err != nil {
		return LockFile{}, err
	}
	if !ok {
		lock = LockFile{Version: 1, Root: "root", Packages: []LockedPackage{{ID: "root", Name: projectName, Path: ".", Dependencies: map[string]string{}}}}
	}
	if lock.Version == 0 {
		lock.Version = 1
	}
	if lock.Root == "" {
		lock.Root = "root"
	}
	packages := map[string]LockedPackage{}
	for _, pkg := range lock.Packages {
		if pkg.Dependencies == nil {
			pkg.Dependencies = map[string]string{}
		}
		packages[pkg.ID] = pkg
	}
	rootPkg, ok := packages[lock.Root]
	if !ok {
		rootPkg = LockedPackage{ID: lock.Root, Name: projectName, Path: ".", Dependencies: map[string]string{}}
	}
	if rootPkg.Dependencies == nil {
		rootPkg.Dependencies = map[string]string{}
	}
	if packageName == "" {
		packageName = alias
	}
	var pkg LockedPackage
	if dep.SourcePath != "" {
		path := dep.SourcePath
		if rel, err := filepath.Rel(projectRoot, path); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			path = rel
		}
		pkg = LockedPackage{ID: "path:" + dep.SourcePath, Name: packageName, Path: path, Dependencies: map[string]string{}}
	} else {
		if commit == "" {
			commit = dep.Commit
		}
		if commit == "" {
			return LockFile{}, fmt.Errorf("dependency %q needs a resolved commit for ard.lock", alias)
		}
		pkg = LockedPackage{ID: GitPackageID(dep.Git, commit), Name: packageName, Git: dep.Git, Commit: commit, Dependencies: map[string]string{}}
	}
	packages[pkg.ID] = pkg
	rootPkg.Dependencies[alias] = pkg.ID
	packages[rootPkg.ID] = rootPkg
	lock.Packages = make([]LockedPackage, 0, len(packages))
	for _, pkg := range packages {
		lock.Packages = append(lock.Packages, pkg)
	}
	return lock, nil
}

func PruneLockDependency(projectRoot string, alias string) error {
	lock, ok, err := ReadDependencyLock(projectRoot)
	if err != nil || !ok {
		return err
	}
	packages := map[string]LockedPackage{}
	for _, pkg := range lock.Packages {
		if pkg.Dependencies == nil {
			pkg.Dependencies = map[string]string{}
		}
		packages[pkg.ID] = pkg
	}
	rootPkg, ok := packages[lock.Root]
	if ok {
		delete(rootPkg.Dependencies, alias)
		packages[rootPkg.ID] = rootPkg
	}
	reachable := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if id == "" || reachable[id] {
			return
		}
		reachable[id] = true
		for _, child := range packages[id].Dependencies {
			visit(child)
		}
	}
	visit(lock.Root)
	lock.Packages = lock.Packages[:0]
	for id, pkg := range packages {
		if reachable[id] {
			lock.Packages = append(lock.Packages, pkg)
		}
	}
	return WriteDependencyLock(projectRoot, lock)
}

func parseProjectTarget(tomlPath string) (string, error) {
	content, err := os.ReadFile(tomlPath)
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`(?m)^\s*target\s*=\s*["']([^"']+)["']`)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) < 2 {
		return backend.DefaultTarget, nil
	}

	return backend.ParseTarget(matches[1])
}

// NewModuleResolver creates a new module resolver for the given working directory
func NewModuleResolver(workingDir string) (*ModuleResolver, error) {
	project, err := FindProjectRoot(workingDir)
	if err != nil {
		return nil, err
	}

	return &ModuleResolver{
		project:      project,
		moduleCache:  make(map[string]Module),
		astCache:     make(map[string]*parse.Program),
		overlays:     make(map[string]string),
		loadingChain: make([]string, 0),
	}, nil
}

// SetOverlay provides unsaved source text for a resolved module file path.
// The LSP uses this so imported open documents are checked from the editor
// buffer instead of stale on-disk contents.
func (mr *ModuleResolver) SetOverlay(filePath string, source string) {
	if mr == nil || strings.TrimSpace(filePath) == "" {
		return
	}
	if mr.overlays == nil {
		mr.overlays = make(map[string]string)
	}
	clean := filepath.Clean(filePath)
	mr.overlays[clean] = source
	delete(mr.astCache, clean)
}

func FetchDependency(startPath string, alias string) (DependencyInfo, error) {
	project, err := FindProjectRoot(startPath)
	if err != nil {
		return DependencyInfo{}, err
	}
	dep, ok := project.Dependencies[alias]
	if !ok {
		return DependencyInfo{}, fmt.Errorf("dependency %q is not declared in ard.toml", alias)
	}
	return fetchDependency(dep)
}

func FetchDependencies(startPath string) ([]DependencyInfo, error) {
	project, err := FindProjectRoot(startPath)
	if err != nil {
		return nil, err
	}
	aliases := make([]string, 0, len(project.Dependencies))
	for alias := range project.Dependencies {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	fetched := make([]DependencyInfo, 0, len(aliases))
	for _, alias := range aliases {
		dep, err := fetchDependency(project.Dependencies[alias])
		if err != nil {
			return fetched, err
		}
		fetched = append(fetched, dep)
	}
	return fetched, nil
}

func VerifyDependencies(startPath string) error {
	project, err := FindProjectRoot(startPath)
	if err != nil {
		return err
	}
	aliases := make([]string, 0, len(project.Dependencies))
	for alias := range project.Dependencies {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		dep := project.Dependencies[alias]
		if dep.Git == "" {
			continue
		}
		if dep.PackageID == "" || dep.Commit == "" || dep.RootPath == "" {
			return fmt.Errorf("dependency %q is not locked; run `ard add` with a tag, commit, or latest", dep.Alias)
		}
		if _, err := os.Stat(filepath.Join(dep.RootPath, "ard.toml")); err != nil {
			return fmt.Errorf("dependency %q is not available in Ard cache at %s; run `ard deps fetch`", dep.Alias, dep.RootPath)
		}
	}
	return nil
}

func fetchDependency(dep DependencyInfo) (DependencyInfo, error) {
	if dep.SourcePath != "" {
		dep.RootPath = dep.SourcePath
		return dep, nil
	}
	if dep.Git != "" {
		if dep.PackageID == "" || dep.Commit == "" || dep.RootPath == "" {
			return DependencyInfo{}, fmt.Errorf("dependency %q is not locked; run `ard add` with a tag, commit, or latest", dep.Alias)
		}
		if _, err := os.Stat(filepath.Join(dep.RootPath, "ard.toml")); err == nil {
			return dep, nil
		}
		if err := fetchGitDependency(dep); err != nil {
			return DependencyInfo{}, fmt.Errorf("fetch dependency %s: %w", dep.Alias, err)
		}
		return dep, nil
	}
	return DependencyInfo{}, fmt.Errorf("dependency %q has no source", dep.Alias)
}

func fetchGitDependency(dep DependencyInfo) error {
	if dep.RootPath == "" {
		return fmt.Errorf("dependency %q has no cache path", dep.Alias)
	}
	tmp, err := os.MkdirTemp("", "ard-dep-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if dep.Commit == "" {
		return fmt.Errorf("git dependency must be locked to a commit")
	}
	cloneDir := filepath.Join(tmp, "repo")
	args := []string{"clone", dep.Git, cloneDir}
	if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	cmd := exec.Command("git", "checkout", dep.Commit)
	cmd.Dir = cloneDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w\n%s", dep.Commit, err, strings.TrimSpace(string(output)))
	}
	if err := os.RemoveAll(dep.RootPath); err != nil {
		return err
	}
	return copyDir(cloneDir, dep.RootPath)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() && (name == ".git" || name == "ard-out" || name == ".ard") {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// ResolveImportPath converts an import path to a file system path
// Example: "my_project/utils" -> "utils.ard"
// Example: "my_project/math/operations" -> "math/operations.ard"
func (mr *ModuleResolver) ResolveImportPath(importPath string) (string, error) {
	// Check if this is a standard library import
	if strings.HasPrefix(importPath, "ard/") {
		return "", fmt.Errorf("standard library imports should be handled separately")
	}

	// Split the import path
	parts := strings.Split(importPath, "/")
	if len(parts) < 1 {
		return "", fmt.Errorf("invalid import path: %s", importPath)
	}

	if dep, ok := mr.project.Dependencies[parts[0]]; ok {
		depRoot := dep.RootPath
		if depRoot == "" && dep.SourcePath != "" {
			depRoot = dep.SourcePath
		}
		if depRoot == "" && dep.Git != "" {
			return "", fmt.Errorf("dependency %q is not locked; run `ard add` with a tag, commit, or latest", dep.Alias)
		}
		if depRoot == "" {
			return "", fmt.Errorf("dependency %q has no source root", dep.Alias)
		}
		if _, err := os.Stat(depRoot); os.IsNotExist(err) {
			if dep.Git != "" {
				return "", fmt.Errorf("dependency %q is not available in Ard cache at %s; run `ard deps fetch`", dep.Alias, depRoot)
			}
			return "", fmt.Errorf("dependency %q path does not exist: %s", dep.Alias, depRoot)
		}
		relativePath := strings.Join(parts[1:], "/")
		if relativePath == "" {
			relativePath = parts[0]
		}
		fullPath := filepath.Join(depRoot, relativePath+".ard")
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return "", fmt.Errorf("module file not found: %s", fullPath)
		}
		return fullPath, nil
	}

	// First part should be the project name
	if parts[0] != mr.project.ProjectName {
		return "", fmt.Errorf("import path '%s' does not match project name '%s' or a dependency alias", importPath, mr.project.ProjectName)
	}

	// Remove project name and construct relative path
	if len(parts) == 1 {
		return "", fmt.Errorf("invalid import path: %s (missing module name)", importPath)
	}

	relativePath := strings.Join(parts[1:], "/") + ".ard"
	fullPath := filepath.Join(mr.project.RootPath, relativePath)

	// Validate file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", fmt.Errorf("module file not found: %s", fullPath)
	}

	return fullPath, nil
}

// GetProjectInfo returns the current project information
func (mr *ModuleResolver) GetProjectInfo() *ProjectInfo {
	return mr.project
}

// LoadModule loads and parses a module file from the given import path
func (mr *ModuleResolver) LoadModule(importPath string) (*parse.Program, error) {
	// Resolve import path to filesystem path
	filePath, err := mr.ResolveImportPath(importPath)
	if err != nil {
		return nil, err
	}

	filePath = filepath.Clean(filePath)

	// Check cache first
	if cachedAST, exists := mr.astCache[filePath]; exists {
		return cachedAST, nil
	}

	var sourceCode []byte
	if overlay, ok := mr.overlays[filePath]; ok {
		sourceCode = []byte(overlay)
	} else {
		var err error
		// Read the module file
		sourceCode, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read module file %s: %w", filePath, err)
		}
	}

	// Parse the module
	result := parse.Parse(sourceCode, filePath)
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("failed to parse module %s: %s", filePath, result.Errors[0].Message)
	}
	program := result.Program

	// Cache the parsed AST
	mr.astCache[filePath] = program

	return program, nil
}

// LoadModuleWithDependencies loads a module and all its dependencies, detecting circular dependencies
func (mr *ModuleResolver) LoadModuleWithDependencies(importPath string) (*parse.Program, error) {
	return mr.loadModuleRecursive(importPath)
}

// loadModuleRecursive is the internal method that handles recursive loading with cycle detection
func (mr *ModuleResolver) loadModuleRecursive(importPath string) (*parse.Program, error) {
	// Check for circular dependency using import path (not file path)
	if slices.Contains(mr.loadingChain, importPath) {
		chain := append(mr.loadingChain, importPath)
		return nil, fmt.Errorf("circular dependency detected: %s", strings.Join(chain, " -> "))
	}

	// Add to loading chain
	mr.loadingChain = append(mr.loadingChain, importPath)

	// Ensure we remove from loading chain when done
	defer func() {
		if len(mr.loadingChain) > 0 {
			mr.loadingChain = mr.loadingChain[:len(mr.loadingChain)-1]
		}
	}()

	// Load the module (this will use cache if available)
	program, err := mr.LoadModule(importPath)
	if err != nil {
		return nil, err
	}

	// Now recursively load its dependencies
	for _, imp := range program.Imports {
		// Skip standard library imports
		if strings.HasPrefix(imp.Path, "ard/") {
			continue
		}

		// Load the imported module recursively
		_, err := mr.loadModuleRecursive(imp.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to load dependency %s: %w", imp.Path, err)
		}
	}

	return program, nil
}
