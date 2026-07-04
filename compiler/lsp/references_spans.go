package lsp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// referencesFromSpans resolves find-references through the span table
// (ADR 0043). References span the whole project: the definition module's
// own records join records in other files that target the same entity.
// Nil result lets callers fall back to legacy heuristics.
func (s *Server) referencesFromSpans(docURI uri.URI, position protocol.Position, includeDeclaration bool) []protocol.Location {
	filePath, err := filePathFromURI(docURI)
	if err != nil {
		return nil
	}
	group := s.spanGroupAt(docURI, position)
	if group == nil {
		return nil
	}

	var out []protocol.Location
	appendRec := func(path string, loc parse.Location) {
		out = append(out, protocol.Location{
			URI:   protocol.DocumentURI(uri.File(path)),
			Range: parseLocationToLSPRange(loc),
		})
	}
	for _, rec := range group.records {
		if rec.IsDef && !includeDeclaration {
			continue
		}
		appendRec(filePath, s.refLocation(rec, group.name, filePath))
	}

	// Nominal entities are visible across the project: gather references in
	// other files, and when the position itself was a cross-module use,
	// gather the definition module's own group.
	if _, isLocal := group.key.(*checker.Symbol); !isLocal {
		out = append(out, s.workspaceReferences(filePath, group, includeDeclaration)...)
		if includeDeclaration {
			if defLoc, ok := s.definitionLocation(group, filePath); ok {
				out = append(out, defLoc)
			}
		}
	}

	out = dedupeLocations(out)
	sortLocationsByFile(out)
	// Definition first: editors expect the declaration to lead the list.
	if defLoc, ok := s.definitionLocation(group, filePath); ok {
		for i, loc := range out {
			if loc == defLoc && i > 0 {
				copy(out[1:i+1], out[:i])
				out[0] = defLoc
				break
			}
		}
	}
	return out
}

// definitionLocation finds the group's definition as an LSP location.
func (s *Server) definitionLocation(group *spanGroup, fromFile string) (protocol.Location, bool) {
	for _, rec := range group.records {
		if rec.IsDef {
			return protocol.Location{
				URI:   protocol.DocumentURI(uri.File(fromFile)),
				Range: parseLocationToLSPRange(s.refLocation(rec, group.name, fromFile)),
			}, true
		}
	}
	if group.target != nil && group.target.File != "" {
		snap := s.workspaceFor(fromFile).Snapshot()
		if fa, err := snap.Analyze(group.target.File); err == nil && fa != nil && fa.Spans != nil {
			// The defining module's own key uses its local module path, which
			// differs from the canonical import path in the target; match by
			// kind/symbol/owner suffix instead of exact key.
			for _, rec := range fa.Spans.Records() {
				if !rec.IsDef {
					continue
				}
				key, ok := rec.Key.(string)
				if !ok || !keyMatches(key, group.target.Kind, group.target.Symbol, group.target.Owner) {
					continue
				}
				return protocol.Location{
					URI:   protocol.DocumentURI(uri.File(group.target.File)),
					Range: parseLocationToLSPRange(s.refLocation(rec, group.name, group.target.File)),
				}, true
			}
		}
	}
	return protocol.Location{}, false
}

