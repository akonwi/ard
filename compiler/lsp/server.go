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

	"github.com/akonwi/ard/lsp/analysis"
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
	handlers    map[string]jsonrpc2.Handler
	conn        jsonrpc2.Conn
	projectRoot string

	engineMu  sync.Mutex
	engine    *analysis.Engine
	workspace *analysis.Workspace

	// requestTimeout bounds any single feature request so a runaway analysis
	// cannot hang the editor. Zero disables the watchdog (tests).
	requestTimeout time.Duration

	diagnosticsMu        sync.Mutex
	diagnosticsTimers    map[uri.URI]*time.Timer
	diagnosticsDelay     time.Duration
	diagnosticsPublisher func(context.Context, uri.URI)
	diagnosticsAnalyzer  diagnosticAnalyzer
}

// NewServer creates a new Ard LSP server.
func NewServer() *Server {
	s := &Server{
		cache:                NewDocumentCache(),
		handlers:             make(map[string]jsonrpc2.Handler),
		diagnosticsTimers:    make(map[uri.URI]*time.Timer),
		diagnosticsDelay:     100 * time.Millisecond,
		requestTimeout:       5 * time.Second,
		diagnosticsPublisher: nil,
		diagnosticsAnalyzer:  nil,
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

	conn.Go(ctx, s.jsonRPCHandler())
	<-conn.Done()
	return conn.Err()
}

func (s *Server) jsonRPCHandler() jsonrpc2.Handler {
	return protocol.CancelHandler(
		s.concurrentRequestHandler(
			jsonrpc2.ReplyHandler(jsonrpc2.Handler(s.dispatch)),
		),
	)
}

// concurrentRequestHandler runs feature requests concurrently while preserving
// lifecycle and document-sync ordering on the reader goroutine. Every spawned
// request is panic-guarded and watchdog-bounded: a panic or deadline yields an
// LSP error reply instead of a dead server or a hung editor.
func (s *Server) concurrentRequestHandler(handler jsonrpc2.Handler) jsonrpc2.Handler {
	return func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		if handleRequestInline(req.Method()) {
			return handler(ctx, reply, req)
		}

		go func() {
			var once sync.Once
			guardedReply := func(ctx context.Context, result interface{}, err error) error {
				var replyErr error
				once.Do(func() { replyErr = reply(ctx, result, err) })
				return replyErr
			}

			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "ard-lsp panic in %s: %v\n%s", req.Method(), r, debug.Stack())
					_ = guardedReply(ctx, nil, fmt.Errorf("internal server error handling %s", req.Method()))
				}
			}()

			if s.requestTimeout <= 0 {
				_ = handler(ctx, guardedReply, req)
				return
			}

			reqCtx, cancel := context.WithTimeout(ctx, s.requestTimeout)
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Fprintf(os.Stderr, "ard-lsp panic in %s: %v\n%s", req.Method(), r, debug.Stack())
						_ = guardedReply(ctx, nil, fmt.Errorf("internal server error handling %s", req.Method()))
					}
					close(done)
				}()
				_ = handler(reqCtx, guardedReply, req)
			}()

			select {
			case <-done:
			case <-reqCtx.Done():
				if ctx.Err() != nil {
					// The client cancelled the request; reply with the LSP
					// cancellation error rather than a fake timeout.
					_ = guardedReply(ctx, nil, protocol.ErrRequestCancelled)
					return
				}
				fmt.Fprintf(os.Stderr, "ard-lsp watchdog: %s exceeded %s\n", req.Method(), s.requestTimeout)
				_ = guardedReply(ctx, nil, fmt.Errorf("%s timed out after %s", req.Method(), s.requestTimeout))
			}
		}()
		return nil
	}
}

