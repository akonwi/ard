package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// stdio wraps os.Stdin and os.Stdout into a single io.ReadWriteCloser.
type stdio struct {
	io.ReadCloser
	io.WriteCloser
}

func (s *stdio) Close() error {
	err1 := s.ReadCloser.Close()
	err2 := s.WriteCloser.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// Server is the Ard LSP server.
type Server struct {
	cache       *DocumentCache
	mu          sync.Mutex
	handlers    map[string]jsonrpc2.Handler
	conn        jsonrpc2.Conn
	projectRoot string

	diagnosticsMu        sync.Mutex
	diagnosticsTimers    map[uri.URI]*time.Timer
	diagnosticsDelay     time.Duration
	diagnosticsPublisher func(context.Context, uri.URI)
}

// NewServer creates a new Ard LSP server.
func NewServer() *Server {
	s := &Server{
		cache:                NewDocumentCache(),
		handlers:             make(map[string]jsonrpc2.Handler),
		diagnosticsTimers:    make(map[uri.URI]*time.Timer),
		diagnosticsDelay:     100 * time.Millisecond,
		diagnosticsPublisher: nil,
	}
	s.diagnosticsPublisher = s.publishDiagnostics
	s.registerHandlers()
	return s
}

// Run starts the LSP server on stdin/stdout and blocks until the connection closes.
func (s *Server) Run(ctx context.Context) error {
	rwc := &stdio{
		ReadCloser:  os.Stdin,
		WriteCloser: os.Stdout,
	}
	stream := jsonrpc2.NewStream(rwc)
	conn := jsonrpc2.NewConn(stream)
	s.conn = conn

	handler := protocol.Handlers(
		jsonrpc2.Handler(s.dispatch),
	)

	conn.Go(ctx, handler)
	<-conn.Done()
	return conn.Err()
}

// dispatch routes incoming LSP requests and notifications to registered handlers.
func (s *Server) dispatch(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) (err error) {
	method := req.Method()
	replied := false
	safeReply := func(ctx context.Context, result interface{}, replyErr error) error {
		replied = true
		return reply(ctx, result, replyErr)
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "ard-lsp panic handling %s: %v\n%s", method, r, debug.Stack())
			if !replied {
				err = safeReply(ctx, nil, fmt.Errorf("internal server error handling %s: %v", method, r))
				return
			}
			err = fmt.Errorf("internal server error after reply handling %s: %v", method, r)
		}
	}()

	// LSP clients may send feature requests concurrently. The current hover,
	// definition, references, and signature-help paths share parse/type-resolution
	// caches, so serialize handlers until those caches are request-scoped.
	s.mu.Lock()
	defer s.mu.Unlock()

	if handler, ok := s.handlers[method]; ok {
		return handler(ctx, safeReply, req)
	}

	return jsonrpc2.MethodNotFoundHandler(ctx, safeReply, req)
}

// registerHandlers registers all LSP method handlers.
func (s *Server) registerHandlers() {
	// Lifecycle
	s.handlers[protocol.MethodInitialize] = s.handleInitialize
	s.handlers[protocol.MethodInitialized] = s.handleInitialized
	s.handlers[protocol.MethodShutdown] = s.handleShutdown
	s.handlers[protocol.MethodExit] = s.handleExit

	// Document synchronization
	s.handlers[protocol.MethodTextDocumentDidOpen] = s.handleDidOpen
	s.handlers[protocol.MethodTextDocumentDidChange] = s.handleDidChange
	s.handlers[protocol.MethodTextDocumentDidSave] = s.handleDidSave
	s.handlers[protocol.MethodTextDocumentDidClose] = s.handleDidClose

	// Language features
	s.handlers[protocol.MethodTextDocumentHover] = s.handleHover
	s.handlers[protocol.MethodTextDocumentDefinition] = s.handleDefinition
	s.handlers[protocol.MethodTextDocumentReferences] = s.handleReferences
	s.handlers[protocol.MethodTextDocumentDocumentSymbol] = s.handleDocumentSymbol
	s.handlers[protocol.MethodTextDocumentCompletion] = s.handleCompletion
	s.handlers[protocol.MethodTextDocumentFormatting] = s.handleFormatting
	s.handlers[protocol.MethodTextDocumentCodeAction] = s.handleCodeAction
	s.handlers[protocol.MethodTextDocumentSignatureHelp] = s.handleSignatureHelp
	s.handlers[protocol.MethodTextDocumentDocumentHighlight] = s.handleDocumentHighlight
	s.handlers[protocol.MethodTextDocumentRename] = s.handleRename
	s.handlers[protocol.MethodTextDocumentPrepareRename] = s.handlePrepareRename
}

//-------------------------------------------------------------------------
// Lifecycle handlers
//-------------------------------------------------------------------------

