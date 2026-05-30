package lsp

import (
	"context"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"go.lsp.dev/uri"
)

// TestParseAndCheckWithError verifies diagnostics are produced for broken code.
// Uses basic types that don't require stdlib imports.
func TestParseAndCheckWithError(t *testing.T) {
	source := `let x: Int = "hello"`
	diags, err := parseAndCheck(source, "/tmp/test.ard")
	if err != nil {
		t.Fatalf("parseAndCheck failed: %v", err)
	}
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
	diags, err := parseAndCheck(source, "/tmp/test.ard")
	if err != nil {
		t.Fatalf("parseAndCheck failed: %v", err)
	}
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for valid code, got %d: %v", len(diags), diags)
	}
}

// TestParseAndCheckWithParseError verifies diagnostics for parse errors.
func TestParseAndCheckWithParseError(t *testing.T) {
	source := `let x = `
	diags, err := parseAndCheck(source, "/tmp/test.ard")
	if err != nil {
		t.Fatalf("parseAndCheck failed: %v", err)
	}
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for parse error, got none")
	}
	t.Logf("parse error diagnostics: %v", diags)
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
