package analysis

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestAnalyzeReportsDiagnostics(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.ard": "fn main() {\n  let x: Str = 42\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	snap := ws.Snapshot()

	fa, err := snap.Analyze(filepath.Join(root, "main.ard"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fa.Diagnostics) == 0 {
		t.Fatal("expected type mismatch diagnostic")
	}
}

func TestAnalyzeMemoizesAcrossSnapshots(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.ard": "fn main() {\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	path := filepath.Join(root, "main.ard")

	fa1, err := ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	// A new snapshot with unchanged content must reuse the same analysis.
	fa2, err := ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if fa1 != fa2 {
		t.Fatal("expected memoized analysis to be reused for identical content")
	}
}

func TestOverlayChangeInvalidatesAnalysis(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.ard": "fn main() {\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	path := filepath.Join(root, "main.ard")

	fa1, err := ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetOverlay(path, "fn main() {\n  let x: Str = 42\n}\n")
	fa2, err := ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if fa1 == fa2 {
		t.Fatal("expected new analysis after overlay change")
	}
	if len(fa2.Diagnostics) == 0 {
		t.Fatal("expected diagnostic from overlay content")
	}
}

func TestDependencyOverlayInvalidatesDependent(t *testing.T) {
	root := writeProject(t, map[string]string{
		"ard.toml": "name = \"proj\"\nard = \">= 0.1.0\"\n",
		"lib.ard":  "fn helper() Int {\n  1\n}\n",
		"main.ard": "use proj/lib\n\nfn main() {\n  let x = lib::helper()\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	mainPath := filepath.Join(root, "main.ard")
	libPath := filepath.Join(root, "lib.ard")

	fa1, err := ws.Snapshot().Analyze(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(fa1.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", fa1.Diagnostics)
	}

	// Break the dependency in an overlay: helper now needs an argument.
	ws.SetOverlay(libPath, "fn helper(n: Int) Int {\n  n\n}\n")
	fa2, err := ws.Snapshot().Analyze(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if fa1 == fa2 {
		t.Fatal("dependency change did not invalidate dependent analysis")
	}
	if len(fa2.Diagnostics) == 0 {
		t.Fatal("expected arity diagnostic after dependency change")
	}
}

func TestUnrelatedOverlayKeepsMemoizedAnalysis(t *testing.T) {
	root := writeProject(t, map[string]string{
		"ard.toml":  "name = \"proj\"\nard = \">= 0.1.0\"\n",
		"main.ard":  "fn main() {\n}\n",
		"other.ard": "fn other() {\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	mainPath := filepath.Join(root, "main.ard")
	otherPath := filepath.Join(root, "other.ard")

	fa1, err := ws.Snapshot().Analyze(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetOverlay(otherPath, "fn other() Int {\n  1\n}\n")
	fa2, err := ws.Snapshot().Analyze(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if fa1 != fa2 {
		t.Fatal("unrelated overlay change invalidated memoized analysis")
	}
}

func TestParseErrorsShortCircuitChecking(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.ard": "fn main( {\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	fa, err := ws.Snapshot().Analyze(filepath.Join(root, "main.ard"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fa.ParseErrors) == 0 {
		t.Fatal("expected parse errors")
	}
	if fa.Checker != nil {
		t.Fatal("checker should not run on parse errors")
	}
}

func TestSnapshotIsImmutableAfterWorkspaceChanges(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.ard": "fn main() {\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	path := filepath.Join(root, "main.ard")

	snap := ws.Snapshot()
	ws.SetOverlay(path, "fn main() {\n  broken(\n}\n")

	content, err := snap.Content(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "fn main() {\n}\n" {
		t.Fatal("snapshot observed later workspace mutation")
	}
}

func TestAnalyzeRecordsSpans(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.ard": "fn main() {\n  let x = 1\n  let y = x\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	fa, err := ws.Snapshot().Analyze(filepath.Join(root, "main.ard"))
	if err != nil {
		t.Fatal(err)
	}
	if fa.Spans == nil || fa.Spans.Len() == 0 {
		t.Fatal("expected recorded spans")
	}
}

func TestSubdirectoryModuleResolution(t *testing.T) {
	root := writeProject(t, map[string]string{
		"ard.toml":    "name = \"proj\"\nard = \">= 0.1.0\"\n",
		"sub/lib.ard": "fn helper() Int {\n  1\n}\n",
		"main.ard":    "use proj/sub/lib\n\nfn main() {\n  let x = lib::helper()\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	mainPath := filepath.Join(root, "main.ard")

	fa1, err := ws.Snapshot().Analyze(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(fa1.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", fa1.Diagnostics)
	}

	// Editing the subdirectory dep must invalidate the dependent.
	ws.SetOverlay(filepath.Join(root, "sub", "lib.ard"), "fn helper(n: Int) Int {\n  n\n}\n")
	fa2, err := ws.Snapshot().Analyze(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if fa1 == fa2 {
		t.Fatal("subdirectory dependency change did not invalidate analysis")
	}
	if len(fa2.Diagnostics) == 0 {
		t.Fatal("expected diagnostic after dependency change")
	}
}

func TestManifestChangeInvalidatesAnalysis(t *testing.T) {
	root := writeProject(t, map[string]string{
		"ard.toml": "name = \"proj\"\nard = \">= 0.1.0\"\n",
		"main.ard": "fn main() {\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	path := filepath.Join(root, "main.ard")

	fa1, err := ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"renamed\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fa2, err := ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if fa1 == fa2 {
		t.Fatal("manifest change did not invalidate analysis")
	}
}

func TestConcurrentAnalyzeIsPointerStable(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.ard": "fn main() {\n  let x = 1\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	path := filepath.Join(root, "main.ard")
	snap := ws.Snapshot()

	const workers = 8
	results := make([]*FileAnalysis, workers)
	errs := make([]error, workers)
	done := make(chan int, workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			results[i], errs[i] = snap.Analyze(path)
			done <- i
		}(i)
	}
	for i := 0; i < workers; i++ {
		<-done
	}
	for i := 0; i < workers; i++ {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
	}
	// After the dust settles, a fresh Analyze must return the cached winner.
	winner, err := snap.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < workers; i++ {
		if results[i] != winner {
			t.Fatalf("worker %d returned a non-cached instance; insert race must serve the cached winner", i)
		}
	}
}

func TestCheckCacheEviction(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.ard": "fn main() {\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	path := filepath.Join(root, "main.ard")

	first, err := ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	// Push more than maxCheckEntries distinct signatures through the cache.
	for i := 0; i <= maxCheckEntries; i++ {
		ws.SetOverlay(path, "fn main() {\n  let a = "+string(rune('0'+i%10))+"\n  let b = "+string(rune('a'+i%26))+"\n}\n")
		if _, err := ws.Snapshot().Analyze(path); err != nil {
			t.Fatal(err)
		}
	}
	// The original entry was evicted; re-analyzing must still work.
	ws.DeleteOverlay(path)
	again, err := ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if again == nil || again.Checker == nil {
		t.Fatal("re-analysis after eviction failed")
	}
	_ = first
}

func TestOverlayOnlyFileAnalyzes(t *testing.T) {
	root := writeProject(t, map[string]string{
		"ard.toml": "name = \"proj\"\nard = \">= 0.1.0\"\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	path := filepath.Join(root, "scratch.ard")
	ws.SetOverlay(path, "fn main() {\n  let x: Str = 1\n}\n")

	fa, err := ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(fa.Diagnostics) == 0 {
		t.Fatal("expected diagnostics from overlay-only file")
	}
}
