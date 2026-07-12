package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/formatter"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type diagnosticAnalyzer func(source string, filePath string, overlays map[string]string) ([]checker.Diagnostic, error)

func formatSource(source string, filePath string) (string, error) {
	formatted, err := formatter.Format([]byte(source), filePath)
	if err != nil {
		return source, err
	}
	return string(formatted), nil
}

// checkerDiagnosticsToLSP converts checker.Diagnostics to LSP Diagnostics.
// Always returns a non-nil slice so JSON serializes as [] not null.
func checkerDiagnosticsToLSP(diagnostics []checker.Diagnostic) []protocol.Diagnostic {
	return convertCheckerDiagnosticsToLSP(
		diagnostics,
		func(_ string, location parse.Location) protocol.Range { return checkerLocationToLSPRange(location) },
		func(path string) string {
			if abs, err := filepath.Abs(path); err == nil {
				return abs
			}
			return path
		},
	)
}

func convertCheckerDiagnosticsToLSP(
	diagnostics []checker.Diagnostic,
	rangeFor func(string, parse.Location) protocol.Range,
	resolvePath func(string) string,
) []protocol.Diagnostic {
	if len(diagnostics) == 0 {
		return []protocol.Diagnostic{}
	}

	result := make([]protocol.Diagnostic, 0, len(diagnostics))
	for _, d := range diagnostics {
		lspDiag := protocol.Diagnostic{
			Range:    rangeFor(d.FilePath(), d.Location()),
			Severity: diagnosticKindToLSPSeverity(d.Kind),
			Source:   "ard",
			Message:  checkerDiagnosticMessage(d),
		}
		for _, secondary := range d.Secondary {
			path := resolvePath(secondary.Span.FilePath)
			lspDiag.RelatedInformation = append(lspDiag.RelatedInformation, protocol.DiagnosticRelatedInformation{
				Location: protocol.Location{
					URI:   protocol.DocumentURI(uri.File(path)),
					Range: rangeFor(path, secondary.Span.Location),
				},
				Message: secondary.Message,
			})
		}
		result = append(result, lspDiag)
	}
	return result
}

func checkerDiagnosticMessage(d checker.Diagnostic) string {
	if d.Code == "" {
		return d.Message
	}
	parts := []string{d.Title}
	if d.Primary.Message != "" {
		parts = append(parts, d.Primary.Message)
	}
	if d.Text != "" {
		parts = append(parts, d.Text)
	}
	for _, secondary := range d.Secondary {
		if secondary.Message != "" {
			parts = append(parts, secondary.Message)
		}
	}
	return strings.Join(parts, ": ")
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
		// parse.Location ends are inclusive; LSP ends are exclusive.
		endChar = uint32(loc.End.Col)
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

func (s *Server) checkerDiagnosticsToLSP(diagnostics []checker.Diagnostic, docURI uri.URI) []protocol.Diagnostic {
	docPath, err := filePathFromURI(docURI)
	if err != nil {
		return []protocol.Diagnostic{}
	}
	docPath = filepath.Clean(docPath)
	resolvePath := func(path string) string {
		if filepath.IsAbs(path) {
			return filepath.Clean(path)
		}
		candidates := make([]string, 0, 3)
		if s.projectRoot != "" {
			candidates = append(candidates, filepath.Join(s.projectRoot, path))
		}
		if abs, absErr := filepath.Abs(path); absErr == nil {
			candidates = append(candidates, abs)
		}
		candidates = append(candidates, filepath.Join(filepath.Dir(docPath), path))
		for _, candidate := range candidates {
			if filepath.Clean(candidate) == docPath {
				return docPath
			}
		}
		for _, candidate := range candidates {
			if _, statErr := os.Stat(candidate); statErr == nil {
				return filepath.Clean(candidate)
			}
		}
		return filepath.Clean(candidates[0])
	}

	filtered := make([]checker.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if resolvePath(diagnostic.FilePath()) == docPath {
			filtered = append(filtered, diagnostic)
		}
	}
	return convertCheckerDiagnosticsToLSP(filtered, func(path string, location parse.Location) protocol.Range {
		return s.diagnosticRangeFor(resolvePath(path), location)
	}, resolvePath)
}

// publishDiagnostics analyzes the document at the given URI and publishes diagnostics.
func (s *Server) publishDiagnostics(ctx context.Context, docURI uri.URI) {
	docs, revision := s.cache.SnapshotWithRevision()
	doc := findDiagnosticDocument(docs, docURI)
	if doc == nil {
		// Document not in cache; clear diagnostics.
		s.sendDiagnostics(ctx, docURI, -1, nil)
		return
	}

	diags, err := s.analyzeDiagnostics(doc, docs)
	if !s.isDiagnosticSnapshotCurrent(docURI, doc.Version, revision) {
		return
	}
	if err != nil {
		// If we can't analyze, publish the error as a diagnostic so stale diagnostics
		// are replaced instead of lingering until the server is restarted.
		s.sendDiagnostics(ctx, docURI, doc.Version, []protocol.Diagnostic{analysisErrorDiagnostic(err)})
		return
	}

	s.sendDiagnostics(ctx, docURI, doc.Version, s.checkerDiagnosticsToLSP(diags, docURI))
}

func findDiagnosticDocument(docs []Doc, docURI uri.URI) *Doc {
	for i := range docs {
		if docs[i].URI == docURI {
			return &docs[i]
		}
	}
	return nil
}

func (s *Server) analyzeDiagnostics(doc *Doc, docs []Doc) (diagnostics []checker.Diagnostic, err error) {
	defer func() {
		if r := recover(); r != nil {
			diagnostics = nil
			err = fmt.Errorf("analysis panic: %v", r)
		}
	}()

	filePath, err := filePathFromURI(doc.URI)
	if err != nil {
		return nil, err
	}
	// Tests may inject a custom analyzer; the default path goes through the
	// snapshot engine so parses and checks are memoized.
	if s.diagnosticsAnalyzer != nil {
		return s.diagnosticsAnalyzer(doc.Text, filePath, overlaySources(docs))
	}
	fa, err := s.analyzeSnapshot(context.Background(), doc.URI)
	if err != nil {
		return nil, err
	}
	if len(fa.ParseErrors) > 0 {
		diags := make([]checker.Diagnostic, 0, len(fa.ParseErrors))
		for _, perr := range fa.ParseErrors {
			diags = append(diags, checker.NewDiagnostic(checker.Error, perr.Message, filePath, perr.Location))
		}
		return diags, nil
	}
	return fa.Diagnostics, nil
}

func analysisErrorDiagnostic(err error) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range:    protocol.Range{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 0, Character: 1}},
		Severity: protocol.DiagnosticSeverityError,
		Source:   "ard",
		Message:  fmt.Sprintf("Analysis error: %v", err),
	}
}

func (s *Server) isDiagnosticSnapshotCurrent(docURI uri.URI, version int32, revision uint64) bool {
	current := s.cache.Get(docURI)
	return current != nil && current.Version == version && s.cache.Revision() == revision
}

// sendDiagnostics sends a textDocument/publishDiagnostics notification to the client.
// diags is converted to an empty slice if nil so JSON serializes as [] not null.
func (s *Server) sendDiagnostics(ctx context.Context, docURI uri.URI, version int32, diags []protocol.Diagnostic) {
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
	if version >= 0 {
		params.Version = uint32(version)
	}

	// Ignore error — this is a notification; client may disconnect.
	_ = s.conn.Notify(ctx, protocol.MethodTextDocumentPublishDiagnostics, params)
}