func (s *Server) handleInitialize(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.InitializeParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	// Detect project root from workspace folders or root URI
	if len(params.WorkspaceFolders) > 0 {
		s.projectRoot = string(params.WorkspaceFolders[0].URI)
	} else if params.RootURI != "" {
		s.projectRoot = string(params.RootURI)
	}

	result := &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync:       protocol.TextDocumentSyncKindFull,
			HoverProvider:          true,
			DefinitionProvider:     true,
			ReferencesProvider:     true,
			DocumentSymbolProvider: true,
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{".", ":", "/"},
			},
			SignatureHelpProvider: &protocol.SignatureHelpOptions{
				TriggerCharacters:   []string{"(", ",", ":"},
				RetriggerCharacters: []string{",", ":"},
			},
			DocumentHighlightProvider:  true,
			DocumentFormattingProvider: true,
			CodeActionProvider:         true,
			RenameProvider:             &protocol.RenameOptions{PrepareProvider: true},
		},
		ServerInfo: &protocol.ServerInfo{
			Name:    "ard-lsp",
			Version: "0.1.0",
		},
	}

	return reply(ctx, result, nil)
}

func (s *Server) handleInitialized(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	return reply(ctx, nil, nil)
}

func (s *Server) handleShutdown(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	return reply(ctx, nil, nil)
}

func (s *Server) handleExit(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	return reply(ctx, nil, nil)
}

func (s *Server) scheduleDiagnostics(docURI uri.URI) {
	if s.diagnosticsDelay <= 0 {
		go s.runDiagnostics(docURI)
		return
	}

	s.diagnosticsMu.Lock()
	defer s.diagnosticsMu.Unlock()
	if timer := s.diagnosticsTimers[docURI]; timer != nil {
		timer.Stop()
	}
	s.diagnosticsTimers[docURI] = time.AfterFunc(s.diagnosticsDelay, func() {
		s.diagnosticsMu.Lock()
		delete(s.diagnosticsTimers, docURI)
		s.diagnosticsMu.Unlock()
		s.runDiagnostics(docURI)
	})
}

func (s *Server) runDiagnostics(docURI uri.URI) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "ard-lsp panic publishing diagnostics for %s: %v\n%s", docURI, r, debug.Stack())
		}
	}()
	publisher := s.diagnosticsPublisher
	if publisher == nil {
		publisher = s.publishDiagnostics
	}
	publisher(context.Background(), docURI)
}

//-------------------------------------------------------------------------
// Document sync handlers
//-------------------------------------------------------------------------

func (s *Server) handleDidOpen(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidOpenTextDocumentParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	s.cache.Open(
		params.TextDocument.URI,
		string(params.TextDocument.LanguageID),
		params.TextDocument.Version,
		params.TextDocument.Text,
	)
	s.scheduleDiagnostics(params.TextDocument.URI)

	return reply(ctx, nil, nil)
}

func (s *Server) handleDidChange(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidChangeTextDocumentParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	if len(params.ContentChanges) > 0 {
		change := params.ContentChanges[len(params.ContentChanges)-1]
		s.cache.Update(params.TextDocument.URI, params.TextDocument.Version, change.Text)
	}

	s.scheduleDiagnostics(params.TextDocument.URI)

	return reply(ctx, nil, nil)
}

func (s *Server) handleDidSave(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	return reply(ctx, nil, nil)
}

func (s *Server) handleDidClose(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidCloseTextDocumentParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	s.cache.Close(params.TextDocument.URI)
	s.scheduleDiagnostics(params.TextDocument.URI)

	return reply(ctx, nil, nil)
}

//-------------------------------------------------------------------------
// Language feature handlers (stubs)
//-------------------------------------------------------------------------

func (s *Server) handleHover(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.HoverParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	doc := s.cache.Get(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, nil, nil)
	}

	// Recover from panics in hover computation so the LSP server stays alive.
	var info *hoverInfo
	func() {
		defer func() {
			if r := recover(); r != nil {
				info = nil
			}
		}()
		info = computeHover(doc.Text, doc.URI.Filename(), params.Position)
	}()

	if info == nil || info.content == "" {
		return reply(ctx, nil, nil)
	}

	result := &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: info.content,
		},
	}

	return reply(ctx, result, nil)
}

func (s *Server) handleDefinition(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DefinitionParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	doc := s.cache.Get(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, []protocol.Location{}, nil)
	}

	var locations []protocol.Location
	func() {
		defer func() {
			if r := recover(); r != nil {
				locations = []protocol.Location{}
			}
		}()
		locations = computeDefinition(doc.Text, doc.URI.Filename(), params.Position)
	}()
	if locations == nil {
		locations = []protocol.Location{}
	}

	return reply(ctx, locations, nil)
}

