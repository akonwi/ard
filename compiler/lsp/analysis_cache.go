package lsp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

type parseCacheEntry struct {
	sourceHash string
	program    *parse.Program
	errors     []parse.ParseError
}

type diagnosticCacheEntry struct {
	sourceHash    string
	overlayHash   string
	workspaceHash string
	diagnostics   []checker.Diagnostic
	err           error
}

// AnalysisCache stores parsed and checked LSP analysis results keyed by content.
type AnalysisCache struct {
	mu          sync.Mutex
	parsed      map[string]parseCacheEntry
	diagnostics map[string]diagnosticCacheEntry
	checkCount  int
}

func NewAnalysisCache() *AnalysisCache {
	return &AnalysisCache{
		parsed:      make(map[string]parseCacheEntry),
		diagnostics: make(map[string]diagnosticCacheEntry),
	}
}

func (c *AnalysisCache) Parse(source string, filePath string) parseCacheEntry {
	if c == nil {
		return parseSource(source, filePath)
	}

	sourceHash := contentHash(source)
	c.mu.Lock()
	if entry, ok := c.parsed[filePath]; ok && entry.sourceHash == sourceHash {
		c.mu.Unlock()
		return entry
	}
	c.mu.Unlock()

	entry := parseSource(source, filePath)
	entry.sourceHash = sourceHash

	c.mu.Lock()
	c.parsed[filePath] = entry
	c.mu.Unlock()
	return entry
}

func (c *AnalysisCache) Program(source string, filePath string) *parse.Program {
	return c.Parse(source, filePath).program
}

func (c *AnalysisCache) Diagnostics(source string, filePath string, overlays map[string]string) ([]checker.Diagnostic, error) {
	if c == nil {
		return analyzeDiagnosticsUncached(source, filePath, overlays)
	}

	sourceHash := contentHash(source)
	overlayHash := overlaysHash(overlays)
	workspaceHash := workspaceFingerprint(filePath)
	key := filePath

	c.mu.Lock()
	if entry, ok := c.diagnostics[key]; ok &&
		entry.sourceHash == sourceHash &&
		entry.overlayHash == overlayHash &&
		entry.workspaceHash == workspaceHash {
		c.mu.Unlock()
		return cloneDiagnostics(entry.diagnostics), entry.err
	}
	c.mu.Unlock()

	diagnostics, err := c.analyzeDiagnostics(source, filePath, overlays)

	c.mu.Lock()
	c.diagnostics[key] = diagnosticCacheEntry{
		sourceHash:    sourceHash,
		overlayHash:   overlayHash,
		workspaceHash: workspaceHash,
		diagnostics:   cloneDiagnostics(diagnostics),
		err:           err,
	}
	c.mu.Unlock()

	return diagnostics, err
}

func (c *AnalysisCache) Invalidate(filePath string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.parsed, filePath)
	delete(c.diagnostics, filePath)
}

func (c *AnalysisCache) InvalidateAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.parsed = make(map[string]parseCacheEntry)
	c.diagnostics = make(map[string]diagnosticCacheEntry)
}

func (c *AnalysisCache) CheckCount() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.checkCount
}

func (c *AnalysisCache) analyzeDiagnostics(source string, filePath string, overlays map[string]string) ([]checker.Diagnostic, error) {
	diagnostics, err := c.analyzeDiagnosticsUncached(source, filePath, overlays)
	c.mu.Lock()
	c.checkCount++
	c.mu.Unlock()
	return diagnostics, err
}

func (c *AnalysisCache) analyzeDiagnosticsUncached(source string, filePath string, overlays map[string]string) ([]checker.Diagnostic, error) {
	result := c.Parse(source, filePath)
	return diagnosticsFromParseResult(result, filePath, overlays)
}

func analyzeDiagnosticsUncached(source string, filePath string, overlays map[string]string) ([]checker.Diagnostic, error) {
	result := parseSource(source, filePath)
	return diagnosticsFromParseResult(result, filePath, overlays)
}

func diagnosticsFromParseResult(result parseCacheEntry, filePath string, overlays map[string]string) ([]checker.Diagnostic, error) {
	if result.program == nil {
		return nil, fmt.Errorf("failed to parse: no program returned")
	}
	if len(result.errors) > 0 {
		diags := make([]checker.Diagnostic, 0, len(result.errors))
		for _, err := range result.errors {
			diags = append(diags, checker.NewDiagnostic(checker.Error, err.Message, filePath, err.Location))
		}
		return diags, nil
	}

	workingDir := filepath.Dir(filePath)
	moduleResolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		return nil, fmt.Errorf("error initializing module resolver: %w", err)
	}
	for overlayPath, overlaySource := range overlays {
		moduleResolver.SetOverlay(overlayPath, overlaySource)
	}

	relPath, err := filepath.Rel(workingDir, filePath)
	if err != nil {
		relPath = filePath
	}

	c := checker.New(relPath, result.program, moduleResolver, checker.CheckOptions{})
	c.Check()
	return c.Diagnostics(), nil
}

func parseSource(source string, filePath string) parseCacheEntry {
	result := parse.Parse([]byte(source), filePath)
	return parseCacheEntry{
		program: result.Program,
		errors:  append([]parse.ParseError(nil), result.Errors...),
	}
}

func contentHash(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func overlaysHash(overlays map[string]string) string {
	if len(overlays) == 0 {
		return ""
	}
	paths := make([]string, 0, len(overlays))
	for path := range overlays {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, path := range paths {
		h.Write([]byte(path))
		h.Write([]byte{0})
		h.Write([]byte(overlays[path]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func workspaceFingerprint(filePath string) string {
	root := workspaceRoot(filepath.Dir(filePath))
	h := sha256.New()
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if filepath.Ext(path) != ".ard" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func workspaceRoot(start string) string {
	dir := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(dir, "ard.toml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Clean(start)
		}
		dir = parent
	}
}

func cloneDiagnostics(diagnostics []checker.Diagnostic) []checker.Diagnostic {
	if diagnostics == nil {
		return nil
	}
	return append([]checker.Diagnostic(nil), diagnostics...)
}

var defaultAnalysisCache = NewAnalysisCache()
