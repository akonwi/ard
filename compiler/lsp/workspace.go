package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/lsp/analysis"
	"go.lsp.dev/uri"
)

// workspaceFor returns the shared analysis workspace, creating the engine on
// first use. The project root comes from initialize when available, otherwise
// it is discovered from the first document's directory.
func (s *Server) workspaceFor(filePath string) *analysis.Workspace {
	s.engineMu.Lock()
	defer s.engineMu.Unlock()
	if s.workspace != nil {
		return s.workspace
	}

	root := s.projectRootPath()
	if root == "" && filePath != "" {
		if info, err := checker.FindProjectRoot(filepath.Dir(filePath)); err == nil {
			root = info.RootPath
		} else {
			root = filepath.Dir(filePath)
		}
	}
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		}
	}
	s.engine = analysis.NewEngine(root)
	s.workspace = analysis.NewWorkspace(s.engine)
	return s.workspace
}

// projectRootPath converts the initialize-provided root URI to a file path.
func (s *Server) projectRootPath() string {
	if s.projectRoot == "" {
		return ""
	}
	path, err := filePathFromURI(uri.URI(s.projectRoot))
	if err != nil {
		return ""
	}
	return path
}

// syncOverlay mirrors document content into the analysis workspace.
func (s *Server) syncOverlay(docURI uri.URI, text string) {
	filePath, err := filePathFromURI(docURI)
	if err != nil {
		return
	}
	s.workspaceFor(filePath).SetOverlay(filePath, text)
}

// dropOverlay removes a closed document from the analysis workspace.
func (s *Server) dropOverlay(docURI uri.URI) {
	filePath, err := filePathFromURI(docURI)
	if err != nil {
		return
	}
	s.workspaceFor(filePath).DeleteOverlay(filePath)
}

// analyzeSnapshot analyzes the document against the current snapshot. Open
// document contents are synced from the document cache first, so the cache
// remains the single source of truth for editor state. The context cancels
// between analysis stages (watchdog and superseded requests).
func (s *Server) analyzeSnapshot(ctx context.Context, docURI uri.URI) (*analysis.FileAnalysis, error) {
	filePath, err := filePathFromURI(docURI)
	if err != nil {
		return nil, err
	}
	ws := s.workspaceFor(filePath)
	// The document cache is authoritative: sync the full overlay set so
	// closed documents are removed even if a didClose raced an earlier sync.
	overlays := map[string]string{}
	for _, doc := range s.cache.Snapshot() {
		if p, err := filePathFromURI(doc.URI); err == nil {
			overlays[p] = doc.Text
		}
	}
	ws.SyncOverlays(overlays)
	snap := ws.Snapshot()
	fa, err := snap.AnalyzeCtx(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("analyze %s: %w", filePath, err)
	}
	return fa, nil
}