func handleRequestInline(method string) bool {
	switch method {
	case protocol.MethodInitialize,
		protocol.MethodInitialized,
		protocol.MethodShutdown,
		protocol.MethodExit,
		protocol.MethodTextDocumentDidOpen,
		protocol.MethodTextDocumentDidChange,
		protocol.MethodTextDocumentDidSave,
		protocol.MethodTextDocumentDidClose:
		return true
	default:
		return false
	}
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

	// Detect project root from workspace folders or root URI. Guarded by
	// engineMu because feature goroutines read it through workspaceFor.
	s.engineMu.Lock()
	if len(params.WorkspaceFolders) > 0 {
		s.projectRoot = string(params.WorkspaceFolders[0].URI)
	} else if params.RootURI != "" {
		s.projectRoot = string(params.RootURI)
	}
	s.engineMu.Unlock()

	result := &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync:       protocol.TextDocumentSyncKindIncremental,
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

func (s *Server) scheduleDiagnosticsForOpenDocuments() {
	for _, doc := range s.cache.Snapshot() {
		s.scheduleDiagnostics(doc.URI)
	}
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

type didChangeParams struct {
	TextDocument   protocol.VersionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []documentContentChange                  `json:"contentChanges"`
}

type documentContentChange struct {
	Range       *protocol.Range `json:"range,omitempty"`
	RangeLength *uint32         `json:"rangeLength,omitempty"`
	Text        string          `json:"text"`
}

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
	s.syncOverlay(params.TextDocument.URI, params.TextDocument.Text)
	s.scheduleDiagnosticsForOpenDocuments()

	return reply(ctx, nil, nil)
}

func (s *Server) handleDidChange(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params didChangeParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, fmt.Errorf("%s: %w", jsonrpc2.ErrParse, err))
	}

	if len(params.ContentChanges) > 0 {
		doc := s.cache.Get(params.TextDocument.URI)
		if doc != nil {
			updated, err := applyDocumentChanges(doc.Text, params.ContentChanges)
			if err != nil {
				return reply(ctx, nil, fmt.Errorf("invalid document change: %w", err))
			}
			s.cache.Update(params.TextDocument.URI, params.TextDocument.Version, updated)
			s.syncOverlay(params.TextDocument.URI, updated)
		}
	}

	s.scheduleDiagnosticsForOpenDocuments()

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
	s.dropOverlay(params.TextDocument.URI)
	s.scheduleDiagnostics(params.TextDocument.URI)
	s.scheduleDiagnosticsForOpenDocuments()

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
		filePath, ok := docFilePath(doc)
		if !ok {
			return
		}
		info = computeHover(doc.Text, filePath, params.Position)
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
		filePath, ok := docFilePath(doc)
		if !ok {
			return
		}
		locations = computeDefinition(doc.Text, filePath, params.Position)
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
		filePath, ok := docFilePath(doc)
		if !ok {
			return
		}
		overlays := overlaySources(s.cache.Snapshot())
		locations = computeReferencesWithOverlays(doc.Text, filePath, params.Position, params.Context.IncludeDeclaration, overlays)
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
		filePath, ok := docFilePath(doc)
		if !ok {
			return
		}
		items = computeCompletions(doc.Text, filePath, params.Position)
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

	filePath, ok := docFilePath(doc)
	if !ok {
		return reply(ctx, []protocol.TextEdit{}, nil)
	}
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
	filePath, ok := docFilePath(doc)
	if !ok {
		return reply(ctx, []protocol.CodeAction{}, nil)
	}
	formatted, err := formatSource(doc.Text, filePath)
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
		filePath, ok := docFilePath(doc)
		if !ok {
			return
		}
		help = computeSignatureHelp(doc.Text, filePath, params.Position)
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

	filePath, ok := docFilePath(doc)
	if !ok {
		return reply(ctx, []protocol.DocumentHighlight{}, nil)
	}
	highlights := computeDocumentHighlights(doc.Text, filePath, params.Position)
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
	filePath, ok := docFilePath(doc)
	if !ok {
		return reply(ctx, nil, nil)
	}
	rng := prepareRename(doc.Text, filePath, params.Position)
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
	filePath, ok := docFilePath(doc)
	if !ok {
		return reply(ctx, nil, nil)
	}
	overlays := overlaySources(s.cache.Snapshot())
	edit := computeRename(doc.Text, filePath, params.Position, params.NewName, overlays)
	return reply(ctx, edit, nil)
}
