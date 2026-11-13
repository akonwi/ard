package formatter

import "testing"

// Comments in a parameter list were dropped because the formatter never
// printed them. They now stay where they were, like in other sequences.
func TestFormatKeepsCommentsInParameterLists(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "trailing comment on a parameter",
			input: `fn add(
  a: Int, // first
  b: Int,
) Int {
  a + b
}
`,
		},
		{
			name: "own-line comment before a parameter",
			input: `fn add(
  a: Int,
  // the bonus
  b: Int,
) Int {
  a + b
}
`,
		},
		{
			name: "a parameter list that would otherwise fit on one line breaks",
			input: `fn add(a: Int, b: Int // second
) Int {
  a + b
}
`,
			want: `fn add(
  a: Int,
  b: Int, // second
) Int {
  a + b
}
`,
		},
		{
			name: "comment after the opening paren",
			input: `fn add( // operands
  a: Int,
  b: Int,
) Int {
  a + b
}
`,
			want: `fn add(
  // operands
  a: Int,
  b: Int,
) Int {
  a + b
}
`,
		},
		{
			name: "method",
			input: `impl Point {
  fn shift(
    by: Int, // distance
  ) Int {
    self.x + by
  }
}
`,
		},
		{
			name: "static function",
			input: `fn Point::new(
  x: Int, // horizontal
  y: Int,
) Point {
  Point{x: x, y: y}
}
`,
		},
		{
			name: "anonymous function",
			input: `let f = fn(
  a: Int, // first
  // the bonus
  b: Int,
) Int {
  a + b
}
`,
		},
	})
}
