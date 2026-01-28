package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestFindProjectRoot(t *testing.T) {
	// Create a temporary directory structure for testing
	tempDir, err := os.MkdirTemp("", "ard_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml file
	tomlContent := `name = "test_project"`
	tomlPath := filepath.Join(tempDir, "ard.toml")
	err = os.WriteFile(tomlPath, []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create subdirectories
	subDir := filepath.Join(tempDir, "src", "utils")
	err = os.MkdirAll(subDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Test project root discovery from subdirectory
	resolver, err := checker.NewModuleResolver(subDir)
	if err != nil {
		t.Fatalf("Failed to create module resolver: %v", err)
	}

	project := resolver.GetProjectInfo()
	if project.ProjectName != "test_project" {
		t.Errorf("Expected project name 'test_project', got '%s'", project.ProjectName)
	}

	if project.RootPath != tempDir {
		t.Errorf("Expected root path '%s', got '%s'", tempDir, project.RootPath)
	}
}

func TestFindProjectRootFallback(t *testing.T) {
	// Create a temporary directory without ard.toml
	tempDir, err := os.MkdirTemp("", "fallback_project_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatalf("Failed to create module resolver: %v", err)
	}

	project := resolver.GetProjectInfo()
	expectedName := filepath.Base(tempDir)
	if project.ProjectName != expectedName {
		t.Errorf("Expected project name '%s', got '%s'", expectedName, project.ProjectName)
	}
}

func TestResolveImportPath(t *testing.T) {
	// Create test project structure
	tempDir, err := os.MkdirTemp("", "ard_resolve_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "my_calculator"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module files
	utilsPath := filepath.Join(tempDir, "utils.ard")
	err = os.WriteFile(utilsPath, []byte("// utils module"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create nested module
	mathDir := filepath.Join(tempDir, "math")
	err = os.MkdirAll(mathDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	opsPath := filepath.Join(mathDir, "operations.ard")
	err = os.WriteFile(opsPath, []byte("// operations module"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatalf("Failed to create module resolver: %v", err)
	}

	tests := []struct {
		name       string
		importPath string
		expected   string
		shouldErr  bool
	}{
		{
			name:       "simple module",
			importPath: "my_calculator/utils",
			expected:   utilsPath,
			shouldErr:  false,
		},
		{
			name:       "nested module",
			importPath: "my_calculator/math/operations",
			expected:   opsPath,
			shouldErr:  false,
		},
		{
			name:       "wrong project name",
			importPath: "other_project/utils",
			expected:   "",
			shouldErr:  true,
		},
		{
			name:       "missing module",
			importPath: "my_calculator/nonexistent",
			expected:   "",
			shouldErr:  true,
		},
		{
			name:       "standard library (should error)",
			importPath: "ard/io",
			expected:   "",
			shouldErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := resolver.ResolveImportPath(tt.importPath)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if resolved != tt.expected {
				t.Errorf("Expected path '%s', got '%s'", tt.expected, resolved)
			}
		})
	}
}
