// Package analysis is the LSP's snapshot-based analysis engine
// (docs/adrs/0043-rebuild-lsp-on-snapshot-analysis.md).
//
// A Workspace owns open-document overlays. Each change produces a new
// immutable Snapshot; feature requests run against a snapshot and never
// re-parse or re-check what a previous snapshot already computed. Memoization
// is content-addressed:
//
//   - parse results are keyed by file content hash
//   - check results are keyed by the content signature of the file's import
//     closure, so editing one module only re-checks its dependents
//   - Go package metadata lives in one shared, mutex-guarded resolver per
//     project, invalidated only when go.mod/go.sum change
package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// Engine holds project-wide shared analysis state. It is safe for concurrent
// use by multiple snapshots.
type Engine struct {
	projectRoot string

	mu         sync.Mutex
	goResolver *lockedGoResolver
	goModHash  string
	goPrimed   map[string]bool        // go import paths primed into the session
	parseCache map[string]*parseEntry // content hash -> parse result
	checkCache map[string]*FileAnalysis
	// insertion order for simple bounded eviction
	parseOrder []string
	checkOrder []string
}

const (
	maxParseEntries = 256
	maxCheckEntries = 64
)

// parseEntry caches one parse result. The *parse.Program is shared across
// concurrent checks and snapshots; the checker must never mutate the parse
// tree (it builds its own checked program). This is a load-bearing invariant
// of the cache.
type parseEntry struct {
	program *parse.Program
	errors  []parse.ParseError
}

// FileAnalysis is the immutable result of analyzing one file. It retains
// only what features consume — not the whole Checker — so the bounded cache
// stays small.
type FileAnalysis struct {
	FilePath    string
	Program     *parse.Program
	ParseErrors []parse.ParseError
	Diagnostics []checker.Diagnostic
	Spans       *checker.SpanIndex
	// Checked is the checker's output program: imports, struct-method side
	// tables, and public symbols. Like Program, it is shared across snapshot
	// consumers and must be treated as read-only.
	Checked *checker.Program
	// Module is the checked module view exposing public symbols (functions,
	// types, enums, module values) by name. Read-only.
	Module checker.Module
	// Signature identifies the inputs this analysis was computed from.
	Signature string
}

// NewEngine creates an engine rooted at the given project directory.
func NewEngine(projectRoot string) *Engine {
	return &Engine{
		projectRoot: projectRoot,
		parseCache:  map[string]*parseEntry{},
		checkCache:  map[string]*FileAnalysis{},
	}
}

// ProjectRoot returns the engine's project root directory.
func (e *Engine) ProjectRoot() string { return e.projectRoot }

// --- shared Go resolver ---

// lockedGoResolver serializes access to the underlying GoPackagesResolver,
// which memoizes go/packages loads but is not goroutine-safe.
type lockedGoResolver struct {
	mu    sync.Mutex
	inner *checker.GoPackagesResolver
}

func (r *lockedGoResolver) ResolveGoPackage(path string) (*checker.GoPackage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inner.ResolveGoPackage(path)
}

// goResolverFor returns the shared primed Go resolver session (ADR 0044 A3).
// The session is one go/packages load covering every Go import path the
// workspace has needed, so all Go types within a check share one go/types
// universe. It is rebuilt when go.mod/go.sum change or when a check needs
// paths the session has not primed; rebuilds prime the union of previous and
// newly needed paths so other files' packages stay in the same session.
func (e *Engine) goResolverFor(projectInfo *checker.ProjectInfo, goPaths []string) checker.GoPackageResolver {
	sig := e.goModSignature()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.goResolver != nil && e.goModHash == sig {
		covered := true
		for _, path := range goPaths {
			if !e.goPrimed[path] {
				covered = false
				break
			}
		}
		if covered {
			return e.goResolver
		}
	}
	root := e.projectRoot
	var tags []string
	if projectInfo != nil {
		root = projectInfo.RootPath
		tags = projectInfo.Go.BuildTags
	}
	primed := map[string]bool{}
	if e.goModHash == sig {
		// Carry previously primed paths into the new session so the rest of
		// the workspace keeps resolving from the same universe. Paths that
		// fail to load only cost a per-path error entry.
		for path := range e.goPrimed {
			primed[path] = true
		}
	}
	for _, path := range goPaths {
		primed[path] = true
	}
	union := make([]string, 0, len(primed))
	for path := range primed {
		union = append(union, path)
	}
	sort.Strings(union)
	resolver := checker.NewGoPackagesResolver(root, tags)
	if err := resolver.Prime(union); err != nil {
		// A load-level failure (for example an unreadable go.mod) degrades
		// to the lazy per-path resolver so diagnostics surface at each Go
		// import instead of failing analysis wholesale.
		resolver = checker.NewGoPackagesResolver(root, tags)
		primed = map[string]bool{}
	}
	e.goResolver = &lockedGoResolver{inner: resolver}
	e.goModHash = sig
	e.goPrimed = primed
	return e.goResolver
}

