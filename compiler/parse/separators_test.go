package parse

import (
	"testing"
)

// Items in lists, maps, call arguments, struct literals and parameter lists
// must be separated by commas; the comma is optional only after the last
// item. Enum variants and match/select arms may also be separated by a new
// line. Previously every comma was optional, so `[5 4]` silently became
// `[5, 4]` and `f(1 2)` passed two arguments.
func TestMissingSeparatorsAreReported(t *testing.T) {
	at := func(row, col int) Location {
		return Location{Start: Point{Row: row, Col: col}, End: Point{Row: row, Col: col}}
	}
	tests := []struct {
		name    string
		input   string
		wantErr ParseError
	}{
		{
			name:    "list items on one line",
			input:   "let a = [5 4]\n",
			wantErr: ParseError{Location: at(1, 12), Message: "Expected ',' between list items"},
		},
		{
			name:    "list items on separate lines",
			input:   "let a = [\n  5\n  4\n]\n",
			wantErr: ParseError{Location: at(3, 3), Message: "Expected ',' between list items"},
		},
		{
			name:    "list items after a trailing comment",
			input:   "let a = [\n  5 // five\n  4,\n]\n",
			wantErr: ParseError{Location: at(3, 3), Message: "Expected ',' between list items"},
		},
		{
			name:    "bitwise-looking & in a list",
			input:   "let a = [5 & 4]\n",
			wantErr: ParseError{Location: at(1, 12), Message: "Unexpected '&' after expression; Ard has no bitwise '&' operator"},
		},
		{
			name:    "map entries",
			input:   "let m = [\"a\": 1 \"b\": 2]\n",
			wantErr: ParseError{Location: Location{Start: Point{Row: 1, Col: 17}, End: Point{Row: 1, Col: 19}}, Message: "Expected ',' between map entries"},
		},
		{
			name:    "call arguments",
			input:   "f(1 2)\n",
			wantErr: ParseError{Location: at(1, 5), Message: "Expected ',' between arguments"},
		},
		{
			name:    "bitwise-looking & in call arguments",
			input:   "f(5 & 4)\n",
			wantErr: ParseError{Location: at(1, 5), Message: "Unexpected '&' after expression; Ard has no bitwise '&' operator"},
		},
		{
			name:    "named call arguments",
			input:   "f(a: 1 b: 2)\n",
			wantErr: ParseError{Location: at(1, 8), Message: "Expected ',' between arguments"},
		},
		{
			name:    "struct literal fields",
			input:   "let p = P{x: 1 y: 2}\n",
			wantErr: ParseError{Location: at(1, 16), Message: "Expected ',' between fields"},
		},
		{
			name:    "function parameters",
			input:   "fn f(a: Int b: Int) Int { a }\n",
			wantErr: ParseError{Location: at(1, 13), Message: "Expected ',' between parameters"},
		},
		{
			name:    "anonymous function parameters",
			input:   "let f = fn(a: Int b: Int) Int { a }\n",
			wantErr: ParseError{Location: at(1, 19), Message: "Expected ',' between parameters"},
		},
		{
			name:    "enum variants on one line",
			input:   "enum C { a b }\n",
			wantErr: ParseError{Location: at(1, 12), Message: "Expected ',' or new line between enum variants"},
		},
		{
			name:    "match arms on one line",
			input:   "fn g(x: Int) Int {\n  match x {\n    1 => 1 _ => 2\n  }\n}\n",
			wantErr: ParseError{Location: at(3, 12), Message: "Expected ',' or new line between match arms"},
		},
		{
			name:    "conditional match arms on one line",
			input:   "fn g(x: Int) Int {\n  match {\n    x > 1 => 1 _ => 2\n  }\n}\n",
			wantErr: ParseError{Location: at(3, 16), Message: "Expected ',' or new line between match arms"},
		},
		{
			name:    "select cases on one line",
			input:   "select {\n  a.recv() => 1 _ => 2\n}\n",
			wantErr: ParseError{Location: at(2, 17), Message: "Expected ',' or new line between select cases"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) != 1 || result.Errors[0] != tt.wantErr {
				t.Fatalf("errors = %v, want [%v]", result.Errors, tt.wantErr)
			}
		})
	}
}

// A doubled comma is reported once, as an unexpected comma, not also as a
// missing separator after the item it broke.
func TestDoubledCommaReportedOnce(t *testing.T) {
	for name, input := range map[string]string{
		"list": "let a = [1,,2]\n",
		"call": "f(1,,2)\n",
	} {
		t.Run(name, func(t *testing.T) {
			result := Parse([]byte(input), "test.ard")
			if len(result.Errors) != 1 || result.Errors[0].Message != "Unexpected token: comma" {
				t.Fatalf("errors = %v, want only the unexpected comma", result.Errors)
			}
		})
	}
}

// Recovery treats the missing comma as present, so the remaining items are
// still parsed.
func TestMissingSeparatorRecoveryKeepsItems(t *testing.T) {
	result := Parse([]byte("let a = [1 2, 3]\n"), "test.ard")
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %v, want exactly one", result.Errors)
	}
	list := result.Program.Statements[0].(*VariableDeclaration).Value.(*ListLiteral)
	if len(list.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(list.Items))
	}
}

func TestSeparatorsAccepted(t *testing.T) {
	for name, input := range map[string]string{
		"list trailing comma":            "let a = [1, 2,]\n",
		"list without trailing comma":    "let a = [1, 2]\n",
		"multiline list":                 "let a = [\n  1,\n  2\n]\n",
		"multiline list with comments":   "let a = [\n  1, // one\n  // two\n  2 // last\n]\n",
		"map entries":                    "let m = [\"a\": 1, \"b\": 2,]\n",
		"multiline call arguments":       "fn f(a: Int, b: Int) Int { a }\nf(\n  1,\n  2\n)\n",
		"struct literal fields":          "struct P { x: Int, y: Int }\nlet p = P{\n  x: 1,\n  y: 2\n}\n",
		"multiline parameters":           "fn f(\n  a: Int,\n  b: Int\n) Int { a }\n",
		"enum variants on new lines":     "enum C {\n  a\n  b\n}\n",
		"enum variants with commas":      "enum C { a, b }\n",
		"match arms on new lines":        "fn g(x: Int) Int {\n  match x {\n    1 => 1\n    _ => 2\n  }\n}\n",
		"match arms after a comment":     "fn g(x: Int) Int {\n  match x {\n    1 => 1 // one\n    _ => 2\n  }\n}\n",
		"match arms with commas":         "fn g(x: Int) Int {\n  match x {\n    1 => 1, _ => 2\n  }\n}\n",
		"block match arm on a new line":  "fn g(x: Int) Int {\n  match x {\n    1 => {\n      1\n    }\n    _ => 2\n  }\n}\n",
		"select cases on new lines":      "select {\n  a.recv() => 1\n  _ => 2\n}\n",
		"conditional match on new lines": "fn g(x: Int) Int {\n  match {\n    x > 1 => 1\n    _ => 2\n  }\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			parseOK(t, input)
		})
	}
}
