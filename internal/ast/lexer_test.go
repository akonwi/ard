package ast

import (
	"strings"
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
			lexer := newLexer([]byte(test.input))
			lexer.scan()

			diff := cmp.Diff(test.want, lexer.tokens, compareOptions)
			if diff != "" {
				t.Errorf("Tokens do not match (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLexing(t *testing.T) {
	runLexing(t, []lexTest{
		{
			name: "comments",
			input: strings.Join([]string{
				"// this is a comment",
				"/*",
				"this is a comment",
				"*/",
			}, "\n"),
			want: []token{
				{kind: slash_slash, line: 1, column: 1},
				{kind: identifier, line: 1, column: 4, text: "this"},
				{kind: identifier, line: 1, column: 9, text: "is"},
				{kind: identifier, line: 1, column: 12, text: "a"},
				{kind: identifier, line: 1, column: 14, text: "comment"},

				{kind: slash_star, line: 2, column: 1},
				{kind: identifier, line: 3, column: 1, text: "this"},
				{kind: identifier, line: 3, column: 6, text: "is"},
				{kind: identifier, line: 3, column: 9, text: "a"},
				{kind: identifier, line: 3, column: 11, text: "comment"},
				{kind: star_slash, line: 4, column: 1},

				{kind: eof},
			},
		},

		{
			name: "variables",
			input: strings.Join([]string{
				"mut x = 5",
				"let y = 10",
				`let string: Str = "hello"`,
				"x = 20",
			}, "\n"),
			want: []token{
				{kind: mut, line: 1, column: 1},
				{kind: identifier, line: 1, column: 5, text: "x"},
				{kind: equal, line: 1, column: 7},
				{kind: number, line: 1, column: 9, text: "5"},

				{kind: let, line: 2, column: 1},
				{kind: identifier, line: 2, column: 5, text: "y"},
				{kind: equal, line: 2, column: 7},
				{kind: number, line: 2, column: 9, text: "10"},

				{kind: let, line: 3, column: 1},
				{kind: identifier, line: 3, column: 5, text: "string"},
				{kind: colon, line: 3, column: 11},
				{kind: identifier, line: 3, column: 13, text: "Str"},
				{kind: equal, line: 3, column: 17},
				{kind: string_, line: 3, column: 19, text: `"hello"`},

				{kind: identifier, line: 4, column: 1, text: "x"},
				{kind: equal, line: 4, column: 3},
				{kind: number, line: 4, column: 5, text: "20"},

				{kind: eof},
			},
		},

		{
			name: "if statements",
			input: strings.Join([]string{
				"if not is_true {}",
				"",
				"if age > 18 or something_else {}",
				"else if age == 18 {",
				"} else {}",
			}, "\n"),
			want: []token{
				{kind: if_, line: 1, column: 1},
				{kind: not, line: 1, column: 4},
				{kind: identifier, line: 1, column: 8, text: "is_true"},
				{kind: left_brace, line: 1, column: 16},
				{kind: right_brace, line: 1, column: 17},

				{kind: if_, line: 3, column: 1},
				{kind: identifier, line: 3, column: 4, text: "age"},
				{kind: greater_than, line: 3, column: 8},
				{kind: number, line: 3, column: 10, text: "18"},
				{kind: or, line: 3, column: 13},
				{kind: identifier, line: 3, column: 16, text: "something_else"},
				{kind: left_brace, line: 3, column: 31},
				{kind: right_brace, line: 3, column: 32},

				{kind: else_, line: 4, column: 1},
				{kind: if_, line: 4, column: 6},
				{kind: identifier, line: 4, column: 9, text: "age"},
				{kind: equal_equal, line: 4, column: 13},
				{kind: number, line: 4, column: 16, text: "18"},
				{kind: left_brace, line: 4, column: 19},
				{kind: right_brace, line: 5, column: 1},
				{kind: else_, line: 5, column: 3},
				{kind: left_brace, line: 5, column: 8},
				{kind: right_brace, line: 5, column: 9},

				{kind: eof},
			},
		},

		{
			name: "function: empty params and no return type",
			input: strings.Join([]string{
				"fn get_hello() {",
				`  "Hello, world!"`,
				"}",
				"",
				"get_hello()",
			}, "\n"),
			want: []token{
				{kind: fn, line: 1, column: 1},
				{kind: identifier, line: 1, column: 4, text: "get_hello"},
				{kind: left_paren, line: 1, column: 13},
				{kind: right_paren, line: 1, column: 14},
				{kind: left_brace, line: 1, column: 16},

				{kind: string_, line: 2, column: 3, text: `"Hello, world!"`},

				{kind: right_brace, line: 3, column: 1},

				{kind: identifier, line: 5, column: 1, text: "get_hello"},
				{kind: left_paren, line: 5, column: 10},
				{kind: right_paren, line: 5, column: 11},

				{kind: eof},
			},
		},
		{
			name: "function: one param and return type",
			input: strings.Join([]string{
				`fn greet(person: Str) Str {`,
				`  "Hello, {{person}}!"`,
				`}`,
				``,
				`greet("Alice")`,
				`greet(get_hello())`,
			}, "\n"),
			want: []token{
				{kind: fn, line: 1, column: 1},
				{kind: identifier, line: 1, column: 4, text: "greet"},
				{kind: left_paren, line: 1, column: 9},
				{kind: identifier, line: 1, column: 10, text: "person"},
				{kind: colon, line: 1, column: 16},
				{kind: identifier, line: 1, column: 18, text: "Str"},
				{kind: right_paren, line: 1, column: 21},
				{kind: identifier, line: 1, column: 23, text: "Str"},
				{kind: left_brace, line: 1, column: 27},

				{kind: string_, line: 2, column: 3, text: `"Hello, {{person}}!"`},

				{kind: right_brace, line: 3, column: 1},

				{kind: identifier, line: 5, column: 1, text: "greet"},
				{kind: left_paren, line: 5, column: 6},
				{kind: string_, line: 5, column: 7, text: `"Alice"`},
				{kind: right_paren, line: 5, column: 14},

				{kind: identifier, line: 6, column: 1, text: "greet"},
				{kind: left_paren, line: 6, column: 6},
				{kind: identifier, line: 6, column: 7, text: "get_hello"},
				{kind: left_paren, line: 6, column: 16},
				{kind: right_paren, line: 6, column: 17},
				{kind: right_paren, line: 6, column: 18},

				{kind: eof},
			},
		},
		{
			name: "function: multiple params and return type",
			input: strings.Join([]string{
				"fn add(x: Int, y: Int) Int {",
				"  x + y",
				"}",
				"add(1, 2)",
			}, "\n"),
			want: []token{
				{kind: fn, line: 1, column: 1},
				{kind: identifier, line: 1, column: 4, text: "add"},
				{kind: left_paren, line: 1, column: 7},
				{kind: identifier, line: 1, column: 8, text: "x"},
				{kind: colon, line: 1, column: 9},
				{kind: identifier, line: 1, column: 11, text: "Int"},
				{kind: comma, line: 1, column: 14},
				{kind: identifier, line: 1, column: 16, text: "y"},
				{kind: colon, line: 1, column: 17},
				{kind: identifier, line: 1, column: 19, text: "Int"},
				{kind: right_paren, line: 1, column: 22},
				{kind: identifier, line: 1, column: 24, text: "Int"},
				{kind: left_brace, line: 1, column: 28},

				{kind: identifier, line: 2, column: 3, text: "x"},
				{kind: plus, line: 2, column: 5},
				{kind: identifier, line: 2, column: 7, text: "y"},

				{kind: right_brace, line: 3, column: 1},

				{kind: identifier, line: 4, column: 1, text: "add"},
				{kind: left_paren, line: 4, column: 4},
				{kind: number, line: 4, column: 5, text: "1"},
				{kind: comma, line: 4, column: 6},
				{kind: number, line: 4, column: 8, text: "2"},
				{kind: right_paren, line: 4, column: 9},

				{kind: eof},
			},
		},
		{
			name: "function: anonymous functions",
			input: strings.Join([]string{
				"() {",
				`  print("Hello, anon!")`,
				"}",
				"",
				"(n: Int) Int {",
				"  do_stuff()",
				"  n + 1",
				"}",
				"",
				"list.map((n) { n + 1 })",
			}, "\n"),
			want: []token{
				{kind: left_paren, line: 1, column: 1},
				{kind: right_paren, line: 1, column: 2},
				{kind: left_brace, line: 1, column: 4},

				{kind: identifier, line: 2, column: 3, text: "print"},
				{kind: left_paren, line: 2, column: 8},
				{kind: string_, line: 2, column: 9, text: `"Hello, anon!"`},
				{kind: right_paren, line: 2, column: 23},

				{kind: right_brace, line: 3, column: 1},

				{kind: left_paren, line: 5, column: 1},
				{kind: identifier, line: 5, column: 2, text: "n"},
				{kind: colon, line: 5, column: 3},
				{kind: identifier, line: 5, column: 5, text: "Int"},
				{kind: right_paren, line: 5, column: 8},
				{kind: identifier, line: 5, column: 10, text: "Int"},
				{kind: left_brace, line: 5, column: 14},

				{kind: identifier, line: 6, column: 3, text: "do_stuff"},
				{kind: left_paren, line: 6, column: 11},
				{kind: right_paren, line: 6, column: 12},

				{kind: identifier, line: 7, column: 3, text: "n"},
				{kind: plus, line: 7, column: 5},
				{kind: number, line: 7, column: 7, text: "1"},

				{kind: right_brace, line: 8, column: 1},

				{kind: identifier, line: 10, column: 1, text: "list"},
				{kind: dot, line: 10, column: 5},
				{kind: identifier, line: 10, column: 6, text: "map"},
				{kind: left_paren, line: 10, column: 9},
				{kind: left_paren, line: 10, column: 10},
				{kind: identifier, line: 10, column: 11, text: "n"},
				{kind: right_paren, line: 10, column: 12},
				{kind: left_brace, line: 10, column: 14},
				{kind: identifier, line: 10, column: 16, text: "n"},
				{kind: plus, line: 10, column: 18},
				{kind: number, line: 10, column: 20, text: "1"},
				{kind: right_brace, line: 10, column: 22},
				{kind: right_paren, line: 10, column: 23},

				{kind: eof},
			},
		},
		{
			name:  "function: mutable parameters",
			input: "fn grow(mut t: Thing, size: Int) Int {}",
			want: []token{
				{kind: fn, line: 1, column: 1},
				{kind: identifier, line: 1, column: 4, text: "grow"},
				{kind: left_paren, line: 1, column: 8},
				{kind: mut, line: 1, column: 9},
				{kind: identifier, line: 1, column: 13, text: "t"},
				{kind: colon, line: 1, column: 14},
				{kind: identifier, line: 1, column: 16, text: "Thing"},
				{kind: comma, line: 1, column: 21},
				{kind: identifier, line: 1, column: 23, text: "size"},
				{kind: colon, line: 1, column: 27},
				{kind: identifier, line: 1, column: 29, text: "Int"},
				{kind: right_paren, line: 1, column: 32},
				{kind: identifier, line: 1, column: 34, text: "Int"},
				{kind: left_brace, line: 1, column: 38},
				{kind: right_brace, line: 1, column: 39},
				{kind: eof},
			},
		},

		{
			name: "enums",
			input: `
enum Payload {
  Plain,
  Rich
}

let data = Payload::Plain`,
			want: []token{
				{kind: enum, line: 2, column: 1},
				{kind: identifier, line: 2, column: 6, text: "Payload"},
				{kind: left_brace, line: 2, column: 14},
				{kind: identifier, line: 3, column: 3, text: "Plain"},
				{kind: comma, line: 3, column: 8},
				{kind: identifier, line: 4, column: 3, text: "Rich"},
				{kind: right_brace, line: 5, column: 1},

				{kind: let, line: 7, column: 1},
				{kind: identifier, line: 7, column: 5, text: "data"},
				{kind: equal, line: 7, column: 10},
				{kind: identifier, line: 7, column: 12, text: "Payload"},
				{kind: colon_colon, line: 7, column: 19},
				{kind: identifier, line: 7, column: 21, text: "Plain"},

				{kind: eof},
			},
		},

		{
			name: "imports",
			input: strings.Join([]string{
				"use ard/io",
				"use github.com/foo/bar",
				"use maybe as option",
			}, "\n"),
			want: []token{
				{kind: use, line: 1, column: 1},
				{kind: identifier, line: 1, column: 5, text: "ard"},
				{kind: slash, line: 1, column: 8},
				{kind: identifier, line: 1, column: 9, text: "io"},

				{kind: use, line: 2, column: 1},
				{kind: identifier, line: 2, column: 5, text: "github"},
				{kind: dot, line: 2, column: 11},
				{kind: identifier, line: 2, column: 12, text: "com"},
				{kind: slash, line: 2, column: 15},
				{kind: identifier, line: 2, column: 16, text: "foo"},
				{kind: slash, line: 2, column: 19},
				{kind: identifier, line: 2, column: 20, text: "bar"},

				{kind: use, line: 3, column: 1},
				{kind: identifier, line: 3, column: 5, text: "maybe"},
				{kind: as, line: 3, column: 11},
				{kind: identifier, line: 3, column: 14, text: "option"},

				{kind: eof},
			},
		},
	})
}