// workspaceReferences finds records in other project files that refer to the
// same entity as group.
func (s *Server) workspaceReferences(fromFile string, group *spanGroup, includeDeclaration bool) []protocol.Location {
	snap := s.workspaceFor(fromFile).Snapshot()
	root := snap.Engine().ProjectRoot()
	if root == "" {
		return nil
	}

	// Establish the defining file and symbol identity.
	defFile := fromFile
	var kind checker.TargetKind
	symbol := group.name
	owner := ""
	if key, ok := group.key.(string); ok {
		switch {
		case strings.HasPrefix(key, "fn:"):
			kind = checker.TargetFunction
		case strings.HasPrefix(key, "type:"):
			kind = checker.TargetType
		case strings.HasPrefix(key, "field:"):
			kind = checker.TargetField
		case strings.HasPrefix(key, "method:"):
			kind = checker.TargetMethod
		case strings.HasPrefix(key, "val:"):
			kind = checker.TargetValue
		default:
			return nil
		}
		if kind == checker.TargetField || kind == checker.TargetMethod {
			tail := key[strings.LastIndex(key, ":")+1:]
			if dot := strings.LastIndex(tail, "."); dot >= 0 {
				owner = tail[:dot]
			}
		}
	}
	// A use-site group may carry a target pointing at the defining file.
	if group.target != nil && group.target.File != "" {
		defFile = group.target.File
		kind = group.target.Kind
		symbol = group.target.Symbol
		owner = group.target.Owner
	} else {
		for _, rec := range group.records {
			if rec.Target != nil && rec.Target.File != "" {
				defFile = rec.Target.File
				kind = rec.Target.Kind
				symbol = rec.Target.Symbol
				owner = rec.Target.Owner
				break
			}
		}
	}

	var out []protocol.Location
	defFileCanon := canonicalPath(defFile)
	seenFiles := map[string]bool{canonicalPath(fromFile): true}

	addMatches := func(path string) {
		canon := canonicalPath(path)
		if seenFiles[canon] {
			return
		}
		seenFiles[canon] = true
		fa, err := snap.Analyze(path)
		if err != nil || fa == nil || fa.Spans == nil {
			return
		}
		isDefFile := canon == defFileCanon
		for _, rec := range fa.Spans.Records() {
			match := false
			if rec.Target != nil && rec.Target.Symbol == symbol && rec.Target.Kind == kind && rec.Target.Owner == owner &&
				canonicalPath(rec.Target.File) == defFileCanon {
				match = true
			}
			// The defining file's own group: same string key.
			if isDefFile && rec.Key != nil {
				if key, ok := rec.Key.(string); ok && keyMatches(key, kind, symbol, owner) {
					if rec.IsDef && !includeDeclaration {
						continue
					}
					match = true
				}
			}
			if match {
				out = append(out, protocol.Location{
					URI:   protocol.DocumentURI(uri.File(path)),
					Range: parseLocationToLSPRange(s.refLocation(rec, symbol, path)),
				})
			}
		}
	}

	for _, path := range projectArdFiles(root) {
		addMatches(path)
	}
	return out
}

// canonicalPath normalizes a file path for identity comparison, resolving
// symlinked temp dirs (macOS /var vs /private/var) and lexical differences.
func canonicalPath(path string) string {
	if path == "" {
		return ""
	}
	cleaned := filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return resolved
	}
	return cleaned
}

// keyMatches reports whether a canonical string key names the entity.
// Suffix matching (ignoring the module segment) is only sound file-scoped:
// callers must already have restricted the scan to the defining file, where
// one module means kind+owner+symbol is unique.
func keyMatches(key string, kind checker.TargetKind, symbol, owner string) bool {
	var prefix string
	switch kind {
	case checker.TargetFunction:
		prefix = "fn:"
	case checker.TargetType:
		prefix = "type:"
	case checker.TargetField:
		prefix = "field:"
	case checker.TargetMethod:
		prefix = "method:"
	case checker.TargetValue:
		prefix = "val:"
	}
	if !strings.HasPrefix(key, prefix) {
		return false
	}
	tail := key[strings.LastIndex(key, ":")+1:]
	if owner != "" {
		return tail == owner+"."+symbol
	}
	return tail == symbol
}

func dedupeLocations(locs []protocol.Location) []protocol.Location {
	seen := map[protocol.Location]bool{}
	out := locs[:0]
	for _, loc := range locs {
		if seen[loc] {
			continue
		}
		seen[loc] = true
		out = append(out, loc)
	}
	return out
}

func sortLocationsByFile(locs []protocol.Location) {
	sort.Slice(locs, func(a, b int) bool {
		if locs[a].URI != locs[b].URI {
			return locs[a].URI < locs[b].URI
		}
		if locs[a].Range.Start.Line != locs[b].Range.Start.Line {
			return locs[a].Range.Start.Line < locs[b].Range.Start.Line
		}
		return locs[a].Range.Start.Character < locs[b].Range.Start.Character
	})
}

// highlightsFromSpans resolves document highlights: same grouping as
// references, restricted to the current file by construction, with Write
// kind on definitions.
func (s *Server) highlightsFromSpans(docURI uri.URI, position protocol.Position) []protocol.DocumentHighlight {
	filePath, err := filePathFromURI(docURI)
	if err != nil {
		return nil
	}
	group := s.spanGroupAt(docURI, position)
	if group == nil {
		return nil
	}

	var out []protocol.DocumentHighlight
	for _, rec := range group.records {
		kind := protocol.DocumentHighlightKindRead
		if rec.IsDef {
			kind = protocol.DocumentHighlightKindWrite
		}
		out = append(out, protocol.DocumentHighlight{
			Range: parseLocationToLSPRange(s.refLocation(rec, group.name, filePath)),
			Kind:  kind,
		})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Range.Start.Line != out[b].Range.Start.Line {
			return out[a].Range.Start.Line < out[b].Range.Start.Line
		}
		return out[a].Range.Start.Character < out[b].Range.Start.Character
	})
	return out
}

