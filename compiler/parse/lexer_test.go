package parse

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

type lexTest struct {
	name  string
	input string
	want  []token
}

func runLexing(t *testing.T, tests []lexTest) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lexer := NewLexer([]byte(test.input))
			diff := cmp.Diff(test.want, lexer.Scan(), compareOptions)
			if diff != "" {
				t.Errorf("Tokens do not match (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUnsupportedSourceCharactersReportLexErrors(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		message  string
		location Location
	}{
		{
			name:     "caret",
			source:   "^",
			message:  "Unexpected character '^'",
			location: Location{Start: Point{Row: 1, Col: 1}, End: Point{Row: 1, Col: 1}},
		},
		{
			name:     "tilde",
			source:   "\n  ~",
			message:  "Unexpected character '~'",
			location: Location{Start: Point{Row: 2, Col: 3}, End: Point{Row: 2, Col: 3}},
		},
		{
			name:     "ampersand",
			source:   "&",
			message:  "Unexpected character '&'",
			location: Location{Start: Point{Row: 1, Col: 1}, End: Point{Row: 1, Col: 1}},
		},
		{
			name:     "backslash",
			source:   "\\",
			message:  "Unexpected character '\\\\'",
			location: Location{Start: Point{Row: 1, Col: 1}, End: Point{Row: 1, Col: 1}},
		},
		{
			name:     "UTF-8 rune",
			source:   "§",
			message:  "Unexpected character '§'",
			location: Location{Start: Point{Row: 1, Col: 1}, End: Point{Row: 1, Col: 2}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lexer := NewLexer([]byte(test.source))
			lexer.Scan()
			want := []ParseError{{Location: test.location, Message: test.message}}
			if diff := cmp.Diff(want, lexer.errors); diff != "" {
				t.Fatalf("lex errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMalformedUTF8ReportsEachInvalidByteAndRecovers(t *testing.T) {
	lexer := NewLexer([]byte{0xff, '^'})
	lexer.Scan()
	want := []ParseError{
		{
			Location: Location{Start: Point{Row: 1, Col: 1}, End: Point{Row: 1, Col: 1}},
			Message:  "Invalid UTF-8 byte 0xFF",
		},
		{
			Location: Location{Start: Point{Row: 1, Col: 2}, End: Point{Row: 1, Col: 2}},
			Message:  "Unexpected character '^'",
		},
	}
	if diff := cmp.Diff(want, lexer.errors); diff != "" {
		t.Fatalf("lex errors mismatch (-want +got):\n%s", diff)
	}
}

func TestUnsupportedSourceCharactersFlowIntoParseErrors(t *testing.T) {
	result := Parse([]byte("fn main() Int { 1^ }"), "test.ard")
	want := []ParseError{{
		Location: Location{Start: Point{Row: 1, Col: 18}, End: Point{Row: 1, Col: 18}},
		Message:  "Unexpected character '^'",
	}}
	if diff := cmp.Diff(want, result.Errors); diff != "" {
		t.Fatalf("parse errors mismatch (-want +got):\n%s", diff)
	}
}

func TestUnsupportedCharactersRemainValidInOpaqueContexts(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "quoted string", source: `"^~&"`},
		{name: "raw string", source: "`^~&\\`"},
		{name: "comment", source: "// ^~&\\"},
		{name: "import path", source: "use foo~bar"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lexer := NewLexer([]byte(test.source))
			lexer.Scan()
			if diff := cmp.Diff([]ParseError(nil), lexer.errors); diff != "" {
				t.Fatalf("lex errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
