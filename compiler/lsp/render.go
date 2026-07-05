package lsp

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
)

// Shared rendering and position helpers retained from the legacy hover
// implementation; everything else was replaced by the span-table features
// (ADR 0043).

type hoverInfo struct {
	content string // Markdown content
}

type hoverParam struct {
	Name    string
	Type    string
	Mutable bool
}

// asciiPositionToPoint converts an LSP position to a parse point assuming
// byte columns equal UTF-16 columns. Only valid for ASCII-safe scanning
// (paren matching, completion-context detection); position-sensitive
// features must convert through docLines.positionToPoint instead.
func asciiPositionToPoint(pos protocol.Position) parse.Point {
	return parse.Point{
		Row: int(pos.Line) + 1,
		Col: int(pos.Character) + 1,
	}
}

// pointInRange checks if a point falls within a location range.
// Returns false if the location has zero-value (unset by parser).

func pointInRange(p parse.Point, loc parse.Location) bool {
	if loc.Start.Row == 0 && loc.Start.Col == 0 && loc.End.Row == 0 && loc.End.Col == 0 {
		return false
	}
	if p.Row < loc.Start.Row {
		return false
	}
	if p.Row == loc.Start.Row && p.Col < loc.Start.Col {
		return false
	}
	if loc.End.Row > 0 && p.Row > loc.End.Row {
		return false
	}
	if loc.End.Row > 0 && p.Row == loc.End.Row && p.Col > loc.End.Col {
		return false
	}
	return true
}

// typeDeclString converts a parse.DeclaredType to a readable string.

func checkerTypeString(t checker.Type) string {
	if t == nil {
		return "?"
	}
	// Use the checker's String() which produces canonical type names.
	s := t.String()
	// Map canonical names to Ard surface names
	switch s {
	case "String":
		return "Str"
	case "Boolean":
		return "Bool"
	}
	return s
}

// simpleHover builds a hoverInfo from a label string.

func simpleHover(label string) *hoverInfo {
	return &hoverInfo{content: fmt.Sprintf("```ard\n%s\n```", label)}
}

func normalizeDisplayType(t string) string {
	if strings.HasPrefix(t, "?") && len(t) > 1 {
		return strings.TrimPrefix(t, "?") + "?"
	}
	return t
}

func isTypeIdentPart(ch byte) bool {
	return isTypeIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func isTypeIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

// mutParamTypeString renders a parameter type with its mutability marker.
// Pointer-form foreign types and mutable references already spell "mut " in
// their type string; prepending the flag's marker again would double it.
func mutParamTypeString(typeText string, mutable bool) string {
	if !mutable || strings.HasPrefix(typeText, "mut ") {
		return typeText
	}
	return "mut " + typeText
}
