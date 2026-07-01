package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestGoNamedScalarTypes(t *testing.T) {
	run(t, []test{
		{
			name: "literal argument contextually types as named Go scalar",
			input: `use go:time
fn main() {
  time::Sleep(1)
}`,
		},
		{
			name: "literal annotation contextually types as named Go scalar",
			input: `use go:time
let d: time::Duration = 1`,
		},
		{
			name: "non literal does not implicitly convert to named Go scalar",
			input: `use go:time
fn main() {
  let d = 1
  time::Sleep(d)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected time::Duration, got Int"}},
		},
		{
			name: "literal range checks named unsigned Go scalar",
			input: `use go:os
fn main() {
  os::Mkdir("tmp", -1)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Integer literal -1 overflows os::FileMode"}},
		},
	})
}
