package lsp

import (
	"strings"

	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
)

const completionPlaceholder = "__ard_completion__"

type completionKind int

const (
	completionInstance completionKind = iota + 1
	completionStatic
	completionImport
)

type completionContext struct {
	kind       completionKind
	prefix     string
	importPath string
	sepEnd     int
	offset     int
}

func completionContextAt(source string, position protocol.Position) (completionContext, bool) {
	offset := parsePointToOffset(source, asciiPositionToPoint(position))
	if offset < 0 || offset > len(source) {
		return completionContext{}, false
	}

	identStart := offset
	for identStart > 0 && isTypeIdentPart(source[identStart-1]) {
		identStart--
	}
	prefix := source[identStart:offset]
	lineStart := strings.LastIndex(source[:offset], "\n") + 1
	linePrefix := source[lineStart:offset]
	if importPrefix, ok := importCompletionPrefix(linePrefix); ok {
		segmentStart := strings.LastIndex(importPrefix, "/") + 1
		return completionContext{kind: completionImport, prefix: importPrefix[segmentStart:], importPath: importPrefix, sepEnd: lineStart + len(linePrefix) - len(importPrefix) + segmentStart, offset: offset}, true
	}

	sepEnd := identStart
	if sepEnd >= 1 && source[sepEnd-1] == '.' {
		return completionContext{kind: completionInstance, prefix: prefix, sepEnd: sepEnd, offset: offset}, true
	}
	if sepEnd >= 2 && source[sepEnd-2:sepEnd] == "::" {
		return completionContext{kind: completionStatic, prefix: prefix, sepEnd: sepEnd, offset: offset}, true
	}
	return completionContext{}, false
}

func withCompletionTextEdits(items []protocol.CompletionItem, ctx completionContext, position protocol.Position) []protocol.CompletionItem {
	if len(items) == 0 {
		return items
	}
	startChar := position.Character
	if prefixLen := uint32(len(ctx.prefix)); prefixLen <= startChar {
		startChar -= prefixLen
	} else {
		startChar = 0
	}
	editRange := protocol.Range{
		Start: protocol.Position{Line: position.Line, Character: startChar},
		End:   position,
	}
	for i := range items {
		newText := items[i].InsertText
		if newText == "" {
			newText = items[i].Label
		}
		items[i].TextEdit = &protocol.TextEdit{Range: editRange, NewText: newText}
	}
	return items
}

func offsetToParsePoint(source string, offset int) parse.Point {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	row := 1
	lineStart := 0
	for i := 0; i < offset; i++ {
		if source[i] == '\n' {
			row++
			lineStart = i + 1
		}
	}
	return parse.Point{Row: row, Col: offset - lineStart + 1}
}
