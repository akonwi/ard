package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
