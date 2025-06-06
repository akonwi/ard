package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker"
)

func TestUserModulePathResolution(t *testing.T) {
	// Create a temporary project for testing
	tempDir, err := os.MkdirTemp("", "ard_user_imports_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "test_project"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a simple module file
	moduleContent := `pub fn helper() Int { 42 }`
	err = os.WriteFile(filepath.Join(tempDir, "utils.ard"), []byte(moduleContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create nested module
	mathDir := filepath.Join(tempDir, "math")
	err = os.MkdirAll(mathDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	nestedContent := `pub fn add(a: Int, b: Int) Int { a + b }`
	err = os.WriteFile(filepath.Join(mathDir, "operations.ard"), []byte(nestedContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test project info
	project := resolver.GetProjectInfo()
	if project.ProjectName != "test_project" {
		t.Errorf("Expected project name 'test_project', got '%s'", project.ProjectName)
	}

	// Test simple import resolution
	filePath, err := resolver.ResolveImportPath("test_project/utils")
	if err != nil {
		t.Fatalf("Failed to resolve import: %v", err)
	}

	expectedPath := filepath.Join(tempDir, "utils.ard")
	if filePath != expectedPath {
		t.Errorf("Expected path '%s', got '%s'", expectedPath, filePath)
	}

	// Test nested import resolution
	filePath, err = resolver.ResolveImportPath("test_project/math/operations")
	if err != nil {
		t.Fatalf("Failed to resolve nested import: %v", err)
	}

	expectedPath = filepath.Join(tempDir, "math", "operations.ard")
	if filePath != expectedPath {
		t.Errorf("Expected nested path '%s', got '%s'", expectedPath, filePath)
	}
}

func TestUserModuleCheckerIntegration(t *testing.T) {
	// Create a temporary project for testing
	tempDir, err := os.MkdirTemp("", "ard_checker_integration_*")
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

	// Create a simple module file
	moduleContent := `pub fn helper() Int { 42 }`
	err = os.WriteFile(filepath.Join(tempDir, "utils.ard"), []byte(moduleContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test checker integration (should fail with "not yet implemented" for now)
	input := `use my_calculator/utils`
	astTree, err := ast.Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	_, diagnostics := checker.Check(astTree, resolver)
	if len(diagnostics) == 0 {
		t.Error("Expected error for unimplemented user module loading")
	}

	// Should contain "not yet implemented" message
	found := false
	for _, diag := range diagnostics {
		if strings.Contains(diag.Message, "not yet implemented") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'not yet implemented' error message, got: %v", diagnostics)
	}
}

func TestUserModuleErrors(t *testing.T) {
	// Create a temporary project for testing
	tempDir, err := os.MkdirTemp("", "ard_error_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "error_project"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		input       string
		expectError string
	}{
		{
			name:        "wrong project name",
			input:       `use other_project/utils`,
			expectError: "does not match project name",
		},
		{
			name:        "missing module file",
			input:       `use error_project/nonexistent`,
			expectError: "module file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			astTree, err := ast.Parse([]byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}

			_, diagnostics := checker.Check(astTree, resolver)
			if len(diagnostics) == 0 {
				t.Error("Expected error but got none")
				return
			}

			found := false
			for _, diag := range diagnostics {
				if strings.Contains(diag.Message, tt.expectError) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected error containing '%s', got: %v", tt.expectError, diagnostics)
			}
		})
	}
}

func TestModuleResolverWithoutArdToml(t *testing.T) {
	// Create a temporary directory without ard.toml
	tempDir, err := os.MkdirTemp("", "fallback_project_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a module file
	moduleContent := `pub fn helper() -> Int { 42 }`
	err = os.WriteFile(filepath.Join(tempDir, "utils.ard"), []byte(moduleContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatalf("Failed to create module resolver: %v", err)
	}

	project := resolver.GetProjectInfo()
	expectedName := filepath.Base(tempDir)
	if project.ProjectName != expectedName {
		t.Errorf("Expected project name '%s', got '%s'", expectedName, project.ProjectName)
	}

	// Test import with fallback project name
	importPath := expectedName + "/utils"
	filePath, err := resolver.ResolveImportPath(importPath)
	if err != nil {
		t.Fatalf("Failed to resolve import with fallback name: %v", err)
	}

	expectedPath := filepath.Join(tempDir, "utils.ard")
	if filePath != expectedPath {
		t.Errorf("Expected path '%s', got '%s'", expectedPath, filePath)
	}
}
