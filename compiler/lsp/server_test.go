package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func completionItemByLabel(items []protocol.CompletionItem, label string) (protocol.CompletionItem, bool) {
	for _, item := range items {
		if item.Label == label {
			return item, true
		}
	}
	return protocol.CompletionItem{}, false
}

func assertCompletion(t *testing.T, items []protocol.CompletionItem, label string, detail string) {
	t.Helper()
	item, ok := completionItemByLabel(items, label)
	if !ok {
		t.Fatalf("completion %q not found in %#v", label, items)
	}
	if detail != "" && item.Detail != detail {
		t.Fatalf("completion %q detail = %q, want %q", label, item.Detail, detail)
	}
}

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
		"textDocument/codeAction",
		"textDocument/signatureHelp",
		"textDocument/documentHighlight",
	}

	for _, method := range expectedHandlers {
		if _, ok := server.handlers[method]; !ok {
			t.Errorf("missing handler for %s", method)
		}
	}
}

// TestDispatchRecoversFromPanic verifies handler panics become request errors instead of killing the server.
func TestDispatchRecoversFromPanic(t *testing.T) {
	server := NewServer()
	server.handlers["ard/testPanic"] = func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		panic("boom")
	}
	req, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(1), "ard/testPanic", nil)
	if err != nil {
		t.Fatal(err)
	}

	replied := false
	err = server.dispatch(context.Background(), func(ctx context.Context, result interface{}, replyErr error) error {
		replied = true
		if replyErr == nil {
			t.Fatal("expected panic to be returned as an error")
		}
		return nil
	}, req)
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	if !replied {
		t.Fatal("expected dispatch to reply")
	}
}
func TestDocumentSyncRepliesBeforeDiagnosticsComplete(t *testing.T) {
	t.Run("didOpen", func(t *testing.T) {
		assertDocumentSyncRepliesBeforeDiagnostics(t, func(server *Server, docURI uri.URI, reply jsonrpc2.Replier) error {
			req, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(1), protocol.MethodTextDocumentDidOpen, protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:        docURI,
					LanguageID: protocol.LanguageIdentifier("ard"),
					Version:    7,
					Text:       "let x = 1",
				},
			})
			if err != nil {
				return err
			}
			return server.handleDidOpen(context.Background(), reply, req)
		})
	})

	t.Run("didChange", func(t *testing.T) {
		assertDocumentSyncRepliesBeforeDiagnostics(t, func(server *Server, docURI uri.URI, reply jsonrpc2.Replier) error {
			server.cache.Open(docURI, "ard", 1, "let x = 1")
			req, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(1), protocol.MethodTextDocumentDidChange, didChangeParams{
				TextDocument: protocol.VersionedTextDocumentIdentifier{
					TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: docURI},
					Version:                2,
				},
				ContentChanges: []documentContentChange{{Text: "let x = 2"}},
			})
			if err != nil {
				return err
			}
			return server.handleDidChange(context.Background(), reply, req)
		})
	})
}
func TestDidChangeAppliesFullAndIncrementalChanges(t *testing.T) {
	t.Run("full document change", func(t *testing.T) {
		server := NewServer()
		server.diagnosticsDelay = 0
		server.diagnosticsPublisher = func(context.Context, uri.URI) {}
		docURI := uri.New("file:///main.ard")
		server.cache.Open(docURI, "ard", 1, "let x = 1")

		req, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(1), protocol.MethodTextDocumentDidChange, didChangeParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: docURI},
				Version:                2,
			},
			ContentChanges: []documentContentChange{{Text: "let x = 2"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		reply := jsonrpc2.Replier(func(ctx context.Context, result interface{}, err error) error { return err })
		if err := server.handleDidChange(context.Background(), reply, req); err != nil {
			t.Fatal(err)
		}
		if got := server.cache.Get(docURI).Text; got != "let x = 2" {
			t.Fatalf("text = %q, want full replacement", got)
		}
	})

	t.Run("incremental range change", func(t *testing.T) {
		server := NewServer()
		server.diagnosticsDelay = 0
		server.diagnosticsPublisher = func(context.Context, uri.URI) {}
		docURI := uri.New("file:///main.ard")
		server.cache.Open(docURI, "ard", 1, "let x = 1\nlet y = 2\n")

		req, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(1), protocol.MethodTextDocumentDidChange, didChangeParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: docURI},
				Version:                2,
			},
			ContentChanges: []documentContentChange{{
				Range: &protocol.Range{
					Start: protocol.Position{Line: 0, Character: 8},
					End:   protocol.Position{Line: 0, Character: 9},
				},
				Text: "3",
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		reply := jsonrpc2.Replier(func(ctx context.Context, result interface{}, err error) error { return err })
		if err := server.handleDidChange(context.Background(), reply, req); err != nil {
			t.Fatal(err)
		}
		if got := server.cache.Get(docURI).Text; got != "let x = 3\nlet y = 2\n" {
			t.Fatalf("text = %q, want incremental replacement", got)
		}
	})
}
func TestApplyDocumentChangesHandlesUTF16AndClamping(t *testing.T) {
	updated, err := applyDocumentChanges("a😀b", []documentContentChange{{
		Range: &protocol.Range{
			Start: protocol.Position{Line: 0, Character: 3},
			End:   protocol.Position{Line: 0, Character: 4},
		},
		Text: "c",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if updated != "a😀c" {
		t.Fatalf("updated = %q, want UTF-16 range replacement", updated)
	}

	updated, err = applyDocumentChanges("abc\n", []documentContentChange{{
		Range: &protocol.Range{
			Start: protocol.Position{Line: 0, Character: 99},
			End:   protocol.Position{Line: 0, Character: 99},
		},
		Text: "!",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if updated != "abc!\n" {
		t.Fatalf("updated = %q, want line-end clamped insertion", updated)
	}

	updated, err = applyDocumentChanges("abc", []documentContentChange{{
		Range: &protocol.Range{
			Start: protocol.Position{Line: 99, Character: 0},
			End:   protocol.Position{Line: 99, Character: 0},
		},
		Text: "!",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if updated != "abc!" {
		t.Fatalf("updated = %q, want EOF clamped insertion", updated)
	}
}
func TestDocumentSyncSchedulesDiagnosticsForAllOpenDocuments(t *testing.T) {
	t.Run("didChange", func(t *testing.T) {
		server := NewServer()
		server.diagnosticsDelay = 0
		scheduled := make(chan uri.URI, 4)
		server.diagnosticsPublisher = func(ctx context.Context, docURI uri.URI) {
			scheduled <- docURI
		}
		mainURI := uri.New("file:///main.ard")
		toolsURI := uri.New("file:///tools.ard")
		server.cache.Open(mainURI, "ard", 1, "use app/tools\nlet x = tools::value()")
		server.cache.Open(toolsURI, "ard", 1, "fn value() Int { 1 }")

		req, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(1), protocol.MethodTextDocumentDidChange, didChangeParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: toolsURI},
				Version:                2,
			},
			ContentChanges: []documentContentChange{{Text: "fn value() Int { 2 }"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		reply := jsonrpc2.Replier(func(ctx context.Context, result interface{}, err error) error { return err })
		if err := server.handleDidChange(context.Background(), reply, req); err != nil {
			t.Fatal(err)
		}

		assertScheduledDiagnostics(t, scheduled, mainURI, toolsURI)
	})

	t.Run("didClose", func(t *testing.T) {
		server := NewServer()
		server.diagnosticsDelay = 0
		scheduled := make(chan uri.URI, 4)
		server.diagnosticsPublisher = func(ctx context.Context, docURI uri.URI) {
			scheduled <- docURI
		}
		mainURI := uri.New("file:///main.ard")
		toolsURI := uri.New("file:///tools.ard")
		server.cache.Open(mainURI, "ard", 1, "use app/tools\nlet x = tools::value()")
		server.cache.Open(toolsURI, "ard", 1, "fn value() Int { 1 }")

		req, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(1), protocol.MethodTextDocumentDidClose, protocol.DidCloseTextDocumentParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: toolsURI},
		})
		if err != nil {
			t.Fatal(err)
		}
		reply := jsonrpc2.Replier(func(ctx context.Context, result interface{}, err error) error { return err })
		if err := server.handleDidClose(context.Background(), reply, req); err != nil {
			t.Fatal(err)
		}

		assertScheduledDiagnostics(t, scheduled, mainURI, toolsURI)
	})
}

func assertScheduledDiagnostics(t *testing.T, scheduled <-chan uri.URI, expected ...uri.URI) {
	t.Helper()
	remaining := map[uri.URI]bool{}
	for _, docURI := range expected {
		remaining[docURI] = true
	}
	deadline := time.After(time.Second)
	for len(remaining) > 0 {
		select {
		case got := <-scheduled:
			delete(remaining, got)
		case <-deadline:
			t.Fatalf("missing scheduled diagnostics for %#v", remaining)
		}
	}
}

func assertDocumentSyncRepliesBeforeDiagnostics(t *testing.T, handle func(*Server, uri.URI, jsonrpc2.Replier) error) {
	t.Helper()
	server := NewServer()
	server.diagnosticsDelay = 0
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	server.diagnosticsPublisher = func(context.Context, uri.URI) {
		started <- struct{}{}
		<-release
	}

	replied := make(chan struct{}, 1)
	reply := jsonrpc2.Replier(func(ctx context.Context, result interface{}, err error) error {
		replied <- struct{}{}
		return nil
	})
	done := make(chan error, 1)
	docURI := uri.New("file:///test.ard")
	go func() {
		done <- handle(server, docURI, reply)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("diagnostics were not scheduled")
	}

	select {
	case <-replied:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("document sync handler waited for diagnostics before replying")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("document sync handler did not return")
	}
}
func TestJSONRPCHandlerDoesNotSerializeFeatureRequests(t *testing.T) {
	server := NewServer()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	server.handlers["ard/block"] = func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		entered <- struct{}{}
		<-release
		return reply(ctx, nil, nil)
	}

	handler := server.jsonRPCHandler()
	replied := make(chan error, 2)
	reply := func(ctx context.Context, result interface{}, replyErr error) error {
		replied <- replyErr
		return nil
	}

	for id := int32(1); id <= 2; id++ {
		req, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(id), "ard/block", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := handler(context.Background(), reply, req); err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
	}

	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(500 * time.Millisecond):
			close(release)
			t.Fatal("expected concurrent feature handlers to enter without waiting for each other")
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case err := <-replied:
			if err != nil {
				t.Fatalf("reply error: %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("expected feature handler reply")
		}
	}
}
func TestJSONRPCHandlerDoesNotBlockDocumentSyncBehindFeatureRequests(t *testing.T) {
	server := NewServer()
	server.diagnosticsDelay = 0
	server.diagnosticsPublisher = func(context.Context, uri.URI) {}
	docURI := uri.New("file:///main.ard")
	server.cache.Open(docURI, "ard", 1, "let x = 1")

	entered := make(chan struct{})
	release := make(chan struct{})
	server.handlers["ard/block"] = func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		close(entered)
		<-release
		return reply(ctx, nil, nil)
	}

	handler := server.jsonRPCHandler()
	replied := make(chan error, 2)
	reply := func(ctx context.Context, result interface{}, replyErr error) error {
		replied <- replyErr
		return nil
	}

	blockingReq, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(1), "ard/block", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler(context.Background(), reply, blockingReq); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocking feature request did not start")
	}

	changeReq, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(2), protocol.MethodTextDocumentDidChange, didChangeParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: docURI},
			Version:                2,
		},
		ContentChanges: []documentContentChange{{Text: "let x = 2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler(context.Background(), reply, changeReq); err != nil {
		t.Fatal(err)
	}

	if got := server.cache.Get(docURI).Text; got != "let x = 2" {
		close(release)
		t.Fatalf("didChange was blocked behind feature request; text = %q", got)
	}
	select {
	case err := <-replied:
		if err != nil {
			close(release)
			t.Fatalf("didChange reply error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		close(release)
		t.Fatal("didChange did not reply while feature request was blocked")
	}

	close(release)
	select {
	case err := <-replied:
		if err != nil {
			t.Fatalf("feature reply error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("feature request did not finish")
	}
}

func checkerDiagnosticsToLSPForTest(diagnostics []checker.Diagnostic) []protocol.Diagnostic {
	return checkerDiagnosticsToLSP(
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

// TestCheckerDiagnosticsToLSP verifies the diagnostic conversion handles edge cases.
func TestCheckerDiagnosticsToLSP(t *testing.T) {
	// Nil input should produce empty slice (not nil) so JSON is [] not null
	if result := checkerDiagnosticsToLSPForTest(nil); result == nil {
		t.Error("expected non-nil empty slice for nil input")
	} else if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}

	// Empty input
	if result := checkerDiagnosticsToLSPForTest([]checker.Diagnostic{}); result == nil {
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
	result := checkerDiagnosticsToLSPForTest(diags)
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
	if result[0].Range.End.Character != 10 {
		t.Errorf("expected exclusive end character 10 (0-based), got %d", result[0].Range.End.Character)
	}
	if result[0].Source != "ard" {
		t.Errorf("expected source 'ard', got %q", result[0].Source)
	}
}

func TestCheckerDiagnosticsToLSPPreservesStructuredLabels(t *testing.T) {
	diagnostic := checker.Diagnostic{
		Kind:  checker.Error,
		Code:  checker.DiagnosticCodeTypeMismatch,
		Title: "Type mismatch",
		Primary: checker.DiagnosticLabel{
			Span: checker.SourceSpan{FilePath: "main.ard", Location: parse.Location{
				Start: parse.Point{Row: 2, Col: 10}, End: parse.Point{Row: 2, Col: 11},
			}},
			Message: "this expression has type `Int`",
		},
		Secondary: []checker.DiagnosticLabel{{
			Span: checker.SourceSpan{FilePath: "main.ard", Location: parse.Location{
				Start: parse.Point{Row: 2, Col: 5}, End: parse.Point{Row: 2, Col: 7},
			}},
			Message: "this annotation requires `Str`",
		}},
	}

	result := checkerDiagnosticsToLSPForTest([]checker.Diagnostic{diagnostic})
	if len(result) != 1 {
		t.Fatalf("diagnostics = %#v, want one", result)
	}
	if got, want := result[0].Message, "Type mismatch: this expression has type `Int`: this annotation requires `Str`"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	if result[0].Range.Start.Line != 1 || result[0].Range.Start.Character != 9 || result[0].Range.End.Character != 11 {
		t.Fatalf("primary range = %#v", result[0].Range)
	}
	if len(result[0].RelatedInformation) != 1 {
		t.Fatalf("related information = %#v, want one", result[0].RelatedInformation)
	}
	if result[0].RelatedInformation[0].Message != "this annotation requires `Str`" {
		t.Fatalf("related message = %q", result[0].RelatedInformation[0].Message)
	}
}

func TestServerDiagnosticsDoNotPublishAnotherFilesErrors(t *testing.T) {
	root := t.TempDir()
	server := NewServer()
	server.projectRoot = root
	diagnostic := checker.NewDiagnostic(
		checker.Error,
		"broken dependency",
		filepath.Join("dependency", "main.ard"),
		parse.Location{Start: parse.Point{Row: 8, Col: 2}, End: parse.Point{Row: 8, Col: 3}},
	)

	result := server.checkerDiagnosticsToLSP([]checker.Diagnostic{diagnostic}, uri.File(filepath.Join(root, "main.ard")))
	if len(result) != 0 {
		t.Fatalf("published another file's diagnostic: %#v", result)
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

// TestFormatSource verifies the formatting flow end-to-end.
func TestFormatSource(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "adds trailing newline",
			input:    "let x = 5",
			expected: "let x = 5\n",
		},
		{
			name:     "normalizes spacing",
			input:    "let   x  =  5",
			expected: "let x = 5\n",
		},
		{
			name:     "handles empty content",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatSource(tt.input, "test.ard")
			if err != nil {
				t.Fatalf("formatSource error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}
func TestFeatureHandlersIgnoreNonFileDocuments(t *testing.T) {
	server := NewServer()
	docURI := uri.URI("untitled:Untitled-1")
	server.cache.Open(docURI, "ard", 1, "let   x  =  5")

	formatReq, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(1), protocol.MethodTextDocumentFormatting, protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	})
	if err != nil {
		t.Fatal(err)
	}
	var edits []protocol.TextEdit
	formatReply := jsonrpc2.Replier(func(ctx context.Context, result interface{}, err error) error {
		if err != nil {
			return err
		}
		var ok bool
		edits, ok = result.([]protocol.TextEdit)
		if !ok {
			t.Fatalf("formatting result = %T, want []protocol.TextEdit", result)
		}
		return nil
	})
	if err := server.handleFormatting(context.Background(), formatReply, formatReq); err != nil {
		t.Fatal(err)
	}
	if len(edits) != 0 {
		t.Fatalf("expected no formatting edits for non-file URI, got %#v", edits)
	}

	hoverReq, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(2), protocol.MethodTextDocumentHover, protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	replied := false
	hoverReply := jsonrpc2.Replier(func(ctx context.Context, result interface{}, err error) error {
		replied = true
		if err != nil {
			return err
		}
		if result != nil {
			t.Fatalf("expected nil hover for non-file URI, got %#v", result)
		}
		return nil
	})
	if err := server.handleHover(context.Background(), hoverReply, hoverReq); err != nil {
		t.Fatal(err)
	}
	if !replied {
		t.Fatal("expected hover handler to reply")
	}
}
func TestCodeActionRemoveUnusedImports(t *testing.T) {
	server := NewServer()
	docURI := uri.New("file:///test.ard")
	server.cache.Open(docURI, "ard", 1, "use app/unused\nuse app/text\n\nlet label = text::new(\"hi\")\n")

	req, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(1), protocol.MethodTextDocumentCodeAction, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{protocol.SourceOrganizeImports}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var actions []protocol.CodeAction
	reply := jsonrpc2.Replier(func(ctx context.Context, result interface{}, err error) error {
		if err != nil {
			return err
		}
		var ok bool
		actions, ok = result.([]protocol.CodeAction)
		if !ok {
			t.Fatalf("result = %T, want []protocol.CodeAction", result)
		}
		return nil
	})
	if err := server.handleCodeAction(context.Background(), reply, req); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Kind != protocol.SourceOrganizeImports {
		t.Fatalf("action kind = %q, want %q", actions[0].Kind, protocol.SourceOrganizeImports)
	}
	edits := actions[0].Edit.Changes[protocol.DocumentURI(docURI)]
	if len(edits) != 1 {
		t.Fatalf("expected 1 edit, got %d", len(edits))
	}
	if strings.Contains(edits[0].NewText, "app/unused") {
		t.Fatalf("expected unused import to be removed, got:\n%s", edits[0].NewText)
	}
}

// TestFormattingHandler verifies the full handleFormatting flow.
func TestFormattingHandler(t *testing.T) {
	server := NewServer()
	uri := uri.New("file:///test.ard")
	server.cache.Open(uri, "ard", 1, "let   x  =  5")

	// We can't easily call handleFormatting directly because it needs
	// a jsonrpc2.Replier. Instead, verify the formatting through formatSource.
	formatted, err := formatSource("let   x  =  5", "test.ard")
	if err != nil {
		t.Fatalf("formatSource error: %v", err)
	}
	if formatted != "let x = 5\n" {
		t.Errorf("expected formatted 'let x = 5\\n', got %q", formatted)
	}
}

func requireDefinition(t *testing.T, source string, filePath string, line uint32, char uint32) protocol.Location {
	t.Helper()
	srv, docURI := spanServer(t, source, filePath)
	locations := srv.definitionFromSpans(context.Background(), docURI, protocol.Position{Line: line, Character: char})
	if len(locations) != 1 {
		t.Fatalf("expected 1 definition, got %d: %#v", len(locations), locations)
	}
	return locations[0]
}

func assertDefinitionStart(t *testing.T, loc protocol.Location, filePath string, line uint32, char uint32) {
	t.Helper()
	if got := loc.URI.Filename(); got != filePath {
		t.Fatalf("definition file = %q, want %q", got, filePath)
	}
	if loc.Range.Start.Line != line || loc.Range.Start.Character != char {
		t.Fatalf("definition start = %d:%d, want %d:%d", loc.Range.Start.Line, loc.Range.Start.Character, line, char)
	}
}

func requireReferences(t *testing.T, source string, filePath string, line uint32, char uint32, includeDeclaration bool) []protocol.Location {
	t.Helper()
	srv, docURI := spanServer(t, source, filePath)
	locations := srv.referencesFromSpans(context.Background(), docURI, protocol.Position{Line: line, Character: char}, includeDeclaration)
	if len(locations) == 0 {
		t.Fatalf("expected references at %d:%d, got none", line, char)
	}
	return locations
}

func assertLocationStart(t *testing.T, loc protocol.Location, filePath string, line uint32, char uint32) {
	t.Helper()
	if got := loc.URI.Filename(); got != filePath {
		t.Fatalf("location file = %q, want %q", got, filePath)
	}
	if loc.Range.Start.Line != line || loc.Range.Start.Character != char {
		t.Fatalf("location start = %d:%d, want %d:%d", loc.Range.Start.Line, loc.Range.Start.Character, line, char)
	}
}

func assertHighlightStart(t *testing.T, h protocol.DocumentHighlight, line uint32, char uint32) {
	t.Helper()
	if h.Range.Start.Line != line || h.Range.Start.Character != char {
		t.Fatalf("highlight start = %d:%d, want %d:%d", h.Range.Start.Line, h.Range.Start.Character, line, char)
	}
}

// TestDocumentHighlightLocalSymbols verifies current-file highlights for locals.
func TestDocumentHighlightLocalSymbols(t *testing.T) {
	source := `fn main() Int {
  let value = 40
  let result = value + 2
  result + value
}
`
	srv, docURI := spanServer(t, source, "")
	highlights := srv.highlightsFromSpans(context.Background(), docURI, protocol.Position{Line: 2, Character: 16})
	if len(highlights) != 3 {
		t.Fatalf("expected 3 highlights, got %d: %#v", len(highlights), highlights)
	}
	assertHighlightStart(t, highlights[0], 1, 6)
	assertHighlightStart(t, highlights[1], 2, 15)
	assertHighlightStart(t, highlights[2], 3, 11)
	if highlights[0].Kind != protocol.DocumentHighlightKindWrite {
		t.Fatalf("declaration highlight kind = %v, want Write", highlights[0].Kind)
	}
}

// TestDocumentHighlightDoesNotCrossFiles verifies document highlights stay in the current file.
func TestDocumentHighlightDoesNotCrossFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mathPath := filepath.Join(root, "math.ard")
	if err := os.WriteFile(mathPath, []byte("fn add(left: Int, right: Int) Int { left + right }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/math

fn main() Int {
  math::add(1, 2) + math::add(3, 4)
}
`
	filePath := filepath.Join(root, "main.ard")
	srv, docURI := spanServer(t, source, filePath)
	highlights := srv.highlightsFromSpans(context.Background(), docURI, protocol.Position{Line: 3, Character: 9})
	if len(highlights) != 2 {
		t.Fatalf("expected 2 current-file highlights, got %d: %#v", len(highlights), highlights)
	}
	assertHighlightStart(t, highlights[0], 3, 8)
	assertHighlightStart(t, highlights[1], 3, 26)
}

// TestReferencesLocalSymbols verifies find-references for local symbols.
func TestReferencesLocalSymbols(t *testing.T) {
	source := `fn add(left: Int, right: Int) Int {
  left + right
}

fn main() Int {
  let value = 40
  let result = add(value, 2)
  add(result, value)
}
`
	filePath := filepath.Join(t.TempDir(), "test.ard")

	t.Run("function", func(t *testing.T) {
		refs := requireReferences(t, source, filePath, 6, 16, true)
		if len(refs) != 3 {
			t.Fatalf("expected 3 refs, got %d: %#v", len(refs), refs)
		}
		assertLocationStart(t, refs[0], filePath, 0, 3)
		assertLocationStart(t, refs[1], filePath, 6, 15)
		assertLocationStart(t, refs[2], filePath, 7, 2)
	})

	t.Run("local variable without declaration", func(t *testing.T) {
		refs := requireReferences(t, source, filePath, 6, 20, false)
		if len(refs) != 2 {
			t.Fatalf("expected 2 refs, got %d: %#v", len(refs), refs)
		}
		assertLocationStart(t, refs[0], filePath, 6, 19)
		assertLocationStart(t, refs[1], filePath, 7, 14)
	})

	t.Run("parameter", func(t *testing.T) {
		refs := requireReferences(t, source, filePath, 1, 3, true)
		if len(refs) != 2 {
			t.Fatalf("expected 2 refs, got %d: %#v", len(refs), refs)
		}
		assertLocationStart(t, refs[0], filePath, 0, 7)
		assertLocationStart(t, refs[1], filePath, 1, 2)
	})
}

// TestReferencesScopedLocalSymbols verifies same-named locals in different scopes stay separate.
func TestReferencesScopedLocalSymbols(t *testing.T) {
	source := `fn first() Int {
  let value = 1
  value
}

fn second() Int {
  let value = 2
  value
}
`
	filePath := filepath.Join(t.TempDir(), "test.ard")

	refs := requireReferences(t, source, filePath, 2, 3, true)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d: %#v", len(refs), refs)
	}
	assertLocationStart(t, refs[0], filePath, 1, 6)
	assertLocationStart(t, refs[1], filePath, 2, 2)
}

// TestReferencesStructSymbols verifies find-references from struct declarations.
func TestReferencesStructSymbols(t *testing.T) {
	source := `struct Box {
  item: Int,
}

impl Box {
  fn get() Int {
    self.item
  }
}

fn main(box: Box) Int {
  let made = Box { item: 1 }
  box.item + made.item
}
`
	filePath := filepath.Join(t.TempDir(), "test.ard")

	t.Run("field declaration", func(t *testing.T) {
		refs := requireReferences(t, source, filePath, 1, 4, true)
		if len(refs) != 5 {
			t.Fatalf("expected 5 refs, got %d: %#v", len(refs), refs)
		}
		assertLocationStart(t, refs[0], filePath, 1, 2)
		assertLocationStart(t, refs[1], filePath, 6, 9)
		assertLocationStart(t, refs[2], filePath, 11, 19)
		assertLocationStart(t, refs[3], filePath, 12, 6)
		assertLocationStart(t, refs[4], filePath, 12, 18)
	})

	assertBoxTypeRefs := func(t *testing.T, refs []protocol.Location) {
		t.Helper()
		if len(refs) != 4 {
			t.Fatalf("expected 4 refs, got %d: %#v", len(refs), refs)
		}
		assertLocationStart(t, refs[0], filePath, 0, 7)
		assertLocationStart(t, refs[1], filePath, 4, 5)
		assertLocationStart(t, refs[2], filePath, 10, 13)
		assertLocationStart(t, refs[3], filePath, 11, 13)
	}

	t.Run("type declaration", func(t *testing.T) {
		assertBoxTypeRefs(t, requireReferences(t, source, filePath, 0, 8, true))
	})

	t.Run("type annotation", func(t *testing.T) {
		assertBoxTypeRefs(t, requireReferences(t, source, filePath, 10, 14, true))
	})
}

// TestReferencesStaticFunctionDeclarationTarget verifies references from the type target in `fn Type::name` declarations.
func TestReferencesStaticFunctionDeclarationTarget(t *testing.T) {
	source := `struct Scrollview {
  scroll: Int,
}

fn Scrollview::new() Scrollview {
  Scrollview{scroll: 0}
}

impl Scrollview {
  fn draw() Void { () }
}

fn main() Scrollview {
  Scrollview::new()
}
`
	filePath := filepath.Join(t.TempDir(), "components.ard")

	refs := requireReferences(t, source, filePath, 4, 4, true)
	if len(refs) != 7 {
		t.Fatalf("expected 7 refs, got %d: %#v", len(refs), refs)
	}
	assertLocationStart(t, refs[0], filePath, 0, 7)
	assertLocationStart(t, refs[1], filePath, 4, 3)
	assertLocationStart(t, refs[2], filePath, 4, 21)
	assertLocationStart(t, refs[3], filePath, 5, 2)
	assertLocationStart(t, refs[4], filePath, 8, 5)
	assertLocationStart(t, refs[5], filePath, 12, 10)
	assertLocationStart(t, refs[6], filePath, 13, 2)
}

// TestReferencesWorkspaceFiles verifies find-references across project files.
func TestReferencesWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mathSource := `fn inc(value: Int) Int {
  value + 1
}
`
	mathPath := filepath.Join(root, "math.ard")
	if err := os.WriteFile(mathPath, []byte(mathSource), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/math

fn main() Int {
  math::inc(1)
}
`
	filePath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	otherSource := `use test_project/math

fn other() Int {
  math::inc(2)
}
`
	otherPath := filepath.Join(root, "other.ard")
	if err := os.WriteFile(otherPath, []byte(otherSource), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("from call", func(t *testing.T) {
		refs := requireReferences(t, source, filePath, 3, 9, true)
		if len(refs) != 3 {
			t.Fatalf("expected 3 refs, got %d: %#v", len(refs), refs)
		}
		assertLocationStart(t, refs[0], mathPath, 0, 3)
		assertLocationStart(t, refs[1], filePath, 3, 8)
		assertLocationStart(t, refs[2], otherPath, 3, 8)
	})

	t.Run("from declaration", func(t *testing.T) {
		refs := requireReferences(t, mathSource, mathPath, 0, 4, true)
		if len(refs) != 3 {
			t.Fatalf("expected 3 refs, got %d: %#v", len(refs), refs)
		}
		assertLocationStart(t, refs[0], mathPath, 0, 3)
		assertLocationStart(t, refs[1], filePath, 3, 8)
		assertLocationStart(t, refs[2], otherPath, 3, 8)
	})
}

// TestReferencesOpenDocumentOverlays verifies references use unsaved open-document content.
func TestReferencesOpenDocumentOverlays(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mathSource := `fn inc(value: Int) Int {
  value + 1
}
`
	mathPath := filepath.Join(root, "math.ard")
	if err := os.WriteFile(mathPath, []byte(mathSource), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/math

fn main() Int {
  math::inc(1)
}
`
	filePath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(root, "other.ard")
	otherOverlay := `use test_project/math

fn other() Int {
  math::inc(2)
}
`

	srv, docURI := spanServer(t, source, filePath)
	// The other file exists only as an unsaved editor overlay.
	srv.cache.Open(uri.File(otherPath), "ard", 1, otherOverlay)
	if err := os.WriteFile(otherPath, []byte(otherOverlay), 0o644); err != nil {
		t.Fatal(err)
	}
	refs := srv.referencesFromSpans(context.Background(), docURI, protocol.Position{Line: 3, Character: 9}, true)
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d: %#v", len(refs), refs)
	}
	assertLocationStart(t, refs[0], mathPath, 0, 3)
	assertLocationStart(t, refs[1], filePath, 3, 8)
	assertLocationStart(t, refs[2], otherPath, 3, 8)
}

// TestReferencesImportedModuleLocalUses verifies imported symbols also include uses in their defining module.
func TestReferencesImportedModuleLocalUses(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	responsesSource := `fn json_error(status: Int) Int {
  status
}

fn retry() Int {
  json_error(500)
}
`
	responsesPath := filepath.Join(root, "responses.ard")
	if err := os.WriteFile(responsesPath, []byte(responsesSource), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/responses

fn main() Int {
  responses::json_error(400)
}
`
	filePath := filepath.Join(root, "routes.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	refs := requireReferences(t, source, filePath, 3, 15, true)
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d: %#v", len(refs), refs)
	}
	assertLocationStart(t, refs[0], responsesPath, 0, 3)
	assertLocationStart(t, refs[1], responsesPath, 5, 2)
	assertLocationStart(t, refs[2], filePath, 3, 13)
}

// TestReferencesImportedVariableSkipsModuleAlias verifies module aliases are not reported as variable refs.
func TestReferencesImportedVariableSkipsModuleAlias(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	responsesSource := `let api_name = "ranger"
`
	responsesPath := filepath.Join(root, "responses.ard")
	if err := os.WriteFile(responsesPath, []byte(responsesSource), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/responses

fn main() Str {
  let a = responses::api_name
  let b = responses::api_name
  a
}
`
	filePath := filepath.Join(root, "routes.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	refs := requireReferences(t, source, filePath, 3, 23, true)
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d: %#v", len(refs), refs)
	}
	assertLocationStart(t, refs[0], responsesPath, 0, 4)
	assertLocationStart(t, refs[1], filePath, 3, 21)
	assertLocationStart(t, refs[2], filePath, 4, 21)
}

// TestReferencesImportedInstanceMembers verifies find-references for imported fields and methods.
func TestReferencesImportedInstanceMembers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	boxesSource := `struct Box {
  item: Int,
}

impl Box {
  fn get() Int {
    self.item
  }
}
`
	boxesPath := filepath.Join(root, "boxes.ard")
	if err := os.WriteFile(boxesPath, []byte(boxesSource), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/boxes

fn main(box: boxes::Box) {
  let a = box.item
  let b = box.item
  box.get()
  box.get()
}
`
	filePath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("field", func(t *testing.T) {
		refs := requireReferences(t, source, filePath, 3, 16, true)
		if len(refs) != 4 {
			t.Fatalf("expected 4 refs, got %d: %#v", len(refs), refs)
		}
		assertLocationStart(t, refs[0], boxesPath, 1, 2)
		assertLocationStart(t, refs[1], boxesPath, 6, 9)
		assertLocationStart(t, refs[2], filePath, 3, 14)
		assertLocationStart(t, refs[3], filePath, 4, 14)
	})

	t.Run("method", func(t *testing.T) {
		refs := requireReferences(t, source, filePath, 5, 7, true)
		if len(refs) != 3 {
			t.Fatalf("expected 3 refs, got %d: %#v", len(refs), refs)
		}
		assertLocationStart(t, refs[0], boxesPath, 5, 5)
		assertLocationStart(t, refs[1], filePath, 5, 6)
		assertLocationStart(t, refs[2], filePath, 6, 6)
	})

	t.Run("imported type annotation", func(t *testing.T) {
		refs := requireReferences(t, source, filePath, 2, 21, true)
		if len(refs) != 3 {
			t.Fatalf("expected 3 refs, got %d: %#v", len(refs), refs)
		}
		assertLocationStart(t, refs[0], boxesPath, 0, 7)
		assertLocationStart(t, refs[1], boxesPath, 4, 5)
		// The reference narrows to the type name itself (`Box`), not the
		// module-qualified prefix, so rename edits stay valid.
		assertLocationStart(t, refs[2], filePath, 2, 20)
	})

	t.Run("method declaration", func(t *testing.T) {
		refs := requireReferences(t, boxesSource, boxesPath, 5, 6, true)
		if len(refs) != 3 {
			t.Fatalf("expected 3 refs, got %d: %#v", len(refs), refs)
		}
		assertLocationStart(t, refs[0], boxesPath, 5, 5)
		assertLocationStart(t, refs[1], filePath, 5, 6)
		assertLocationStart(t, refs[2], filePath, 6, 6)
	})
}

// TestDefinitionLocalSymbols verifies go-to-definition for local symbols.
func TestDefinitionLocalSymbols(t *testing.T) {
	source := `fn add(left: Int, right: Int) Int {
  left + right
}

fn main() Int {
  let value = 40
  let result = add(value, 2)
  result
}
`
	filePath := filepath.Join(t.TempDir(), "test.ard")

	t.Run("function call", func(t *testing.T) {
		loc := requireDefinition(t, source, filePath, 6, 16)
		assertDefinitionStart(t, loc, filePath, 0, 0)
	})
	t.Run("local variable use", func(t *testing.T) {
		loc := requireDefinition(t, source, filePath, 7, 3)
		assertDefinitionStart(t, loc, filePath, 6, 2)
	})
	t.Run("argument local variable", func(t *testing.T) {
		loc := requireDefinition(t, source, filePath, 6, 20)
		assertDefinitionStart(t, loc, filePath, 5, 2)
	})
	t.Run("parameter", func(t *testing.T) {
		loc := requireDefinition(t, source, filePath, 1, 3)
		assertDefinitionStart(t, loc, filePath, 0, 7)
	})
}

// TestDefinitionImportedModuleSymbols verifies go-to-definition across imported modules.
func TestDefinitionNestedStaticFunctionUsesInnerModuleAlias(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "text.ard"), []byte(`fn new(label: Str) Str { label }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "stack.ard"), []byte(`fn hstack(children: [Str]) Str { "" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/stack
use test_project/text

fn main() {
  let tabs = stack::hstack([
    text::new("Inbox"),
  ])
}
`
	filePath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	textPath := filepath.Join(root, "text.ard")

	loc := requireDefinition(t, source, filePath, 5, 5)
	assertDefinitionStart(t, loc, textPath, 0, 0)
}

// TestDefinitionImportedInstanceMembers verifies go-to-definition for imported fields and methods.
func TestDefinitionImportedInstanceMembers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	boxesSource := `struct Box {
  item: $T,
}

impl Box {
  fn get() $T {
    self.item
  }
}
`
	boxesPath := filepath.Join(root, "boxes.ard")
	if err := os.WriteFile(boxesPath, []byte(boxesSource), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/boxes

fn main(box: boxes::Box<Int>) {
  let item = box.item
  box.get()
}
`
	filePath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("field", func(t *testing.T) {
		loc := requireDefinition(t, source, filePath, 3, 18)
		assertDefinitionStart(t, loc, boxesPath, 1, 2)
	})
	t.Run("method", func(t *testing.T) {
		loc := requireDefinition(t, source, filePath, 4, 7)
		assertDefinitionStart(t, loc, boxesPath, 5, 2)
	})
}

func requireSignatureHelp(t *testing.T, source string, filePath string, line uint32, char uint32) *protocol.SignatureHelp {
	t.Helper()
	srv, docURI := spanServer(t, source, filePath)
	help := srv.signatureHelpFromSpans(context.Background(), docURI, source, protocol.Position{Line: line, Character: char})
	if help == nil || len(help.Signatures) == 0 {
		t.Fatalf("expected signature help at %d:%d, got %#v", line, char, help)
	}
	return help
}

func requireSignatureHelpAtMarker(t *testing.T, markedSource string, filePath string) (*protocol.SignatureHelp, string) {
	t.Helper()
	source, line, char := sourceMarkerPosition(t, markedSource)
	return requireSignatureHelp(t, source, filePath, line, char), source
}

func sourceMarkerPosition(t *testing.T, markedSource string) (string, uint32, uint32) {
	t.Helper()
	idx := strings.Index(markedSource, "|")
	if idx < 0 {
		t.Fatalf("marked source must contain | cursor marker")
	}
	source := markedSource[:idx] + markedSource[idx+1:]
	before := markedSource[:idx]
	line := uint32(strings.Count(before, "\n"))
	lineStart := strings.LastIndex(before, "\n")
	char := idx
	if lineStart >= 0 {
		char = idx - lineStart - 1
	}
	return source, line, uint32(char)
}

func assertSignature(t *testing.T, help *protocol.SignatureHelp, want string, active uint32) {
	t.Helper()
	if got := help.Signatures[0].Label; got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
	if got := help.ActiveParameter; got != active {
		t.Fatalf("active parameter = %d, want %d", got, active)
	}
}

// TestSignatureHelpLocalFunction verifies signature help for local calls.
func TestSignatureHelpLocalFunction(t *testing.T) {
	source := `fn add(left: Int, right: Int) Int { left + right }
fn main() {
  let n = add(1, 2)
}
`
	help := requireSignatureHelp(t, source, "test.ard", 2, 17)
	assertSignature(t, help, "fn add(left: Int, right: Int) Int", 1)
}

// TestSignatureHelpInstanceMethod verifies signature help for instance methods.
func TestSignatureHelpInstanceMethod(t *testing.T) {
	source := `struct Board {
  cells: [Str]
}
impl Board {
  fn mut play(player: Str, pos: Int) {
    self.cells.set(pos, player)
  }
}
`
	help := requireSignatureHelp(t, source, "test.ard", 5, 24)
	assertSignature(t, help, "fn mut [Str].set(index: Int, value: Str) Bool", 1)
}

// TestSignatureHelpImportedGenericMethod verifies imported generic method signatures substitute type args.
func TestSignatureHelpImportedGenericMethod(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	boxesSource := `struct Box {
  item: $T,
}

impl Box {
  fn replace(item: $T) $T {
    item
  }
}
`
	if err := os.WriteFile(filepath.Join(root, "boxes.ard"), []byte(boxesSource), 0o644); err != nil {
		t.Fatal(err)
	}
	marked := `use test_project/boxes

fn main(box: boxes::Box<Int>) {
  box.replace(1|)
}
`
	source, line, char := sourceMarkerPosition(t, marked)
	filePath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	help := requireSignatureHelp(t, source, filePath, line, char)
	// The span path renders the checked type name; module qualification was
	// legacy display sugar.
	assertSignature(t, help, "fn Box<Int>.replace(item: Int) Int", 0)
}

// TestSignatureHelpNamedArguments maps named arguments back to the matching parameter.
func TestSignatureHelpNamedArguments(t *testing.T) {
	help, _ := requireSignatureHelpAtMarker(t, `fn configure(width: Int, height: Int, title: Str) {}
fn main() {
  configure(title: "Demo", width: 80|, height: 24)
}
`, "test.ard")
	assertSignature(t, help, "fn configure(width: Int, height: Int, title: Str) Void", 0)
}

// TestSignatureHelpNestedCommas ignores commas inside nested calls when selecting the active parameter.
func TestSignatureHelpNestedCommas(t *testing.T) {
	help, _ := requireSignatureHelpAtMarker(t, `fn wrap(left: Int, right: Int) Int { left + right }
fn outer(label: Str, value: Int, done: Bool) {}
fn main() {
  outer("x", wrap(1, 2), true|)
}
`, "test.ard")
	assertSignature(t, help, "fn outer(label: Str, value: Int, done: Bool) Void", 2)
}

// TestSignatureHelpIncompleteCall verifies help while the user is still typing a call.
func TestSignatureHelpIncompleteCall(t *testing.T) {
	help, _ := requireSignatureHelpAtMarker(t, `fn add(left: Int, right: Int) Int { left + right }
fn main() {
  let n = add(1, |
`, "test.ard")
	assertSignature(t, help, "fn add(left: Int, right: Int) Int", 1)
}

// TestSignatureHelpIncompleteEmptyCall verifies help immediately after typing an opening paren.
func TestSignatureHelpIncompleteEmptyCall(t *testing.T) {
	help, _ := requireSignatureHelpAtMarker(t, `fn add(left: Int, right: Int) Int { left + right }
fn main() {
  let n = add(|
`, "test.ard")
	assertSignature(t, help, "fn add(left: Int, right: Int) Int", 0)
}

// TestSignatureHelpNestedIncompleteCall keeps the innermost active call when parent calls are unfinished.
func TestSignatureHelpNestedIncompleteCall(t *testing.T) {
	help, _ := requireSignatureHelpAtMarker(t, `fn add(left: Int, right: Int) Int { left + right }
fn outer(value: Int) Int { value }
fn main() {
  let n = outer(add(1, |
`, "test.ard")
	assertSignature(t, help, "fn add(left: Int, right: Int) Int", 1)
}

// TestSignatureHelpIncompleteNamedArgument maps an unfinished named arg to its parameter.
func TestSignatureHelpIncompleteNamedArgument(t *testing.T) {
	help, _ := requireSignatureHelpAtMarker(t, `fn configure(width: Int, height: Int, title: Str) {}
fn main() {
  configure(title: |
`, "test.ard")
	assertSignature(t, help, "fn configure(width: Int, height: Int, title: Str) Void", 2)
}

// TestSignatureHelpTicTacToeLineDoesNotPanic covers the sample line that Zed requests while typing.
func TestSignatureHelpTicTacToeLineDoesNotPanic(t *testing.T) {
	filePath := filepath.Join("..", "samples", "tic-tac-toe.ard")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	srvSig, sigURI := spanServer(t, string(content), filepath.Join(t.TempDir(), "tic-tac-toe.ard"))
	source := string(content)
	lines := strings.Split(source, "\n")
	if len(lines) < 42 {
		t.Fatalf("expected tic-tac-toe sample to have at least 42 lines")
	}
	for char := 0; char <= len(lines[41]); char++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("signature help panicked at line 42 char %d: %v", char, r)
				}
			}()
			_ = srvSig.signatureHelpFromSpans(context.Background(), sigURI, source, protocol.Position{Line: 41, Character: uint32(char)})
		}()
	}
}

// TestTicTacToeLine42TypingDoesNotHang covers incomplete call states produced while typing in Zed.
func TestTicTacToeLine42TypingDoesNotHang(t *testing.T) {
	filePath := filepath.Join("..", "samples", "tic-tac-toe.ard")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	baseLines := strings.Split(string(content), "\n")
	if len(baseLines) < 42 {
		t.Fatalf("expected tic-tac-toe sample to have at least 42 lines")
	}

	srvSig, sigURI := spanServer(t, string(content), filepath.Join(t.TempDir(), "tic-tac-toe.ard"))
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic while typing line 42: %v", r)
			}
		}()

		variants := []string{"", "i", "io", "io:", "io::", "io::p", "io::print", "io::print(", "io::print()", `io::print("`, `io::print("x`}
		for _, variant := range variants {
			lines := append([]string(nil), baseLines...)
			lines[41] = "    " + variant
			source := strings.Join(lines, "\n")
			for char := 0; char <= len(lines[41]); char++ {
				_ = srvSig.signatureHelpFromSpans(context.Background(), sigURI, source, protocol.Position{Line: 41, Character: uint32(char)})
			}
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("typing line 42 in tic-tac-toe sample timed out")
	}
}
func TestCompletionImportPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tui", "core"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tui", "core", "text.ard"), []byte("fn new() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tui", "core", "stack.ard"), []byte("fn hstack() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, "main.ard")

	rootItems := computeImportCompletions("use ", filePath, protocol.Position{Line: 0, Character: 4})
	assertCompletion(t, rootItems, "ard", "")
	assertCompletion(t, rootItems, "test_project", "")

	moduleItems := computeImportCompletions("use test_project/tui/core/", filePath, protocol.Position{Line: 0, Character: 26})
	assertCompletion(t, moduleItems, "stack", "")
	assertCompletion(t, moduleItems, "text", "")

	prefixedItems := computeImportCompletions("use test_project/tui/core/st", filePath, protocol.Position{Line: 0, Character: 28})
	item, ok := completionItemByLabel(prefixedItems, "stack")
	if !ok || item.TextEdit == nil {
		t.Fatalf("expected stack completion with text edit, got %#v", prefixedItems)
	}
	if item.TextEdit.NewText != "stack" {
		t.Fatalf("completion edit text = %q, want stack", item.TextEdit.NewText)
	}
}

// TestCompletionInstanceMembers verifies dot completions for fields and methods.
func TestCompletionInstanceMembers(t *testing.T) {
	source := `struct Board {
  cells: [Str]
}
impl Board {
  fn can_play(pos: Int) Bool { true }
}
fn main() {
  mut board = Board{cells: []}
  board.
}
`

	srv, docURI := spanServer(t, source, "")
	items := srv.completionFromSpans(context.Background(), docURI, source, protocol.Position{Line: 8, Character: 8})
	assertCompletion(t, items, "cells", "[Str]")
	assertCompletion(t, items, "can_play", "fn (pos: Int) Bool")

	typeTarget := strings.Replace(source, "  board.\n", "  Board.\n", 1)
	srv.cache.Update(docURI, 2, typeTarget)
	typeTargetItems := srv.completionFromSpans(context.Background(), docURI, typeTarget, protocol.Position{Line: 8, Character: 8})
	if len(typeTargetItems) != 0 {
		t.Fatalf("Board. completions = %#v, want none for type-only target", typeTargetItems)
	}

	prefixed := strings.Replace(source, "  board.\n", "  board.ca\n", 1)
	srv.cache.Update(docURI, 3, prefixed)
	prefixedItems := srv.completionFromSpans(context.Background(), docURI, prefixed, protocol.Position{Line: 8, Character: 10})
	item, ok := completionItemByLabel(prefixedItems, "can_play")
	if !ok || item.TextEdit == nil {
		t.Fatalf("expected can_play completion with text edit, got %#v", item)
	}
	if item.TextEdit.NewText != "can_play" || item.TextEdit.Range.Start.Character != 8 || item.TextEdit.Range.End.Character != 10 {
		t.Fatalf("can_play text edit = %#v, want replace typed prefix", item.TextEdit)
	}
}

// TestCompletionUserModuleStaticMembers verifies user module functions and variables complete after ::.
func TestCompletionUserModuleStaticMembersExcludesTests(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	toolsSource := `fn helper() Int { 1 }

test fn helper_test() Void!Str { Result::ok(()) }
`
	if err := os.WriteFile(filepath.Join(root, "tools.ard"), []byte(toolsSource), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/tools

fn main() {
  tools::
}
`
	filePath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, docURI := spanServer(t, source, filePath)
	items := srv.completionFromSpans(context.Background(), docURI, source, protocol.Position{Line: 3, Character: 9})
	assertCompletion(t, items, "helper", "fn () Int")
	if _, ok := completionItemByLabel(items, "helper_test"); ok {
		t.Fatalf("test function completion should be excluded: %#v", items)
	}
}

// TestCompletionTraitMethods verifies dot completions for trait-typed receivers.
func TestCompletionTraitMethods(t *testing.T) {
	dir := t.TempDir()
	source := `trait Render {
  fn describe() Str
}

fn render(value: Render) Str {
  value.
}
`
	path := filepath.Join(dir, "test.ard")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer()
	docURI := uri.File(path)
	srv.cache.Open(docURI, "ard", 1, source)
	items := srv.completionFromSpans(context.Background(), docURI, source, protocol.Position{Line: 5, Character: 8})
	assertCompletion(t, items, "describe", "fn () Str")
}

// TestHoverPositions verifies hover returns correct type info.
func TestHoverPositions(t *testing.T) {
	tests := []struct {
		name   string
		source string
		line   uint32
		char   uint32
		want   string
	}{
		{
			name:   "string literal",
			source: `let x = "hello"` + "\n",
			line:   0,
			char:   8,
			want:   "Str",
		},
		{
			name:   "int literal",
			source: `let y = 42` + "\n",
			line:   0,
			char:   8,
			want:   "Int",
		},
		{
			name:   "bool literal",
			source: `let z = true` + "\n",
			line:   0,
			char:   8,
			want:   "Bool",
		},
		{
			name:   "float literal",
			source: `let f = 3.14` + "\n",
			line:   0,
			char:   8,
			want:   "Float64",
		},
		{
			name:   "variable declaration with type",
			source: `let name: Str = "hello"` + "\n",
			line:   0,
			char:   4,
			want:   "Str",
		},
		{
			name:   "builtin true",
			source: `let a = true` + "\n",
			line:   0,
			char:   8,
			want:   "Bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := spanHoverAt(t, tt.source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil")
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

// TestHoverInsideFunction verifies we find inner expressions, not the outer function signature.
func TestHoverInsideFunction(t *testing.T) {
	source := `fn greet(name: Str) Str {
    let msg = "Hello"
    msg
}` + "\n"

	tests := []struct {
		name string
		line uint32
		char uint32
		want string
	}{
		{
			name: "hover on string literal",
			line: 1,
			char: 14,
			want: "Str",
		},
		{
			name: "hover on variable name in body",
			line: 2,
			char: 4,
			want: "Str",
		},
		{
			name: "hover on function name",
			line: 0,
			char: 3,
			want: "fn greet",
		},
		{
			name: "hover on parameter",
			line: 0,
			char: 11,
			want: "Str",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := spanHoverAt(t, source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil at %d:%d", tt.line, tt.char)
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

// TestHoverOnSampleFile verifies hover doesn't panic across positions in a
// realistic multi-feature source file.
func TestHoverOnSampleFile(t *testing.T) {
	source := `use go:fmt

struct Task {
  title: Str,
  done: Bool,
}

impl Task {
  fn describe() Str {
    "{self.title}"
  }
}

fn main() {
  mut tasks: [Task] = []
  tasks.push(Task{title: "write tests", done: false})
  for task in tasks {
    fmt::Println(task.describe())
  }
  let titles: [Str: Bool] = ["a": true]
  match titles.get("a") {
    val? => fmt::Println(val),
    _ => {},
  }
}
`
	// Sweep every position; hover must never panic and may return nil.
	lines := strings.Split(source, "\n")
	for row, line := range lines {
		for col := 0; col <= len(line); col++ {
			pt := protocol.Position{Line: uint32(row), Character: uint32(col)}
			info := spanHoverAt(t, source, "test.ard", pt)
			_ = info
		}
	}
}

// TestHoverInferredExpression verifies type inference from function calls and identifiers.
func TestHoverInferredExpression(t *testing.T) {
	tests := []struct {
		name   string
		source string
		line   uint32
		char   uint32
		want   string
	}{
		{
			name:   "variable from function call",
			source: "fn get_value() Int { 42 }\nlet x = get_value()\n",
			line:   1,
			char:   6,
			want:   "Int",
		},
		{
			name:   "variable from another variable",
			source: "let a: Str = \"hi\"\nlet b = a\n",
			line:   1,
			char:   6,
			want:   "Str",
		},
		{
			name:   "variable from binary expression",
			source: "let x = 1 + 2\n",
			line:   0,
			char:   6,
			want:   "Int",
		},
		{
			name:   "variable from string concat",
			source: "let x = \"hello\" + \"world\"\n",
			line:   0,
			char:   6,
			want:   "Str",
		},
		{
			name:   "variable from comparison",
			source: "let x = 1 > 2\n",
			line:   0,
			char:   6,
			want:   "Bool",
		},
		{
			name:   "variable from struct instance",
			source: "struct Board { cells: [Str] }\nmut board = Board{cells: []}\n",
			line:   1,
			char:   10,
			want:   "Board",
		},
		{
			name:   "board in while loop method chain",
			source: "struct Board {\n  cells: [Str]\n}\nimpl Board {\n  fn is_full() Bool { false }\n}\nfn main() {\n  mut board = Board{cells: []}\n  while not board.is_full() {\n    board.do_stuff()\n  }\n}\n",
			line:   7,
			char:   13,
			want:   "Board",
		},
		{
			name:   "variable from maybe method chain",
			source: "fn main() {\n  let items = [9]\n  let input = items.at(0).or(-1)\n  input\n}\n",
			line:   2,
			char:   6,
			want:   "Int",
		},
		{
			name:   "variable in match case body",
			source: "fn read_move() Int {\n  let items = [9]\n  let input = items.at(0).or(-1)\n  match input >= 1 and input <= 9 {\n    true => input - 1,\n    false => -1,\n  }\n}\n",
			line:   4,
			char:   13,
			want:   "Int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := spanHoverAt(t, tt.source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil")
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

// TestHoverInImplBlock verifies variables and parameters hover inside impl methods.
func TestHoverInImplBlock(t *testing.T) {
	source := `struct Board {
  cells: [Str]
}
impl Board {
  fn mut play(player: Str, pos: Int) {
    self.cells.set(pos, player)
  }
  fn is_full() Bool {
    mut full = true
    for cell in self.cells {
      if cell.is_empty() {
        full = false
      }
    }
    full
  }
}
`

	tests := []struct {
		name string
		line uint32
		char uint32
		want string
	}{
		{name: "method parameter player", line: 4, char: 14, want: "Str"},
		{name: "method parameter pos", line: 4, char: 27, want: "Int"},
		{name: "self receiver", line: 5, char: 5, want: "Board"},
		{name: "field in method call", line: 5, char: 10, want: "Board.cells: [Str]"},
		{name: "list method", line: 5, char: 15, want: "fn mut [Str].set(index: Int, value: Str) Bool"},
		{name: "pos argument", line: 5, char: 19, want: "Int"},
		{name: "player argument", line: 5, char: 24, want: "Str"},
		{name: "local variable declaration", line: 8, char: 8, want: "Bool"},
		{name: "for loop cursor", line: 9, char: 8, want: "Str"},
		{name: "self in loop iterable", line: 9, char: 16, want: "Board"},
		{name: "field in loop iterable", line: 9, char: 22, want: "Board.cells: [Str]"},
		{name: "for loop cursor in condition", line: 10, char: 9, want: "Str"},
		{name: "string method", line: 10, char: 15, want: "fn Str.is_empty() Bool"},
		{name: "local variable assignment", line: 11, char: 8, want: "Bool"},
		{name: "local variable return", line: 14, char: 4, want: "Bool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := spanHoverAt(t, source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil at %d:%d", tt.line, tt.char)
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

// TestHoverInstanceMethodSignatures moved to hover_spans_test.go
// (TestSpanHoverInstanceMethodSignatures) on the snapshot/span path.

// TestHoverStaticFunctionSignatures verifies static function hovers include qualifier, params, and return type.
// TestHoverGenericImportedMembers verifies imported generic fields and methods substitute type args.
func TestHoverGenericImportedMembers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	boxesSource := `struct Box {
  item: $T,
}

impl Box {
  fn get() $T {
    self.item
  }
}
`
	if err := os.WriteFile(filepath.Join(root, "boxes.ard"), []byte(boxesSource), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/boxes

fn main(box: boxes::Box<Int>) {
  let item = box.item
  box.get()
}
`
	filePath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		line uint32
		char uint32
		want string
	}{
		// The span path renders the checked type name; module qualification
		// was legacy display sugar.
		{name: "generic field", line: 3, char: 18, want: "Box<Int>.item: Int"},
		{name: "generic method", line: 4, char: 7, want: "fn Box<Int>.get() Int"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := spanHoverAt(t, source, filePath, pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil at %d:%d", tt.line, tt.char)
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
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
func TestRenameLocalVariable(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "main.ard")
	source := `fn main() {
  let count = 1
  let next = count + 1
  count
}
`
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, docURI := spanServer(t, source, filePath)
	edit := srv.renameFromSpans(context.Background(), docURI, protocol.Position{Line: 1, Character: 7}, "total")
	assertRenameEdits(t, edit, filePath, []renameWant{{1, 6, 11}, {2, 13, 18}, {3, 2, 7}})
}
func TestRenameImportedFunction(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	modPath := filepath.Join(root, "responses.ard")
	modSource := `fn json_error(status: Int) Int {
  status
}
`
	if err := os.WriteFile(modPath, []byte(modSource), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	mainSource := `use test_project/responses

fn main() {
  responses::json_error(400)
}
`
	if err := os.WriteFile(mainPath, []byte(mainSource), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, docURI := spanServer(t, mainSource, mainPath)
	edit := srv.renameFromSpans(context.Background(), docURI, protocol.Position{Line: 3, Character: 15}, "error_json")
	assertRenameEdits(t, edit, modPath, []renameWant{{0, 3, 13}})
	assertRenameEdits(t, edit, mainPath, []renameWant{{3, 13, 23}})
}
func TestRenameField(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "main.ard")
	source := `struct Board {
  cells: [Str],
}

fn main(board: Board) {
  let cells = board.cells
}
`
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, docURI := spanServer(t, source, filePath)
	edit := srv.renameFromSpans(context.Background(), docURI, protocol.Position{Line: 1, Character: 3}, "spaces")
	assertRenameEdits(t, edit, filePath, []renameWant{{1, 2, 7}, {5, 20, 25}})
}

type renameWant struct {
	line  uint32
	start uint32
	end   uint32
}

func assertRenameEdits(t *testing.T, edit *protocol.WorkspaceEdit, filePath string, wants []renameWant) {
	t.Helper()
	if edit == nil {
		t.Fatalf("expected workspace edit, got nil")
	}
	edits := edit.Changes[uri.File(filePath)]
	if len(edits) != len(wants) {
		t.Fatalf("got %d edits for %s, want %d: %#v", len(edits), filePath, len(wants), edits)
	}
	for i, want := range wants {
		got := edits[i].Range
		if got.Start.Line != want.line || got.End.Line != want.line || got.Start.Character != want.start || got.End.Character != want.end {
			t.Fatalf("edit[%d] range = %#v, want line %d chars %d-%d", i, got, want.line, want.start, want.end)
		}
	}
}

// TestHoverOnMutRefBinding pins that reference bindings hover with their
// reference identity (`mut Int`), even though reads through them produce the
// referent (ADR 0045).
func TestHoverOnMutRefBinding(t *testing.T) {
	source := `mut counter = 0
let r = mut counter
let snapshot: Int = r
` + "\n"

	tests := []struct {
		name    string
		line    uint32
		char    uint32
		want    string
		notWant string
	}{
		{
			name: "hover on reference binding definition",
			line: 1,
			char: 4,
			want: "mut Int",
		},
		{
			name: "hover on reference binding use",
			line: 2,
			char: 20,
			want: "mut Int",
		},
		{
			name:    "hover on plain mutable binding stays plain",
			line:    0,
			char:    4,
			want:    "Int",
			notWant: "mut",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := spanHoverAt(t, source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil at %d:%d", tt.line, tt.char)
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
			if tt.notWant != "" && strings.Contains(info.content, tt.notWant) {
				t.Errorf("hover content = %q, want without %q", info.content, tt.notWant)
			}
		})
	}
}
