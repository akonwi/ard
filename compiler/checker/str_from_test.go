package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestStrFrom(t *testing.T) {
	run(t, []test{
		{
			name:  "from bytes builds a Str",
			input: `let s: Str = Str::from("hé".bytes())`,
		},
		{
			name:  "from runes builds a Str",
			input: `let s: Str = Str::from("hé".runes())`,
		},
		{
			name: "from a byte-list binding",
			input: `let raw: [Byte] = "x".bytes()
let s: Str = Str::from(raw)`,
		},
		{
			name:  "from is not optional",
			input: `let s: Str? = Str::from("x".bytes())`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str?, got Str"},
			},
		},
		{
			name:  "rejects a non-list argument",
			input: `let s = Str::from("hi")`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Str::from expects [Byte] or [Rune], got Str"},
			},
		},
		{
			name:  "rejects a non-byte-or-rune list",
			input: `let s = Str::from([1, 2, 3])`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Str::from expects [Byte] or [Rune], got [Int]"},
			},
		},
		{
			name:  "rejects the wrong argument count",
			input: `let s = Str::from("x".bytes(), "y".bytes())`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 1, got 2"},
			},
		},
	})
}
