package lsp

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
)

// LSP positions count UTF-16 code units; the parser counts bytes. These
// helpers convert at the feature boundary using the document's line text.
// ASCII lines convert 1:1 and short-circuit.

// utf16ColToByteCol converts a 0-based UTF-16 character offset on a line to
// a 0-based byte offset.
func utf16ColToByteCol(line string, utf16Col int) int {
	if utf16Col <= 0 {
		return 0
	}
	if isASCII(line) {
		if utf16Col > len(line) {
			return len(line)
		}
		return utf16Col
	}
	units := 0
	for byteIdx, r := range line {
		if units >= utf16Col {
			return byteIdx
		}
		units += utf16.RuneLen(r)
		if units > utf16Col {
			return byteIdx
		}
	}
	return len(line)
}

// byteColToUTF16Col converts a 0-based byte offset on a line to a 0-based
// UTF-16 character offset.
func byteColToUTF16Col(line string, byteCol int) int {
	if byteCol <= 0 {
		return 0
	}
	if isASCII(line) {
		if byteCol > len(line) {
			return len(line)
		}
		return byteCol
	}
	if byteCol > len(line) {
		byteCol = len(line)
	}
	units := 0
	for byteIdx := 0; byteIdx < byteCol; {
		r, size := utf8.DecodeRuneInString(line[byteIdx:])
		if size == 0 {
			break
		}
		units += utf16.RuneLen(r)
		byteIdx += size
	}
	return units
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// docLines caches a document's split lines for one request.
type docLines struct {
	lines []string
}

func newDocLines(content string) *docLines {
	return &docLines{lines: strings.Split(content, "\n")}
}

// line returns the 1-based row's text.
func (d *docLines) line(row int) string {
	if d == nil || row < 1 || row > len(d.lines) {
		return ""
	}
	return d.lines[row-1]
}

// positionToPoint converts an LSP position (0-based, UTF-16 columns) to a
// parse point (1-based, byte columns).
func (d *docLines) positionToPoint(pos protocol.Position) parse.Point {
	row := int(pos.Line) + 1
	col := utf16ColToByteCol(d.line(row), int(pos.Character)) + 1
	return parse.Point{Row: row, Col: col}
}

// locationToRange converts a parse location (1-based, byte columns) to an
// LSP range (0-based, UTF-16 columns). Callers define the end convention.
func (d *docLines) locationToRange(loc parse.Location) protocol.Range {
	start := protocol.Position{}
	if loc.Start.Row > 0 {
		start.Line = uint32(loc.Start.Row - 1)
		if loc.Start.Col > 0 {
			start.Character = uint32(byteColToUTF16Col(d.line(loc.Start.Row), loc.Start.Col-1))
		}
	}
	end := start
	if loc.End.Row > 0 {
		end = protocol.Position{Line: uint32(loc.End.Row - 1)}
		if loc.End.Col > 0 {
			end.Character = uint32(byteColToUTF16Col(d.line(loc.End.Row), loc.End.Col-1))
		}
	}
	return protocol.Range{Start: start, End: end}
}

// docLinesFor loads a file's line index from the current snapshot.
func (s *Server) docLinesFor(filePath string) *docLines {
	snap := s.workspaceFor(filePath).Snapshot()
	content, err := snap.Content(filePath)
	if err != nil {
		return newDocLines("")
	}
	return newDocLines(string(content))
}

// rangeFor converts a parse location in the given file to an LSP range with
// UTF-16 columns.
func (s *Server) rangeFor(filePath string, loc parse.Location) protocol.Range {
	return s.docLinesFor(filePath).locationToRange(loc)
}

// diagnosticRangeFor converts diagnostics' inclusive parse range into LSP's
// exclusive UTF-16 range without changing the span convention used by other
// semantic features.
func (s *Server) diagnosticRangeFor(filePath string, loc parse.Location) protocol.Range {
	return diagnosticLocationToRange(s.docLinesFor(filePath), loc)
}

func diagnosticLocationToRange(lines *docLines, loc parse.Location) protocol.Range {
	if loc.End.Row > 0 && loc.End.Col > 0 {
		loc.End.Col++
	} else {
		loc.End = loc.Start
		loc.End.Col++
	}
	return lines.locationToRange(loc)
}
