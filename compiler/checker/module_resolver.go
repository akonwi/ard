package checker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"slices"

	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/version"
)

// ProjectInfo holds information about the current project
type ProjectInfo struct {
	RootPath     string                    // absolute path to project root
	ProjectName  string                    // project name from ard.toml or directory name
	Target       string                    // default build target from ard.toml
	Dependencies map[string]DependencyInfo // dependency aliases from ard.toml
}

type DependencyInfo struct {
	Alias      string
	SourcePath string // original local path for path dependencies
	Git        string
	Tag        string
	Commit     string
	VendorPath string // materialized project-local dependency path
}

// ModuleResolver handles finding and loading user modules
type ModuleResolver struct {
	project      *ProjectInfo
	moduleCache  map[string]Module         // cache loaded modules by file path
	astCache     map[string]*parse.Program // cache parsed ASTs by file path
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

			return &ProjectInfo{
				RootPath:     current,
				ProjectName:  projectName,
				Target:       target,
				Dependencies: dependencies,
			}, nil
		}

		// Move up one directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root, use directory name as fallback
			dirName := filepath.Base(absPath)
			return &ProjectInfo{
				RootPath:     absPath,
				ProjectName:  dirName,
				Target:       backend.DefaultTarget,
				Dependencies: map[string]DependencyInfo{},
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
		deps[alias] = dep
	}
	return deps, nil
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
		loadingChain: make([]string, 0),
	}, nil
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

func fetchDependency(dep DependencyInfo) (DependencyInfo, error) {
	if dep.VendorPath == "" {
		return DependencyInfo{}, fmt.Errorf("dependency %q has no vendor path", dep.Alias)
	}
	if err := os.RemoveAll(dep.VendorPath); err != nil {
		return DependencyInfo{}, err
	}
	if dep.SourcePath != "" {
		if err := copyDir(dep.SourcePath, dep.VendorPath); err != nil {
			return DependencyInfo{}, fmt.Errorf("vendor dependency %s: %w", dep.Alias, err)
		}
	} else if dep.Git != "" {
		if err := fetchGitDependency(dep); err != nil {
			return DependencyInfo{}, fmt.Errorf("vendor dependency %s: %w", dep.Alias, err)
		}
	} else {
		return DependencyInfo{}, fmt.Errorf("dependency %q has no source", dep.Alias)
	}
	return dep, nil
}

func fetchGitDependency(dep DependencyInfo) error {
	tmp, err := os.MkdirTemp("", "ard-dep-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if dep.Tag == "" && dep.Commit == "" {
		return fmt.Errorf("git dependency must specify tag or commit")
	}
	args := []string{"clone"}
	if dep.Tag != "" {
		args = append(args, "--branch", dep.Tag, "--depth", "1")
	}
	cloneDir := filepath.Join(tmp, "repo")
	args = append(args, dep.Git, cloneDir)
	if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	if dep.Commit != "" {
		cmd := exec.Command("git", "checkout", dep.Commit)
		cmd.Dir = cloneDir
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout %s: %w\n%s", dep.Commit, err, strings.TrimSpace(string(output)))
		}
	}
	return copyDir(cloneDir, dep.VendorPath)
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
		if _, err := os.Stat(dep.VendorPath); os.IsNotExist(err) {
			return "", fmt.Errorf("dependency %q is not vendored at %s; restore .ard/vendor or run `ard add`", dep.Alias, dep.VendorPath)
		}
		relativePath := strings.Join(parts[1:], "/")
		if relativePath == "" {
			relativePath = parts[0]
		}
		fullPath := filepath.Join(dep.VendorPath, relativePath+".ard")
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

	// Check cache first
	if cachedAST, exists := mr.astCache[filePath]; exists {
		return cachedAST, nil
	}

	// Read the module file
	sourceCode, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read module file %s: %w", filePath, err)
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
