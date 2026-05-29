package lsp

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// parseAndCheck runs the Ard parser and checker on a source file.
// It returns the parsed AST, the checked module, and any diagnostics.
func parseAndCheck(source string, filePath string) ([]checker.Diagnostic, error) {
	// Parse the source
	result := parse.Parse([]byte(source), filePath)
	if result.Program == nil {
		return nil, fmt.Errorf("failed to parse: no program returned")
	}

	if len(result.Errors) > 0 {
		// Convert parse errors to checker-style diagnostics
		diags := make([]checker.Diagnostic, 0, len(result.Errors))
		for _, err := range result.Errors {
			diags = append(diags, checker.NewDiagnostic(checker.Error, err.Message, filePath, err.Location))
		}
		return diags, nil
	}

	program := result.Program

	// Initialize the module resolver
	workingDir := filepath.Dir(filePath)
	moduleResolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		return nil, fmt.Errorf("error initializing module resolver: %w", err)
	}

	relPath, err := filepath.Rel(workingDir, filePath)
	if err != nil {
		relPath = filePath
	}

	c := checker.New(relPath, program, moduleResolver, checker.CheckOptions{})
	c.Check()

	return c.Diagnostics(), nil
}

// formatSource formats an Ard source file using the existing formatter.
func formatSource(source string, filePath string) (string, error) {
	formatted, err := formatArdSource([]byte(source), filePath)
	if err != nil {
		return source, err
	}
	return string(formatted), nil
}

// formatArdSource wraps the formatter package call.
// This is a thin wrapper so we can test separately if needed.
var formatArdSource = func(source []byte, filePath string) ([]byte, error) {
	return formatViaFormatter(source, filePath)
}

// formatViaFormatter is the actual formatter call, separated for testability.
func formatViaFormatter(source []byte, filePath string) ([]byte, error) {
	if formatFn == nil {
		return source, fmt.Errorf("formatter not initialized")
	}
	return formatFn(source, filePath)
}

// formatFn is set from main.go to point at the real formatter.
var formatFn func(source []byte, filePath string) ([]byte, error)

// SetFormatter wires the compiler's formatter into the LSP package.
func SetFormatter(fn func(source []byte, filePath string) ([]byte, error)) {
	formatFn = fn
}

// checkerDiagnosticsToLSP converts checker.Diagnostics to LSP Diagnostics.
// Always returns a non-nil slice so JSON serializes as [] not null.
func checkerDiagnosticsToLSP(diagnostics []checker.Diagnostic) []protocol.Diagnostic {
	if len(diagnostics) == 0 {
		return []protocol.Diagnostic{}
	}

	result := make([]protocol.Diagnostic, 0, len(diagnostics))
	for _, d := range diagnostics {
		lspDiag := protocol.Diagnostic{
			Range:    checkerLocationToLSPRange(d.Location()),
			Severity: diagnosticKindToLSPSeverity(d.Kind),
			Source:   "ard",
			Message:  d.Message,
		}
		result = append(result, lspDiag)
	}
	return result
}

// checkerLocationToLSPRange converts a parse.Location to an LSP Range.
// Parser uses 1-based (Row, Col); LSP uses 0-based (line, character).
func checkerLocationToLSPRange(loc parse.Location) protocol.Range {
	// Convert 1-based to 0-based
	startLine := uint32(0)
	startChar := uint32(0)
	endLine := uint32(0)
	endChar := uint32(0)

	if loc.Start.Row > 0 {
		startLine = uint32(loc.Start.Row - 1)
	}
	if loc.Start.Col > 0 {
		startChar = uint32(loc.Start.Col - 1)
	}
	if loc.End.Row > 0 {
		endLine = uint32(loc.End.Row - 1)
	}
	if loc.End.Col > 0 {
		endChar = uint32(loc.End.Col - 1)
	} else {
		// If end is zero, use start as a single point
		endLine = startLine
		endChar = startChar + 1
	}

	return protocol.Range{
		Start: protocol.Position{Line: startLine, Character: startChar},
		End:   protocol.Position{Line: endLine, Character: endChar},
	}
}

// diagnosticKindToLSPSeverity converts checker.DiagnosticKind to LSP DiagnosticSeverity.
func diagnosticKindToLSPSeverity(kind checker.DiagnosticKind) protocol.DiagnosticSeverity {
	switch kind {
	case checker.Error:
		return protocol.DiagnosticSeverityError
	case checker.Warn:
		return protocol.DiagnosticSeverityWarning
	default:
		return protocol.DiagnosticSeverityInformation
	}
}

// publishDiagnostics analyzes the document at the given URI and publishes diagnostics.
func (s *Server) publishDiagnostics(ctx context.Context, docURI uri.URI) {
	doc := s.cache.Get(docURI)
	if doc == nil {
		// Document not in cache; clear diagnostics.
		s.sendDiagnostics(ctx, docURI, nil)
		return
	}

	filePath := doc.URI.Filename()
	diags, err := parseAndCheck(doc.Text, filePath)
	if err != nil {
		// If we can't analyze, publish the error as a diagnostic
		s.sendDiagnostics(ctx, docURI, []protocol.Diagnostic{
			{
				Range:    protocol.Range{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 0, Character: 1}},
				Severity: protocol.DiagnosticSeverityError,
				Source:   "ard",
				Message:  fmt.Sprintf("Analysis error: %v", err),
			},
		})
		return
	}

	s.sendDiagnostics(ctx, docURI, checkerDiagnosticsToLSP(diags))
}

// sendDiagnostics sends a textDocument/publishDiagnostics notification to the client.
// diags is converted to an empty slice if nil so JSON serializes as [] not null.
func (s *Server) sendDiagnostics(ctx context.Context, docURI uri.URI, diags []protocol.Diagnostic) {
	if s.conn == nil {
		return
	}
	if diags == nil {
		diags = []protocol.Diagnostic{}
	}

	params := &protocol.PublishDiagnosticsParams{
		URI:         docURI,
		Diagnostics: diags,
	}

	// Ignore error — this is a notification; client may disconnect.
	_ = s.conn.Notify(ctx, protocol.MethodTextDocumentPublishDiagnostics, params)
}
