package lsp

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
)

func computeDocumentHighlights(source string, filePath string, position protocol.Position) []protocol.DocumentHighlight {
	previousModulePathCache := referenceModulePathCache
	referenceModulePathCache = map[string]referenceModulePathEntry{}
	defer func() { referenceModulePathCache = previousModulePathCache }()

	target := lspPositionToParsePoint(position)
	prog := parseAndCache(source, filePath)
	if prog == nil {
		return nil
	}

	var def *definitionTarget
	var targetKind string
	var targetName string
	if resolved := findReferenceDeclarationTarget(prog.Statements, target, filePath, prog); resolved != nil {
		def = resolved.def
		targetKind = resolved.kind
		targetName = resolved.name
	} else {
		expr := findInStmts(prog.Statements, target)
		if expr == nil {
			return nil
		}
		def = definitionForExpr(expr, prog, filePath)
		if def == nil {
			return nil
		}
		targetKind = referenceTargetKind(expr, prog, filePath, def)
		targetName = referenceExprName(expr)
	}

	targetName = referenceNameTail(targetName)
	highlights := []protocol.DocumentHighlight{}
	seen := map[string]bool{}
	add := func(_ string, loc parse.Location) {
		loc = documentHighlightNameLocation(source, loc, targetName)
		if loc.Start.Row == 0 && loc.Start.Col == 0 {
			return
		}
		rng := checkerLocationToLSPRange(loc)
		key := fmt.Sprintf("%d:%d:%d:%d", rng.Start.Line, rng.Start.Character, rng.End.Line, rng.End.Character)
		if seen[key] {
			return
		}
		seen[key] = true
		highlights = append(highlights, protocol.DocumentHighlight{Range: rng, Kind: documentHighlightKind(source, loc)})
	}

	// Include the declaration even if it is outside normal expression traversal.
	if def != nil && cleanReferencePath(def.filePath) == cleanReferencePath(filePath) {
		add(filePath, def.loc)
	}
	scanReferenceDocument(referenceDocument{filePath: filePath, source: source, prog: prog}, targetKind, targetName, def, add)
	return highlights
}

func documentHighlightKind(source string, loc parse.Location) protocol.DocumentHighlightKind {
	if documentHighlightLooksLikeWrite(source, loc) {
		return protocol.DocumentHighlightKindWrite
	}
	return protocol.DocumentHighlightKindRead
}

func documentHighlightLooksLikeWrite(source string, loc parse.Location) bool {
	if loc.Start.Row <= 0 || loc.Start.Col <= 0 {
		return false
	}
	line := sourceLine(source, loc.Start.Row)
	start := loc.Start.Col - 1
	end := loc.End.Col - 1
	if start < 0 || start > len(line) {
		return false
	}
	if end < start || end > len(line) {
		end = start
	}
	before := line[:start]
	after := line[end:]
	trimmedBefore := strings.TrimSpace(before)
	trimmedAfter := strings.TrimSpace(after)
	if hasAnySuffix(trimmedBefore, "let", "mut", "for", "catch", "fn") {
		return true
	}
	return hasAnyPrefix(trimmedAfter, "=", "+=", "-=", "*=", "/=")
}

func documentHighlightNameLocation(source string, loc parse.Location, name string) parse.Location {
	if name == "" || loc.Start.Row <= 0 || loc.Start.Row != loc.End.Row {
		return loc
	}
	line := sourceLine(source, loc.Start.Row)
	start := loc.Start.Col - 1
	end := loc.End.Col - 1
	if start < 0 || start >= len(line) || end <= start || end > len(line) {
		return loc
	}
	snippet := line[start:end]
	if snippet == name {
		return loc
	}
	idx := strings.Index(snippet, name)
	for idx >= 0 {
		beforeOK := idx == 0 || !isIdentifierChar(snippet[idx-1])
		afterIdx := idx + len(name)
		afterOK := afterIdx >= len(snippet) || !isIdentifierChar(snippet[afterIdx])
		if beforeOK && afterOK {
			col := start + idx + 1
			return parse.Location{Start: parse.Point{Row: loc.Start.Row, Col: col}, End: parse.Point{Row: loc.Start.Row, Col: col + len(name)}}
		}
		next := strings.Index(snippet[idx+1:], name)
		if next < 0 {
			break
		}
		idx += next + 1
	}
	return loc
}

func isIdentifierChar(b byte) bool {
	return b == '_' || b == '$' || ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z') || ('0' <= b && b <= '9')
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if len(s) == len(suffix) && s == suffix {
			return true
		}
		if len(s) > len(suffix) && s[len(s)-len(suffix):] == suffix && (s[len(s)-len(suffix)-1] == ' ' || s[len(s)-len(suffix)-1] == '\t') {
			return true
		}
	}
	return false
}
