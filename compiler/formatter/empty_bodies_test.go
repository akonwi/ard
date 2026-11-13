package formatter

import "testing"

// Empty impl, trait, and trait implementation bodies print as `{}`, which
// must parse again.
func TestFormatEmptyImplAndTraitBodies(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{name: "impl", input: "impl X {\n}\n", want: "impl X {}\n"},
		{name: "trait", input: "trait X {\n}\n", want: "trait X {}\n"},
		{name: "trait implementation", input: "impl Shape for X {\n}\n", want: "impl Shape for X {}\n"},
	})
}
