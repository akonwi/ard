package parse

import (
	"strings"
	"testing"
)

// A member after `.` or `::` must be a name. The parser used to parse any
// primary expression there and silently drop it when it was not a name, so
// `pair.0` parsed as `pair` (#606).
func TestMemberAccessRequiresName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "tuple position zero", input: `let x = pair.0`, wantErr: "Expected a field or method name after '.'; Ard has no positional access like `.0`. Read a tuple by destructuring it (`let first, second = pair`) or with `match`"},
		{name: "tuple position one", input: `let x = pair.1`, wantErr: "Ard has no positional access like `.1`"},
		{name: "call through position", input: `let x = fns.0(10)`, wantErr: "(`let first, second = fns`)"},
		{name: "number on non-tuple receiver", input: `let x = n.3`, wantErr: "Expected a field or method name after '.'"},
		{name: "nested positions lex as a float", input: `let x = pair.0.1`, wantErr: "Ard has no positional access like `.0.1`"},
		{name: "position on a call result", input: `let x = get_pair().0`, wantErr: "(`let first, second = tuple`)"},
		{name: "position after newline chain", input: "let x = pair\n  .0", wantErr: "Ard has no positional access like `.0`"},
		{name: "string after dot", input: `let x = pair."a"`, wantErr: "Expected a field or method name after '.'"},
		{name: "parenthesized after dot", input: `let x = pair.(a)`, wantErr: "Expected a field or method name after '.'"},
		{name: "list after dot", input: `let x = pair.[a]`, wantErr: "Expected a field or method name after '.'"},
		{name: "struct literal after dot", input: `let x = a.Foo{b: 1}`, wantErr: "Expected a field or method name after '.'"},
		{name: "number after static path", input: `let x = pair::0`, wantErr: "Expected a name after '::'"},
		{name: "string after static path", input: `let x = pair::"a"`, wantErr: "Expected a name after '::'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) != 1 {
				t.Fatalf("expected 1 error, got %d: %v", len(result.Errors), result.Errors)
			}
			if !strings.Contains(result.Errors[0].Message, tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, result.Errors[0].Message)
			}
		})
	}
}

func TestNumbersAndRangesStillParse(t *testing.T) {
	runTests(t, []test{
		{
			name:  "float literal",
			input: `let x = 1.0`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{Name: "x", Value: &NumLiteral{Value: "1.0"}},
			}},
		},
		{
			name:  "method on float literal",
			input: `let x = 1.5.to_str()`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{Name: "x", Value: &InstanceMethod{
					Target: &NumLiteral{Value: "1.5"},
					Method: FunctionCall{Name: "to_str", Args: []Argument{}, Comments: []Comment{}},
				}},
			}},
		},
		{
			name:  "range of literals",
			input: `for i in 0..10 {}`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&RangeLoop{
					Cursor: Identifier{Name: "i"},
					Start:  &NumLiteral{Value: "0"},
					End:    &NumLiteral{Value: "10"},
					Body:   []Statement{},
				},
			}},
		},
		{
			name:  "range from a field",
			input: `for i in r.low..r.high {}`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&RangeLoop{
					Cursor: Identifier{Name: "i"},
					Start:  &InstanceProperty{Target: &Identifier{Name: "r"}, Property: Identifier{Name: "low"}},
					End:    &InstanceProperty{Target: &Identifier{Name: "r"}, Property: Identifier{Name: "high"}},
					Body:   []Statement{},
				},
			}},
		},
		{
			name:  "keyword member name",
			input: `let x = value.match`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{Name: "x", Value: &InstanceProperty{
					Target:   &Identifier{Name: "value"},
					Property: Identifier{Name: "match"},
				}},
			}},
		},
	})
}

func TestNumericMemberErrorPointsAtTheNumber(t *testing.T) {
	result := Parse([]byte("let x = fns.10(1)"), "test.ard")
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(result.Errors), result.Errors)
	}
	want := Location{Start: Point{Row: 1, Col: 13}, End: Point{Row: 1, Col: 14}}
	if got := result.Errors[0].Location; got != want {
		t.Fatalf("expected error at %v, got %v", want, got)
	}
}

// A member that is not even an expression gets one diagnostic at the member,
// not an "Unexpected token" error followed by the member error.
func TestMissingMemberReportsOnce(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "dot before newline", input: "let x = pair.\n", want: []string{"Expected a field or method name after '.'"}},
		{name: "dot at end of input", input: "let x = pair.", want: []string{"Expected a field or method name after '.'"}},
		{name: "static path before newline", input: "let x = m::\n", want: []string{"Expected a name after '::'"}},
		{
			name:  "unclosed call keeps its own error",
			input: "let x = pair.0(\n",
			want: []string{
				"Expected ')' to close function call",
				"Expected ')' to close function call",
				"Ard has no positional access like `.0`",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) != len(tt.want) {
				t.Fatalf("expected %d errors, got %d: %v", len(tt.want), len(result.Errors), result.Errors)
			}
			for i, want := range tt.want {
				if !strings.Contains(result.Errors[i].Message, want) {
					t.Fatalf("expected error %d containing %q, got %q", i, want, result.Errors[i].Message)
				}
			}
		})
	}
}

func TestParsingContinuesAfterNumericMember(t *testing.T) {
	result := Parse([]byte("let x = pair.0\nlet y = pair.1\n"), "test.ard")
	if len(result.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(result.Errors), result.Errors)
	}
	if len(result.Program.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(result.Program.Statements))
	}
}
