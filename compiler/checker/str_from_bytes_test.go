package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestStrFromBytes(t *testing.T) {
	run(t, []test{
		{
			name:  "from_bytes returns an optional Str",
			input: `let s: Str? = Str::from_bytes("hé".bytes())`,
		},
		{
			name:  "from_bytes accepts a byte list binding",
			input: `let raw: [Byte] = "x".bytes()
let s: Str? = Str::from_bytes(raw)`,
		},
		{
			name:  "from_bytes rejects a non-byte-list argument",
			input: `let s = Str::from_bytes("hi")`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected [Byte], got Str"},
			},
		},
		{
			name:  "from_bytes rejects the wrong argument count",
			input: `let s = Str::from_bytes("x".bytes(), "y".bytes())`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 1, got 2"},
			},
		},
	})
}
