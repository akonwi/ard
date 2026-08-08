package analysis

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

func TestAffectedFilesTracksTransitiveDependents(t *testing.T) {
	root := writeProject(t, map[string]string{
		"ard.toml":  "name = \"proj\"\nard = \">= 0.1.0\"\n",
		"leaf.ard":  "fn value() Int { 1 }\n",
		"mid.ard":   "use proj/leaf\n\nfn value() Int { leaf::value() }\n",
		"main.ard":  "use proj/mid\n\nfn main() { let _ = mid::value() }\n",
		"other.ard": "fn other() {}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	paths := []string{
		filepath.Join(root, "leaf.ard"),
		filepath.Join(root, "main.ard"),
		filepath.Join(root, "mid.ard"),
		filepath.Join(root, "other.ard"),
	}
	if _, err := ws.Snapshot().Analyze(filepath.Join(root, "main.ard")); err != nil {
		t.Fatal(err)
	}
	if _, complete := engine.AffectedFiles(filepath.Join(root, "leaf.ard"), paths); complete {
		t.Fatal("dependency information reported complete before every candidate was analyzed")
	}
	if _, err := ws.Snapshot().Analyze(filepath.Join(root, "other.ard")); err != nil {
		t.Fatal(err)
	}

	got, complete := engine.AffectedFiles(filepath.Join(root, "leaf.ard"), paths)
	if !complete {
		t.Fatal("dependency information remained incomplete after every candidate was analyzed")
	}
	want := []string{
		filepath.Join(root, "leaf.ard"),
		filepath.Join(root, "main.ard"),
		filepath.Join(root, "mid.ard"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("affected files = %#v, want %#v", got, want)
	}
}

func TestStaleSnapshotCannotRestoreRemovedDependencyEdge(t *testing.T) {
	root := writeProject(t, map[string]string{
		"ard.toml": "name = \"proj\"\nard = \">= 0.1.0\"\n",
		"leaf.ard": "fn value() Int { 1 }\n",
		"main.ard": "use proj/leaf\n\nfn main() { let _ = leaf::value() }\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	mainPath := filepath.Join(root, "main.ard")
	leafPath := filepath.Join(root, "leaf.ard")
	old := ws.Snapshot()
	if _, err := old.Analyze(mainPath); err != nil {
		t.Fatal(err)
	}

	ws.SetOverlay(mainPath, "fn main() {}\n")
	if _, err := ws.Snapshot().Analyze(mainPath); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Analyze(mainPath); err != nil {
		t.Fatal(err)
	}

	got, complete := engine.AffectedFiles(leafPath, []string{leafPath, mainPath})
	if !complete {
		t.Fatal("dependency information is incomplete")
	}
	if want := []string{leafPath}; !slices.Equal(got, want) {
		t.Fatalf("affected files after stale analysis = %#v, want %#v", got, want)
	}
}

func TestAffectedFilesIsIncompleteForUnresolvedDependencyClosure(t *testing.T) {
	root := writeProject(t, map[string]string{
		"ard.toml": "name = \"proj\"\nard = \">= 0.1.0\"\n",
		"main.ard": "use proj/missing\n\nfn main() {}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	mainPath := filepath.Join(root, "main.ard")
	if _, err := ws.Snapshot().Analyze(mainPath); err != nil {
		t.Fatal(err)
	}

	if _, complete := engine.AffectedFiles(mainPath, []string{mainPath}); complete {
		t.Fatal("unresolved import closure reported complete dependency information")
	}
}

func TestEphemeralAnalysisDoesNotChangeDependencies(t *testing.T) {
	root := writeProject(t, map[string]string{
		"ard.toml": "name = \"proj\"\nard = \">= 0.1.0\"\n",
		"leaf.ard": "fn value() Int { 1 }\n",
		"main.ard": "fn main() {}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	mainPath := filepath.Join(root, "main.ard")
	leafPath := filepath.Join(root, "leaf.ard")
	for _, path := range []string{mainPath, leafPath} {
		if _, err := ws.Snapshot().Analyze(path); err != nil {
			t.Fatal(err)
		}
	}
	candidates := []string{leafPath, mainPath}
	withImport := "use proj/leaf\n\nfn main() { let _ = leaf::value() }\n"
	if _, err := ws.Snapshot().WithOverlay(mainPath, withImport).AnalyzeEphemeral(context.Background(), mainPath); err != nil {
		t.Fatal(err)
	}
	if got, complete := engine.AffectedFiles(leafPath, candidates); !complete || !slices.Equal(got, []string{leafPath}) {
		t.Fatalf("affected files after synthetic import = %#v, complete %t", got, complete)
	}

	ws.SetOverlay(mainPath, withImport)
	if _, err := ws.Snapshot().Analyze(mainPath); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Snapshot().WithOverlay(mainPath, "fn main() {}\n").AnalyzeEphemeral(context.Background(), mainPath); err != nil {
		t.Fatal(err)
	}
	if got, complete := engine.AffectedFiles(leafPath, candidates); !complete || !slices.Equal(got, candidates) {
		t.Fatalf("affected files after synthetic import removal = %#v, complete %t", got, complete)
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
	if fa.Checked != nil {
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

func TestConcurrentCheckIsDeduplicated(t *testing.T) {
	engine := NewEngine(t.TempDir())
	want := &FileAnalysis{Signature: "shared"}
	started := make(chan struct{})
	release := make(chan struct{})
	var runs atomic.Int32
	run := func() (*FileAnalysis, error) {
		if runs.Add(1) == 1 {
			close(started)
			<-release
		}
		return want, nil
	}

	const workers = 8
	results := make(chan *FileAnalysis, workers)
	errs := make(chan error, workers)
	for range workers {
		go func() {
			result, err := engine.checkOnce(context.Background(), "shared", true, run)
			results <- result
			errs <- err
		}()
	}
	<-started
	close(release)
	for range workers {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		if result := <-results; result != want {
			t.Fatalf("result = %p, want shared result %p", result, want)
		}
	}
	if got := runs.Load(); got != 1 {
		t.Fatalf("check ran %d times, want once", got)
	}
}

func TestCanceledCheckWaiterDoesNotCancelSharedWork(t *testing.T) {
	engine := NewEngine(t.TempDir())
	want := &FileAnalysis{Signature: "shared"}
	started := make(chan struct{})
	release := make(chan struct{})
	var runs atomic.Int32
	run := func() (*FileAnalysis, error) {
		runs.Add(1)
		close(started)
		<-release
		return want, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := engine.checkOnce(ctx, "shared", true, run)
		firstDone <- err
	}()
	<-started
	cancel()
	select {
	case err := <-firstDone:
		if err != context.Canceled {
			t.Fatalf("canceled waiter error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter did not return while shared check was running")
	}

	secondDone := make(chan *FileAnalysis, 1)
	secondErr := make(chan error, 1)
	go func() {
		result, err := engine.checkOnce(context.Background(), "shared", true, run)
		secondDone <- result
		secondErr <- err
	}()
	close(release)
	if err := <-secondErr; err != nil {
		t.Fatal(err)
	}
	if result := <-secondDone; result != want {
		t.Fatalf("shared result = %p, want %p", result, want)
	}
	if got := runs.Load(); got != 1 {
		t.Fatalf("check ran %d times after waiter cancellation, want once", got)
	}
}

func TestPanickedCheckIsSharedButNotCached(t *testing.T) {
	engine := NewEngine(t.TempDir())
	var runs atomic.Int32
	_, err := engine.checkOnce(context.Background(), "panic", true, func() (*FileAnalysis, error) {
		runs.Add(1)
		panic("checker exploded")
	})
	if err == nil || !strings.Contains(err.Error(), "analysis panic: checker exploded") {
		t.Fatalf("panic error = %v", err)
	}

	want := &FileAnalysis{Signature: "panic"}
	got, err := engine.checkOnce(context.Background(), "panic", true, func() (*FileAnalysis, error) {
		runs.Add(1)
		return want, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("retry result = %p, want %p", got, want)
	}
	if count := runs.Load(); count != 2 {
		t.Fatalf("check attempts = %d, want panic plus retry", count)
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
	if again == nil || again.Checked == nil {
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

func TestSnapshotWithOverlayIsIsolated(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.ard": "fn main() {\n}\n",
		"api.ard":  "let disk_value = 0\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	mainPath := filepath.Join(root, "main.ard")
	apiPath := filepath.Join(root, "api.ard")
	ws.SetOverlay(apiPath, "let api_overlay = 1\n")
	base := ws.Snapshot()
	synthetic := base.WithOverlay(mainPath, "let synthetic = 2\n")
	ws.SetOverlay(apiPath, "let later_workspace = 3\n")

	for _, test := range []struct {
		name string
		snap *Snapshot
		path string
		want string
	}{
		{name: "synthetic target", snap: synthetic, path: mainPath, want: "let synthetic = 2\n"},
		{name: "synthetic sibling", snap: synthetic, path: apiPath, want: "let api_overlay = 1\n"},
		{name: "base target", snap: base, path: mainPath, want: "fn main() {\n}\n"},
		{name: "base sibling", snap: base, path: apiPath, want: "let api_overlay = 1\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			content, err := test.snap.Content(test.path)
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != test.want {
				t.Fatalf("content = %q, want %q", content, test.want)
			}
		})
	}
	if synthetic.Revision() != base.Revision() {
		t.Fatalf("synthetic revision = %d, want base revision %d", synthetic.Revision(), base.Revision())
	}
}

func TestAnalyzeCtxHonorsCancellation(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.ard": "fn main() {\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	path := filepath.Join(root, "main.ard")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ws.Snapshot().AnalyzeCtx(ctx, path); err == nil {
		t.Fatal("expected cancellation error from AnalyzeCtx")
	}
	// A live context still analyzes.
	fa, err := ws.Snapshot().AnalyzeCtx(context.Background(), path)
	if err != nil || fa == nil {
		t.Fatalf("analyze failed after cancellation test: %v", err)
	}
}

// TestGoSessionPrimesSharedUniverse pins ADR 0044 A3: the engine's Go
// resolver session loads a check's whole Go import set in one universe, so
// interface satisfaction holds even when the interface's package and the
// implementer's package do not import each other (the case cross-universe
// translation cannot anchor).
func TestGoSessionPrimesSharedUniverse(t *testing.T) {
	root := writeProject(t, map[string]string{
		"ard.toml": "name = \"universe\"\nard = \">= 0.1.0\"\n",
		"go.mod":   "module universe\n\ngo 1.26\n",
		"ffi/shared/shared.go": `package shared

type Record struct {
	Value int
}
`,
		"ffi/api/api.go": `package api

import "universe/ffi/shared"

type Handler interface {
	Handle(rec shared.Record) int
}

func Dispatch(h Handler) int {
	return h.Handle(shared.Record{Value: 21})
}
`,
		"ffi/impl/impl.go": `package impl

import "universe/ffi/shared"

type Client struct{}

func (c *Client) Handle(rec shared.Record) int {
	return rec.Value * 2
}

func New() *Client {
	return &Client{}
}
`,
		"clients.ard": `use go:universe/ffi/api

fn dispatch(h: api::Handler) Int {
  api::Dispatch(h)
}
`,
		"main.ard": `use go:universe/ffi/impl
use universe/clients

fn main() {
  let client = impl::New()
  if not clients::dispatch(client) == 42 {
    panic("dispatch mismatch")
  }
}
`,
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)

	fa, err := ws.Snapshot().Analyze(filepath.Join(root, "main.ard"))
	if err != nil {
		t.Fatal(err)
	}
	for _, diag := range fa.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", diag)
	}
}

// TestGoSessionRepricesForNewImports pins that adding a Go import through an
// overlay re-primes the session instead of surfacing the post-prime internal
// error, and that unrelated checks reuse the session.
func TestGoSessionRepricesForNewImports(t *testing.T) {
	root := writeProject(t, map[string]string{
		"ard.toml": "name = \"session\"\nard = \">= 0.1.0\"\n",
		"go.mod":   "module session\n\ngo 1.26\n",
		"main.ard": "use go:fmt\n\nfn main() {\n  fmt::Println(\"hi\")\n}\n",
	})
	engine := NewEngine(root)
	ws := NewWorkspace(engine)
	path := filepath.Join(root, "main.ard")

	fa, err := ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, diag := range fa.Diagnostics {
		t.Errorf("unexpected diagnostic before overlay: %s", diag)
	}
	first := engine.goResolver

	// Re-analyzing without new imports reuses the primed session.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetOverlay(path, string(content)+"\n// trailing comment\n")
	if _, err := ws.Snapshot().Analyze(path); err != nil {
		t.Fatal(err)
	}
	if engine.goResolver != first {
		t.Fatal("session should be reused when no new Go imports appear")
	}

	// A new Go import re-primes; the analysis must not report the
	// post-prime internal error.
	ws.SetOverlay(path, "use go:fmt\nuse go:strings\n\nfn main() {\n  fmt::Println(strings::ToUpper(\"hi\"))\n}\n")
	fa, err = ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, diag := range fa.Diagnostics {
		t.Errorf("unexpected diagnostic after new import: %s", diag)
	}
	if engine.goResolver == first {
		t.Fatal("session should be rebuilt for a newly appearing Go import")
	}

	// A bogus in-progress import path yields a normal resolve diagnostic,
	// never the internal pre-scan error.
	ws.SetOverlay(path, "use go:net/htt\n\nfn main() {}\n")
	fa, err = ws.Snapshot().Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	foundResolveError := false
	for _, diag := range fa.Diagnostics {
		if strings.Contains(diag.Message, "internal compiler bug") {
			t.Fatalf("internal error leaked to diagnostics: %s", diag)
		}
		if strings.Contains(diag.Message, "Failed to resolve Go import") {
			foundResolveError = true
			if diag.Code != "go_import_resolution" {
				t.Fatalf("diagnostic code = %q", diag.Code)
			}
			if filepath.Base(diag.Primary.Span.FilePath) != filepath.Base(path) || diag.Primary.Span.Location.Start.Row != 1 || diag.Primary.Span.Location.Start.Col != 8 || diag.Primary.Span.Location.End.Col != 14 {
				t.Fatalf("diagnostic path span = %#v", diag.Primary.Span)
			}
		}
	}
	if !foundResolveError {
		t.Error("expected a resolve diagnostic for the bogus import path")
	}
}