// manifestSignature hashes the project manifest and lockfile so dependency
// configuration changes invalidate cached checks.
func (e *Engine) manifestSignature() string {
	h := sha256.New()
	for _, name := range []string{"ard.toml", "ard.lock"} {
		data, err := os.ReadFile(filepath.Join(e.projectRoot, name))
		if err == nil {
			h.Write(data)
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

func (e *Engine) goModSignature() string {
	h := sha256.New()
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(e.projectRoot, name))
		if err == nil {
			h.Write(data)
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// --- memoized parsing ---

// parseFile parses source, memoized by content hash.
func (e *Engine) parseFile(content []byte, filePath string) *parseEntry {
	key := hashBytes(content)
	e.mu.Lock()
	if entry, ok := e.parseCache[key]; ok {
		e.mu.Unlock()
		return entry
	}
	e.mu.Unlock()

	result := parse.Parse(content, filePath)
	entry := &parseEntry{program: result.Program, errors: result.Errors}

	e.mu.Lock()
	if _, ok := e.parseCache[key]; !ok {
		e.parseCache[key] = entry
		e.parseOrder = append(e.parseOrder, key)
		if len(e.parseOrder) > maxParseEntries {
			evict := e.parseOrder[0]
			e.parseOrder = e.parseOrder[1:]
			delete(e.parseCache, evict)
		}
	}
	e.mu.Unlock()
	return entry
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16])
}

// --- snapshots ---

// Workspace owns the mutable open-document state and produces snapshots.
type Workspace struct {
	engine *Engine

	mu       sync.Mutex
	overlays map[string]string // absolute file path -> content
	revision uint64
}

// NewWorkspace creates a workspace over the engine.
func NewWorkspace(engine *Engine) *Workspace {
	return &Workspace{
		engine:   engine,
		overlays: map[string]string{},
	}
}

// SetOverlay records unsaved editor content for a file and bumps the
// revision. Setting identical content is a no-op so callers may sync
// overlays idempotently.
func (w *Workspace) SetOverlay(filePath string, content string) uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if existing, ok := w.overlays[filePath]; ok && existing == content {
		return w.revision
	}
	w.overlays[filePath] = content
	w.revision++
	return w.revision
}

// SyncOverlays replaces the overlay set atomically: files present in the map
// are set, files absent are removed. The revision only bumps when content
// actually changed. This lets the server make its document cache
// authoritative and heal races between doc-sync and feature requests.
func (w *Workspace) SyncOverlays(overlays map[string]string) uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	changed := false
	for path, content := range overlays {
		if existing, ok := w.overlays[path]; !ok || existing != content {
			w.overlays[path] = content
			changed = true
		}
	}
	for path := range w.overlays {
		if _, ok := overlays[path]; !ok {
			delete(w.overlays, path)
			changed = true
		}
	}
	if changed {
		w.revision++
	}
	return w.revision
}

// DeleteOverlay removes editor content for a closed file.
func (w *Workspace) DeleteOverlay(filePath string) uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.overlays[filePath]; !ok {
		return w.revision
	}
	delete(w.overlays, filePath)
	w.revision++
	return w.revision
}

// Revision returns the current workspace revision.
func (w *Workspace) Revision() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.revision
}

// Snapshot captures the current overlay state as an immutable view.
func (w *Workspace) Snapshot() *Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	overlays := make(map[string]string, len(w.overlays))
	for k, v := range w.overlays {
		overlays[k] = v
	}
	return &Snapshot{
		engine:   w.engine,
		overlays: overlays,
		revision: w.revision,
	}
}

// Snapshot is an immutable view of workspace state. Feature requests run
// against a snapshot.
type Snapshot struct {
	engine   *Engine
	overlays map[string]string
	revision uint64
}

// Revision returns the workspace revision this snapshot was taken at.
func (s *Snapshot) Revision() uint64 { return s.revision }

