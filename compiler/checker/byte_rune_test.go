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
			name: "rune literals have Rune type",
			input: `let slash: Rune = '/'
let accent: Rune = 'é'
let newline: Rune = '\n'
let same = slash == '/'`,
		},
		{
			name: "rune literal can match string iteration cursor",
			input: `for ch in "a/b" {
  let label = match ch {
    '/' => "slash",
    _ => "other",
  }
}`,
		},
		{
			name:  "invalid rune literal reports diagnostic",
			input: `let bad = 'ab'`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Rune literal must contain exactly one Unicode scalar value"},
			},
		},
		{
			name:  "integer literals can contextually type as byte",
			input: `let b: Byte = 65`,
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