func (s *Server) handleReferences(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.ReferenceParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	doc := s.cache.Get(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, []protocol.Location{}, nil)
	}

	var locations []protocol.Location
	func() {
		defer func() {
			if r := recover(); r != nil {
				locations = []protocol.Location{}
			}
		}()
		overlays := map[string]string{}
		for _, cached := range s.cache.Snapshot() {
			overlays[cached.URI.Filename()] = cached.Text
		}
		locations = computeReferencesWithOverlays(doc.Text, doc.URI.Filename(), params.Position, params.Context.IncludeDeclaration, overlays)
	}()
	if locations == nil {
		locations = []protocol.Location{}
	}

	return reply(ctx, locations, nil)
}

func (s *Server) handleDocumentSymbol(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DocumentSymbolParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}
	_ = params

	return reply(ctx, []protocol.SymbolInformation{}, nil)
}

func (s *Server) handleCompletion(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.CompletionParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	doc := s.cache.Get(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, &protocol.CompletionList{IsIncomplete: false, Items: []protocol.CompletionItem{}}, nil)
	}

	items := []protocol.CompletionItem{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				items = []protocol.CompletionItem{}
			}
		}()
		items = computeCompletions(doc.Text, doc.URI.Filename(), params.Position)
	}()
	if items == nil {
		items = []protocol.CompletionItem{}
	}

	return reply(ctx, &protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil)
}

func fullDocumentEdit(oldText string, newText string) protocol.TextEdit {
	lines := strings.Count(oldText, "\n")
	endChar := 0
	if lastLineStart := strings.LastIndex(oldText, "\n"); lastLineStart >= 0 {
		endChar = len(oldText) - lastLineStart - 1
	} else {
		endChar = len(oldText)
	}
	return protocol.TextEdit{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: uint32(lines), Character: uint32(endChar)},
		},
		NewText: newText,
	}
}

func (s *Server) handleFormatting(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DocumentFormattingParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	doc := s.cache.Get(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, []protocol.TextEdit{}, nil)
	}

	filePath := doc.URI.Filename()
	formatted, err := formatSource(doc.Text, filePath)
	if err != nil || formatted == doc.Text {
		return reply(ctx, []protocol.TextEdit{}, nil)
	}

	return reply(ctx, []protocol.TextEdit{fullDocumentEdit(doc.Text, string(formatted))}, nil)
}

func (s *Server) handleCodeAction(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.CodeActionParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	if len(params.Context.Only) > 0 {
		allowed := false
		for _, kind := range params.Context.Only {
			if kind == protocol.Source || kind == protocol.SourceOrganizeImports {
				allowed = true
				break
			}
		}
		if !allowed {
			return reply(ctx, []protocol.CodeAction{}, nil)
		}
	}

	doc := s.cache.Get(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, []protocol.CodeAction{}, nil)
	}
	formatted, err := formatSource(doc.Text, doc.URI.Filename())
	if err != nil || formatted == doc.Text {
		return reply(ctx, []protocol.CodeAction{}, nil)
	}

	action := protocol.CodeAction{
		Title:       "Remove unused imports",
		Kind:        protocol.SourceOrganizeImports,
		IsPreferred: true,
		Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			params.TextDocument.URI: {fullDocumentEdit(doc.Text, string(formatted))},
		}},
	}
	return reply(ctx, []protocol.CodeAction{action}, nil)
}

func (s *Server) handleSignatureHelp(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.SignatureHelpParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	doc := s.cache.Get(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, nil, nil)
	}

	var help *protocol.SignatureHelp
	func() {
		defer func() {
			if r := recover(); r != nil {
				help = nil
			}
		}()
		help = computeSignatureHelp(doc.Text, doc.URI.Filename(), params.Position)
	}()

	return reply(ctx, help, nil)
}

func (s *Server) handleDocumentHighlight(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DocumentHighlightParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	doc := s.cache.Get(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, []protocol.DocumentHighlight{}, nil)
	}

	highlights := computeDocumentHighlights(doc.Text, doc.URI.Filename(), params.Position)
	if highlights == nil {
		highlights = []protocol.DocumentHighlight{}
	}
	return reply(ctx, highlights, nil)
}

func (s *Server) handlePrepareRename(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.PrepareRenameParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}
	doc := s.cache.Get(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, nil, nil)
	}
	rng := prepareRename(doc.Text, doc.URI.Filename(), params.Position)
	return reply(ctx, rng, nil)
}

func (s *Server) handleRename(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.RenameParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}
	doc := s.cache.Get(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, nil, nil)
	}
	overlays := map[string]string{}
	for _, cached := range s.cache.Snapshot() {
		overlays[cached.URI.Filename()] = cached.Text
	}
	edit := computeRename(doc.Text, doc.URI.Filename(), params.Position, params.NewName, overlays)
	return reply(ctx, edit, nil)
}