// Content returns the file's current content: the overlay when open,
// otherwise the on-disk bytes.
func (s *Snapshot) Content(filePath string) ([]byte, error) {
	if text, ok := s.overlays[filePath]; ok {
		return []byte(text), nil
	}
	return os.ReadFile(filePath)
}

// Parse returns the memoized parse of the file at this snapshot.
func (s *Snapshot) Parse(filePath string) (*parse.Program, []parse.ParseError, error) {
	content, err := s.Content(filePath)
	if err != nil {
		return nil, nil, err
	}
	entry := s.engine.parseFile(content, filePath)
	return entry.program, entry.errors, nil
}

// Analyze parses and checks the file at this snapshot, memoized by the
// content signature of the file's import closure. Panics in the checker are
// recovered and reported as errors; failed analyses are not cached.
func (s *Snapshot) Analyze(filePath string) (*FileAnalysis, error) {
	return s.analyze(context.Background(), filePath, true)
}

// AnalyzeCtx is Analyze with cooperative cancellation: the context is
// checked between pipeline stages (before parse, signature, and check), so
// a superseded or timed-out request stops before the expensive stage instead
// of burning a full check. The checker itself is not interruptible.
func (s *Snapshot) AnalyzeCtx(ctx context.Context, filePath string) (*FileAnalysis, error) {
	return s.analyze(ctx, filePath, true)
}

// AnalyzeEphemeral analyzes without inserting into the check cache. Used for
// synthetic content (e.g. completion placeholder patching) that would
// otherwise thrash the bounded cache with never-reused entries.
func (s *Snapshot) AnalyzeEphemeral(ctx context.Context, filePath string) (*FileAnalysis, error) {
	return s.analyze(ctx, filePath, false)
}

