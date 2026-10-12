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

// writeLockDriftManifest replaces the app manifest's dependencies.
func writeLockDriftManifest(t *testing.T, root string, entries ...string) {
	t.Helper()
	manifest := "name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\n" + strings.Join(entries, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newLockDriftApp(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("ARD_CACHE_DIR", t.TempDir())
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLockDriftManifest(t, root)
	return workspace, root
}

func writePackage(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A local checkout of a locked Git dependency reuses the lock's pins for the
// checkout's own Git dependencies, so switching to the path builds at once.
func TestPathOverrideLinksGitDependenciesOfTheCheckout(t *testing.T) {
	workspace, root := newLockDriftApp(t)
	libRepo, libCommit := createGitPackageRepo(t, workspace, "lib-repo", map[string]string{
		"ard.toml": "name = \"lib\"\nard = \">= 0.1.0\"\n",
		"lib.ard":  "fn value() Int { 1 }\n",
	})
	depManifest := "name = \"dep\"\nard = \">= 0.1.0\"\n\n[dependencies]\nlib = { git = \"" + libRepo + "\", commit = \"" + libCommit + "\" }\n"
	depFiles := map[string]string{"ard.toml": depManifest, "dep.ard": "use lib\n\nfn answer() Int { lib::value() }\n"}
	depRepo, depCommit := createGitPackageRepo(t, workspace, "dep-repo", depFiles)
	lock, err := checker.LockDependencyGraph(root, "app", "dep", checker.DependencyInfo{Alias: "dep", Name: "dep", Git: depRepo, Commit: depCommit}, "dep", depCommit)
	if err != nil {
		t.Fatalf("lock dependency graph: %v", err)
	}
	if err := checker.WriteDependencyLock(root, lock); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(workspace, "dep-local")
	writePackage(t, local, depFiles)
	writeLockDriftManifest(t, root, `dep = { path = "../dep-local" }`)

	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	depImport, err := resolver.ResolveImport("", "dep")
	if err != nil {
		t.Fatalf("resolve dep: %v", err)
	}
	if want := filepath.Join(local, "dep.ard"); depImport.FilePath != want {
		t.Fatalf("dep resolved to %q, want %q", depImport.FilePath, want)
	}
	libImport, err := resolver.ResolveImport(depImport.ModulePath, "lib")
	if err != nil {
		t.Fatalf("resolve the checkout's Git dependency: %v", err)
	}
	if want := filepath.Join(checker.DependencyCachePath(libRepo, libCommit), "lib.ard"); libImport.FilePath != want {
		t.Fatalf("lib resolved to %q, want the locked %q", libImport.FilePath, want)
	}
	if err := checker.VerifyDependencies(root); err != nil {
		t.Fatalf("build-time verification: %v", err)
	}

	// A Git dependency the lock never pinned is reported with a fix.
	depFiles["ard.toml"] = depManifest + "extra = { git = \"https://example.invalid/extra.git\", tag = \"v1.0.0\" }\n"
	depFiles["dep.ard"] = "use extra\n\nfn answer() Int { extra::value() }\n"
	writePackage(t, local, depFiles)
	resolver, err = checker.NewModuleResolver(root)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	depImport, err = resolver.ResolveImport("", "dep")
	if err != nil {
		t.Fatalf("resolve dep: %v", err)
	}
	if _, err := resolver.ResolveImport(depImport.ModulePath, "extra"); err == nil || !strings.Contains(err.Error(), "not pinned in ard.lock") || !strings.Contains(err.Error(), "ard update") {
		t.Fatalf("resolve unpinned Git dependency error = %v, want a hint to run ard update", err)
	}
	if err := checker.VerifyLockMatchesManifest(root); err == nil || !strings.Contains(err.Error(), "ard update dep") {
		t.Fatalf("verify lock error = %v, want a hint to run ard update dep", err)
	}
}

// Path dependencies outside the project are locked as absolute paths; a lock
// committed on one machine must keep verifying on another.
func TestLockedPathDependencyVerifiesAfterTheProjectMoves(t *testing.T) {
	workspace, root := newLockDriftApp(t)
	writePackage(t, filepath.Join(workspace, "shared"), map[string]string{
		"ard.toml":   "name = \"shared\"\nard = \">= 0.1.0\"\n",
		"shared.ard": "fn value() Int { 1 }\n",
	})
	writeLockDriftManifest(t, root, `shared = { path = "../shared" }`)
	lock, err := checker.LockDependencyGraph(root, "app", "shared", checker.DependencyInfo{Alias: "shared", Name: "shared", SourcePath: filepath.Join(workspace, "shared")}, "", "")
	if err != nil {
		t.Fatalf("lock dependency graph: %v", err)
	}
	if err := checker.WriteDependencyLock(root, lock); err != nil {
		t.Fatal(err)
	}

	moved := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Rename(workspace, moved); err != nil {
		t.Fatal(err)
	}
	movedRoot := filepath.Join(moved, "app")
	if err := checker.VerifyLockMatchesManifest(movedRoot); err != nil {
		t.Fatalf("verify lock after moving the project: %v", err)
	}
	resolver, err := checker.NewModuleResolver(movedRoot)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	path, err := resolver.ResolveImportPath("shared")
	if err != nil {
		t.Fatalf("resolve shared: %v", err)
	}
	if want := filepath.Join(moved, "shared", "shared.ard"); path != want {
		t.Fatalf("shared resolved to %q, want the declared path %q", path, want)
	}
}

func TestGitLockMatchingIsNotFooledByRefShapes(t *testing.T) {
	workspace, root := newLockDriftApp(t)
	repo, commit := createGitPackageRepo(t, workspace, "dep-repo", map[string]string{
		"ard.toml": "name = \"dep\"\nard = \">= 0.1.0\"\n",
		"dep.ard":  "fn answer() Int { 1 }\n",
	})
	runGit(t, repo, "tag", "v1.0.0")

	lockFromManifest := func(t *testing.T, entries ...string) {
		t.Helper()
		writeLockDriftManifest(t, root, entries...)
		_ = os.Remove(filepath.Join(root, "ard.lock"))
		lock, err := checker.UpdateDependencyGraph(root, "app", nil)
		if err != nil {
			t.Fatalf("lock from manifest: %v", err)
		}
		if err := checker.WriteDependencyLock(root, lock); err != nil {
			t.Fatal(err)
		}
	}
	assertNoDrift := func(t *testing.T) {
		t.Helper()
		if err := checker.VerifyLockMatchesManifest(root); err != nil {
			t.Fatalf("verify lock: %v", err)
		}
		if err := checker.VerifyDependencies(root); err != nil {
			t.Fatalf("verify: %v", err)
		}
	}

	t.Run("matching tag", func(t *testing.T) {
		lockFromManifest(t, `dep = { git = "`+repo+`", tag = "v1.0.0" }`)
		assertNoDrift(t)
	})
	t.Run("one source under a tag alias and a commit alias", func(t *testing.T) {
		lockFromManifest(t, `a = { git = "`+repo+`", tag = "v1.0.0" }`, `b = { git = "`+repo+`", commit = "`+commit+`" }`)
		assertNoDrift(t)
	})
	t.Run("non-hex commit revision", func(t *testing.T) {
		lockFromManifest(t, `dep = { git = "`+repo+`", commit = "v1.0.0" }`)
		assertNoDrift(t)
	})
	t.Run("old lock without a requested ref", func(t *testing.T) {
		lockFromManifest(t, `dep = { git = "`+repo+`", tag = "v1.0.0" }`)
		data, err := os.ReadFile(filepath.Join(root, "ard.lock"))
		if err != nil {
			t.Fatal(err)
		}
		stripped := strings.ReplaceAll(string(data), `"requested": "v1.0.0",`, "")
		if stripped == string(data) {
			t.Fatalf("lock did not record the tag:\n%s", data)
		}
		if err := os.WriteFile(filepath.Join(root, "ard.lock"), []byte(stripped), 0o644); err != nil {
			t.Fatal(err)
		}
		assertNoDrift(t)
	})
	t.Run("commit changed to a tag", func(t *testing.T) {
		lockFromManifest(t, `dep = { git = "`+repo+`", commit = "`+commit+`" }`)
		writeLockDriftManifest(t, root, `dep = { git = "`+repo+`", tag = "v1.0.0" }`)
		if err := checker.VerifyLockMatchesManifest(root); err == nil || !strings.Contains(err.Error(), "ard add "+repo+"@v1.0.0") {
			t.Fatalf("verify lock error = %v, want a concrete ard add hint", err)
		}
	})
}
