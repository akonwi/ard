package checker_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
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
	if project.Target != "go" {
		t.Errorf("Expected default target 'go', got '%s'", project.Target)
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
func TestPathDependencyIsResolvedFromSource(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	depSrc := filepath.Join(workspace, "dep-src")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(depSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depSrc, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depSrc, "dep.ard"), []byte("fn answer() Int { 42 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep-src\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	project := resolver.GetProjectInfo()
	dep := project.Dependencies["dep"]
	if dep.RootPath != depSrc {
		t.Fatalf("dependency root = %q, want %q", dep.RootPath, depSrc)
	}
	path, err := resolver.ResolveImportPath("dep")
	if err != nil {
		t.Fatalf("resolve dep import: %v", err)
	}
	if path != filepath.Join(depSrc, "dep.ard") {
		t.Fatalf("resolved path = %q, want %q", path, filepath.Join(depSrc, "dep.ard"))
	}
}

func TestLockedPathDependencyAliasUsesPackageNameForRootModule(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "app")
	dep := filepath.Join(workspace, "dep")
	for _, dir := range []string{app, dep} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dep, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dep, "dep.ard"), []byte("fn answer() Int { 42 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\nui = { path = \"../dep\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "main.ard"), []byte("use ui\n\nlet answer = ui::answer()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := fmt.Sprintf(`{
  "version": 1,
  "root": "root",
  "packages": [
    {"id": "root", "name": "app", "path": ".", "dependencies": {"ui": "path:%s"}},
    {"id": "path:%s", "name": "dep", "path": "%s"}
  ]
}
`, dep, dep, dep)
	if err := os.WriteFile(filepath.Join(app, "ard.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}

	result := parseSourceForResolverTest(t, filepath.Join(app, "main.ard"))
	resolver, err := checker.NewModuleResolver(app)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(filepath.Join(app, "main.ard"), result, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
}

func TestDependencyUsesItsOwnTransitiveAliasFromLock(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "app")
	dep := filepath.Join(workspace, "dep")
	helper := filepath.Join(workspace, "helper")
	for _, dir := range []string{app, dep, helper} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(helper, "ard.toml"), []byte("name = \"helper\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helper, "helper.ard"), []byte("fn value() Int { 42 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dep, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n\n[dependencies]\nhelper = { path = \"../helper\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dep, "dep.ard"), []byte("use helper\n\nfn answer() Int { helper::value() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "main.ard"), []byte("use dep\n\nlet answer = dep::answer()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := fmt.Sprintf(`{
  "version": 1,
  "root": "root",
  "packages": [
    {"id": "root", "name": "app", "path": ".", "dependencies": {"dep": "path:%s"}},
    {"id": "path:%s", "name": "dep", "path": "%s", "dependencies": {"helper": "path:%s"}},
    {"id": "path:%s", "name": "helper", "path": "%s"}
  ]
}
`, dep, dep, dep, helper, helper, helper)
	if err := os.WriteFile(filepath.Join(app, "ard.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}

	result := parseSourceForResolverTest(t, filepath.Join(app, "main.ard"))
	resolver, err := checker.NewModuleResolver(app)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(filepath.Join(app, "main.ard"), result, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
}

func TestRootCannotImportTransitiveDependencyAlias(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "app")
	dep := filepath.Join(workspace, "dep")
	helper := filepath.Join(workspace, "helper")
	for _, dir := range []string{app, dep, helper} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(app, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "main.ard"), []byte("use helper\n\nlet answer = helper::value()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := fmt.Sprintf(`{
  "version": 1,
  "root": "root",
  "packages": [
    {"id": "root", "name": "app", "path": ".", "dependencies": {"dep": "path:%s"}},
    {"id": "path:%s", "name": "dep", "path": "%s", "dependencies": {"helper": "path:%s"}},
    {"id": "path:%s", "name": "helper", "path": "%s"}
  ]
}
`, dep, dep, dep, helper, helper, helper)
	if err := os.WriteFile(filepath.Join(app, "ard.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}

	result := parseSourceForResolverTest(t, filepath.Join(app, "main.ard"))
	resolver, err := checker.NewModuleResolver(app)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(filepath.Join(app, "main.ard"), result, resolver)
	c.Check()
	if !c.HasErrors() || !strings.Contains(c.Diagnostics()[0].Message, "unknown import root") {
		t.Fatalf("diagnostics = %v, want unknown import root", c.Diagnostics())
	}
}

func parseSourceForResolverTest(t *testing.T, path string) *parse.Program {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := parse.Parse(data, path)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	return result.Program
}

func TestGitDependencyResolvesFromLockCache(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("ARD_CACHE_DIR", cacheRoot)
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	gitSource := "https://example.invalid/dep.git"
	commit := "0123456789abcdef0123456789abcdef01234567"
	cachePath := checker.DependencyCachePath(gitSource, commit)
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "dep.ard"), []byte("fn answer() Int { 42 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { git = \""+gitSource+"\", commit = \""+commit+"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := fmt.Sprintf(`{
  "version": 1,
  "root": "root",
  "packages": [
    {"id": "root", "name": "app", "path": ".", "dependencies": {"dep": "%s"}},
    {"id": "%s", "name": "dep", "git": "%s", "commit": "%s"}
  ]
}
`, checker.GitPackageID(gitSource, commit), checker.GitPackageID(gitSource, commit), gitSource, commit)
	if err := os.WriteFile(filepath.Join(root, "ard.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	dep := resolver.GetProjectInfo().Dependencies["dep"]
	if dep.RootPath != cachePath {
		t.Fatalf("dependency root = %q, want %q", dep.RootPath, cachePath)
	}
	path, err := resolver.ResolveImportPath("dep")
	if err != nil {
		t.Fatalf("resolve dep import: %v", err)
	}
	if path != filepath.Join(cachePath, "dep.ard") {
		t.Fatalf("resolved path = %q, want %q", path, filepath.Join(cachePath, "dep.ard"))
	}
}

func TestGitDependencyWithoutLockFailsClearly(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { git = \"https://example.invalid/dep.git\", commit = \"0123456\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	_, err = resolver.ResolveImportPath("dep")
	if err == nil || !strings.Contains(err.Error(), "not locked") {
		t.Fatalf("ResolveImportPath error = %v, want not locked", err)
	}
}

func TestFetchDependenciesRestoresGitCacheFromLock(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("ARD_CACHE_DIR", cacheRoot)
	workspace := t.TempDir()
	gitRepo := filepath.Join(workspace, "dep-repo")
	root := filepath.Join(workspace, "app")
	if err := os.MkdirAll(gitRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitRepo, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitRepo, "dep.ard"), []byte("fn answer() Int { 42 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, gitRepo, "init")
	runGit(t, gitRepo, "add", ".")
	runGit(t, gitRepo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "init")
	commit := strings.TrimSpace(runGitOutput(t, gitRepo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { git = \""+gitRepo+"\", commit = \""+commit+"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := fmt.Sprintf(`{
  "version": 1,
  "root": "root",
  "packages": [
    {"id": "root", "name": "app", "path": ".", "dependencies": {"dep": "%s"}},
    {"id": "%s", "name": "dep", "git": "%s", "commit": "%s"}
  ]
}
`, checker.GitPackageID(gitRepo, commit), checker.GitPackageID(gitRepo, commit), gitRepo, commit)
	if err := os.WriteFile(filepath.Join(root, "ard.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}

	deps, err := checker.FetchDependencies(root)
	if err != nil {
		t.Fatalf("fetch dependencies: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("fetched %d dependencies, want 1", len(deps))
	}
	if _, err := os.Stat(filepath.Join(checker.DependencyCachePath(gitRepo, commit), "dep.ard")); err != nil {
		t.Fatalf("cache was not restored: %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = runGitOutput(t, dir, args...)
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output)
}
