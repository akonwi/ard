package lsp

import (
	"strings"

	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
)

func signatureParseSource(source string, position protocol.Position) string {
	target := asciiPositionToPoint(position)
	offset := parsePointToOffset(source, target)
	opens := openParensBefore(source, offset)
	if len(opens) == 0 {
		return source
	}

	insert := ""
	if needsSignaturePlaceholder(source, offset) {
		insert += "__ard_signature_arg__"
	}
	for _, open := range opens {
		if !hasMatchingParen(source, open) {
			insert += ")"
		}
	}
	if insert == "" {
		return source
	}
	return source[:offset] + insert + source[offset:]
}

func signatureParameterInformation(params []hoverParam) []protocol.ParameterInformation {
	out := make([]protocol.ParameterInformation, len(params))
	for i, p := range params {
		out[i] = protocol.ParameterInformation{Label: formatHoverParam(p)}
	}
	return out
}

func formatHoverParam(p hoverParam) string {
	text := mutParamTypeString(normalizeDisplayType(p.Type), p.Mutable)
	if p.Name == "" {
		return text
	}
	return p.Name + ": " + text
}

func openParensBefore(source string, offset int) []int {
	if offset > len(source) {
		offset = len(source)
	}
	stack := []int{}
	inString := false
	inLineComment := false
	escaped := false
	for i := 0; i < offset; i++ {
		ch := source[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '/' && i+1 < offset && source[i+1] == '/' {
			inLineComment = true
			i++
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '(':
			stack = append(stack, i)
		case ')':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return stack
}

func hasMatchingParen(source string, open int) bool {
	if open < 0 || open >= len(source) || source[open] != '(' {
		return false
	}
	depth := 0
	inString := false
	inLineComment := false
	escaped := false
	for i := open; i < len(source); i++ {
		ch := source[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '/' && i+1 < len(source) && source[i+1] == '/' {
			inLineComment = true
			i++
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

func needsSignaturePlaceholder(source string, offset int) bool {
	i := previousNonWhitespace(source, offset-1)
	if i < 0 {
		return false
	}
	if source[i] == ',' || source[i] == ':' {
		return true
	}
	if !isTypeIdentPart(source[i]) {
		return false
	}
	end := i + 1
	start := i
	for start > 0 && isTypeIdentPart(source[start-1]) {
		start--
	}
	if source[start:end] != "mut" {
		return false
	}
	prev := previousNonWhitespace(source, start-1)
	return prev >= 0 && (source[prev] == '(' || source[prev] == ',')
}

func previousNonWhitespace(source string, i int) int {
	for i >= 0 {
		switch source[i] {
		case ' ', '\t', '\n', '\r':
			i--
		default:
			return i
		}
	}
	return -1
}

func activeParameterIndex(source string, target parse.Point, callLoc parse.Location, args []parse.Argument, params []hoverParam) uint32 {
	if len(params) == 0 {
		return 0
	}

	targetOffset := parsePointToOffset(source, target)
	for _, arg := range args {
		if arg.Name == "" {
			continue
		}
		argStart := parsePointToOffset(source, arg.Location.Start)
		argEnd := parsePointToOffset(source, arg.Location.End) + 1
		if !pointInRange(target, arg.Location) && (targetOffset < argStart || targetOffset > argEnd) {
			continue
		}
		for i, p := range params {
			if p.Name == arg.Name {
				return uint32(i)
			}
		}
	}

	openOffset := findCallOpenParen(source, callLoc, targetOffset)
	if openOffset < 0 || targetOffset <= openOffset {
		return 0
	}
	idx := countTopLevelCommas(source[openOffset+1 : targetOffset])
	if idx >= len(params) {
		idx = len(params) - 1
	}
	return uint32(idx)
}

func parsePointToOffset(source string, point parse.Point) int {
	if point.Row <= 1 {
		col := point.Col - 1
		if col < 0 {
			return 0
		}
		if col > len(source) {
			return len(source)
		}
		return col
	}
	offset := 0
	row := 1
	for offset < len(source) && row < point.Row {
		if source[offset] == '\n' {
			row++
		}
		offset++
	}
	col := point.Col - 1
	if col < 0 {
		col = 0
	}
	if offset+col > len(source) {
		return len(source)
	}
	return offset + col
}

func findCallOpenParen(source string, callLoc parse.Location, targetOffset int) int {
	start := parsePointToOffset(source, callLoc.Start)
	if start < 0 {
		start = 0
	}
	if targetOffset > len(source) {
		targetOffset = len(source)
	}
	if start > targetOffset {
		return -1
	}
	idx := strings.IndexByte(source[start:targetOffset], '(')
	if idx < 0 {
		// Cursor can be just before the first argument after the trigger character.
		if targetOffset < len(source) && source[targetOffset] == '(' {
			return targetOffset
		}
		return -1
	}
	return start + idx
}

func countTopLevelCommas(s string) int {
	count := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0
	inString := false
	escaped := false
	for _, r := range s {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case ',':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				count++
			}
		}
	}
	return count
}
