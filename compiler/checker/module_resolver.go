package checker

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"slices"

	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/version"
)

// ProjectInfo holds information about the current project
type ProjectInfo struct {
	RootPath      string                    // absolute path to project root
	ProjectName   string                    // project name from ard.toml or directory name
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
	Requested  string
	Integrity  string
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
	Requested    string
	Integrity    string
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
	Requested    string            `json:"requested,omitempty"`
	Integrity    string            `json:"integrity,omitempty"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
}

// ModuleResolver handles finding and loading user modules
type ModuleResolver struct {
	project        *ProjectInfo
	moduleCache    map[string]Module         // cache loaded modules by file path
	astCache       map[string]*parse.Program // cache parsed ASTs by file path
	overlays       map[string]string         // unsaved source text by resolved file path
	loadingChain   []string                  // track canonical module paths currently being loaded for circular dependency detection
	modulePackages map[string]string         // canonical module path -> package ID
}

type ResolvedImport struct {
	FilePath   string
	ModulePath string
	PackageID  string
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
			addPathDependencyPackages(packages, dependencies)

			return &ProjectInfo{
				RootPath:      current,
				ProjectName:   projectName,
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
		dep := DependencyInfo{Alias: alias, Name: alias}
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
			dep.Requested = dep.Tag
		}
		if commitMatches := commitRe.FindStringSubmatch(body); len(commitMatches) >= 2 {
			dep.Commit = commitMatches[1]
			dep.Requested = dep.Commit
		}
		if dep.SourcePath == "" && dep.Git == "" {
			continue
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
	lock = normalizeLockPackageIDs(lock)
	return lock, true, nil
}

func normalizeLockPackageIDs(lock LockFile) LockFile {
	rootDepIDs := map[string]bool{}
	for _, pkg := range lock.Packages {
		if pkg.ID == lock.Root {
			for _, depID := range pkg.Dependencies {
				rootDepIDs[depID] = true
			}
			break
		}
	}
	idMap := map[string]string{}
	packages := map[string]LockedPackage{}
	rootPreferred := map[string]bool{}
	for _, pkg := range lock.Packages {
		oldID := pkg.ID
		if pkg.Git != "" && pkg.Commit != "" {
			pkg.ID = GitPackageID(pkg.Git, pkg.Commit)
		}
		if oldID != "" && oldID != pkg.ID {
			idMap[oldID] = pkg.ID
		}
		isRootDep := rootDepIDs[oldID]
		if existing, ok := packages[pkg.ID]; ok {
			if isRootDep && !rootPreferred[pkg.ID] {
				pkg = mergeLockedPackages(pkg, existing)
			} else {
				pkg = mergeLockedPackages(existing, pkg)
			}
		}
		packages[pkg.ID] = pkg
		if isRootDep {
			rootPreferred[pkg.ID] = true
		}
	}
	if mappedRoot := idMap[lock.Root]; mappedRoot != "" {
		lock.Root = mappedRoot
	}
	lock.Packages = lock.Packages[:0]
	for _, pkg := range packages {
		if pkg.Dependencies == nil {
			pkg.Dependencies = map[string]string{}
		}
		for alias, depID := range pkg.Dependencies {
			if mapped := idMap[depID]; mapped != "" {
				pkg.Dependencies[alias] = mapped
			}
		}
		lock.Packages = append(lock.Packages, pkg)
	}
	return lock
}

func mergeLockedPackages(left LockedPackage, right LockedPackage) LockedPackage {
	if left.Name == "" {
		left.Name = right.Name
	}
	if left.Path == "" {
		left.Path = right.Path
	}
	if left.Git == "" {
		left.Git = right.Git
	}
	if left.Commit == "" {
		left.Commit = right.Commit
	}
	if left.Requested == "" {
		left.Requested = right.Requested
	}
	if left.Integrity == "" {
		left.Integrity = right.Integrity
	}
	if left.Dependencies == nil {
		left.Dependencies = map[string]string{}
	}
	for alias, depID := range right.Dependencies {
		left.Dependencies[alias] = depID
	}
	return left
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
			Requested:    locked.Requested,
			Integrity:    locked.Integrity,
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
		packageID := rootPkg.Dependencies[alias]
		if packageID != "" {
			if pkg, ok := packages[packageID]; ok {
				dep.PackageID = packageID
				dep.Name = pkg.Name
				dep.RootPath = pkg.RootPath
				if pkg.Git != "" {
					dep.Git = pkg.Git
				}
				if pkg.Commit != "" {
					dep.Commit = pkg.Commit
				}
				if pkg.Requested != "" {
					dep.Requested = pkg.Requested
				}
				if pkg.Integrity != "" {
					dep.Integrity = pkg.Integrity
				}
				deps[alias] = dep
				continue
			}
		}
		if dep.SourcePath != "" {
			dep.RootPath = dep.SourcePath
			if dep.PackageID == "" {
				dep.PackageID = "path:" + dep.SourcePath
			}
		}
		deps[alias] = dep
	}
	return rootPackageID, packages, deps
}

func addPathDependencyPackages(packages map[string]PackageInfo, deps map[string]DependencyInfo) {
	for alias, dep := range deps {
		addPathDependencyPackage(packages, dep, map[string]bool{})
		if dep.PackageID != "" {
			if pkg, ok := packages[dep.PackageID]; ok {
				dep.Name = pkg.Name
				dep.RootPath = pkg.RootPath
				deps[alias] = dep
			}
		}
	}
}

func addPathDependencyPackage(packages map[string]PackageInfo, dep DependencyInfo, seen map[string]bool) {
	if dep.SourcePath == "" || dep.PackageID == "" || seen[dep.PackageID] {
		return
	}
	if _, exists := packages[dep.PackageID]; exists {
		return
	}
	seen[dep.PackageID] = true
	name := dep.Name
	manifestPath := filepath.Join(dep.SourcePath, "ard.toml")
	if parsedName, err := parseProjectName(manifestPath); err == nil && parsedName != "" {
		name = parsedName
	}
	childDeps, _ := parseProjectDependencies(manifestPath, dep.SourcePath)
	depAliases := map[string]string{}
	for alias, child := range childDeps {
		if child.PackageID != "" {
			depAliases[alias] = child.PackageID
			addPathDependencyPackage(packages, child, seen)
		}
	}
	packages[dep.PackageID] = PackageInfo{ID: dep.PackageID, Name: name, RootPath: dep.SourcePath, Path: dep.SourcePath, Dependencies: depAliases}
}

func copyStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func GitPackageID(git string, commit string) string {
	return "git:" + cacheSourceHash(CanonicalGitSource(git)) + ":" + commit
}

func CanonicalGitSource(source string) string {
	source = strings.TrimRight(strings.TrimSpace(source), "/")
	if source == "" {
		return source
	}
	lower := strings.ToLower(source)
	if rest, ok := strings.CutPrefix(lower, "git@github.com:"); ok {
		return canonicalGitHubPath(rest)
	}
	if rest, ok := strings.CutPrefix(lower, "github.com/"); ok {
		return canonicalGitHubPath(rest)
	}
	if parsed, err := url.Parse(source); err == nil && strings.EqualFold(parsed.Hostname(), "github.com") {
		return canonicalGitHubPath(parsed.Path)
	}
	return source
}

func canonicalGitHubPath(path string) string {
	path = strings.Trim(strings.ToLower(path), "/")
	path = strings.TrimSuffix(path, ".git")
	return "https://github.com/" + path + ".git"
}

func PackageModulePrefix(packageID string) string {
	return "_pkg_" + cacheSourceHash(packageID)
}

func DependencyCachePath(git string, commit string) string {
	return filepath.Join(ArdCacheDir(), "git", cacheSourceHash(CanonicalGitSource(git)), commit)
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

func PackageTreeIntegrity(rootPath string) (string, error) {
	files := []string{}
	if err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() && (name == ".git" || name == "ard-out" || name == ".ard") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, path := range files {
		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return "", err
		}
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "file %d %s %d\n", len(rel), rel, len(data))
		if _, err := h.Write(data); err != nil {
			return "", err
		}
		if _, err := h.Write([]byte("\n")); err != nil {
			return "", err
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func LockWithDependency(projectRoot string, projectName string, alias string, dep DependencyInfo, packageName string, commit string) (LockFile, error) {
	return LockDependencyGraph(projectRoot, projectName, alias, dep, packageName, commit)
}

func LockDependencyGraph(projectRoot string, projectName string, alias string, dep DependencyInfo, packageName string, commit string) (LockFile, error) {
	return LockDependencyGraphReplacingAliases(projectRoot, projectName, alias, nil, dep, packageName, commit)
}

func LockDependencyGraphReplacingAliases(projectRoot string, projectName string, alias string, replacedAliases []string, dep DependencyInfo, packageName string, commit string) (LockFile, error) {
	builder, err := newLockGraphBuilder(projectRoot, projectName)
	if err != nil {
		return LockFile{}, err
	}
	builder.dropRootDependency(alias)
	for _, replacedAlias := range replacedAliases {
		builder.dropRootDependency(replacedAlias)
	}
	if dep.Git != "" {
		builder.dropRootDependenciesByGit(dep.Git)
	}
	if packageName != "" {
		dep.Name = packageName
	}
	if commit != "" {
		dep.Commit = commit
	}
	packageID, err := builder.resolveDependencyPackage(dep, alias, true)
	if err != nil {
		return LockFile{}, err
	}
	rootPkg := builder.packages[builder.lock.Root]
	if rootPkg.Dependencies == nil {
		rootPkg.Dependencies = map[string]string{}
	}
	rootPkg.Dependencies[alias] = packageID
	builder.packages[rootPkg.ID] = rootPkg
	return builder.lockFile(), nil
}

type lockGraphBuilder struct {
	projectRoot string
	projectName string
	lock        LockFile
	packages    map[string]LockedPackage
	gitCommits  map[string]string
	visiting    map[string]bool
}

func newLockGraphBuilder(projectRoot string, projectName string) (*lockGraphBuilder, error) {
	lock, ok, err := ReadDependencyLock(projectRoot)
	if err != nil {
		return nil, err
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
	builder := &lockGraphBuilder{
		projectRoot: projectRoot,
		projectName: projectName,
		lock:        lock,
		packages:    map[string]LockedPackage{},
		gitCommits:  map[string]string{},
		visiting:    map[string]bool{},
	}
	for _, pkg := range lock.Packages {
		if pkg.Dependencies == nil {
			pkg.Dependencies = map[string]string{}
		}
		builder.packages[pkg.ID] = pkg
		if pkg.Git != "" && pkg.Commit != "" {
			builder.gitCommits[CanonicalGitSource(pkg.Git)] = pkg.Commit
		}
	}
	rootPkg, ok := builder.packages[lock.Root]
	if !ok {
		rootPkg = LockedPackage{ID: lock.Root, Name: projectName, Path: ".", Dependencies: map[string]string{}}
	}
	if rootPkg.Dependencies == nil {
		rootPkg.Dependencies = map[string]string{}
	}
	builder.packages[rootPkg.ID] = rootPkg
	return builder, nil
}

func (b *lockGraphBuilder) dropRootDependency(alias string) {
	rootPkg := b.packages[b.lock.Root]
	if rootPkg.Dependencies != nil {
		delete(rootPkg.Dependencies, alias)
	}
	b.packages[rootPkg.ID] = rootPkg
	b.pruneUnreachablePackages()
}

func (b *lockGraphBuilder) dropRootDependenciesByGit(git string) {
	git = CanonicalGitSource(git)
	rootPkg := b.packages[b.lock.Root]
	for alias, packageID := range rootPkg.Dependencies {
		if CanonicalGitSource(b.packages[packageID].Git) == git {
			delete(rootPkg.Dependencies, alias)
		}
	}
	b.packages[rootPkg.ID] = rootPkg
	b.pruneUnreachablePackages()
}

func (b *lockGraphBuilder) pruneUnreachablePackages() {
	reachable := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if id == "" || reachable[id] {
			return
		}
		reachable[id] = true
		for _, child := range b.packages[id].Dependencies {
			visit(child)
		}
	}
	visit(b.lock.Root)
	for id := range b.packages {
		if !reachable[id] {
			delete(b.packages, id)
		}
	}
	b.gitCommits = map[string]string{}
	for _, pkg := range b.packages {
		if pkg.Git != "" && pkg.Commit != "" {
			b.gitCommits[CanonicalGitSource(pkg.Git)] = pkg.Commit
		}
	}
}

func (b *lockGraphBuilder) resolveDependencyPackage(dep DependencyInfo, fallbackName string, preferMetadata bool) (string, error) {
	if dep.Name == "" {
		dep.Name = fallbackName
	}
	if dep.Requested == "" {
		dep.Requested = requestedDependencyRef(dep)
	}
	if dep.SourcePath != "" {
		return b.resolvePathDependencyPackage(dep, preferMetadata)
	}
	if dep.Git != "" {
		return b.resolveGitDependencyPackage(dep, preferMetadata)
	}
	return "", fmt.Errorf("dependency %q has no source", fallbackName)
}

func (b *lockGraphBuilder) resolvePathDependencyPackage(dep DependencyInfo, preferMetadata bool) (string, error) {
	if !filepath.IsAbs(dep.SourcePath) {
		dep.SourcePath = filepath.Clean(filepath.Join(b.projectRoot, dep.SourcePath))
	}
	packageID := "path:" + dep.SourcePath
	if b.visiting[packageID] {
		return packageID, nil
	}
	b.visiting[packageID] = true
	defer delete(b.visiting, packageID)

	name := dep.Name
	manifestPath := filepath.Join(dep.SourcePath, "ard.toml")
	if parsedName, err := parseProjectName(manifestPath); err == nil && parsedName != "" {
		name = parsedName
	}
	path := dep.SourcePath
	if rel, err := filepath.Rel(b.projectRoot, path); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		path = rel
	}
	pkg := b.packages[packageID]
	pkg.ID = packageID
	pkg.Name = name
	pkg.Path = path
	pkg.Dependencies = map[string]string{}
	childDeps, err := parseProjectDependencies(manifestPath, dep.SourcePath)
	if err != nil {
		return "", err
	}
	for alias, child := range childDeps {
		childID, err := b.resolveDependencyPackage(child, alias, false)
		if err != nil {
			return "", err
		}
		pkg.Dependencies[alias] = childID
	}
	b.packages[packageID] = pkg
	return packageID, nil
}

func (b *lockGraphBuilder) resolveGitDependencyPackage(dep DependencyInfo, preferMetadata bool) (string, error) {
	sourceKey := CanonicalGitSource(dep.Git)
	if !preferMetadata {
		dep.Git = b.transportForGitSource(sourceKey, dep.Git)
	}
	commit, err := resolveGitDependencyCommit(dep)
	if err != nil {
		return "", err
	}
	if existing := b.gitCommits[sourceKey]; existing != "" && existing != commit {
		return "", fmt.Errorf("dependency conflict for %s: already locked to %s, requested %s", dep.Git, existing, commit)
	}
	b.gitCommits[sourceKey] = commit
	packageID := GitPackageID(dep.Git, commit)
	if b.visiting[packageID] {
		return packageID, nil
	}
	b.visiting[packageID] = true
	defer delete(b.visiting, packageID)

	dep.Commit = commit
	dep.PackageID = packageID
	dep.RootPath = DependencyCachePath(dep.Git, commit)
	if dep.Requested == "" {
		dep.Requested = requestedDependencyRef(dep)
	}
	existingPkg := b.packages[packageID]
	if existingPkg.Integrity != "" {
		dep.Integrity = existingPkg.Integrity
	}
	if dep.Integrity == "" {
		if err := os.RemoveAll(dep.RootPath); err != nil {
			return "", err
		}
	}
	if _, err := fetchDependency(dep); err != nil {
		return "", err
	}
	integrity, err := PackageTreeIntegrity(dep.RootPath)
	if err != nil {
		return "", err
	}
	name := dep.Name
	manifestPath := filepath.Join(dep.RootPath, "ard.toml")
	if parsedName, err := parseProjectName(manifestPath); err == nil && parsedName != "" {
		name = parsedName
	}
	pkg := existingPkg
	requested := dep.Requested
	if !preferMetadata && pkg.Requested != "" {
		requested = pkg.Requested
	}
	transportGit := dep.Git
	if !preferMetadata && pkg.Git != "" {
		transportGit = pkg.Git
	}
	pkg.ID = packageID
	pkg.Name = name
	pkg.Git = transportGit
	pkg.Commit = commit
	pkg.Requested = requested
	pkg.Integrity = integrity
	pkg.Dependencies = map[string]string{}
	childDeps, err := parseProjectDependencies(manifestPath, dep.RootPath)
	if err != nil {
		return "", err
	}
	for alias, child := range childDeps {
		childID, err := b.resolveDependencyPackage(child, alias, false)
		if err != nil {
			return "", err
		}
		pkg.Dependencies[alias] = childID
	}
	b.packages[packageID] = pkg
	return packageID, nil
}

func (b *lockGraphBuilder) transportForGitSource(sourceKey string, fallback string) string {
	for _, pkg := range b.packages {
		if pkg.Git != "" && CanonicalGitSource(pkg.Git) == sourceKey {
			return pkg.Git
		}
	}
	return fallback
}

func resolveGitDependencyCommit(dep DependencyInfo) (string, error) {
	if dep.Commit != "" {
		return resolveGitRefCommit(dep.Git, dep.Commit)
	}
	if dep.Tag != "" {
		return resolveGitTagCommit(dep.Git, dep.Tag)
	}
	return "", fmt.Errorf("git dependency %q must specify tag or commit", dep.Alias)
}

func isFullGitCommit(ref string) bool {
	matched, _ := regexp.MatchString("^[0-9a-fA-F]{40}$", ref)
	return matched
}

func resolveGitRefCommit(gitURL string, ref string) (string, error) {
	return resolveGitRevisionCommit(gitURL, ref+"^{commit}")
}

func resolveGitTagCommit(gitURL string, tag string) (string, error) {
	return resolveGitRevisionCommit(gitURL, "refs/tags/"+tag+"^{commit}")
}

func resolveGitRevisionCommit(gitURL string, revision string) (string, error) {
	tmp, err := os.MkdirTemp("", "ard-resolve-git-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	cloneDir := filepath.Join(tmp, "repo")
	args := []string{"clone", "--no-checkout", gitURL, cloneDir}
	if output, err := gitCommand(args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	cmd := gitCommand("rev-parse", revision)
	cmd.Dir = cloneDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w\n%s", revision, err, strings.TrimSpace(string(output)))
	}
	commit := strings.TrimSpace(string(output))
	if !isFullGitCommit(commit) {
		return "", fmt.Errorf("git ref %s resolved to non-commit %q", revision, commit)
	}
	return commit, nil
}

func requestedDependencyRef(dep DependencyInfo) string {
	if dep.Requested != "" {
		return dep.Requested
	}
	if dep.Tag != "" {
		return dep.Tag
	}
	return dep.Commit
}

func (b *lockGraphBuilder) lockFile() LockFile {
	lock := b.lock
	lock.Packages = lock.Packages[:0]
	for _, pkg := range b.reachablePackages() {
		lock.Packages = append(lock.Packages, pkg)
	}
	return lock
}

func (b *lockGraphBuilder) reachablePackages() []LockedPackage {
	reachable := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if id == "" || reachable[id] {
			return
		}
		reachable[id] = true
		for _, child := range b.packages[id].Dependencies {
			visit(child)
		}
	}
	visit(b.lock.Root)
	ids := make([]string, 0, len(reachable))
	for id := range reachable {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	packages := make([]LockedPackage, 0, len(ids))
	for _, id := range ids {
		packages = append(packages, b.packages[id])
	}
	return packages
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

// NewModuleResolver creates a new module resolver for the given working directory
func NewModuleResolver(workingDir string) (*ModuleResolver, error) {
	project, err := FindProjectRoot(workingDir)
	if err != nil {
		return nil, err
	}

	return &ModuleResolver{
		project:        project,
		moduleCache:    make(map[string]Module),
		astCache:       make(map[string]*parse.Program),
		overlays:       make(map[string]string),
		loadingChain:   make([]string, 0),
		modulePackages: make(map[string]string),
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
	deps := projectGitDependencies(project)
	fetched := make([]DependencyInfo, 0, len(deps))
	for _, dep := range deps {
		fetchedDep, err := fetchDependency(dep)
		if err != nil {
			return fetched, err
		}
		fetched = append(fetched, fetchedDep)
	}
	return fetched, nil
}

func VerifyDependencies(startPath string) error {
	project, err := FindProjectRoot(startPath)
	if err != nil {
		return err
	}
	for _, dep := range projectGitDependencies(project) {
		if dep.PackageID == "" || dep.Commit == "" || dep.RootPath == "" {
			return fmt.Errorf("dependency %q is not locked; run `ard add` with a tag, commit, or latest", dep.Alias)
		}
		if _, err := os.Stat(filepath.Join(dep.RootPath, "ard.toml")); err != nil {
			return fmt.Errorf("dependency %q is not available in Ard cache at %s; run `ard deps fetch`", dep.Alias, dep.RootPath)
		}
		if err := verifyDependencyIntegrity(dep); err != nil {
			return err
		}
	}
	return nil
}

func projectGitDependencies(project *ProjectInfo) []DependencyInfo {
	if project == nil {
		return nil
	}
	byID := map[string]DependencyInfo{}
	for alias, dep := range project.Dependencies {
		if dep.Git == "" {
			continue
		}
		if dep.Alias == "" {
			dep.Alias = alias
		}
		if dep.PackageID != "" {
			byID[dep.PackageID] = dep
		} else {
			byID[alias] = dep
		}
	}
	for packageID, pkg := range project.Packages {
		if packageID == project.RootPackageID || pkg.Git == "" {
			continue
		}
		if _, exists := byID[packageID]; exists {
			continue
		}
		alias := pkg.Name
		if alias == "" {
			alias = PackageModulePrefix(packageID)
		}
		byID[packageID] = DependencyInfo{Alias: alias, Name: pkg.Name, Git: pkg.Git, Commit: pkg.Commit, Requested: pkg.Requested, Integrity: pkg.Integrity, RootPath: pkg.RootPath, PackageID: packageID}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	deps := make([]DependencyInfo, 0, len(ids))
	for _, id := range ids {
		deps = append(deps, byID[id])
	}
	return deps
}

func verifyDependencyIntegrity(dep DependencyInfo) error {
	if dep.Integrity == "" {
		return nil
	}
	got, err := PackageTreeIntegrity(dep.RootPath)
	if err != nil {
		return err
	}
	if got != dep.Integrity {
		return fmt.Errorf("dependency %q cache integrity mismatch: got %s, want %s", dep.Alias, got, dep.Integrity)
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
			if err := verifyDependencyIntegrity(dep); err == nil {
				return dep, nil
			}
			if err := os.RemoveAll(dep.RootPath); err != nil {
				return DependencyInfo{}, err
			}
		} else if !os.IsNotExist(err) {
			return DependencyInfo{}, err
		}
		if err := fetchGitDependency(dep); err != nil {
			return DependencyInfo{}, fmt.Errorf("fetch dependency %s: %w", dep.Alias, err)
		}
		if err := verifyDependencyIntegrity(dep); err != nil {
			return DependencyInfo{}, err
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
	args := []string{"clone", "--no-checkout", dep.Git, cloneDir}
	if output, err := gitCommand(args...).CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	if err := os.RemoveAll(dep.RootPath); err != nil {
		return err
	}
	return archiveGitTree(cloneDir, dep.Commit, dep.RootPath)
}

func gitCommand(args ...string) *exec.Cmd {
	gitArgs := append([]string{"-c", "core.autocrlf=false", "-c", "core.eol=lf", "-c", "filter.lfs.smudge=", "-c", "filter.lfs.required=false"}, args...)
	cmd := exec.Command("git", gitArgs...)
	cmd.Env = append(os.Environ(), "GIT_LFS_SKIP_SMUDGE=1")
	return cmd
}

func archiveGitTree(repoDir string, commit string, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	cmd := gitCommand("archive", "--format=tar", commit)
	cmd.Dir = repoDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	tr := tar.NewReader(stdout)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = cmd.Wait()
			return err
		}
		if shouldSkipArchivePath(header.Name) {
			continue
		}
		target := filepath.Join(dst, filepath.FromSlash(header.Name))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dst)+string(os.PathSeparator)) && filepath.Clean(target) != filepath.Clean(dst) {
			_ = cmd.Wait()
			return fmt.Errorf("git archive contains invalid path %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)&0o777); err != nil {
				_ = cmd.Wait()
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				_ = cmd.Wait()
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)&0o777)
			if err != nil {
				_ = cmd.Wait()
				return err
			}
			_, copyErr := io.Copy(file, tr)
			closeErr := file.Close()
			if copyErr != nil {
				_ = cmd.Wait()
				return copyErr
			}
			if closeErr != nil {
				_ = cmd.Wait()
				return closeErr
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git archive %s: %w\n%s", commit, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func shouldSkipArchivePath(name string) bool {
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == ".git" || part == "ard-out" || part == ".ard" {
			return true
		}
	}
	return false
}

// ResolveImportPath converts an import path to a file system path from the root package context.
// Example: "my_project/utils" -> "utils.ard"
// Example: "my_project/math/operations" -> "math/operations.ard"
func (mr *ModuleResolver) ResolveImportPath(importPath string) (string, error) {
	resolved, err := mr.ResolveImport("", importPath)
	if err != nil {
		return "", err
	}
	return resolved.FilePath, nil
}

func (mr *ModuleResolver) ResolveImport(importerModulePath string, importPath string) (ResolvedImport, error) {
	if strings.HasPrefix(importPath, "ard/") {
		return ResolvedImport{}, fmt.Errorf("standard library imports should be handled separately")
	}
	parts := strings.Split(importPath, "/")
	if len(parts) < 1 || parts[0] == "" {
		return ResolvedImport{}, fmt.Errorf("invalid import path: %s", importPath)
	}
	importerPackageID := mr.packageIDForModule(importerModulePath)
	pkg := mr.packageInfo(importerPackageID)
	rootName := parts[0]
	if rootName == pkg.Name {
		if len(parts) == 1 {
			return ResolvedImport{}, fmt.Errorf("invalid import path: %s (missing module name)", importPath)
		}
		modulePath := strings.Join(parts[1:], "/")
		return mr.resolvePackageModule(pkg.ID, modulePath)
	}
	if pkg.ID == mr.project.RootPackageID {
		if dep, ok := mr.project.Dependencies[rootName]; ok {
			modulePath := strings.Join(parts[1:], "/")
			if modulePath == "" {
				modulePath = dep.Name
				if modulePath == "" {
					modulePath = dep.Alias
				}
			}
			if dep.PackageID != "" {
				if _, ok := mr.project.Packages[dep.PackageID]; ok {
					return mr.resolvePackageModule(dep.PackageID, modulePath)
				}
			}
			return mr.resolveDependencyModule(dep, modulePath)
		}
	} else if depPackageID := pkg.Dependencies[rootName]; depPackageID != "" {
		depPkg := mr.packageInfo(depPackageID)
		modulePath := strings.Join(parts[1:], "/")
		if modulePath == "" {
			modulePath = depPkg.Name
		}
		return mr.resolvePackageModule(depPkg.ID, modulePath)
	}
	if pkg.ID == mr.project.RootPackageID {
		return ResolvedImport{}, fmt.Errorf("unknown import root %q for package %q; import path '%s' does not match project name '%s' or a dependency alias", rootName, pkg.Name, importPath, mr.project.ProjectName)
	}
	return ResolvedImport{}, fmt.Errorf("unknown import root %q for package %q", rootName, pkg.Name)
}

func (mr *ModuleResolver) packageIDForModule(modulePath string) string {
	if mr == nil || mr.project == nil {
		return "root"
	}
	if modulePath != "" {
		if packageID, ok := mr.modulePackages[modulePath]; ok && packageID != "" {
			return packageID
		}
		if rootPkg, ok := mr.project.Packages[mr.project.RootPackageID]; ok {
			if modulePath == rootPkg.Name || strings.HasPrefix(modulePath, rootPkg.Name+"/") {
				return rootPkg.ID
			}
		}
		for id := range mr.project.Packages {
			if id == mr.project.RootPackageID {
				continue
			}
			prefix := PackageModulePrefix(id)
			if modulePath == prefix || strings.HasPrefix(modulePath, prefix+"/") {
				return id
			}
		}
	}
	if mr.project.RootPackageID != "" {
		return mr.project.RootPackageID
	}
	return "root"
}

func (mr *ModuleResolver) packageInfo(packageID string) PackageInfo {
	if mr != nil && mr.project != nil {
		if pkg, ok := mr.project.Packages[packageID]; ok {
			if pkg.Dependencies == nil {
				pkg.Dependencies = map[string]string{}
			}
			return pkg
		}
		if pkg, ok := mr.project.Packages[mr.project.RootPackageID]; ok {
			if pkg.Dependencies == nil {
				pkg.Dependencies = map[string]string{}
			}
			return pkg
		}
		return PackageInfo{ID: "root", Name: mr.project.ProjectName, RootPath: mr.project.RootPath, Dependencies: map[string]string{}}
	}
	return PackageInfo{ID: "root", Name: "", RootPath: ".", Dependencies: map[string]string{}}
}

func (mr *ModuleResolver) resolvePackageModule(packageID string, modulePath string) (ResolvedImport, error) {
	pkg, ok := mr.project.Packages[packageID]
	if !ok {
		return ResolvedImport{}, fmt.Errorf("unknown package ID %q", packageID)
	}
	if pkg.RootPath == "" {
		return ResolvedImport{}, fmt.Errorf("package %q has no source root", pkg.Name)
	}
	dep := DependencyInfo{Alias: pkg.Name, Name: pkg.Name, RootPath: pkg.RootPath, Git: pkg.Git, Commit: pkg.Commit, Requested: pkg.Requested, PackageID: pkg.ID, SourcePath: pkg.Path}
	if pkg.Git != "" {
		dep.SourcePath = ""
	}
	return mr.resolveDependencyModule(dep, modulePath)
}

func (mr *ModuleResolver) resolveDependencyModule(dep DependencyInfo, modulePath string) (ResolvedImport, error) {
	depRoot := dep.RootPath
	if depRoot == "" && dep.SourcePath != "" {
		depRoot = dep.SourcePath
	}
	if depRoot == "" && dep.Git != "" {
		return ResolvedImport{}, fmt.Errorf("dependency %q is not locked; run `ard add` with a tag, commit, or latest", dep.Alias)
	}
	if depRoot == "" {
		return ResolvedImport{}, fmt.Errorf("dependency %q has no source root", dep.Alias)
	}
	if _, err := os.Stat(depRoot); os.IsNotExist(err) {
		if dep.Git != "" {
			return ResolvedImport{}, fmt.Errorf("dependency %q is not available in Ard cache at %s; run `ard deps fetch`", dep.Alias, depRoot)
		}
		return ResolvedImport{}, fmt.Errorf("dependency %q path does not exist: %s", dep.Alias, depRoot)
	}
	fullPath := filepath.Join(depRoot, modulePath+".ard")
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return ResolvedImport{}, fmt.Errorf("module file not found: %s", fullPath)
	}
	packageID := dep.PackageID
	if packageID == "" {
		packageID = dep.Alias
	}
	canonicalModulePath := mr.canonicalModulePath(packageID, dep.Name, modulePath)
	if mr.modulePackages != nil {
		mr.modulePackages[canonicalModulePath] = packageID
	}
	return ResolvedImport{FilePath: fullPath, ModulePath: canonicalModulePath, PackageID: packageID}, nil
}

func (mr *ModuleResolver) canonicalModulePath(packageID string, packageName string, modulePath string) string {
	if packageID == "" || packageID == mr.project.RootPackageID {
		if packageName != "" && modulePath != packageName && !strings.HasPrefix(modulePath, packageName+"/") {
			return packageName + "/" + modulePath
		}
		return modulePath
	}
	return PackageModulePrefix(packageID) + "/" + modulePath
}

// GetProjectInfo returns the current project information
func (mr *ModuleResolver) GetProjectInfo() *ProjectInfo {
	return mr.project
}

// LoadModule loads and parses a module file from the given import path.
func (mr *ModuleResolver) LoadModule(importPath string) (*parse.Program, error) {
	filePath, err := mr.ResolveImportPath(importPath)
	if err != nil {
		return nil, err
	}
	return mr.LoadModuleFile(filePath)
}

func (mr *ModuleResolver) LoadModuleFile(filePath string) (*parse.Program, error) {
	filePath = filepath.Clean(filePath)
	if cachedAST, exists := mr.astCache[filePath]; exists {
		return cachedAST, nil
	}
	var sourceCode []byte
	if overlay, ok := mr.overlays[filePath]; ok {
		sourceCode = []byte(overlay)
	} else {
		var err error
		sourceCode, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read module file %s: %w", filePath, err)
		}
	}
	result := parse.Parse(sourceCode, filePath)
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("failed to parse module %s: %s", filePath, result.Errors[0].Message)
	}
	program := result.Program
	mr.astCache[filePath] = program
	return program, nil
}

// LoadModuleWithDependencies loads a module and all its dependencies, detecting circular dependencies.
func (mr *ModuleResolver) LoadModuleWithDependencies(importPath string) (*parse.Program, error) {
	resolved, err := mr.ResolveImport("", importPath)
	if err != nil {
		return nil, err
	}
	return mr.loadModuleRecursive(resolved)
}

// loadModuleRecursive is the internal method that handles recursive loading with cycle detection.
func (mr *ModuleResolver) loadModuleRecursive(resolved ResolvedImport) (*parse.Program, error) {
	modulePath := resolved.ModulePath
	if slices.Contains(mr.loadingChain, modulePath) {
		chain := append(mr.loadingChain, modulePath)
		return nil, fmt.Errorf("circular dependency detected: %s", strings.Join(chain, " -> "))
	}
	mr.loadingChain = append(mr.loadingChain, modulePath)
	defer func() {
		if len(mr.loadingChain) > 0 {
			mr.loadingChain = mr.loadingChain[:len(mr.loadingChain)-1]
		}
	}()

	program, err := mr.LoadModuleFile(resolved.FilePath)
	if err != nil {
		return nil, err
	}

	for _, imp := range program.Imports {
		if strings.HasPrefix(imp.Path, "ard/") {
			continue
		}
		child, err := mr.ResolveImport(modulePath, imp.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to load dependency %s: %w", imp.Path, err)
		}
		_, err = mr.loadModuleRecursive(child)
		if err != nil {
			return nil, fmt.Errorf("failed to load dependency %s: %w", imp.Path, err)
		}
	}

	return program, nil
}
