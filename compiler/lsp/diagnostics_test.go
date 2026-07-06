package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/akonwi/ard/checker"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestParseAndCheckWithError verifies diagnostics are produced for broken code.
// Uses basic types that don't require stdlib imports.
type recordedNotification struct {
	method string
	params interface{}
}

type recordingConn struct {
	mu            sync.Mutex
	notifications []recordedNotification
	done          chan struct{}
}

func newRecordingConn() *recordingConn {
	return &recordingConn{done: make(chan struct{})}
}

func (c *recordingConn) Call(ctx context.Context, method string, params, result interface{}) (jsonrpc2.ID, error) {
	return jsonrpc2.ID{}, nil
}

func (c *recordingConn) Notify(ctx context.Context, method string, params interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.notifications = append(c.notifications, recordedNotification{method: method, params: params})
	return nil
}

func (c *recordingConn) Go(ctx context.Context, handler jsonrpc2.Handler) {}

func (c *recordingConn) Close() error {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	return nil
}

func (c *recordingConn) Done() <-chan struct{} { return c.done }
func (c *recordingConn) Err() error            { return nil }

func (c *recordingConn) notificationCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.notifications)
}

func (c *recordingConn) lastDiagnostics(t *testing.T) *protocol.PublishDiagnosticsParams {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.notifications) == 0 {
		t.Fatal("expected diagnostic notification")
	}
	n := c.notifications[len(c.notifications)-1]
	if n.method != protocol.MethodTextDocumentPublishDiagnostics {
		t.Fatalf("notification method = %q, want %q", n.method, protocol.MethodTextDocumentPublishDiagnostics)
	}
	params, ok := n.params.(*protocol.PublishDiagnosticsParams)
	if !ok {
		t.Fatalf("notification params = %T, want *protocol.PublishDiagnosticsParams", n.params)
	}
	return params
}

// analyzeDiagnosticsForTest runs the snapshot-engine diagnostics path for a
// single document, with optional sibling overlays as open documents.
func analyzeDiagnosticsForTest(t *testing.T, source string, filePath string, overlays map[string]string) []checker.Diagnostic {
	t.Helper()
	if filepath.Dir(filePath) == "/tmp" {
		filePath = filepath.Join(t.TempDir(), filepath.Base(filePath))
	}
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer()
	docURI := uri.File(filePath)
	server.cache.Open(docURI, "ard", 1, source)
	for path, text := range overlays {
		server.cache.Open(uri.File(path), "ard", 1, text)
	}
	doc := server.cache.Get(docURI)
	diags, err := server.analyzeDiagnostics(doc, server.cache.Snapshot())
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	return diags
}

func TestParseAndCheckWithError(t *testing.T) {
	source := `let x: Int = "hello"`
	diags := analyzeDiagnosticsForTest(t, source, "/tmp/test.ard", nil)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for type error, got none")
	}

	foundTypeError := false
	for _, d := range diags {
		if d.Kind == checker.Error && strings.Contains(d.Message, "Type mismatch") {
			foundTypeError = true
			break
		}
	}
	if !foundTypeError {
		t.Logf("diagnostics: %v", diags)
		t.Error("expected a Type mismatch diagnostic")
	}
}

// TestParseAndCheckWithValidCode verifies no diagnostics for valid code.
func TestParseAndCheckWithValidCode(t *testing.T) {
	source := `let x = 5`
	diags := analyzeDiagnosticsForTest(t, source, "/tmp/test.ard", nil)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for valid code, got %d: %v", len(diags), diags)
	}
}

