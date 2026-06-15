package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestByteAndRunePrimitiveTypes(t *testing.T) {
	run(t, []test{
		{
			name:  "str at returns rune maybe",
			input: `let ch: Rune? = "hé".at(1)`,
		},
		{
			name: "string iteration cursor is rune",
			input: `for ch in "hé" {
  let code: Int = ch.to_int()
}`,
		},
		{
			name: "string bytes and runes views",
			input: `let bytes: [Byte] = "hé".bytes()
let runes: [Rune] = "hé".runes()`,
		},
		{
			name: "byte and rune constructors are prelude imports",
			input: `let b: Byte? = Byte::from_int(65)
let r: Rune? = Rune::from_int(65)`,
		},
		{
			name:  "byte and rune are not implicit ints",
			input: `let b: Byte = 65`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Byte, got Int"},
			},
		},
		{
			name: "byte and int cannot be mixed in comparisons",
			input: `let b = Byte::from_int(65).or(Byte::from_int(0).expect("zero"))
b == 65`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Invalid: Byte == Int"},
			},
		},
		{
			name:  "split method is removed from primitive str",
			input: `"a,b".split(",")`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: `Undefined: "a,b".split`},
			},
		},
	})
}
