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
			lexer := NewLexer([]byte(test.input))
			diff := cmp.Diff(test.want, lexer.Scan(), compareOptions)
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
				{kind: comment, line: 1, column: 1, text: "// this is a comment"},
				{kind: new_line, line: 1, column: 21},

				{kind: block_comment, line: 2, column: 1, text: "/*\nthis is a comment\n*/"},

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
				"mut temp: Float = 98.6",
			}, "\n"),
			want: []token{
				{kind: mut, line: 1, column: 1},
				{kind: identifier, line: 1, column: 5, text: "x"},
				{kind: equal, line: 1, column: 7},
				{kind: number, line: 1, column: 9, text: "5"},
				{kind: new_line, line: 1, column: 10},

				{kind: let, line: 2, column: 1},
				{kind: identifier, line: 2, column: 5, text: "y"},
				{kind: equal, line: 2, column: 7},
				{kind: number, line: 2, column: 9, text: "10"},
				{kind: new_line, line: 2, column: 11},

				{kind: let, line: 3, column: 1},
				{kind: identifier, line: 3, column: 5, text: "string"},
				{kind: colon, line: 3, column: 11},
				{kind: identifier, line: 3, column: 13, text: "Str"},
				{kind: equal, line: 3, column: 17},
				{kind: string_, line: 3, column: 19, text: `hello`},
				{kind: new_line, line: 3, column: 26},

				{kind: identifier, line: 4, column: 1, text: "x"},
				{kind: equal, line: 4, column: 3},
				{kind: number, line: 4, column: 5, text: "20"},
				{kind: new_line, line: 4, column: 7},

				{kind: mut, line: 5, column: 1},
				{kind: identifier, line: 5, column: 5, text: "temp"},
				{kind: colon, line: 5, column: 9},
				{kind: identifier, line: 5, column: 11, text: "Float"},
				{kind: equal, line: 5, column: 17},
				{kind: number, line: 5, column: 19, text: "98.6"},

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
				{kind: new_line, line: 1, column: 18},
				{kind: new_line, line: 2, column: 1},

				{kind: if_, line: 3, column: 1},
				{kind: identifier, line: 3, column: 4, text: "age"},
				{kind: greater_than, line: 3, column: 8},
				{kind: number, line: 3, column: 10, text: "18"},
				{kind: or, line: 3, column: 13},
				{kind: identifier, line: 3, column: 16, text: "something_else"},
				{kind: left_brace, line: 3, column: 31},
				{kind: right_brace, line: 3, column: 32},
				{kind: new_line, line: 3, column: 33},

				{kind: else_, line: 4, column: 1},
				{kind: if_, line: 4, column: 6},
				{kind: identifier, line: 4, column: 9, text: "age"},
				{kind: equal_equal, line: 4, column: 13},
				{kind: number, line: 4, column: 16, text: "18"},
				{kind: left_brace, line: 4, column: 19},
				{kind: new_line, line: 4, column: 20},
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
				{kind: new_line, line: 1, column: 17},

				{kind: string_, line: 2, column: 3, text: "Hello, world!"},
				{kind: new_line, line: 2, column: 18},

				{kind: right_brace, line: 3, column: 1},
				{kind: new_line, line: 3, column: 2},
				{kind: new_line, line: 4, column: 1},

				{kind: identifier, line: 5, column: 1, text: "get_hello"},
				{kind: left_paren, line: 5, column: 10},
				{kind: right_paren, line: 5, column: 11},

				{kind: eof},
			},
		},
		{
			name: "function: one param and return type",
			input: strings.Join([]string{
				`fn greet(person: Str) Str {}`,
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
				{kind: right_brace, line: 1, column: 28},
				{kind: new_line, line: 1, column: 29},
				{kind: new_line, line: 2, column: 1},

				{kind: identifier, line: 3, column: 1, text: "greet"},
				{kind: left_paren, line: 3, column: 6},
				{kind: string_, line: 3, column: 7, text: "Alice"},
				{kind: right_paren, line: 3, column: 14},
				{kind: new_line, line: 3, column: 15},

				{kind: identifier, line: 4, column: 1, text: "greet"},
				{kind: left_paren, line: 4, column: 6},
				{kind: identifier, line: 4, column: 7, text: "get_hello"},
				{kind: left_paren, line: 4, column: 16},
				{kind: right_paren, line: 4, column: 17},
				{kind: right_paren, line: 4, column: 18},

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
				{kind: new_line, line: 1, column: 29},

				{kind: identifier, line: 2, column: 3, text: "x"},
				{kind: plus, line: 2, column: 5},
				{kind: identifier, line: 2, column: 7, text: "y"},
				{kind: new_line, line: 2, column: 8},

				{kind: right_brace, line: 3, column: 1},
				{kind: new_line, line: 3, column: 2},

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
				{kind: new_line, line: 1, column: 5},

				{kind: identifier, line: 2, column: 3, text: "print"},
				{kind: left_paren, line: 2, column: 8},
				{kind: string_, line: 2, column: 9, text: "Hello, anon!"},
				{kind: right_paren, line: 2, column: 23},
				{kind: new_line, line: 2, column: 24},

				{kind: right_brace, line: 3, column: 1},
				{kind: new_line, line: 3, column: 2},
				{kind: new_line, line: 4, column: 1},

				{kind: left_paren, line: 5, column: 1},
				{kind: identifier, line: 5, column: 2, text: "n"},
				{kind: colon, line: 5, column: 3},
				{kind: identifier, line: 5, column: 5, text: "Int"},
				{kind: right_paren, line: 5, column: 8},
				{kind: identifier, line: 5, column: 10, text: "Int"},
				{kind: left_brace, line: 5, column: 14},
				{kind: new_line, line: 5, column: 15},

				{kind: identifier, line: 6, column: 3, text: "do_stuff"},
				{kind: left_paren, line: 6, column: 11},
				{kind: right_paren, line: 6, column: 12},
				{kind: new_line, line: 6, column: 13},

				{kind: identifier, line: 7, column: 3, text: "n"},
				{kind: plus, line: 7, column: 5},
				{kind: number, line: 7, column: 7, text: "1"},
				{kind: new_line, line: 7, column: 8},

				{kind: right_brace, line: 8, column: 1},
				{kind: new_line, line: 8, column: 2},
				{kind: new_line, line: 9, column: 1},

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
			input: strings.Join([]string{
				`enum Payload {`,
				`  Plain,`,
				`  Rich`,
				`}`,
				`let data = Payload::Plain`,
			}, "\n"),
			want: []token{
				{kind: enum, line: 1, column: 1},
				{kind: identifier, line: 1, column: 6, text: "Payload"},
				{kind: left_brace, line: 1, column: 14},
				{kind: new_line, line: 1, column: 15},

				{kind: identifier, line: 2, column: 3, text: "Plain"},
				{kind: comma, line: 2, column: 8},
				{kind: new_line, line: 2, column: 9},

				{kind: identifier, line: 3, column: 3, text: "Rich"},
				{kind: new_line, line: 3, column: 7},

				{kind: right_brace, line: 4, column: 1},
				{kind: new_line, line: 4, column: 2},

				{kind: let, line: 5, column: 1},
				{kind: identifier, line: 5, column: 5, text: "data"},
				{kind: equal, line: 5, column: 10},
				{kind: identifier, line: 5, column: 12, text: "Payload"},
				{kind: colon_colon, line: 5, column: 19},
				{kind: identifier, line: 5, column: 21, text: "Plain"},

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
				{kind: path, line: 1, column: 5, text: "ard/io"},
				{kind: new_line, line: 1, column: 11},

				{kind: use, line: 2, column: 1},
				{kind: path, line: 2, column: 5, text: "github.com/foo/bar"},
				{kind: new_line, line: 2, column: 23},

				{kind: use, line: 3, column: 1},
				{kind: path, line: 3, column: 5, text: "maybe"},
				{kind: as, line: 3, column: 11},
				{kind: identifier, line: 3, column: 14, text: "option"},

				{kind: eof},
			},
		},

		{
			name:  "for integer range",
			input: `for i in 1..10 {}`,
			want: []token{
				{kind: for_, line: 1, column: 1},
				{kind: identifier, line: 1, column: 5, text: "i"},
				{kind: in, line: 1, column: 7},
				{kind: number, line: 1, column: 10, text: "1"},
				{kind: dot_dot, line: 1, column: 11},
				{kind: number, line: 1, column: 13, text: "10"},
				{kind: left_brace, line: 1, column: 16},
				{kind: right_brace, line: 1, column: 17},
				{kind: eof},
			},
		},

		{
			name: "for loop",
			input: strings.Join([]string{
				"for mut count = 0; count <= 9; count = count + 1 {",
				"  foobar",
				"}",
			}, "\n"),
			want: []token{
				{kind: for_, line: 1, column: 1},
				{kind: mut, line: 1, column: 5},
				{kind: identifier, line: 1, column: 9, text: "count"},
				{kind: equal, line: 1, column: 15},
				{kind: number, line: 1, column: 17, text: "0"},
				{kind: semicolon, line: 1, column: 18},
				{kind: identifier, line: 1, column: 20, text: "count"},
				{kind: less_than_equal, line: 1, column: 26},
				{kind: number, line: 1, column: 29, text: "9"},
				{kind: semicolon, line: 1, column: 30},
				{kind: identifier, line: 1, column: 32, text: "count"},
				{kind: equal, line: 1, column: 38},
				{kind: identifier, line: 1, column: 40, text: "count"},
				{kind: plus, line: 1, column: 46},
				{kind: number, line: 1, column: 48, text: "1"},
				{kind: left_brace, line: 1, column: 50},
				{kind: new_line, line: 1, column: 51},
				{kind: identifier, line: 2, column: 3, text: "foobar"},
				{kind: new_line, line: 2, column: 9},
				{kind: right_brace, line: 3, column: 1},
				{kind: eof},
			},
		},

		{
			name:  "while loops",
			input: `while count <= 9 { count =+ 1 }`,
			want: []token{
				{kind: while_, line: 1, column: 1},
				{kind: identifier, line: 1, column: 7, text: "count"},
				{kind: less_than_equal, line: 1, column: 13},
				{kind: number, line: 1, column: 16, text: "9"},
				{kind: left_brace, line: 1, column: 18},
				{kind: identifier, line: 1, column: 20, text: "count"},
				{kind: increment, line: 1, column: 26},
				{kind: number, line: 1, column: 29, text: "1"},
				{kind: right_brace, line: 1, column: 31},
				{kind: eof},
			},
		},

		{
			name: "lists",
			input: strings.Join([]string{
				"let empty: [Int] = []",
				"let numbers = [1, 2, 3]",
			}, "\n"),
			want: []token{
				{kind: let, line: 1, column: 1},
				{kind: identifier, line: 1, column: 5, text: "empty"},
				{kind: colon, line: 1, column: 10},
				{kind: left_bracket, line: 1, column: 12},
				{kind: identifier, line: 1, column: 13, text: "Int"},
				{kind: right_bracket, line: 1, column: 16},
				{kind: equal, line: 1, column: 18},
				{kind: left_bracket, line: 1, column: 20},
				{kind: right_bracket, line: 1, column: 21},
				{kind: new_line, line: 1, column: 22},

				{kind: let, line: 2, column: 1},
				{kind: identifier, line: 2, column: 5, text: "numbers"},
				{kind: equal, line: 2, column: 13},
				{kind: left_bracket, line: 2, column: 15},
				{kind: number, line: 2, column: 16, text: "1"},
				{kind: comma, line: 2, column: 17},
				{kind: number, line: 2, column: 19, text: "2"},
				{kind: comma, line: 2, column: 20},
				{kind: number, line: 2, column: 22, text: "3"},
				{kind: right_bracket, line: 2, column: 23},

				{kind: eof},
			},
		},

		{
			name: "maps",
			input: strings.Join([]string{
				"let empty: [Str:Int] = [:]",
				`let initialized: [Str:Bool] = ["John": true, "Jane": false,]`,
			}, "\n"),
			want: []token{
				{kind: let, line: 1, column: 1},
				{kind: identifier, line: 1, column: 5, text: "empty"},
				{kind: colon, line: 1, column: 10},
				{kind: left_bracket, line: 1, column: 12},
				{kind: identifier, line: 1, column: 13, text: "Str"},
				{kind: colon, line: 1, column: 16},
				{kind: identifier, line: 1, column: 17, text: "Int"},
				{kind: right_bracket, line: 1, column: 20},
				{kind: equal, line: 1, column: 22},
				{kind: left_bracket, line: 1, column: 24},
				{kind: colon, line: 1, column: 25},
				{kind: right_bracket, line: 1, column: 26},
				{kind: new_line, line: 1, column: 27},

				{kind: let, line: 2, column: 1},
				{kind: identifier, line: 2, column: 5, text: "initialized"},
				{kind: colon, line: 2, column: 16},
				{kind: left_bracket, line: 2, column: 18},
				{kind: identifier, line: 2, column: 19, text: "Str"},
				{kind: colon, line: 2, column: 22},
				{kind: identifier, line: 2, column: 23, text: "Bool"},
				{kind: right_bracket, line: 2, column: 27},
				{kind: equal, line: 2, column: 29},
				{kind: left_bracket, line: 2, column: 31},
				{kind: string_, line: 2, column: 32, text: "John"},
				{kind: colon, line: 2, column: 38},
				{kind: true_, line: 2, column: 40, text: "true"},
				{kind: comma, line: 2, column: 44},
				{kind: string_, line: 2, column: 46, text: "Jane"},
				{kind: colon, line: 2, column: 52},
				{kind: false_, line: 2, column: 54, text: "false"},
				{kind: comma, line: 2, column: 59},
				{kind: right_bracket, line: 2, column: 60},

				{kind: eof},
			},
		},

		{
			name: "matching",
			input: strings.Join([]string{
				"match payload {",
				"  Payload::Plain => print(\"Plain text\"),",
				"  Payload::Rich => {",
				"    // block",
				"    print(\"Rich text\")",
				"  },",
				"  _ => print(\"Unknown\")",
				"}",
			}, "\n"),
			want: []token{
				{kind: match, line: 1, column: 1},
				{kind: identifier, line: 1, column: 7, text: "payload"},
				{kind: left_brace, line: 1, column: 15},
				{kind: new_line, line: 1, column: 16},

				{kind: identifier, line: 2, column: 3, text: "Payload"},
				{kind: colon_colon, line: 2, column: 10},
				{kind: identifier, line: 2, column: 12, text: "Plain"},
				{kind: fat_arrow, line: 2, column: 18},
				{kind: identifier, line: 2, column: 21, text: "print"},
				{kind: left_paren, line: 2, column: 26},
				{kind: string_, line: 2, column: 27, text: "Plain text"},
				{kind: right_paren, line: 2, column: 39},
				{kind: comma, line: 2, column: 40},
				{kind: new_line, line: 2, column: 41},

				{kind: identifier, line: 3, column: 3, text: "Payload"},
				{kind: colon_colon, line: 3, column: 10},
				{kind: identifier, line: 3, column: 12, text: "Rich"},
				{kind: fat_arrow, line: 3, column: 17},
				{kind: left_brace, line: 3, column: 20},
				{kind: new_line, line: 3, column: 21},

				{kind: comment, line: 4, column: 5, text: "// block"},
				{kind: new_line, line: 4, column: 13},

				{kind: identifier, line: 5, column: 5, text: "print"},
				{kind: left_paren, line: 5, column: 10},
				{kind: string_, line: 5, column: 11, text: "Rich text"},
				{kind: right_paren, line: 5, column: 22},
				{kind: new_line, line: 5, column: 23},

				{kind: right_brace, line: 6, column: 3},
				{kind: comma, line: 6, column: 4},
				{kind: new_line, line: 6, column: 5},

				{kind: identifier, line: 7, column: 3, text: "_"},
				{kind: fat_arrow, line: 7, column: 5},
				{kind: identifier, line: 7, column: 8, text: "print"},
				{kind: left_paren, line: 7, column: 13},
				{kind: string_, line: 7, column: 14, text: "Unknown"},
				{kind: right_paren, line: 7, column: 23},
				{kind: new_line, line: 7, column: 24},

				{kind: right_brace, line: 8, column: 1},

				{kind: eof},
			},
		},

		{
			name: "unary expressions",
			input: strings.Join([]string{
				"not is_real",
				"-20",
			}, "\n"),
			want: []token{
				{kind: not, line: 1, column: 1},
				{kind: identifier, line: 1, column: 5, text: "is_real"},
				{kind: new_line, line: 1, column: 12},

				{kind: minus, line: 2, column: 1},
				{kind: number, line: 2, column: 2, text: "20"},

				{kind: eof},
			},
		},
		{
			name: "logical expressions",
			input: strings.Join([]string{
				"true and false",
				"true or false",
			}, "\n"),
			want: []token{
				{kind: true_, line: 1, column: 1, text: "true"},
				{kind: and, line: 1, column: 6},
				{kind: false_, line: 1, column: 10, text: "false"},
				{kind: new_line, line: 1, column: 15},

				{kind: true_, line: 2, column: 1, text: "true"},
				{kind: or, line: 2, column: 6},
				{kind: false_, line: 2, column: 9, text: "false"},

				{kind: eof},
			},
		},

		{
			name: "binary expressions",
			input: strings.Join([]string{
				"3 + 4",
				"10 - 5",
				"6 * 7",
				"20 / 4",
				"15 % 4",
				"-5 + 10",
				"8 * -2",
				"100 / -25",
				"7 % -3",
			}, "\n"),
			want: []token{
				{kind: number, line: 1, column: 1, text: "3"},
				{kind: plus, line: 1, column: 3},
				{kind: number, line: 1, column: 5, text: "4"},
				{kind: new_line, line: 1, column: 6},

				{kind: number, line: 2, column: 1, text: "10"},
				{kind: minus, line: 2, column: 4},
				{kind: number, line: 2, column: 6, text: "5"},
				{kind: new_line, line: 2, column: 7},

				{kind: number, line: 3, column: 1, text: "6"},
				{kind: star, line: 3, column: 3},
				{kind: number, line: 3, column: 5, text: "7"},
				{kind: new_line, line: 3, column: 6},

				{kind: number, line: 4, column: 1, text: "20"},
				{kind: slash, line: 4, column: 4},
				{kind: number, line: 4, column: 6, text: "4"},
				{kind: new_line, line: 4, column: 7},

				{kind: number, line: 5, column: 1, text: "15"},
				{kind: percent, line: 5, column: 4},
				{kind: number, line: 5, column: 6, text: "4"},
				{kind: new_line, line: 5, column: 7},

				{kind: minus, line: 6, column: 1},
				{kind: number, line: 6, column: 2, text: "5"},
				{kind: plus, line: 6, column: 4},
				{kind: number, line: 6, column: 6, text: "10"},
				{kind: new_line, line: 6, column: 8},

				{kind: number, line: 7, column: 1, text: "8"},
				{kind: star, line: 7, column: 3},
				{kind: minus, line: 7, column: 5},
				{kind: number, line: 7, column: 6, text: "2"},
				{kind: new_line, line: 7, column: 7},

				{kind: number, line: 8, column: 1, text: "100"},
				{kind: slash, line: 8, column: 5},
				{kind: minus, line: 8, column: 7},
				{kind: number, line: 8, column: 8, text: "25"},
				{kind: new_line, line: 8, column: 10},

				{kind: number, line: 9, column: 1, text: "7"},
				{kind: percent, line: 9, column: 3},
				{kind: minus, line: 9, column: 5},
				{kind: number, line: 9, column: 6, text: "3"},

				{kind: eof},
			},
		},
		{
			name: "comparison expressions",
			input: strings.Join([]string{
				"5 == 5",
				"not 10 == 5",
			}, "\n"),
			want: []token{
				{kind: number, line: 1, column: 1, text: "5"},
				{kind: equal_equal, line: 1, column: 3},
				{kind: number, line: 1, column: 6, text: "5"},
				{kind: new_line, line: 1, column: 7},

				{kind: not, line: 2, column: 1},
				{kind: number, line: 2, column: 5, text: "10"},
				{kind: equal_equal, line: 2, column: 8},
				{kind: number, line: 2, column: 11, text: "5"},

				{kind: eof},
			},
		},

		{
			name:  "optional types",
			input: "let name: Str? = option.none()",
			want: []token{
				{kind: let, line: 1, column: 1},
				{kind: identifier, line: 1, column: 5, text: "name"},
				{kind: colon, line: 1, column: 9},
				{kind: identifier, line: 1, column: 11, text: "Str"},
				{kind: question_mark, line: 1, column: 14},
				{kind: equal, line: 1, column: 16},
				{kind: identifier, line: 1, column: 18, text: "option"},
				{kind: dot, line: 1, column: 24},
				{kind: identifier, line: 1, column: 25, text: "none"},
				{kind: left_paren, line: 1, column: 29},
				{kind: right_paren, line: 1, column: 30},
				{kind: eof},
			},
		},

		{
			name:  "unions",
			input: "type Shape = Square|Circle",
			want: []token{
				{kind: type_, line: 1, column: 1},
				{kind: identifier, line: 1, column: 6, text: "Shape"},
				{kind: equal, line: 1, column: 12},
				{kind: identifier, line: 1, column: 14, text: "Square"},
				{kind: pipe, line: 1, column: 20},
				{kind: identifier, line: 1, column: 21, text: "Circle"},
				{kind: eof},
			},
		},

		{
			name: "string interpolation",
			input: strings.Join([]string{
				`"hello, {name}!"`,
				`"{(1 + 1).to_str()}"`,
				`"Hello, {num}"`,
				`"{c}{res}"`,
			}, "\n"),
			want: []token{
				{kind: string_, line: 1, column: 1, text: "hello, "},
				{kind: expr_open, line: 1, column: 9},
				{kind: identifier, line: 1, column: 10, text: "name"},
				{kind: expr_close, line: 1, column: 14},
				{kind: string_, line: 1, column: 15, text: "!"},
				{kind: new_line, line: 1, column: 17},

				{kind: string_, line: 2, column: 1},
				{kind: expr_open, line: 2, column: 2},
				{kind: left_paren, line: 2, column: 3},
				{kind: number, line: 2, column: 4, text: "1"},
				{kind: plus, line: 2, column: 6},
				{kind: number, line: 2, column: 8, text: "1"},
				{kind: right_paren, line: 2, column: 9},
				{kind: dot, line: 2, column: 10},
				{kind: identifier, line: 2, column: 11, text: "to_str"},
				{kind: left_paren, line: 2, column: 17},
				{kind: right_paren, line: 2, column: 18},
				{kind: expr_close, line: 2, column: 19},
				{kind: string_, line: 2, column: 20},
				{kind: new_line, line: 2, column: 21},

				{kind: string_, line: 3, column: 1, text: "Hello, "},
				{kind: expr_open, line: 3, column: 9},
				{kind: identifier, line: 3, column: 10, text: "num"},
				{kind: expr_close, line: 3, column: 13},
				{kind: string_, line: 3, column: 14},
				{kind: new_line, line: 3, column: 15},

				{kind: string_, line: 4, column: 1},
				{kind: expr_open, line: 4, column: 2},
				{kind: identifier, line: 4, column: 3, text: "c"},
				{kind: expr_close, line: 4, column: 4},
				{kind: string_, line: 4, column: 5},
				{kind: expr_open, line: 4, column: 5},
				{kind: identifier, line: 4, column: 6, text: "res"},
				{kind: expr_close, line: 4, column: 9},
				{kind: string_, line: 4, column: 10},

				{kind: eof},
			},
		},

		{
			name: "escaped braces in strings",
			input: strings.Join([]string{
				`"Text with \{escaped braces}"`,
				`"Mixed {interp} with \{escaped}"`,
			}, "\n"),
			want: []token{
				{kind: string_, line: 1, column: 1, text: "Text with {escaped braces}"},
				{kind: new_line, line: 1, column: 30},

				{kind: string_, line: 2, column: 1, text: "Mixed "},
				{kind: expr_open, line: 2, column: 8},
				{kind: identifier, line: 2, column: 9, text: "interp"},
				{kind: expr_close, line: 2, column: 15},
				{kind: string_, line: 2, column: 16, text: " with {escaped}"},

				{kind: eof},
			},
		},

		{
			name: "escaping characters in strings",
			input: strings.Join([]string{
				`"hello, \"world\"!"`,
			}, "\n"),
			want: []token{
				{kind: string_, line: 1, column: 1, text: `hello, "world"!`},
				{kind: eof},
			},
		},
		{
			name: "escape sequences in strings",
			input: strings.Join([]string{
				`"line 1\nline 2"`,
				`"tab\tspaced"`,
				`"carriage\rreturn"`,
				`"backslash \\ and quote \" together"`,
				`"bell\b, form feed\f, vertical tab\v"`,
			}, "\n"),
			want: []token{
				{kind: string_, line: 1, column: 1, text: "line 1\nline 2"},
				{kind: new_line, line: 1, column: 17},

				{kind: string_, line: 2, column: 1, text: "tab\tspaced"},
				{kind: new_line, line: 2, column: 14},

				{kind: string_, line: 3, column: 1, text: "carriage\rreturn"},
				{kind: new_line, line: 3, column: 19},

				{kind: string_, line: 4, column: 1, text: "backslash \\ and quote \" together"},
				{kind: new_line, line: 4, column: 37},

				{kind: string_, line: 5, column: 1, text: "bell\b, form feed\f, vertical tab\v"},

				{kind: eof},
			},
		},

		{
			name: "member access",
			input: strings.Join([]string{
				`let initialized: [Str:Bool] = [:]`,
				`initialized.count`,
				`initialized.put("key", true)`,
				`location.point.x = 1`,
				`let falsy = not something.done`,
			}, "\n"),
			want: []token{
				{kind: let, line: 1, column: 1},
				{kind: identifier, line: 1, column: 5, text: "initialized"},
				{kind: colon, line: 1, column: 16},
				{kind: left_bracket, line: 1, column: 18},
				{kind: identifier, line: 1, column: 19, text: "Str"},
				{kind: colon, line: 1, column: 22},
				{kind: identifier, line: 1, column: 23, text: "Bool"},
				{kind: right_bracket, line: 1, column: 27},
				{kind: equal, line: 1, column: 29},
				{kind: left_bracket, line: 1, column: 31},
				{kind: colon, line: 1, column: 32},
				{kind: right_bracket, line: 1, column: 33},
				{kind: new_line, line: 1, column: 34},

				{kind: identifier, line: 2, column: 1, text: "initialized"},
				{kind: dot, line: 2, column: 12},
				{kind: identifier, line: 2, column: 13, text: "count"},
				{kind: new_line, line: 2, column: 18},

				{kind: identifier, line: 3, column: 1, text: "initialized"},
				{kind: dot, line: 3, column: 12},
				{kind: identifier, line: 3, column: 13, text: "put"},
				{kind: left_paren, line: 3, column: 16},
				{kind: string_, line: 3, column: 17, text: "key"},
				{kind: comma, line: 3, column: 22},
				{kind: true_, line: 3, column: 24, text: "true"},
				{kind: right_paren, line: 3, column: 28},
				{kind: new_line, line: 3, column: 29},

				{kind: identifier, line: 4, column: 1, text: "location"},
				{kind: dot, line: 4, column: 9},
				{kind: identifier, line: 4, column: 10, text: "point"},
				{kind: dot, line: 4, column: 15},
				{kind: identifier, line: 4, column: 16, text: "x"},
				{kind: equal, line: 4, column: 18},
				{kind: number, line: 4, column: 20, text: "1"},
				{kind: new_line, line: 4, column: 21},

				{kind: let, line: 5, column: 1},
				{kind: identifier, line: 5, column: 5, text: "falsy"},
				{kind: equal, line: 5, column: 11},
				{kind: not, line: 5, column: 13},
				{kind: identifier, line: 5, column: 17, text: "something"},
				{kind: dot, line: 5, column: 26},
				{kind: identifier, line: 5, column: 27, text: "done"},

				{kind: eof},
			},
		},

		{
			name: "structs",
			input: strings.Join([]string{
				"struct Person {",
				"	name: Str,",
				"	age: Int,",
				"	employed: Bool,",
				"}",
				"",
				"let john = Person {	name: \"John\", age: 30, employed: true }",
				"let people: [Person] = [",
				"  john,",
				"  Person {	name: \"Alice\", age: age_value, employed: true }",
				"]",
			}, "\n"),
			want: []token{
				{kind: struct_, line: 1, column: 1},
				{kind: identifier, line: 1, column: 8, text: "Person"},
				{kind: left_brace, line: 1, column: 15},
				{kind: new_line, line: 1, column: 16},

				{kind: identifier, line: 2, column: 2, text: "name"},
				{kind: colon, line: 2, column: 6},
				{kind: identifier, line: 2, column: 8, text: "Str"},
				{kind: comma, line: 2, column: 11},
				{kind: new_line, line: 2, column: 12},

				{kind: identifier, line: 3, column: 2, text: "age"},
				{kind: colon, line: 3, column: 5},
				{kind: identifier, line: 3, column: 7, text: "Int"},
				{kind: comma, line: 3, column: 10},
				{kind: new_line, line: 3, column: 11},

				{kind: identifier, line: 4, column: 2, text: "employed"},
				{kind: colon, line: 4, column: 10},
				{kind: identifier, line: 4, column: 12, text: "Bool"},
				{kind: comma, line: 4, column: 16},
				{kind: new_line, line: 4, column: 17},

				{kind: right_brace, line: 5, column: 1},
				{kind: new_line, line: 5, column: 2},
				{kind: new_line, line: 6, column: 1},

				{kind: let, line: 7, column: 1},
				{kind: identifier, line: 7, column: 5, text: "john"},
				{kind: equal, line: 7, column: 10},
				{kind: identifier, line: 7, column: 12, text: "Person"},
				{kind: left_brace, line: 7, column: 19},
				{kind: identifier, line: 7, column: 21, text: "name"},
				{kind: colon, line: 7, column: 25},
				{kind: string_, line: 7, column: 27, text: "John"},
				{kind: comma, line: 7, column: 33},
				{kind: identifier, line: 7, column: 35, text: "age"},
				{kind: colon, line: 7, column: 38},
				{kind: number, line: 7, column: 40, text: "30"},
				{kind: comma, line: 7, column: 42},
				{kind: identifier, line: 7, column: 44, text: "employed"},
				{kind: colon, line: 7, column: 52},
				{kind: true_, line: 7, column: 54, text: "true"},
				{kind: right_brace, line: 7, column: 59},
				{kind: new_line, line: 7, column: 60},

				{kind: let, line: 8, column: 1},
				{kind: identifier, line: 8, column: 5, text: "people"},
				{kind: colon, line: 8, column: 11},
				{kind: left_bracket, line: 8, column: 13},
				{kind: identifier, line: 8, column: 14, text: "Person"},
				{kind: right_bracket, line: 8, column: 20},
				{kind: equal, line: 8, column: 22},
				{kind: left_bracket, line: 8, column: 24},
				{kind: new_line, line: 8, column: 25},

				{kind: identifier, line: 9, column: 3, text: "john"},
				{kind: comma, line: 9, column: 7},
				{kind: new_line, line: 9, column: 8},

				{kind: identifier, line: 10, column: 3, text: "Person"},
				{kind: left_brace, line: 10, column: 10},
				{kind: identifier, line: 10, column: 12, text: "name"},
				{kind: colon, line: 10, column: 16},
				{kind: string_, line: 10, column: 18, text: "Alice"},
				{kind: comma, line: 10, column: 25},
				{kind: identifier, line: 10, column: 27, text: "age"},
				{kind: colon, line: 10, column: 30},
				{kind: identifier, line: 10, column: 32, text: "age_value"},
				{kind: comma, line: 10, column: 41},
				{kind: identifier, line: 10, column: 43, text: "employed"},
				{kind: colon, line: 10, column: 51},
				{kind: true_, line: 10, column: 53, text: "true"},
				{kind: right_brace, line: 10, column: 58},
				{kind: new_line, line: 10, column: 59},

				{kind: right_bracket, line: 11, column: 1},

				{kind: eof},
			},
		},

		{
			name: "struct impl blocks",
			input: strings.Join([]string{
				`impl (self: Person) {`,
				`  fn describe() Str {}`,
				`}`,
				``,
				`impl (mut self: Person) {`,
				`  fn age() {`,
				`    self.age =+ 1`,
				`  }`,
				`}`,
			}, "\n"),
			want: []token{
				{kind: impl, line: 1, column: 1},
				{kind: left_paren, line: 1, column: 6},
				{kind: identifier, line: 1, column: 7, text: "self"},
				{kind: colon, line: 1, column: 11},
				{kind: identifier, line: 1, column: 13, text: "Person"},
				{kind: right_paren, line: 1, column: 19},
				{kind: left_brace, line: 1, column: 21},
				{kind: new_line, line: 1, column: 22},

				{kind: fn, line: 2, column: 3},
				{kind: identifier, line: 2, column: 6, text: "describe"},
				{kind: left_paren, line: 2, column: 14},
				{kind: right_paren, line: 2, column: 15},
				{kind: identifier, line: 2, column: 17, text: "Str"},
				{kind: left_brace, line: 2, column: 21},
				{kind: right_brace, line: 2, column: 22},
				{kind: new_line, line: 2, column: 23},

				{kind: right_brace, line: 3, column: 1},
				{kind: new_line, line: 3, column: 2},
				{kind: new_line, line: 4, column: 1},

				{kind: impl, line: 5, column: 1},
				{kind: left_paren, line: 5, column: 6},
				{kind: mut, line: 5, column: 7},
				{kind: identifier, line: 5, column: 11, text: "self"},
				{kind: colon, line: 5, column: 15},
				{kind: identifier, line: 5, column: 17, text: "Person"},
				{kind: right_paren, line: 5, column: 23},
				{kind: left_brace, line: 5, column: 25},
				{kind: new_line, line: 5, column: 26},

				{kind: fn, line: 6, column: 3},
				{kind: identifier, line: 6, column: 6, text: "age"},
				{kind: left_paren, line: 6, column: 9},
				{kind: right_paren, line: 6, column: 10},
				{kind: left_brace, line: 6, column: 12},
				{kind: new_line, line: 6, column: 13},

				{kind: identifier, line: 7, column: 5, text: "self"},
				{kind: dot, line: 7, column: 9},
				{kind: identifier, line: 7, column: 10, text: "age"},
				{kind: increment, line: 7, column: 14},
				{kind: number, line: 7, column: 17, text: "1"},
				{kind: new_line, line: 7, column: 18},

				{kind: right_brace, line: 8, column: 3},
				{kind: new_line, line: 8, column: 4},

				{kind: right_brace, line: 9, column: 1},

				{kind: eof},
			},
		},

		{
			name:  "declaring generic types",
			input: `fn call(with: $T) { }`,
			want: []token{
				{kind: fn, line: 1, column: 1},
				{kind: identifier, line: 1, column: 4, text: "call"},
				{kind: left_paren, line: 1, column: 8},
				{kind: identifier, line: 1, column: 9, text: "with"},
				{kind: colon, line: 1, column: 13},
				{kind: identifier, line: 1, column: 15, text: "$T"},
				{kind: right_paren, line: 1, column: 17},
				{kind: left_brace, line: 1, column: 19},
				{kind: right_brace, line: 1, column: 21},
				{kind: eof},
			},
		},

		{
			name: "Narrowing generics in function calls",
			input: strings.Join([]string{
				`json::decode<Person>(val)`,
			}, "\n"),
			want: []token{
				{kind: identifier, line: 1, column: 1, text: "json"},
				{kind: colon_colon, line: 1, column: 5},
				{kind: identifier, line: 1, column: 7, text: "decode"},
				{kind: less_than, line: 1, column: 13},
				{kind: identifier, line: 1, column: 14, text: "Person"},
				{kind: greater_than, line: 1, column: 20},
				{kind: left_paren, line: 1, column: 21},
				{kind: identifier, line: 1, column: 22, text: "val"},
				{kind: right_paren, line: 1, column: 25},
				{kind: eof},
			},
		},
	})
}