// renameFromSpans computes a workspace edit renaming the symbol at position.
// Only local symbols are handled here: their reference set is single-file by
// construction. Nominal entities (functions, types, members) return nil so
// the legacy path produces cross-file edits until the workspace rename
// slice lands.
func (s *Server) renameFromSpans(docURI uri.URI, position protocol.Position, newName string) *protocol.WorkspaceEdit {
	filePath, err := filePathFromURI(docURI)
	if err != nil {
		return nil
	}
	if !isValidRenameIdentifier(newName) {
		return nil
	}
	group := s.spanGroupAt(docURI, position)
	if group == nil || group.name == "" {
		return nil
	}
	if group.name == newName {
		return nil
	}
	if _, isLocal := group.key.(*checker.Symbol); !isLocal {
		return nil
	}

	var edits []protocol.TextEdit
	seen := map[protocol.Range]bool{}
	for _, rec := range group.records {
		r := parseLocationToLSPRange(s.refLocation(rec, group.name, filePath))
		if seen[r] {
			continue
		}
		seen[r] = true
		edits = append(edits, protocol.TextEdit{Range: r, NewText: newName})
	}
	if len(edits) == 0 {
		return nil
	}
	sort.Slice(edits, func(a, b int) bool {
		if edits[a].Range.Start.Line != edits[b].Range.Start.Line {
			return edits[a].Range.Start.Line < edits[b].Range.Start.Line
		}
		return edits[a].Range.Start.Character < edits[b].Range.Start.Character
	})
	return &protocol.WorkspaceEdit{
		Changes: map[uri.URI][]protocol.TextEdit{docURI: edits},
	}
}

// prepareRenameFromSpans reports the exact range that would be renamed.
func (s *Server) prepareRenameFromSpans(docURI uri.URI, position protocol.Position) *protocol.Range {
	filePath, err := filePathFromURI(docURI)
	if err != nil {
		return nil
	}
	group := s.spanGroupAt(docURI, position)
	if group == nil || group.name == "" {
		return nil
	}
	point := parse.Point{Row: int(position.Line) + 1, Col: int(position.Character) + 1}
	for _, rec := range group.records {
		loc := s.refLocation(rec, group.name, filePath)
		if spanContainsPoint(loc, point) {
			r := parseLocationToLSPRange(loc)
			return &r
		}
	}
	return nil
}

// spanGroup is a resolved identity group: all records sharing one key.
type spanGroup struct {
	key     any
	name    string
	records []checker.SpanRecord
	// target is set when the group was formed from a cross-module use.
	target *checker.SpanTarget
}

// spanGroupAt resolves the identity group for the symbol at a position.
func (s *Server) spanGroupAt(docURI uri.URI, position protocol.Position) *spanGroup {
	fa, err := s.analyzeSnapshot(docURI)
	if err != nil || fa == nil || fa.Spans == nil {
		return nil
	}
	point := parse.Point{Row: int(position.Line) + 1, Col: int(position.Character) + 1}
	records := fa.Spans.At(point)
	// A module-level binding records both a *Symbol key and a canonical
	// string key at the same span; prefer the string key so project-wide
	// visibility (cross-file references) wins over the local view.
	for i, rec := range records {
		if _, isSym := rec.Key.(*checker.Symbol); !isSym {
			continue
		}
		for _, other := range records[i+1:] {
			if other.Loc == rec.Loc && other.IsDef == rec.IsDef {
				if _, isStr := other.Key.(string); isStr {
					group := &spanGroup{key: other.Key, records: fa.Spans.ByKey(other.Key)}
					group.name = groupSymbolName(other)
					if rec.IsDef && !onDeclarationLine(rec.Loc, point) {
						break
					}
					return group
				}
			}
		}
		break
	}
	// First pass: keyed or targeted records that name the entity precisely.
	for _, rec := range records {
		if rec.IsDef && !onDeclarationLine(rec.Loc, point) {
			// An enclosing declaration span (e.g. the whole function body)
			// must not capture references for positions inside the body.
			continue
		}
		if rec.Key != nil {
			group := &spanGroup{key: rec.Key, records: fa.Spans.ByKey(rec.Key)}
			group.name = groupSymbolName(rec)
			return group
		}
		if rec.Target != nil {
			// Cross-module use with no local key: group by target identity.
			group := &spanGroup{
				key:     targetKeyOf(rec.Target),
				name:    rec.Target.Symbol,
				records: recordsTargeting(fa.Spans.Records(), rec.Target),
			}
			group.target = rec.Target
			return group
		}
	}
	return nil
}

// onDeclarationLine reports whether the point sits on the first line of a
// declaration span (the signature/name line).
func onDeclarationLine(loc parse.Location, p parse.Point) bool {
	return p.Row == loc.Start.Row
}

