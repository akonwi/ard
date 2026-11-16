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

// An empty match or select prints as `{}` on one line, which parses again;
// the checker reports the missing cases (issue #595).
func TestFormatEmptyMatchAndSelect(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{name: "match", input: "match x {\n}\n", want: "match x {}\n"},
		{name: "match with blank line", input: "match x {\n\n}\n", want: "match x {}\n"},
		{name: "match on one line", input: "match x {}\n", want: "match x {}\n"},
		{name: "conditional match", input: "match {\n}\n", want: "match {}\n"},
		{name: "select", input: "select {\n}\n", want: "select {}\n"},
		{name: "select on one line", input: "select {}\n", want: "select {}\n"},
		{name: "match value", input: "let y = match x {\n}\n", want: "let y = match x {}\n"},
	})
}

// A comment-only match keeps its comment inside the braces.
func TestFormatCommentOnlyMatch(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{name: "match", input: "match x {\n  // todo\n}\n"},
	})
}