func (s *Snapshot) analyze(ctx context.Context, filePath string, cache bool) (analysis *FileAnalysis, err error) {
	defer func() {
		if r := recover(); r != nil {
			analysis = nil
			err = fmt.Errorf("analysis panic: %v\n%s", r, debug.Stack())
		}
	}()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	content, err := s.Content(filePath)
	if err != nil {
		return nil, err
	}
	entry := s.engine.parseFile(content, filePath)
	if entry.program == nil {
		return nil, fmt.Errorf("parse returned no program for %s", filePath)
	}
	if len(entry.errors) > 0 && cache {
		// Parse errors block checking on the cached path; not cached in
		// checkCache because the parse cache already makes this cheap.
		// Ephemeral (tooling-patched) analyses continue: signature help and
		// completion need best-effort checking of partial ASTs, and Analyze
		// recovers checker panics.
		return &FileAnalysis{
			FilePath:    filePath,
			Program:     entry.program,
			ParseErrors: entry.errors,
		}, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	moduleResolver, relPath, err := s.newModuleResolver(filePath)
	if err != nil {
		return nil, err
	}

	sig := s.signature(filePath, content, entry.program, moduleResolver, relPath)

	s.engine.mu.Lock()
	if cached, ok := s.engine.checkCache[sig]; ok {
		s.engine.mu.Unlock()
		return cached, nil
	}
	s.engine.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	analysis, err = s.check(filePath, relPath, entry.program, entry.errors, moduleResolver, sig)
	if err != nil {
		return nil, err
	}

	if !cache {
		return analysis, nil
	}

	s.engine.mu.Lock()
	if cached, ok := s.engine.checkCache[sig]; ok {
		// A concurrent analysis won the insert race; serve the cached
		// instance so memoization stays pointer-stable.
		s.engine.mu.Unlock()
		return cached, nil
	}
	s.engine.checkCache[sig] = analysis
	s.engine.checkOrder = append(s.engine.checkOrder, sig)
	if len(s.engine.checkOrder) > maxCheckEntries {
		evict := s.engine.checkOrder[0]
		s.engine.checkOrder = s.engine.checkOrder[1:]
		delete(s.engine.checkCache, evict)
	}
	s.engine.mu.Unlock()
	return analysis, nil
}

// newModuleResolver builds a resolver rooted at the project with snapshot
// overlays applied, and returns the file's project-relative module path.
func (s *Snapshot) newModuleResolver(filePath string) (*checker.ModuleResolver, string, error) {
	root := s.engine.projectRoot
	if root == "" {
		root = filepath.Dir(filePath)
	}
	moduleResolver, err := checker.NewModuleResolver(root)
	if err != nil {
		return nil, "", fmt.Errorf("module resolver: %w", err)
	}
	for overlayPath, overlaySource := range s.overlays {
		moduleResolver.SetOverlay(overlayPath, overlaySource)
	}
	relPath, err := filepath.Rel(root, filePath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		relPath = filePath
	}
	return moduleResolver, relPath, nil
}

// check runs the checker for the file with snapshot overlays applied.
func (s *Snapshot) check(filePath string, relPath string, program *parse.Program, parseErrors []parse.ParseError, moduleResolver *checker.ModuleResolver, sig string) (*FileAnalysis, error) {
	projectInfo := moduleResolver.GetProjectInfo()
	// Pre-scan this check's import closure for Go paths so the shared
	// session is primed before checking begins (ADR 0044). The module
	// resolver carries the snapshot overlays, so unsaved edits participate.
	goPaths := checker.CollectGoImportPaths(moduleResolver, checker.GoImportScanEntry{Program: program, ModulePath: strings.TrimSuffix(relPath, ".ard")})
	goResolver := s.engine.goResolverFor(projectInfo, goPaths)

	c := checker.New(relPath, program, moduleResolver, checker.CheckOptions{
		GoResolver:  goResolver,
		RecordSpans: true,
	})
	c.Check()

	module := c.Module()
	return &FileAnalysis{
		FilePath:    filePath,
		Program:     program,
		ParseErrors: parseErrors,
		Diagnostics: c.Diagnostics(),
		Spans:       c.Spans(),
		Checked:     module.Program(),
		Module:      module,
		Signature:   sig,
	}, nil
}

// signature computes a content signature over the file and its transitive
// import closure, resolved through the real module resolver so dependency
// aliases and package deps participate. Check results are reusable across
// snapshots that did not touch any input of this file.
//
// Standard-library imports (ard/*) are skipped: the stdlib is embedded in the
// compiler binary and immutable for the lifetime of the LSP process.
func (s *Snapshot) signature(filePath string, content []byte, program *parse.Program, moduleResolver *checker.ModuleResolver, relPath string) string {
	h := sha256.New()
	h.Write([]byte(filePath))
	h.Write([]byte{0})
	h.Write(content)
	h.Write([]byte{0})

	seen := map[string]bool{filePath: true}
	var visit func(prog *parse.Program, importerModulePath string)
	visit = func(prog *parse.Program, importerModulePath string) {
		if prog == nil {
			return
		}
		type dep struct {
			file   string
			module string
		}
		deps := make([]dep, 0, len(prog.Imports))
		for _, imp := range prog.Imports {
			if imp.Kind == parse.ImportKindGo || strings.HasPrefix(imp.Path, "ard/") {
				continue
			}
			resolved, err := moduleResolver.ResolveImport(importerModulePath, imp.Path)
			if err != nil || resolved.FilePath == "" {
				// Unresolvable imports still influence the hash so adding the
				// missing module later invalidates this analysis.
				h.Write([]byte(imp.Path))
				h.Write([]byte(":unresolved"))
				h.Write([]byte{0})
				continue
			}
			deps = append(deps, dep{file: resolved.FilePath, module: resolved.ModulePath})
		}
		sort.Slice(deps, func(a, b int) bool { return deps[a].file < deps[b].file })
		for _, d := range deps {
			if seen[d.file] {
				continue
			}
			seen[d.file] = true
			depContent, err := s.Content(d.file)
			if err != nil {
				h.Write([]byte(d.file))
				h.Write([]byte(":missing"))
				h.Write([]byte{0})
				continue
			}
			h.Write([]byte(d.file))
			h.Write([]byte{0})
			h.Write(depContent)
			h.Write([]byte{0})
			entry := s.engine.parseFile(depContent, d.file)
			visit(entry.program, d.module)
		}
	}
	visit(program, strings.TrimSuffix(relPath, ".ard"))

	// Project manifest and Go module metadata participate so dependency and
	// FFI configuration changes invalidate checks.
	h.Write([]byte(s.engine.manifestSignature()))
	h.Write([]byte(s.engine.goModSignature()))
	return hex.EncodeToString(h.Sum(nil))
}

// Engine returns the engine backing this workspace.
func (w *Workspace) Engine() *Engine { return w.engine }

// Engine returns the engine backing this snapshot.
func (s *Snapshot) Engine() *Engine { return s.engine }
