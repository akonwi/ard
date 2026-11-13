package formatter

import "testing"

// A comment at the end of a match or select arm's line stays on that line
// instead of moving below it, where it reads as describing the next arm, and
// a comment after the opening brace is accepted and goes on the first line
// inside (#583).
func TestFormatKeepsTrailingCommentsOnArms(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "match arms",
			input: `fn main() {
  let r = match 1 {
    // first arm
    1 => 2, // one
    2 => {
      let three = 3
      three
    }, // block arm
    _ => 4, // rest
    // closing
  }
}
`,
		},
		{
			name: "last match arm without a trailing comma",
			input: `fn main() {
  let r = match 1 {
    1 => 2,
    _ => 3 // rest
  }
}
`,
			want: `fn main() {
  let r = match 1 {
    1 => 2,
    _ => 3, // rest
  }
}
`,
		},
		{
			name: "comment after the opening brace of a match",
			input: `fn main() {
  let r = match 1 { // arms
    _ => 3,
  }
}
`,
			want: `fn main() {
  let r = match 1 {
    // arms
    _ => 3,
  }
}
`,
		},
		{
			name: "conditional match",
			input: `fn main() {
  let r = match { // conditions
    // small
    1 < 2 => 1, // yes
    _ => 2,
  }
}
`,
			want: `fn main() {
  let r = match {
    // conditions
    // small
    1 < 2 => 1, // yes
    _ => 2,
  }
}
`,
		},
		{
			name: "select arms",
			input: `fn pick(first: Chan<Int>, second: Chan<Int>) Int? {
  select { // first ready wins
    // first
    let value = first.recv() => value, // got it
    let value = second.recv() => value,
    // closing
  }
}
`,
			want: `fn pick(first: Chan<Int>, second: Chan<Int>) Int? {
  select {
    // first ready wins
    // first
    let value = first.recv() => value, // got it
    let value = second.recv() => value,
    // closing
  }
}
`,
		},
	})
}
