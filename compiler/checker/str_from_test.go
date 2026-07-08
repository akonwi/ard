package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestStrFrom(t *testing.T) {
	run(t, []test{
		{
			name:  "from returns an optional Str",
			input: `let s: Str? = Str::from("hé".bytes())`,
		},
		{
			name:  "from accepts a byte list binding",
			input: `let raw: [Byte] = "x".bytes()
let s: Str? = Str::from(raw)`,
		},
		{
			name:  "from rejects a non-byte-list argument",
			input: `let s = Str::from("hi")`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected [Byte], got Str"},
			},
		},
		{
			name:  "from rejects the wrong argument count",
			input: `let s = Str::from("x".bytes(), "y".bytes())`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 1, got 2"},
			},
		},
	})
}
