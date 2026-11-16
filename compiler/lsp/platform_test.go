package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/platform"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

const platformDocument = `fn value() Int { 1 }

#platform(windows)
fn value() Int { "windows" }

#platform(windows)
fn console() Int {
  let mode = 42
  mode
}

fn main() {
  let v = value()
}
`

// openPlatformDocument opens platformDocument in a server whose primary
// platform is linux/amd64, so the windows declarations are inactive.
func openPlatformDocument(t *testing.T) (*Server, uri.URI) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "main.ard")
	if err := os.WriteFile(path, []byte(platformDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer()
	docURI := uri.File(path)
	server.cache.Open(docURI, "ard", 1, platformDocument)
	server.workspaceFor(path).Engine().SetPrimaryPlatform(platform.Platform{OS: "linux", Arch: "amd64"})
	return server, docURI
}

func TestPublishDiagnosticsChecksDeclarationsForOtherPlatforms(t *testing.T) {
	server, docURI := openPlatformDocument(t)
	conn := newRecordingConn()
	server.conn = conn

	server.publishDiagnostics(context.Background(), docURI)

	params := conn.lastDiagnostics(t)
	if len(params.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want the windows type error only", params.Diagnostics)
	}
	diagnostic := params.Diagnostics[0]
	if diagnostic.Range.Start.Line != 3 || !strings.Contains(diagnostic.Message, "windows/amd64") {
		t.Fatalf("diagnostic = %+v, want a windows/amd64 error on line 4", diagnostic)
	}
}

func TestHoverAndDefinitionInDeclarationsForOtherPlatforms(t *testing.T) {
	server, docURI := openPlatformDocument(t)
	position := protocol.Position{Line: 8, Character: 2}

	info := server.hoverFromSpans(context.Background(), docURI, position)
	if info == nil || !strings.Contains(info.content, "Int") {
		t.Fatalf("hover = %+v, want the Int binding", info)
	}
	locations := server.definitionFromSpans(context.Background(), docURI, position)
	if len(locations) != 1 || locations[0].Range.Start.Line != 7 {
		t.Fatalf("definition = %+v, want the binding on line 8", locations)
	}
}

func TestHoverOnDeclarationListsPlatformSiblings(t *testing.T) {
	server, docURI := openPlatformDocument(t)

	info := server.hoverFromSpans(context.Background(), docURI, protocol.Position{Line: 0, Character: 4})
	if info == nil || !strings.Contains(info.content, "fn value() Int") || !strings.Contains(info.content, "`#platform(windows)` (line 4)") {
		t.Fatalf("fallback hover = %+v, want its signature and the windows sibling", info)
	}
	info = server.hoverFromSpans(context.Background(), docURI, protocol.Position{Line: 3, Character: 4})
	if info == nil || !strings.Contains(info.content, "fn value() Int") || !strings.Contains(info.content, "without `#platform` (line 1)") {
		t.Fatalf("windows hover = %+v, want its signature and the fallback", info)
	}
}
