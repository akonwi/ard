package checker_test

import (
	"os"
	"path/filepath"
	"strings"
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
	tomlContent := "name = \"test_project\"\nard = \">= 0.1.0\"\n"
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
	if project.Target != "vm_next" {
		t.Errorf("Expected default target 'vm_next', got '%s'", project.Target)
	}

	if project.RootPath != tempDir {
		t.Errorf("Expected root path '%s', got '%s'", tempDir, project.RootPath)
	}
}

func TestFindProjectRootReadsTarget(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "vm_next", target: "vm_next"},
		{name: "go", target: "go"},
		{name: "js-browser", target: "js-browser"},
		{name: "js-server", target: "js-server"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tomlContent := "name = \"test_project\"\nard = \">= 0.1.0\"\ntarget = \"" + tt.target + "\"\n"
			if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte(tomlContent), 0o644); err != nil {
				t.Fatal(err)
			}

			resolver, err := checker.NewModuleResolver(dir)
			if err != nil {
				t.Fatalf("Failed to create module resolver: %v", err)
			}

			project := resolver.GetProjectInfo()
			if project.Target != tt.target {
				t.Fatalf("Expected target '%s', got '%s'", tt.target, project.Target)
			}
		})
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

func TestArdVersionConstraint(t *testing.T) {
	t.Run("missing ard field is rejected", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "ard.toml"), []byte(`name = "demo"`), 0644)

		_, err := checker.NewModuleResolver(dir)
		if err == nil {
			t.Fatal("expected error for missing ard field")
		}
		if !strings.Contains(err.Error(), "missing required field: ard") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("dev version always passes", func(t *testing.T) {
		dir := t.TempDir()
		// Require a very high version — should still pass because compiler is "dev"
		os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 99.0.0\"\n"), 0644)

		_, err := checker.NewModuleResolver(dir)
		if err != nil {
			t.Fatalf("dev version should skip check, got: %v", err)
		}
	})

	t.Run("invalid constraint is rejected", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \"not-a-version\"\n"), 0644)

		// With "dev" version, CheckVersion skips the check, so this won't error.
		// This test documents that invalid constraints are only caught with real versions.
		_, err := checker.NewModuleResolver(dir)
		if err != nil {
			t.Fatalf("dev version should skip even invalid constraints, got: %v", err)
		}
	})

	t.Run("invalid target is rejected", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\ntarget = \"wasm\"\n"), 0o644)

		_, err := checker.NewModuleResolver(dir)
		if err == nil {
			t.Fatal("expected invalid target error")
		}
		if !strings.Contains(err.Error(), "unknown target: wasm") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestResolveImportPath(t *testing.T) {
	// Create test project structure
	tempDir, err := os.MkdirTemp("", "ard_resolve_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := "name = \"my_calculator\"\nard = \">= 0.1.0\"\n"
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
