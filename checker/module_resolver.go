package checker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"slices"

	"github.com/akonwi/ard/parse"
)

// ProjectInfo holds information about the current project
type ProjectInfo struct {
	RootPath    string // absolute path to project root
	ProjectName string // project name from ard.toml or directory name
}

// ModuleResolver handles finding and loading user modules
type ModuleResolver struct {
	project      *ProjectInfo
	moduleCache  map[string]Module       // cache loaded modules by file path
	astCache     map[string]*parse.Program // cache parsed ASTs by file path
	loadingChain []string                // track import paths currently being loaded for circular dependency detection
}

// findProjectRoot walks up the directory tree to find ard.toml or falls back to directory name
func findProjectRoot(startPath string) (*ProjectInfo, error) {
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
			return &ProjectInfo{
				RootPath:    current,
				ProjectName: projectName,
			}, nil
		}

		// Move up one directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root, use directory name as fallback
			dirName := filepath.Base(absPath)
			return &ProjectInfo{
				RootPath:    absPath,
				ProjectName: dirName,
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

// NewModuleResolver creates a new module resolver for the given working directory
func NewModuleResolver(workingDir string) (*ModuleResolver, error) {
	project, err := findProjectRoot(workingDir)
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

	// First part should be the project name
	if parts[0] != mr.project.ProjectName {
		return "", fmt.Errorf("import path '%s' does not match project name '%s'", importPath, mr.project.ProjectName)
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