// targetKeyOf builds the canonical string key a target's defining module
// would use for the same entity.
func targetKeyOf(t *checker.SpanTarget) string {
	switch t.Kind {
	case checker.TargetFunction:
		return "fn:" + t.Module + ":" + t.Symbol
	case checker.TargetType:
		return checker.TypeKey(t.Module, t.Symbol)
	case checker.TargetField, checker.TargetMethod:
		return checker.MemberKey(t.Kind, t.Module, t.Owner, t.Symbol)
	case checker.TargetValue:
		return checker.ValueKey(t.Module, t.Symbol)
	}
	return ""
}

// recordsTargeting collects records referring to the same target entity.
func recordsTargeting(records []checker.SpanRecord, target *checker.SpanTarget) []checker.SpanRecord {
	var out []checker.SpanRecord
	for _, rec := range records {
		if rec.Target != nil && rec.Target.Kind == target.Kind &&
			rec.Target.Module == target.Module && rec.Target.Symbol == target.Symbol &&
			rec.Target.Owner == target.Owner {
			out = append(out, rec)
		}
	}
	return out
}

// groupSymbolName extracts the identifier text the group refers to, used for
// lexical narrowing of imprecise definition spans.
func groupSymbolName(rec checker.SpanRecord) string {
	if sym, ok := rec.Key.(*checker.Symbol); ok {
		return sym.Name
	}
	if key, ok := rec.Key.(string); ok {
		// "kind:module:name" or "kind:module:Owner.name"
		if idx := strings.LastIndex(key, ":"); idx >= 0 {
			name := key[idx+1:]
			if dot := strings.LastIndex(name, "."); dot >= 0 {
				name = name[dot+1:]
			}
			return name
		}
	}
	return ""
}

// refLocation narrows imprecise (multi-line or statement-wide) record spans
// to the identifier's own range by scanning the source text.
func (s *Server) refLocation(rec checker.SpanRecord, name string, filePath string) parse.Location {
	loc := rec.Loc
	if name == "" {
		return loc
	}
	// Precise single-name spans: identifier length matches span width.
	if loc.End.Row == loc.Start.Row && loc.End.Col-loc.Start.Col == len(name) {
		return loc
	}
	text := s.lineText(filePath, loc.Start.Row)
	if text == "" {
		return loc
	}
	// Search from the span start column for the first identifier-boundary
	// occurrence of name.
	startIdx := loc.Start.Col - 1
	if startIdx < 0 || startIdx >= len(text) {
		// An out-of-range start column means the span does not describe this
		// line; scanning from 0 could mis-narrow to an unrelated occurrence.
		return loc
	}
	for idx := startIdx; idx+len(name) <= len(text); idx++ {
		if text[idx:idx+len(name)] != name {
			continue
		}
		if idx > 0 && isIdentByte(text[idx-1]) {
			continue
		}
		if idx+len(name) < len(text) && isIdentByte(text[idx+len(name)]) {
			continue
		}
		return parse.Location{
			Start: parse.Point{Row: loc.Start.Row, Col: idx + 1},
			End:   parse.Point{Row: loc.Start.Row, Col: idx + 1 + len(name)},
		}
	}
	return loc
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// lineText returns the 1-based line's text from the open document or disk.
func (s *Server) lineText(filePath string, row int) string {
	snap := s.workspaceFor(filePath).Snapshot()
	content, err := snap.Content(filePath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	if row < 1 || row > len(lines) {
		return ""
	}
	return lines[row-1]
}

func spanContainsPoint(loc parse.Location, p parse.Point) bool {
	if p.Row < loc.Start.Row || (p.Row == loc.Start.Row && p.Col < loc.Start.Col) {
		return false
	}
	if loc.End.Row > 0 {
		if p.Row > loc.End.Row || (p.Row == loc.End.Row && p.Col > loc.End.Col) {
			return false
		}
	}
	return true
}

func sortLocations(locs []protocol.Location) {
	sort.Slice(locs, func(a, b int) bool {
		if locs[a].Range.Start.Line != locs[b].Range.Start.Line {
			return locs[a].Range.Start.Line < locs[b].Range.Start.Line
		}
		return locs[a].Range.Start.Character < locs[b].Range.Start.Character
	})
}

// projectArdFiles enumerates .ard files under the project root, bounded to
// keep reference searches cheap in large trees.
func projectArdFiles(root string) []string {
	const maxFiles = 2000
	var files []string
	_ = filepathWalk(root, &files, maxFiles)
	return files
}

func filepathWalk(root string, files *[]string, limit int) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if len(*files) >= limit {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == "ard-out" || name == ".git" || name == "node_modules" {
				continue
			}
			_ = filepathWalk(root+"/"+name, files, limit)
			continue
		}
		if len(name) > 4 && name[len(name)-4:] == ".ard" {
			*files = append(*files, root+"/"+name)
		}
	}
	return nil
}
