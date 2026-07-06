package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// spanServer builds a server with one open document at filePath (written to
// disk when the path's directory exists) for span-path feature tests.
func spanServer(t *testing.T, source string, filePath string) (*Server, uri.URI) {
	t.Helper()
	if filePath == "" || filePath == "test.ard" {
		filePath = filepath.Join(t.TempDir(), "test.ard")
	}
	if dir := filepath.Dir(filePath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer()
	docURI := uri.File(filePath)
	srv.cache.Open(docURI, "ard", 1, source)
	return srv, docURI
}

// spanHoverAt runs hover through the span path only.
func spanHoverAt(t *testing.T, source string, filePath string, pos protocol.Position) *hoverInfo {
	t.Helper()
	srv, docURI := spanServer(t, source, filePath)
	return srv.hoverFromSpans(context.Background(), docURI, pos)
}
