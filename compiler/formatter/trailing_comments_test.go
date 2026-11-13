package formatter

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/parse"
)

type trailingCommentCase struct {
	name  string
	input string
	want  string // empty when the input is already canonical
}

// assertTrailingCommentCases formats each input, checks the exact output,
// that the output parses, has no trailing whitespace, and that formatting it
// again changes nothing.
func assertTrailingCommentCases(t *testing.T, cases []trailingCommentCase) {
	t.Helper()
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			want := test.want
			if want == "" {
				want = test.input
			}
			formatted, err := Format([]byte(test.input), "test.ard")
			if err != nil {
				t.Fatalf("format: %v", err)
			}
			if string(formatted) != want {
				t.Fatalf("formatted:\n%s\nwant:\n%s", formatted, want)
			}
			if result := parse.Parse(formatted, "test.ard"); len(result.Errors) > 0 {
				t.Fatalf("formatted output does not parse: %v", result.Errors)
			}
			for i, line := range strings.Split(string(formatted), "\n") {
				if line != strings.TrimRight(line, " \t") {
					t.Fatalf("line %d has trailing whitespace: %q", i+1, line)
				}
			}
			again, err := Format(formatted, "test.ard")
			if err != nil {
				t.Fatalf("second format: %v", err)
			}
			if string(again) != want {
				t.Fatalf("formatter is not idempotent:\n%s", again)
			}
		})
	}
}

// A comment at the end of an element's line stays on that line, after the
// separator, instead of moving onto its own line where it reads as describing
// the next element (#583).
func TestFormatKeepsTrailingCommentsInListLiterals(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "trailing comments on every element",
			input: `let l = [
  1, // one
  2, // two
]
`,
		},
		{
			name: "last element without a trailing comma",
			input: `let l = [
  1,
  2 // two
]
`,
			want: `let l = [
  1,
  2, // two
]
`,
		},
		{
			name: "a list that would otherwise fit on one line breaks",
			input: `let l = [1, 2 // two
]
`,
			want: `let l = [
  1,
  2, // two
]
`,
		},
		{
			name: "the comment stays with the last element on its row",
			input: `let l = [1, 2, // small
  3, 4]
`,
			want: `let l = [
  1,
  2, // small
  3,
  4,
]
`,
		},
		{
			name: "own-line and trailing comments on adjacent rows",
			input: `let l = [
  // about one
  1, // one
  // about two
  2, // two
  // closing note
]
`,
		},
		{
			name: "comment after the opening bracket stays on its line",
			input: `let l = [ // ids
  1,
  2,
]
`,
		},
		{
			name: "comment after the opening bracket of an empty list",
			input: `let l = [ // nothing yet
]
`,
		},
		{
			name: "comment after the opening bracket and a comment-only body",
			input: `let l = [ // ids
  // none yet
]
`,
		},
		{
			name: "nested lists keep each comment at its own level",
			input: `let l = [
  [
    1, // inner
  ], // outer
  [ // open
    2,
  ],
]
`,
		},
		{
			name: "fixed array",
			input: `let a: [Int; 2] = [
  1, // first
  2,
]
`,
		},
	})
}

func TestFormatKeepsTrailingCommentsInMapLiterals(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "trailing comment on the last entry",
			input: `let m = [
  1: 2, // last
]
`,
		},
		{
			name: "trailing and own-line comments",
			input: `let m = [ // scores
  "home": 1, // includes the own goal
  // away
  "away": 2
]
`,
			want: `let m = [ // scores
  "home": 1, // includes the own goal
  // away
  "away": 2,
]
`,
		},
		{
			name: "a map that would otherwise fit on one line breaks",
			input: `let m = ["a": 1 // one
]
`,
			want: `let m = [
  "a": 1, // one
]
`,
		},
	})
}

func TestFormatKeepsTrailingCommentsInTupleLiterals(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "trailing comment on the first element",
			input: `let t = (
  1, // lower
  2,
)
`,
		},
		{
			name: "a tuple that would otherwise fit on one line breaks",
			input: `let t = (1, 2 // upper
)
`,
			want: `let t = (
  1,
  2, // upper
)
`,
		},
		{
			name: "comment after the opening paren",
			input: `let t = ( // bounds
  1,
  2,
)
`,
		},
	})
}

func TestFormatKeepsTrailingCommentsInCallArguments(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "positional arguments",
			input: `foo(
  1, // arg
  2,
)
`,
		},
		{
			name: "named arguments",
			input: `foo(
  a: 1, // first
  b: 2, // second
)
`,
		},
		{
			name: "a call that would otherwise fit on one line breaks",
			input: `foo(1, 2 // two
)
`,
			want: `foo(
  1,
  2, // two
)
`,
		},
		{
			name: "comment after the opening paren",
			input: `foo( // args
  1,
  2,
)
`,
		},
		{
			name: "method call",
			input: `list.push(
  1, // one
)
`,
		},
		{
			name: "final spread argument without a trailing comma",
			input: `foo(
  1,
  xs... // rest
)
`,
			want: `foo(
  1,
  xs..., // rest
)
`,
		},
		{
			name: "a trailing closure is not hugged",
			input: `list.map(
  fn(x) { x }, // identity
)
`,
			want: `list.map(
  fn(x) {
    x
  }, // identity
)
`,
		},
	})
}

func TestFormatKeepsTrailingCommentsInStructLiterals(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "trailing comment on the first field",
			input: `let p = Point{
  x: 1, // ex
  y: 2,
}
`,
		},
		{
			name: "own-line comments stay with the field that follows",
			input: `let p = Point{
  x: 1,
  // why y
  y: 2,
}
`,
		},
		{
			name: "shorthand fields and own-line and trailing comments",
			input: `let p = Point{ // origin
  // about x
  x, // shorthand
  y: 2
  // closing note
}
`,
			want: `let p = Point{ // origin
  // about x
  x, // shorthand
  y: 2,
  // closing note
}
`,
		},
		{
			name: "a struct literal that would otherwise fit on one line breaks",
			input: `let p = Point{x: 1, y: 2 // why
}
`,
			want: `let p = Point{
  x: 1,
  y: 2, // why
}
`,
		},
		{
			name: "comment-only struct literal",
			input: `let p = Point{ // empty
}
`,
		},
	})
}
