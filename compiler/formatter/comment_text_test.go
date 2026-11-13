package formatter

import "testing"

// A comment whose text starts with `//` keeps it. The printer used to read
// the text's leading `//` as the comment marker and drop it.
func TestFormatKeepsCommentTextThatStartsWithSlashes(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "in a sequence",
			input: `let x = [
  1, // // keep this prefix
  // // own line
  2,
]
`,
		},
		{
			name: "on a statement",
			input: `let y = 1 // // trailing
/// doc comment
let z = 2
`,
		},
		{
			name: "inside grouping parentheses",
			input: `let x = ( // // why
  1 + 2) * 3
`,
			want: `// // why
let x = (1 + 2) * 3
`,
		},
	})
}
