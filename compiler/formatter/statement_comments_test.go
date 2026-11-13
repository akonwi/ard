package formatter

import (
	"strings"
	"testing"
)

// A comment at the end of a statement's line stays on that line instead of
// moving below it, where it reads as describing the next statement (#583).
func TestFormatKeepsTrailingCommentsOnStatements(t *testing.T) {
	longComment := "// " + strings.TrimSpace(strings.Repeat("a long explanation ", 6))
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "top-level statements",
			input: `let x = 1 // one
let y = 2

let z = 3 // three
`,
		},
		{
			name: "statements in a block",
			input: `fn main() {
  let x = 1 // one
  let y = x + 1 // two

  foo(y) // call
}
`,
		},
		{
			name: "after the closing brace of a function",
			input: `fn main() {
  ()
} // end main
`,
		},
		{
			name: "if and else",
			input: `fn main() {
  if true { // then
    ()
  } else { // otherwise
    ()
  } // after if
}
`,
			want: `fn main() {
  if true {
    // then
    ()
  } else {
    // otherwise
    ()
  } // after if
}
`,
		},
		{
			name: "break",
			input: `fn main() {
  while true {
    break // stop
  }
}
`,
		},
		{
			name: "type declaration",
			input: `type Value = Int | Str // trailing
`,
		},
		{
			name: "type declaration across lines",
			input: `type Value = Int
  | Str // last
`,
			want: `type Value = Int | Str // last
`,
		},
		{
			name: "after a literal that collapses onto one line",
			input: `let l = [
  1,
  2,
] // after list
`,
			want: `let l = [1, 2] // after list
`,
		},
		{
			name:  "a long comment does not break the statement",
			input: "let l = [1, 2] " + longComment + "\n",
		},
		{
			name: "a statement that spans lines",
			input: `let p = Point{
  x: 1,
  y: 2,
  z: 3,
} // the point
`,
		},
		{
			name: "statement in a closure body",
			input: `fn main() {
  list.each(fn(item) {
    print(item) // show it
  })
}
`,
		},
		{
			name: "comment after the opening brace of a block",
			input: `fn main() { // body
  let x = 1
}
`,
			want: `fn main() {
  // body
  let x = 1
}
`,
		},
	})
}
