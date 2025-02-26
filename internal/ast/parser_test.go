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
			name: "variable declarations",
			input: strings.Join([]string{
				"mut x = 5",
				"let y = 10",
				`let string: Str = "hello"`,
			}, "\n"),
			want: []token{
				{kind: mut, line: 1, column: 1, text: "mut"},
				{kind: identifier, line: 1, column: 5, text: "x"},
				{kind: equal, line: 1, column: 7, text: "="},
				{kind: number, line: 1, column: 9, text: "5"},

				{kind: let, line: 2, column: 1, text: "let"},
				{kind: identifier, line: 2, column: 5, text: "y"},
				{kind: equal, line: 2, column: 7, text: "="},
				{kind: number, line: 2, column: 9, text: "10"},

				{kind: let, line: 3, column: 1, text: "let"},
				{kind: identifier, line: 3, column: 5, text: "string"},
				{kind: colon, line: 3, column: 11},
				{kind: identifier, line: 3, column: 13, text: "Str"},
				{kind: equal, line: 3, column: 17, text: "="},
				{kind: string_, line: 3, column: 19, text: `"hello"`},

				{kind: eof},
			},
		},
	})
}