// TestParseAndCheckWithParseError verifies diagnostics for parse errors.
func TestParseAndCheckWithParseError(t *testing.T) {
	source := `let x = `
	diags := analyzeDiagnosticsForTest(t, source, "/tmp/test.ard", nil)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for parse error, got none")
	}
	t.Logf("parse error diagnostics: %v", diags)
}
func TestParseAndCheckUsesOpenDocumentOverlaysForImports(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	modPath := filepath.Join(root, "tools.ard")
	if err := os.WriteFile(modPath, []byte("fn old_name() Int { 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	source := `use test_project/tools

let value = tools::new_name()
`

	diags := analyzeDiagnosticsForTest(t, source, mainPath, map[string]string{
		modPath: "fn new_name() Int { 1 }\n",
	})
	if len(diags) != 0 {
		t.Fatalf("expected overlay import to clear stale diagnostics, got %v", diags)
	}
}

// TestPublishDiagnosticsLifecycle verifies the full cycle doesn't panic.
func TestPublishDiagnosticsLifecycle(t *testing.T) {
	server := NewServer()
	ctx := context.Background()
	docURI := uri.New("file:///tmp/test.ard")

	// Open a document with valid code
	server.cache.Open(docURI, "ard", 1, `let x = 5`)
	server.publishDiagnostics(ctx, docURI)

	// Update to broken code
	server.cache.Update(docURI, 2, `let x: Int = "hello"`)
	server.publishDiagnostics(ctx, docURI)

	// Close
	server.cache.Close(docURI)
	server.publishDiagnostics(ctx, docURI)
}
func TestPublishDiagnosticsIncludesDocumentVersion(t *testing.T) {
	server := NewServer()
	conn := newRecordingConn()
	server.conn = conn
	docURI := uri.New("file:///tmp/test.ard")
	server.cache.Open(docURI, "ard", 7, `let x = 5`)

	server.publishDiagnostics(context.Background(), docURI)

	params := conn.lastDiagnostics(t)
	if params.Version != 7 {
		t.Fatalf("diagnostic version = %d, want 7", params.Version)
	}
	if len(params.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %#v", params.Diagnostics)
	}
}
func TestPublishDiagnosticsClearsClosedDocumentDiagnostics(t *testing.T) {
	server := NewServer()
	conn := newRecordingConn()
	server.conn = conn
	docURI := uri.New("file:///tmp/test.ard")

	server.publishDiagnostics(context.Background(), docURI)

	params := conn.lastDiagnostics(t)
	if params.URI != protocol.DocumentURI(docURI) {
		t.Fatalf("diagnostic URI = %q, want %q", params.URI, docURI)
	}
	if len(params.Diagnostics) != 0 {
		t.Fatalf("expected diagnostics to be cleared, got %#v", params.Diagnostics)
	}
}
func TestPublishDiagnosticsDiscardsStaleOverlaySnapshot(t *testing.T) {
	server := NewServer()
	conn := newRecordingConn()
	server.conn = conn
	mainURI := uri.New("file:///tmp/main.ard")
	toolsURI := uri.New("file:///tmp/tools.ard")
	server.cache.Open(mainURI, "ard", 1, "use app/tools\nlet value = tools::value()\n")
	server.cache.Open(toolsURI, "ard", 1, "fn value() Int { 1 }\n")

	started := make(chan struct{})
	release := make(chan struct{})
	server.diagnosticsAnalyzer = func(source string, filePath string, overlays map[string]string) ([]checker.Diagnostic, error) {
		close(started)
		<-release
		return nil, nil
	}

	done := make(chan struct{})
	go func() {
		server.publishDiagnostics(context.Background(), mainURI)
		close(done)
	}()

	<-started
	server.cache.Update(toolsURI, 2, "fn value() Int { 2 }\n")
	close(release)
	<-done

	if got := conn.notificationCount(); got != 0 {
		t.Fatalf("published %d stale diagnostic notifications, want none", got)
	}
}
func TestPublishDiagnosticsHandlesNonFileURI(t *testing.T) {
	server := NewServer()
	conn := newRecordingConn()
	server.conn = conn
	docURI := uri.URI("untitled:Untitled-1")
	server.cache.Open(docURI, "ard", 3, `let x = 5`)

	server.publishDiagnostics(context.Background(), docURI)

	params := conn.lastDiagnostics(t)
	if params.Version != 3 {
		t.Fatalf("diagnostic version = %d, want 3", params.Version)
	}
	if len(params.Diagnostics) != 1 {
		t.Fatalf("expected one analysis diagnostic, got %#v", params.Diagnostics)
	}
	message := params.Diagnostics[0].Message
	if !strings.Contains(message, "Analysis error") || !strings.Contains(message, "unsupported document URI") {
		t.Fatalf("diagnostic message = %q", message)
	}
}
func TestPublishDiagnosticsSkipsNonFileOverlays(t *testing.T) {
	server := NewServer()
	conn := newRecordingConn()
	server.conn = conn
	fileURI := uri.New("file:///tmp/test.ard")
	server.cache.Open(fileURI, "ard", 1, `let x = 5`)
	server.cache.Open(uri.URI("untitled:Untitled-1"), "ard", 1, `let y = 10`)

	server.publishDiagnostics(context.Background(), fileURI)

	params := conn.lastDiagnostics(t)
	if len(params.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %#v", params.Diagnostics)
	}
}
func TestPublishDiagnosticsReportsAnalysisPanic(t *testing.T) {
	server := NewServer()
	conn := newRecordingConn()
	server.conn = conn
	server.diagnosticsAnalyzer = func(source string, filePath string, overlays map[string]string) ([]checker.Diagnostic, error) {
		panic("checker exploded")
	}
	docURI := uri.New("file:///tmp/test.ard")
	server.cache.Open(docURI, "ard", 9, `let x = 5`)

	server.publishDiagnostics(context.Background(), docURI)

	params := conn.lastDiagnostics(t)
	if params.Version != 9 {
		t.Fatalf("diagnostic version = %d, want 9", params.Version)
	}
	if len(params.Diagnostics) != 1 {
		t.Fatalf("expected one analysis diagnostic, got %#v", params.Diagnostics)
	}
	message := params.Diagnostics[0].Message
	if !strings.Contains(message, "Analysis error") || !strings.Contains(message, "checker exploded") {
		t.Fatalf("diagnostic message = %q", message)
	}
}

// TestPublishDiagnosticsEnginePathParseErrors exercises the default
// (snapshot-engine) diagnostics path with no injected analyzer: parse errors
// must surface as diagnostics.
func TestPublishDiagnosticsEnginePathParseErrors(t *testing.T) {
	server := NewServer()
	conn := newRecordingConn()
	server.conn = conn
	docURI := uri.New("file:///tmp/engine_parse.ard")
	server.cache.Open(docURI, "ard", 1, "fn main( {\n}\n")

	server.publishDiagnostics(context.Background(), docURI)

	params := conn.lastDiagnostics(t)
	if len(params.Diagnostics) == 0 {
		t.Fatal("expected parse-error diagnostics from engine path")
	}
}

// TestPublishDiagnosticsEnginePathTypeErrors exercises the default path with
// a checker diagnostic.
func TestPublishDiagnosticsEnginePathTypeErrors(t *testing.T) {
	server := NewServer()
	conn := newRecordingConn()
	server.conn = conn
	docURI := uri.New("file:///tmp/engine_check.ard")
	server.cache.Open(docURI, "ard", 1, "fn main() {\n  let x: Str = 42\n}\n")

	server.publishDiagnostics(context.Background(), docURI)

	params := conn.lastDiagnostics(t)
	if len(params.Diagnostics) == 0 {
		t.Fatal("expected type-error diagnostics from engine path")
	}
}
