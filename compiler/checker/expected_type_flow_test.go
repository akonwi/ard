package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

// A function's declared return type types a list or map literal that is the
// body's final expression, as a `let` annotation does.
func TestReturnTypeTypesFinalCollectionLiteral(t *testing.T) {
	run(t, []test{
		{
			name: "list of distinct Go interface implementers",
			input: `use go:bytes
use go:io
use go:strings

fn readers() [io::Reader] {
  [strings::NewReader("a"), bytes::NewBufferString("b")]
}`,
		},
		{
			name: "list of union members",
			input: `type Val = Int | Str

fn values() [Val] {
  [1, "two"]
}`,
		},
		{
			name: "map of union members",
			input: `type Val = Int | Str

fn values() [Str: Val] {
  ["a": 1, "b": "two"]
}`,
		},
		{
			name: "list of tuples of union members",
			input: `type Val = Int | Str

fn pairs() [(Val, Val)] {
  [(1, 2), ("x", 3)]
}`,
		},
		{
			name: "element outside the union is still rejected",
			input: `type Val = Int | Str

fn values() [Val] {
  [1, true]
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Val, got Bool"},
			},
		},
	})
}
