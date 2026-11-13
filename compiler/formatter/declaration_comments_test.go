package formatter

import "testing"

// A comment at the end of a struct field's line stays on that line instead of
// moving below it, where it reads as describing the next field (#583).
func TestFormatKeepsTrailingCommentsInStructDefinitions(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "trailing comments on fields",
			input: `struct Point {
  // about x
  x: Int, // horizontal
  y: Int, // vertical
  // closing note
}
`,
		},
		{
			name: "last field without a trailing comma",
			input: `struct Point {
  x: Int,
  y: Int // vertical
}
`,
			want: `struct Point {
  x: Int,
  y: Int, // vertical
}
`,
		},
		{
			name: "comment after the opening brace moves to the first line inside",
			input: `struct Point { // a point
  x: Int,
}
`,
			want: `struct Point {
  // a point
  x: Int,
}
`,
		},
		{
			name: "blank lines between fields are kept",
			input: `struct Point {
  x: Int, // horizontal

  y: Int, // vertical
}
`,
		},
		{
			name: "trailing comment on an attribute",
			input: `struct Row {
  #json("id") // wire name
  // the id
  id: Int, // primary key
}
`,
		},
	})
}

// Comments in an enum stay with their variants: a comment on a line of its
// own above the variant that follows it, and a trailing comment on its
// variant's line. They used to all move above the first variant (#583).
func TestFormatKeepsCommentsWithEnumVariants(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "own-line and trailing comments",
			input: `enum Color {
  // warm
  Red, // red
  // cool
  Green,
  Blue, // blue
  // closing note
}
`,
		},
		{
			name: "comment after the opening brace moves to the first line inside",
			input: `enum Color { // colors
  Red,
  Green
}
`,
			want: `enum Color {
  // colors
  Red,
  Green,
}
`,
		},
		{
			name: "variants with values",
			input: `enum Code {
  Ok = 0, // fine
  // the rest
  Bad = 1,
}
`,
		},
		{
			name: "comment-only enum",
			input: `enum Color {
  // none yet
}
`,
		},
	})
}

// A comment after a method's closing brace or at the end of a trait
// signature stays on that line, instead of moving down to read as describing
// the next method. Trait implementations accept comments like impl blocks
// (#583).
func TestFormatKeepsCommentsWithMethods(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "impl block",
			input: `impl P { // methods
  // doc for a
  fn a() Int {
    self.x
  } // end a

  fn b() Int {
    self.x
  }
  // closing note
}
`,
			want: `impl P {
  // methods
  // doc for a
  fn a() Int {
    self.x
  } // end a

  fn b() Int {
    self.x
  }

  // closing note
}
`,
		},
		{
			name: "trait definition",
			input: `trait Shape { // shapes
  // the area
  fn area() Int // in square units
  fn name() Str
  // closing note
}
`,
			want: `trait Shape {
  // shapes
  // the area
  fn area() Int // in square units

  fn name() Str

  // closing note
}
`,
		},
		{
			name: "trait implementation",
			input: `impl Shape for P { // shapes
  // the area
  fn area() Int {
    self.x
  } // end area

  fn name() Str {
    "p"
  }
  // closing note
}
`,
			want: `impl Shape for P {
  // shapes
  // the area
  fn area() Int {
    self.x
  } // end area

  fn name() Str {
    "p"
  }

  // closing note
}
`,
		},
		{
			name: "comment-only trait implementation",
			input: `impl Shape for P {
  // nothing yet
}
`,
		},
	})
}
