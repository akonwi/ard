package parse

import (
	"testing"
)

// A statement ends at a new line, a closing brace, a trailing comment, or the
// end of the file. Leftover tokens on the same line must be reported rather
// than silently parsed as a separate statement: `let a = 5 & 4` used to parse
// as `let a = 5` followed by a stray `&4` expression statement.
func TestStatementsMustEndBeforeAnotherStatementStarts(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr ParseError
	}{
		{
			name:  "bitwise-looking & after a variable value",
			input: "let a = 5 & 4\n",
			wantErr: ParseError{
				Location: Location{Start: Point{Row: 1, Col: 11}, End: Point{Row: 1, Col: 11}},
				Message:  "Unexpected '&' after expression; Ard has no bitwise '&' operator",
			},
		},
		{
			name:  "&mut after a variable value",
			input: "mut b = 1\nlet a = 5 &mut b\n",
			wantErr: ParseError{
				Location: Location{Start: Point{Row: 2, Col: 11}, End: Point{Row: 2, Col: 11}},
				Message:  "Unexpected '&' after expression; Ard has no bitwise '&' operator",
			},
		},
		{
			name:  "&& after a variable value",
			input: "let a = true && false\n",
			wantErr: ParseError{
				Location: Location{Start: Point{Row: 1, Col: 14}, End: Point{Row: 1, Col: 15}},
				Message:  "Unexpected '&&' after expression; use 'and' for logical and",
			},
		},
		{
			name:  "& after an assignment value",
			input: "mut a = 1\na = 5 & 4\n",
			wantErr: ParseError{
				Location: Location{Start: Point{Row: 2, Col: 7}, End: Point{Row: 2, Col: 7}},
				Message:  "Unexpected '&' after expression; Ard has no bitwise '&' operator",
			},
		},
		{
			name:  "& after an expression statement",
			input: "fn f() Int { 1 }\nf() & 4\n",
			wantErr: ParseError{
				Location: Location{Start: Point{Row: 2, Col: 5}, End: Point{Row: 2, Col: 5}},
				Message:  "Unexpected '&' after expression; Ard has no bitwise '&' operator",
			},
		},
		{
			name:  "juxtaposed literal after a variable value",
			input: "let a = 5 4\n",
			wantErr: ParseError{
				Location: Location{Start: Point{Row: 1, Col: 11}, End: Point{Row: 1, Col: 11}},
				Message:  "Expected new line after statement",
			},
		},
		{
			name:  "juxtaposed list after a variable value",
			input: "let a = 5 [4]\n",
			wantErr: ParseError{
				Location: Location{Start: Point{Row: 1, Col: 11}, End: Point{Row: 1, Col: 11}},
				Message:  "Expected new line after statement",
			},
		},
		{
			name:  "second declaration on the same line",
			input: "let a = 5 let b = 3\n",
			wantErr: ParseError{
				Location: Location{Start: Point{Row: 1, Col: 11}, End: Point{Row: 1, Col: 11}},
				Message:  "Expected new line after statement",
			},
		},
		{
			name:  "& after a for-loop update",
			input: "for mut i = 0; i < 3; i =+ 1 & 2 {\n}\n",
			wantErr: ParseError{
				Location: Location{Start: Point{Row: 1, Col: 30}, End: Point{Row: 1, Col: 30}},
				Message:  "Unexpected '&' after expression; Ard has no bitwise '&' operator",
			},
		},
		{
			name:  "stray tokens inside a single-line block",
			input: "fn f() Int { 5 & 4 }\n",
			wantErr: ParseError{
				Location: Location{Start: Point{Row: 1, Col: 16}, End: Point{Row: 1, Col: 16}},
				Message:  "Unexpected '&' after expression; Ard has no bitwise '&' operator",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) != 1 {
				t.Fatalf("errors = %v, want exactly [%v]", result.Errors, tt.wantErr)
			}
			if result.Errors[0] != tt.wantErr {
				t.Fatalf("error = %v, want %v", result.Errors[0], tt.wantErr)
			}
		})
	}
}

