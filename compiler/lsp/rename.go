package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
)

func computeRename(source string, filePath string, position protocol.Position, newName string, overlays map[string]string) *protocol.WorkspaceEdit {
	if !isValidRenameIdentifier(newName) {
		return nil
	}
	target := resolveRenameTarget(source, filePath, position)
	if target == nil || target.def == nil || !isRenameableTarget(target) {
		return nil
	}

	oldName := referenceNameTail(target.name)
	if oldName == "" || oldName == newName {
		return &protocol.WorkspaceEdit{Changes: map[protocol.DocumentURI][]protocol.TextEdit{}}
	}
	refs := computeReferencesWithOverlays(source, filePath, position, true, overlays)
	if len(refs) == 0 {
		return nil
	}

	sources := map[string]string{cleanReferencePath(filePath): source}
	for path, text := range overlays {
		sources[cleanReferencePath(path)] = text
	}

	changes := map[protocol.DocumentURI][]protocol.TextEdit{}
	seen := map[string]bool{}
	for _, ref := range refs {
		path := cleanReferencePath(ref.URI.Filename())
		text, ok := sources[path]
		if !ok {
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			text = string(b)
			sources[path] = text
		}
		rng := renameRangeForReference(text, ref.Range, oldName)
		if !rangeContainsName(text, rng, oldName) {
			continue
		}
		key := ref.URI.Filename() + rangeKey(rng)
		if seen[key] {
			continue
		}
		seen[key] = true
		changes[ref.URI] = append(changes[ref.URI], protocol.TextEdit{Range: rng, NewText: newName})
	}
	if len(changes) == 0 {
		return nil
	}
	return &protocol.WorkspaceEdit{Changes: changes}
}

func prepareRename(source string, filePath string, position protocol.Position) *protocol.Range {
	target := resolveRenameTarget(source, filePath, position)
	if target == nil || target.def == nil || !isRenameableTarget(target) {
		return nil
	}
	oldName := referenceNameTail(target.name)
	if oldName == "" {
		return nil
	}
	if rng, ok := nameRangeAtPosition(source, position, oldName); ok {
		return &rng
	}
	rng := checkerLocationToLSPRange(target.def.loc)
	rng = renameRangeForReference(source, rng, oldName)
	if !rangeContainsName(source, rng, oldName) {
		return nil
	}
	return &rng
}

func resolveRenameTarget(source string, filePath string, position protocol.Position) *referenceResolvedTarget {
	target := lspPositionToParsePoint(position)
	prog := parseAndCache(source, filePath)
	if prog == nil {
		return nil
	}
	if resolved := findReferenceDeclarationTarget(prog.Statements, target, filePath, prog); resolved != nil {
		return resolved
	}
	expr := findInStmts(prog.Statements, target)
	if expr == nil {
		return nil
	}
	def := definitionForExpr(expr, prog, filePath)
	if def == nil {
		return nil
	}
	return &referenceResolvedTarget{kind: referenceTargetKind(expr, prog, filePath, def), name: referenceExprName(expr), def: def}
}

func isRenameableTarget(target *referenceResolvedTarget) bool {
	if target == nil || target.def == nil || target.name == "" {
		return false
	}
	switch target.kind {
	case "name", "local", "param", "function", "staticFunction", "staticProperty", "type", "instanceProperty", "instanceMethod", "moduleVariable":
	default:
		return false
	}
	path := cleanReferencePath(target.def.filePath)
	if path == "" || strings.Contains(path, string(filepath.Separator)+"compiler"+string(filepath.Separator)+"std_lib"+string(filepath.Separator)) {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

func isValidRenameIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || r == '$' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || (i > 0 && '0' <= r && r <= '9') {
			continue
		}
		return false
	}
	return !(name[0] >= '0' && name[0] <= '9')
}

func renameRangeForReference(source string, rng protocol.Range, oldName string) protocol.Range {
	if oldName == "" {
		return rng
	}
	if found, ok := findNameRangeInRange(source, rng, oldName); ok {
		return found
	}

	loc := parse.Location{
		Start: parse.Point{Row: int(rng.Start.Line) + 1, Col: int(rng.Start.Character) + 1},
		End:   parse.Point{Row: int(rng.End.Line) + 1, Col: int(rng.End.Character) + 1},
	}
	loc = documentHighlightNameLocation(source, loc, oldName)
	return checkerLocationToLSPRange(loc)
}

func nameRangeAtPosition(source string, position protocol.Position, oldName string) (protocol.Range, bool) {
	line := sourceLine(source, int(position.Line)+1)
	char := int(position.Character)
	idx := strings.Index(line, oldName)
	for idx >= 0 {
		candidateEnd := idx + len(oldName)
		beforeOK := idx == 0 || !isIdentifierChar(line[idx-1])
		afterOK := candidateEnd >= len(line) || !isIdentifierChar(line[candidateEnd])
		if beforeOK && afterOK && idx <= char && char <= candidateEnd {
			return protocol.Range{Start: protocol.Position{Line: position.Line, Character: uint32(idx)}, End: protocol.Position{Line: position.Line, Character: uint32(candidateEnd)}}, true
		}
		next := strings.Index(line[idx+1:], oldName)
		if next < 0 {
			break
		}
		idx += next + 1
	}
	return protocol.Range{}, false
}

func findNameRangeInRange(source string, rng protocol.Range, oldName string) (protocol.Range, bool) {
	for lineNo := rng.Start.Line; lineNo <= rng.End.Line; lineNo++ {
		line := sourceLine(source, int(lineNo)+1)
		startLimit := 0
		endLimit := len(line)
		if lineNo == rng.Start.Line {
			startLimit = int(rng.Start.Character)
		}
		if lineNo == rng.End.Line {
			endLimit = int(rng.End.Character)
		}
		idx := strings.Index(line, oldName)
		for idx >= 0 {
			candidateEnd := idx + len(oldName)
			beforeOK := idx == 0 || !isIdentifierChar(line[idx-1])
			afterOK := candidateEnd >= len(line) || !isIdentifierChar(line[candidateEnd])
			if beforeOK && afterOK && idx <= endLimit && candidateEnd >= startLimit {
				return protocol.Range{Start: protocol.Position{Line: lineNo, Character: uint32(idx)}, End: protocol.Position{Line: lineNo, Character: uint32(candidateEnd)}}, true
			}
			next := strings.Index(line[idx+1:], oldName)
			if next < 0 {
				break
			}
			idx += next + 1
		}
		if lineNo == rng.End.Line {
			break
		}
	}
	return protocol.Range{}, false
}

func rangeContainsName(source string, rng protocol.Range, oldName string) bool {
	if rng.Start.Line != rng.End.Line || oldName == "" {
		return false
	}
	line := sourceLine(source, int(rng.Start.Line)+1)
	start := int(rng.Start.Character)
	end := int(rng.End.Character)
	if start < 0 || end < start || end > len(line) {
		return false
	}
	return line[start:end] == oldName
}

func rangeKey(rng protocol.Range) string {
	return fmt.Sprintf("#%d:%d:%d:%d", rng.Start.Line, rng.Start.Character, rng.End.Line, rng.End.Character)
}
