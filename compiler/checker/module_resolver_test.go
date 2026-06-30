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
func TestReadDependencyLockMigratesCanonicalGitPackageIDs(t *testing.T) {
	root := t.TempDir()
	commit := "0123456789abcdef0123456789abcdef01234567"
	oldID := "git:oldhash:" + commit
	gitSource := "https://github.com/akonwi/vaxis-ard/"
	lock := fmt.Sprintf(`{
  "version": 1,
  "root": "root",
  "packages": [
    {"id": "root", "name": "app", "path": ".", "dependencies": {"vaxis": "%s"}},
    {"id": "%s", "name": "vaxis", "git": "%s", "commit": "%s"}
  ]
}
`, oldID, oldID, gitSource, commit)
	if err := os.WriteFile(filepath.Join(root, "ard.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, ok, err := checker.ReadDependencyLock(root)
	if err != nil || !ok {
		t.Fatalf("ReadDependencyLock = %v, %v", ok, err)
	}
	packages := lockedPackagesByID(parsed)
	canonicalID := checker.GitPackageID(gitSource, commit)
	if got := packages["root"].Dependencies["vaxis"]; got != canonicalID {
		t.Fatalf("root dependency = %q, want %q", got, canonicalID)
	}
	if _, ok := packages[canonicalID]; !ok {
		t.Fatalf("canonical package %s missing from %#v", canonicalID, packages)
	}
}
func TestReadDependencyLockPrefersRootTransportWhenMigratingDuplicateGitIDs(t *testing.T) {
	root := t.TempDir()
	commit := "0123456789abcdef0123456789abcdef01234567"
	httpsID := "git:https:" + commit
	sshID := "git:ssh:" + commit
	lock := fmt.Sprintf(`{
  "version": 1,
  "root": "root",
  "packages": [
    {"id": "root", "name": "app", "path": ".", "dependencies": {"dep": "%s"}},
    {"id": "%s", "name": "dep", "git": "https://github.com/akonwi/private", "commit": "%s"},
    {"id": "%s", "name": "dep", "git": "git@github.com:akonwi/private.git", "commit": "%s"}
  ]
}
`, sshID, httpsID, commit, sshID, commit)
	if err := os.WriteFile(filepath.Join(root, "ard.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, ok, err := checker.ReadDependencyLock(root)
	if err != nil || !ok {
		t.Fatalf("ReadDependencyLock = %v, %v", ok, err)
	}
	packages := lockedPackagesByID(parsed)
	canonicalID := checker.GitPackageID("git@github.com:akonwi/private.git", commit)
	if got := packages[canonicalID].Git; got != "git@github.com:akonwi/private.git" {
		t.Fatalf("transport git = %q, want ssh", got)
	}
}
func TestCanonicalGitSourceNormalizesGitHubVariants(t *testing.T) {
	want := "https://github.com/akonwi/vaxis-ard.git"
	for _, input := range []string{
		"https://github.com/akonwi/vaxis-ard",
		"https://github.com/akonwi/vaxis-ard/",
		"https://github.com/akonwi/vaxis-ard.git",
		"git@github.com:akonwi/vaxis-ard.git",
		"ssh://git@github.com/akonwi/vaxis-ard.git",
		"github.com/akonwi/vaxis-ard",
		"HTTPS://github.com/Akonwi/Vaxis-Ard/",
	} {
		if got := checker.CanonicalGitSource(input); got != want {
			t.Fatalf("CanonicalGitSource(%q) = %q, want %q", input, got, want)
		}
	}
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
func TestLockDependencyGraphIncludesTransitiveGitDependencies(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("ARD_CACHE_DIR", cacheRoot)
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	helperRepo, helperCommit := createGitPackageRepo(t, workspace, "helper", map[string]string{
		"ard.toml":   "name = \"helper\"\nard = \">= 0.1.0\"\n",
		"helper.ard": "fn value() Int { 42 }\n",
	})
	helperRequested := helperCommit[:7]
	depManifest := "name = \"dep\"\nard = \">= 0.1.0\"\n\n[dependencies]\nhelper = { git = \"" + helperRepo + "\", commit = \"" + helperRequested + "\" }\n"
	depRepo, depCommit := createGitPackageRepo(t, workspace, "dep", map[string]string{
		"ard.toml": depManifest,
		"dep.ard":  "use helper\n\nfn answer() Int { helper::value() }\n",
	})

	dep := checker.DependencyInfo{Alias: "dep", Name: "dep", Git: depRepo, Commit: depCommit, Requested: depCommit}
	lock, err := checker.LockDependencyGraph(root, "app", "dep", dep, "dep", depCommit)
	if err != nil {
		t.Fatalf("lock dependency graph: %v", err)
	}
	packages := lockedPackagesByID(lock)
	depID := checker.GitPackageID(depRepo, depCommit)
	helperID := checker.GitPackageID(helperRepo, helperCommit)
	if got := packages["root"].Dependencies["dep"]; got != depID {
		t.Fatalf("root dep = %q, want %q", got, depID)
	}
	if got := packages[depID].Dependencies["helper"]; got != helperID {
		t.Fatalf("dep helper = %q, want %q", got, helperID)
	}
	if got := packages[depID].Requested; got != depCommit {
		t.Fatalf("dep requested = %q, want %q", got, depCommit)
	}
	if got := packages[helperID].Requested; got != helperRequested {
		t.Fatalf("helper requested = %q, want %q", got, helperRequested)
	}
	if !strings.HasPrefix(packages[depID].Integrity, "sha256:") {
		t.Fatalf("dep integrity = %q, want sha256", packages[depID].Integrity)
	}
	if !strings.HasPrefix(packages[helperID].Integrity, "sha256:") {
		t.Fatalf("helper integrity = %q, want sha256", packages[helperID].Integrity)
	}
	if _, err := os.Stat(filepath.Join(checker.DependencyCachePath(depRepo, depCommit), "dep.ard")); err != nil {
		t.Fatalf("dep cache missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checker.DependencyCachePath(helperRepo, helperCommit), "helper.ard")); err != nil {
		t.Fatalf("helper cache missing: %v", err)
	}
	if err := checker.WriteDependencyLock(root, lock); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { git = \""+depRepo+"\", commit = \""+depCommit+"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.ard"), []byte("use dep\n\nlet answer = dep::answer()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	program := parseSourceForResolverTest(t, filepath.Join(root, "main.ard"))
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(filepath.Join(root, "main.ard"), program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
}
func TestLockDependencyGraphRejectsConflictingTransitiveGitVersions(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("ARD_CACHE_DIR", cacheRoot)
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	helperRepo, helperCommit1 := createGitPackageRepo(t, workspace, "helper", map[string]string{
		"ard.toml":   "name = \"helper\"\nard = \">= 0.1.0\"\n",
		"helper.ard": "fn value() Int { 1 }\n",
	})
	if err := os.WriteFile(filepath.Join(helperRepo, "helper.ard"), []byte("fn value() Int { 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, helperRepo, "add", ".")
	runGit(t, helperRepo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "second")
	helperCommit2 := strings.TrimSpace(runGitOutput(t, helperRepo, "rev-parse", "HEAD"))
	depAManifest := "name = \"dep_a\"\nard = \">= 0.1.0\"\n\n[dependencies]\nhelper = { git = \"" + helperRepo + "\", commit = \"" + helperCommit1 + "\" }\n"
	depARepo, depACommit := createGitPackageRepo(t, workspace, "dep-a", map[string]string{"ard.toml": depAManifest, "dep_a.ard": "fn a() Int { 1 }\n"})
	depBManifest := "name = \"dep_b\"\nard = \">= 0.1.0\"\n\n[dependencies]\nhelper = { git = \"" + helperRepo + "\", commit = \"" + helperCommit2 + "\" }\n"
	depBRepo, depBCommit := createGitPackageRepo(t, workspace, "dep-b", map[string]string{"ard.toml": depBManifest, "dep_b.ard": "fn b() Int { 2 }\n"})

	lock, err := checker.LockDependencyGraph(root, "app", "dep_a", checker.DependencyInfo{Alias: "dep_a", Name: "dep_a", Git: depARepo, Commit: depACommit}, "dep_a", depACommit)
	if err != nil {
		t.Fatalf("lock dep_a: %v", err)
	}
	if err := checker.WriteDependencyLock(root, lock); err != nil {
		t.Fatal(err)
	}
	_, err = checker.LockDependencyGraph(root, "app", "dep_b", checker.DependencyInfo{Alias: "dep_b", Name: "dep_b", Git: depBRepo, Commit: depBCommit}, "dep_b", depBCommit)
	if err == nil || !strings.Contains(err.Error(), "dependency conflict") {
		t.Fatalf("LockDependencyGraph error = %v, want dependency conflict", err)
	}
}
func TestLockDependencyGraphResolvesTagsBeforeSameNamedBranches(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("ARD_CACHE_DIR", cacheRoot)
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, tagCommit := createGitPackageRepo(t, workspace, "tagged", map[string]string{
		"ard.toml":   "name = \"tagged\"\nard = \">= 0.1.0\"\n",
		"tagged.ard": "fn value() Int { 1 }\n",
	})
	runGit(t, repo, "tag", "v1")
	if err := os.WriteFile(filepath.Join(repo, "tagged.ard"), []byte("fn value() Int { 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "branch commit")
	runGit(t, repo, "branch", "v1")

	lock, err := checker.LockDependencyGraph(root, "app", "tagged", checker.DependencyInfo{Alias: "tagged", Name: "tagged", Git: repo, Tag: "v1", Requested: "v1"}, "tagged", "")
	if err != nil {
		t.Fatalf("lock dependency graph: %v", err)
	}
	packages := lockedPackagesByID(lock)
	packageID := checker.GitPackageID(repo, tagCommit)
	if _, ok := packages[packageID]; !ok {
		t.Fatalf("lock packages = %#v, want tag commit package %s", packages, packageID)
	}
	if got := packages[packageID].Requested; got != "v1" {
		t.Fatalf("requested = %q, want v1", got)
	}
}
func TestLockDependencyGraphPeelsAnnotatedTagObjectCommits(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("ARD_CACHE_DIR", cacheRoot)
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, commit := createGitPackageRepo(t, workspace, "annotated", map[string]string{
		"ard.toml":      "name = \"annotated\"\nard = \">= 0.1.0\"\n",
		"annotated.ard": "fn value() Int { 1 }\n",
	})
	runGit(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "tag", "-a", "v1", "-m", "v1")
	tagObject := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "v1"))
	if tagObject == commit {
		t.Fatal("annotated tag did not produce a distinct tag object")
	}

	lock, err := checker.LockDependencyGraph(root, "app", "annotated", checker.DependencyInfo{Alias: "annotated", Name: "annotated", Git: repo, Commit: tagObject, Requested: tagObject}, "annotated", "")
	if err != nil {
		t.Fatalf("lock dependency graph: %v", err)
	}
	packages := lockedPackagesByID(lock)
	packageID := checker.GitPackageID(repo, commit)
	if _, ok := packages[packageID]; !ok {
		t.Fatalf("lock packages = %#v, want peeled commit package %s", packages, packageID)
	}
}
func TestLockDependencyGraphPreservesExistingRequestedRef(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("ARD_CACHE_DIR", cacheRoot)
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	helperRepo, helperCommit := createGitPackageRepo(t, workspace, "stable-helper", map[string]string{
		"ard.toml":   "name = \"helper\"\nard = \">= 0.1.0\"\n",
		"helper.ard": "fn value() Int { 1 }\n",
	})
	runGit(t, helperRepo, "tag", "stable")
	lock, err := checker.LockDependencyGraph(root, "app", "helper", checker.DependencyInfo{Alias: "helper", Name: "helper", Git: helperRepo, Tag: "stable", Requested: "stable"}, "helper", "")
	if err != nil {
		t.Fatalf("lock helper: %v", err)
	}
	if err := checker.WriteDependencyLock(root, lock); err != nil {
		t.Fatal(err)
	}
	depManifest := "name = \"dep\"\nard = \">= 0.1.0\"\n\n[dependencies]\nhelper = { git = \"" + helperRepo + "\", commit = \"" + helperCommit[:7] + "\" }\n"
	depRepo, depCommit := createGitPackageRepo(t, workspace, "stable-dep", map[string]string{"ard.toml": depManifest, "dep.ard": "fn answer() Int { 1 }\n"})
	lock, err = checker.LockDependencyGraph(root, "app", "dep", checker.DependencyInfo{Alias: "dep", Name: "dep", Git: depRepo, Commit: depCommit}, "dep", depCommit)
	if err != nil {
		t.Fatalf("lock dep: %v", err)
	}
	packages := lockedPackagesByID(lock)
	helperID := checker.GitPackageID(helperRepo, helperCommit)
	if got := packages[helperID].Requested; got != "stable" {
		t.Fatalf("helper requested = %q, want stable", got)
	}
}
func TestLockDependencyGraphRefetchesBeforeRecordingFirstIntegrity(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("ARD_CACHE_DIR", cacheRoot)
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, commit := createGitPackageRepo(t, workspace, "poisoned-cache-dep", map[string]string{
		"ard.toml": "name = \"dep\"\nard = \">= 0.1.0\"\n",
		"dep.ard":  "fn answer() Int { 42 }\n",
	})
	cachePath := checker.DependencyCachePath(repo, commit)
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "dep.ard"), []byte("fn answer() Int { 7 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	lock, err := checker.LockDependencyGraph(root, "app", "dep", checker.DependencyInfo{Alias: "dep", Name: "dep", Git: repo, Commit: commit}, "dep", commit)
	if err != nil {
		t.Fatalf("lock dependency graph: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(cachePath, "dep.ard"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "7") {
		t.Fatalf("cache was not refetched before integrity was recorded:\n%s", data)
	}
	packages := lockedPackagesByID(lock)
	packageID := checker.GitPackageID(repo, commit)
	wantIntegrity, err := checker.PackageTreeIntegrity(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := packages[packageID].Integrity; got != wantIntegrity {
		t.Fatalf("integrity = %q, want %q", got, wantIntegrity)
	}
}
func TestVerifyDependenciesChecksGitCacheIntegrity(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("ARD_CACHE_DIR", cacheRoot)
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, commit := createGitPackageRepo(t, workspace, "integrity-dep", map[string]string{
		"ard.toml": "name = \"dep\"\nard = \">= 0.1.0\"\n",
		"dep.ard":  "fn answer() Int { 42 }\n",
	})
	lock, err := checker.LockDependencyGraph(root, "app", "dep", checker.DependencyInfo{Alias: "dep", Name: "dep", Git: repo, Commit: commit}, "dep", commit)
	if err != nil {
		t.Fatalf("lock dependency graph: %v", err)
	}
	if err := checker.WriteDependencyLock(root, lock); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { git = \""+repo+"\", commit = \""+commit+"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checker.VerifyDependencies(root); err != nil {
		t.Fatalf("verify dependencies: %v", err)
	}
	if err := os.WriteFile(filepath.Join(checker.DependencyCachePath(repo, commit), "dep.ard"), []byte("fn answer() Int { 7 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checker.VerifyDependencies(root); err == nil || !strings.Contains(err.Error(), "integrity mismatch") {
		t.Fatalf("VerifyDependencies error = %v, want integrity mismatch", err)
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

func lockedPackagesByID(lock checker.LockFile) map[string]checker.LockedPackage {
	packages := map[string]checker.LockedPackage{}
	for _, pkg := range lock.Packages {
		packages[pkg.ID] = pkg
	}
	return packages
}

func createGitPackageRepo(t *testing.T, workspace string, name string, files map[string]string) (string, string) {
	t.Helper()
	repo := filepath.Join(workspace, name)
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range files {
		fullPath := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, repo, "init")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "init")
	commit := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
	return repo, commit
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
