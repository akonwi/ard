package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/akonwi/ard/lsp/analysis"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func TestAnalyzeSnapshotUsesNestedProjectRoot(t *testing.T) {
	workspaceRoot := t.TempDir()
	projectRoot := filepath.Join(workspaceRoot, "server")
	dependencyRoot := filepath.Join(workspaceRoot, "sql")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dependencyRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(projectRoot, "ard.toml"), "name = \"maestro\"\nard = \">= 0.26.0\"\n\n[dependencies]\nsql = { path = \"../sql\" }\n")
	write(filepath.Join(projectRoot, "app.ard"), "fn value() Int { 1 }\n")
	write(filepath.Join(dependencyRoot, "ard.toml"), "name = \"sql\"\nard = \">= 0.26.0\"\n")
	write(filepath.Join(dependencyRoot, "sql.ard"), "fn value() Int { 2 }\n")
	mainPath := filepath.Join(projectRoot, "main.ard")
	write(mainPath, "use maestro/app\nuse sql\n\nfn main() {\n  let _ = app::value() + sql::value()\n}\n")

	server := NewServer()
	server.projectRoot = string(uri.File(workspaceRoot))
	analysis, err := server.analyzeSnapshot(context.Background(), uri.File(mainPath))
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	for _, diagnostic := range analysis.Diagnostics {
		messages = append(messages, diagnostic.Message)
	}
	if len(messages) > 0 {
		t.Fatalf("unexpected diagnostics: %s", strings.Join(messages, "; "))
	}
	if got := server.workspaceFor(mainPath).Engine().ProjectRoot(); got != projectRoot {
		t.Fatalf("analysis root = %q, want nested project root %q", got, projectRoot)
	}
}

func TestAnalyzeSnapshotUsesAuthoritativeWorkspaceOverlay(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(mainPath, []byte("let disk_value = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer()
	server.projectRoot = root
	workspaceSource := "let workspace_value = 1\n"
	cacheSource := "let cache_value = 2\n"
	ws := server.workspaceFor(mainPath)
	ws.SetOverlay(mainPath, workspaceSource)
	server.cache.Open(uri.File(mainPath), "ard", 1, cacheSource)
	want, err := ws.Snapshot().Analyze(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	revision := ws.Revision()

	got, err := server.analyzeSnapshot(context.Background(), uri.File(mainPath))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatal("analysis snapshot replaced the authoritative workspace overlay from the document cache")
	}
	if ws.Revision() != revision {
		t.Fatalf("analysis mutated workspace revision %d -> %d", revision, ws.Revision())
	}
}

func TestWorkspaceInitializationSeedsOpenDocumentsOnce(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	apiPath := filepath.Join(root, "api.ard")
	for _, path := range []string{mainPath, apiPath} {
		if err := os.WriteFile(path, []byte("let disk_value = 0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer()
	server.projectRoot = root
	server.cache.Open(uri.File(mainPath), "ard", 1, "let main_overlay = 1\n")
	server.cache.Open(uri.File(apiPath), "ard", 1, "let api_overlay = 1\n")

	ws := server.workspaceFor(mainPath)
	for path, want := range map[string]string{
		mainPath: "let main_overlay = 1\n",
		apiPath:  "let api_overlay = 1\n",
	} {
		content, err := ws.Snapshot().Content(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != want {
			t.Fatalf("initial content for %s = %q, want %q", path, content, want)
		}
	}

	apiPathURI := uri.File(apiPath)
	server.cache.Update(apiPathURI, 2, "let cache_only = 2\n")
	content, err := ws.Snapshot().Content(apiPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "let api_overlay = 1\n" {
		t.Fatalf("workspace was resynchronized from cache update %s: %q", apiPathURI, content)
	}
}

func TestAnalyzeSnapshotWaitsForDocumentStateTransition(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	oldSource := "let old_value = 1\n"
	newSource := "let new_value = 2\n"
	if err := os.WriteFile(mainPath, []byte(oldSource), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer()
	server.projectRoot = root
	docURI := uri.File(mainPath)
	server.cache.Open(docURI, "ard", 1, oldSource)
	server.syncOverlay(docURI, oldSource)

	server.documentStateMu.Lock()
	server.cache.Update(docURI, 2, newSource)
	done := make(chan *analysisResult, 1)
	attempted := make(chan struct{})
	go func() {
		close(attempted)
		result, err := server.analyzeSnapshot(context.Background(), docURI)
		done <- &analysisResult{analysis: result, err: err}
	}()
	<-attempted
	select {
	case <-done:
		server.documentStateMu.Unlock()
		t.Fatal("analysis captured workspace state during an incomplete document transition")
	case <-time.After(50 * time.Millisecond):
	}
	server.syncOverlay(docURI, newSource)
	want, err := server.workspaceFor(mainPath).Snapshot().Analyze(mainPath)
	if err != nil {
		server.documentStateMu.Unlock()
		t.Fatal(err)
	}
	server.documentStateMu.Unlock()

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.analysis != want {
			t.Fatal("analysis did not capture the completed workspace transition")
		}
	case <-time.After(time.Second):
		t.Fatal("analysis did not resume after document transition")
	}
}

type analysisResult struct {
	analysis *analysis.FileAnalysis
	err      error
}

func TestCompletionUsesAuthoritativeWorkspaceSiblingOverlay(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	apiPath := filepath.Join(root, "api.ard")
	mainSource := "use app/api\n\nfn main(value: api::Value) {\n  value.\n}\n"
	cacheAPI := "struct Value {\n  field: Str,\n}\n"
	workspaceAPI := "struct Value {\n  field: Bool,\n}\n"
	if err := os.WriteFile(mainPath, []byte(mainSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(apiPath, []byte(cacheAPI), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer()
	server.projectRoot = root
	docURI := uri.File(mainPath)
	server.cache.Open(docURI, "ard", 1, mainSource)
	server.cache.Open(uri.File(apiPath), "ard", 1, cacheAPI)
	server.workspaceFor(mainPath).SetOverlay(apiPath, workspaceAPI)

	items := server.completionFromSpans(context.Background(), docURI, mainSource, protocol.Position{Line: 3, Character: 8})
	item, ok := completionItemByLabel(items, "field")
	if !ok {
		t.Fatalf("field completion missing from %#v", items)
	}
	if item.Detail != "Bool" {
		t.Fatalf("field completion detail = %q, want authoritative workspace type Bool", item.Detail)
	}
}
