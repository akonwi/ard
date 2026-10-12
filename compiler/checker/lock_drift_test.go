package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
)

// lockDriftFixture locks `dep` to a commit of a local Git repository, the way
// `ard add` does, and returns the paths needed to rewrite the manifest (#542).
type lockDriftFixture struct {
	root      string
	workspace string
	repo      string
	commit    string
}

func newLockDriftFixture(t *testing.T) lockDriftFixture {
	t.Helper()
	t.Setenv("ARD_CACHE_DIR", t.TempDir())
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, commit := createGitPackageRepo(t, workspace, "dep-repo", map[string]string{
		"ard.toml": "name = \"dep\"\nard = \">= 0.1.0\"\n",
		"dep.ard":  "fn answer() Int { 1 }\n",
	})
	lock, err := checker.LockDependencyGraph(root, "app", "dep", checker.DependencyInfo{Alias: "dep", Name: "dep", Git: repo, Commit: commit}, "dep", commit)
	if err != nil {
		t.Fatalf("lock dependency graph: %v", err)
	}
	if err := checker.WriteDependencyLock(root, lock); err != nil {
		t.Fatal(err)
	}
	fixture := lockDriftFixture{root: root, workspace: workspace, repo: repo, commit: commit}
	fixture.writeDependency(t, `dep = { git = "`+repo+`", commit = "`+commit+`" }`)
	if _, err := checker.FetchDependency(root, "dep"); err != nil {
		t.Fatalf("fetch locked dependency: %v", err)
	}
	return fixture
}

func (f lockDriftFixture) writeDependency(t *testing.T, entry string) {
	t.Helper()
	manifest := "name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\n" + entry + "\n"
	if err := os.WriteFile(filepath.Join(f.root, "ard.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

// localCheckout writes a path dependency with a function the locked commit
// does not have.
func (f lockDriftFixture) localCheckout(t *testing.T) string {
	t.Helper()
	local := filepath.Join(f.workspace, "dep-local")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "dep.ard"), []byte("fn answer() Int { 1 }\n\nfn only_in_local_checkout() Int { 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return local
}

func TestLockedDependencyMatchingManifestIsNotDrift(t *testing.T) {
	f := newLockDriftFixture(t)
	for _, entry := range []string{
		`dep = { git = "` + f.repo + `", commit = "` + f.commit + `" }`,
		`dep = { git = "` + f.repo + `", commit = "` + f.commit[:7] + `" }`,
	} {
		f.writeDependency(t, entry)
		resolver, err := checker.NewModuleResolver(f.root)
		if err != nil {
			t.Fatalf("new resolver: %v", err)
		}
		dep := resolver.GetProjectInfo().Dependencies["dep"]
		if dep.LockDrift != "" {
			t.Fatalf("%s: unexpected lock drift: %s", entry, dep.LockDrift)
		}
		if _, err := resolver.ResolveImportPath("dep"); err != nil {
			t.Fatalf("%s: resolve: %v", entry, err)
		}
		if err := checker.VerifyLockMatchesManifest(f.root); err != nil {
			t.Fatalf("%s: verify lock: %v", entry, err)
		}
		if err := checker.VerifyDependencies(f.root); err != nil {
			t.Fatalf("%s: verify: %v", entry, err)
		}
	}
}

func TestPathDependencyOverridesStaleGitLock(t *testing.T) {
	f := newLockDriftFixture(t)
	local := f.localCheckout(t)
	f.writeDependency(t, `dep = { path = "../dep-local" }`)

	resolver, err := checker.NewModuleResolver(f.root)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	path, err := resolver.ResolveImportPath("dep")
	if err != nil {
		t.Fatalf("resolve dep import: %v", err)
	}
	if want := filepath.Join(local, "dep.ard"); path != want {
		t.Fatalf("resolved path = %q, want the local checkout %q", path, want)
	}
	project := resolver.GetProjectInfo()
	for id, pkg := range project.Packages {
		if pkg.Git != "" {
			t.Fatalf("stale locked Git package %s is still part of the project", id)
		}
	}

	// Builds verify dependencies on load; the path they use needs no pin.
	if err := checker.VerifyDependencies(f.root); err != nil {
		t.Fatalf("build-time verification rejected the path dependency: %v", err)
	}
	err = checker.VerifyLockMatchesManifest(f.root)
	if err == nil {
		t.Fatal("lock verification passed although ard.lock pins dep to a Git commit")
	}
	for _, want := range []string{`"dep"`, "path ../dep-local", f.commit[:7], "ard update dep"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("verify error = %q, want it to mention %q", err, want)
		}
	}
}

func TestEditedGitRefFailsAgainstStaleLock(t *testing.T) {
	f := newLockDriftFixture(t)
	for name, entry := range map[string]string{
		"different commit": `dep = { git = "` + f.repo + `", commit = "fedcba9876543210fedcba9876543210fedcba98" }`,
		"tag":              `dep = { git = "` + f.repo + `", tag = "v2.0.0" }`,
		"different source": `dep = { git = "https://example.invalid/other.git", commit = "` + f.commit + `" }`,
	} {
		t.Run(name, func(t *testing.T) {
			f.writeDependency(t, entry)
			resolver, err := checker.NewModuleResolver(f.root)
			if err != nil {
				t.Fatalf("new resolver: %v", err)
			}
			_, err = resolver.ResolveImportPath("dep")
			if err == nil || !strings.Contains(err.Error(), "ard.lock is out of date for dependency \"dep\"") {
				t.Fatalf("resolve error = %v, want an out-of-date lock error", err)
			}
			if !strings.Contains(err.Error(), "ard add") {
				t.Fatalf("resolve error = %q, want a hint to re-lock with ard add", err)
			}
			if err := checker.VerifyLockMatchesManifest(f.root); err == nil || !strings.Contains(err.Error(), "ard.lock is out of date for dependency \"dep\"") {
				t.Fatalf("verify lock error = %v, want an out-of-date lock error", err)
			}
			// Builds verify on load, so they stop before compiling the old commit.
			if err := checker.VerifyDependencies(f.root); err == nil || !strings.Contains(err.Error(), "ard.lock is out of date for dependency \"dep\"") {
				t.Fatalf("verify error = %v, want an out-of-date lock error", err)
			}
			if _, err := checker.FetchDependencies(f.root); err == nil || !strings.Contains(err.Error(), "ard.lock is out of date") {
				t.Fatalf("fetch error = %v, want an out-of-date lock error", err)
			}
		})
	}
}