// Recovery skips the rest of the offending line only, so the following
// statements and the enclosing block still parse.
func TestStatementEndRecoveryKeepsFollowingStatements(t *testing.T) {
	result := Parse([]byte("fn main() {\n  let a = 5 & (4 + 1)\n  let b = a\n}\n"), "test.ard")
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %v, want exactly one", result.Errors)
	}
	fn := result.Program.Statements[0].(*FunctionDeclaration)
	if len(fn.Body) != 2 {
		t.Fatalf("body = %#v, want the two declarations", fn.Body)
	}
	if decl := fn.Body[1].(*VariableDeclaration); decl.Name != "b" {
		t.Fatalf("second statement = %#v, want `let b`", decl)
	}
}

func TestStatementTerminators(t *testing.T) {
	for name, input := range map[string]string{
		"new line":                 "let a = 5\nlet b = 4\n",
		"end of file":              "let a = 5",
		"trailing comment":         "let a = 5 // five\n",
		"closing brace":            "fn f() Int { 5 }\n",
		"assignment closing brace": "mut a = 1\nif true { a = 2 }\n",
		"c-style for loop header":  "for mut i = 0; i < 3; i =+ 1 {\n}\n",
		"for loop header with ';'": "for mut i = 0; i < 3; i =+ 1; {\n}\n",
		"address-of on a new line": "mut b = 1\nlet a = 5\n&mut b\n",
		// The checker, not the parser, rejects a non-reassignment update.
		"declaration as for loop update": "for mut i = 0; i < 3; let x = 1 {\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			parseOK(t, input)
		})
	}
}

// A brace opened by the stray tokens is skipped through its matching close,
// even across lines, so it isn't mistaken for the enclosing block's close.
func TestStatementEndRecoverySkipsMultilineStrayBlock(t *testing.T) {
	result := Parse([]byte("fn main() {\n  let x = 1 junk {\n    let y = 2\n  }\n  let z = 3\n}\n"), "test.ard")
	want := ParseError{
		Location: Location{Start: Point{Row: 2, Col: 13}, End: Point{Row: 2, Col: 16}},
		Message:  "Expected new line after statement",
	}
	if len(result.Errors) != 1 || result.Errors[0] != want {
		t.Fatalf("errors = %v, want [%v]", result.Errors, want)
	}
	fn := result.Program.Statements[0].(*FunctionDeclaration)
	if len(fn.Body) != 2 {
		t.Fatalf("body = %#v, want `let x` and `let z`", fn.Body)
	}
	if decl := fn.Body[1].(*VariableDeclaration); decl.Name != "z" {
		t.Fatalf("second statement = %#v, want `let z`", decl)
	}
}

// The C-style for-loop header ends at its body's `{`, so a comment cannot
// follow the update.
func TestForLoopUpdateRejectsTrailingComment(t *testing.T) {
	for name, tt := range map[string]struct {
		input string
		col   int
	}{
		"body on the next line": {"for mut i = 0; i < 3; i =+ 1 // update\n{\n}\n", 30},
		"no body":               {"for mut i = 0; i < 3; i =+ 1 // update\n", 30},
		"after optional ';'":    {"for mut i = 0; i < 3; i =+ 1; // update\n{\n}\n", 31},
	} {
		t.Run(name, func(t *testing.T) {
			result := Parse([]byte(tt.input), "test.ard")
			want := ParseError{
				Location: Location{Start: Point{Row: 1, Col: tt.col}, End: Point{Row: 1, Col: tt.col + 8}},
				Message:  "Comments are not allowed after a for-loop update",
			}
			if len(result.Errors) == 0 || result.Errors[0] != want {
				t.Fatalf("errors = %v, want first [%v]", result.Errors, want)
			}
		})
	}
	t.Run("recovery parses the body", func(t *testing.T) {
		result := Parse([]byte("for mut i = 0; i < 3; i =+ 1 // update\n{\n  i\n}\n"), "test.ard")
		if len(result.Errors) != 1 {
			t.Fatalf("errors = %v, want exactly one", result.Errors)
		}
		loop := result.Program.Statements[0].(*ForLoop)
		if len(loop.Body) != 1 {
			t.Fatalf("body = %#v, want one statement", loop.Body)
		}
	})
}
