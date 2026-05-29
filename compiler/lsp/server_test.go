package lsp

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestServerInitializes verifies all expected LSP handlers are registered.
func TestServerInitializes(t *testing.T) {
	server := NewServer()

	expectedHandlers := []string{
		"initialize",
		"initialized",
		"shutdown",
		"exit",
		"textDocument/didOpen",
		"textDocument/didChange",
		"textDocument/didSave",
		"textDocument/didClose",
		"textDocument/hover",
		"textDocument/definition",
		"textDocument/references",
		"textDocument/documentSymbol",
		"textDocument/completion",
		"textDocument/formatting",
		"textDocument/signatureHelp",
		"textDocument/documentHighlight",
	}

	for _, method := range expectedHandlers {
		if _, ok := server.handlers[method]; !ok {
			t.Errorf("missing handler for %s", method)
		}
	}
}

// TestCheckerDiagnosticsToLSP verifies the diagnostic conversion handles edge cases.
func TestCheckerDiagnosticsToLSP(t *testing.T) {
	// Nil input should produce empty slice (not nil) so JSON is [] not null
	if result := checkerDiagnosticsToLSP(nil); result == nil {
		t.Error("expected non-nil empty slice for nil input")
	} else if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}

	// Empty input
	if result := checkerDiagnosticsToLSP([]checker.Diagnostic{}); result == nil {
		t.Error("expected non-nil empty slice for empty input")
	} else if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}

	// Single error diagnostic
	diags := []checker.Diagnostic{
		checker.NewDiagnostic(
			checker.Error,
			"type mismatch",
			"test.ard",
			parse.Location{
				Start: parse.Point{Row: 3, Col: 5},
				End:   parse.Point{Row: 3, Col: 10},
			},
		),
	}
	result := checkerDiagnosticsToLSP(diags)
	if len(result) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(result))
	}
	if result[0].Message != "type mismatch" {
		t.Errorf("expected message 'type mismatch', got %q", result[0].Message)
	}
	if result[0].Severity != protocol.DiagnosticSeverityError {
		t.Errorf("expected severity error, got %v", result[0].Severity)
	}
	// Parser uses 1-based, LSP uses 0-based
	if result[0].Range.Start.Line != 2 {
		t.Errorf("expected start line 2 (0-based), got %d", result[0].Range.Start.Line)
	}
	if result[0].Range.Start.Character != 4 {
		t.Errorf("expected start character 4 (0-based), got %d", result[0].Range.Start.Character)
	}
	if result[0].Range.End.Line != 2 {
		t.Errorf("expected end line 2 (0-based), got %d", result[0].Range.End.Line)
	}
	if result[0].Range.End.Character != 9 {
		t.Errorf("expected end character 9 (0-based), got %d", result[0].Range.End.Character)
	}
	if result[0].Source != "ard" {
		t.Errorf("expected source 'ard', got %q", result[0].Source)
	}
}

// TestCheckerLocationToLSPRange verifies the 1-based to 0-based conversion.
func TestCheckerLocationToLSPRange(t *testing.T) {
	tests := []struct {
		name     string
		input    parse.Location
		expected parse.Point
	}{
		{
			name: "normal position",
			input: parse.Location{
				Start: parse.Point{Row: 1, Col: 1},
				End:   parse.Point{Row: 1, Col: 5},
			},
			expected: parse.Point{Row: 0, Col: 0},
		},
		{
			name: "zero position should stay zero",
			input: parse.Location{
				Start: parse.Point{Row: 0, Col: 0},
				End:   parse.Point{Row: 0, Col: 0},
			},
			expected: parse.Point{Row: 0, Col: 0},
		},
		{
			name: "deep position",
			input: parse.Location{
				Start: parse.Point{Row: 10, Col: 15},
				End:   parse.Point{Row: 12, Col: 3},
			},
			expected: parse.Point{Row: 9, Col: 14},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkerLocationToLSPRange(tt.input)
			if result.Start.Line != uint32(tt.expected.Row) {
				t.Errorf("start line: got %d, want %d", result.Start.Line, tt.expected.Row)
			}
			if result.Start.Character != uint32(tt.expected.Col) {
				t.Errorf("start char: got %d, want %d", result.Start.Character, tt.expected.Col)
			}
		})
	}
}

// TestDocumentCache verifies basic document lifecycle.
func TestDocumentCache(t *testing.T) {
	cache := NewDocumentCache()

	uri := uri.New("file:///test.ard")

	// Open
	cache.Open(uri, "ard", 1, "let x = 5")
	doc := cache.Get(uri)
	if doc == nil {
		t.Fatal("expected doc after open")
	}
	if doc.Text != "let x = 5" {
		t.Errorf("expected text 'let x = 5', got %q", doc.Text)
	}

	// Update
	cache.Update(uri, 2, "let y = 10")
	doc = cache.Get(uri)
	if doc.Version != 2 {
		t.Errorf("expected version 2, got %d", doc.Version)
	}
	if doc.Text != "let y = 10" {
		t.Errorf("expected text 'let y = 10', got %q", doc.Text)
	}

	// Close
	cache.Close(uri)
	if doc := cache.Get(uri); doc != nil {
		t.Error("expected nil after close")
	}
}
