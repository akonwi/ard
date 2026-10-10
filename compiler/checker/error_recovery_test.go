package checker_test

import (
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
)

// An undefined variable must not stop the checker from reporting later
// errors (#523).
func TestUndefinedVariableDoesNotHaltChecking(t *testing.T) {
	run(t, []test{
		{
			name: "later top-level declarations are still checked",
			input: strings.Join([]string{
				`fn one() {`,
				`  let a = missing`,
				`}`,
				`fn two() {`,
				`  let b: Int = "two"`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
			},
		},
		{
			name: "later statements in the same body are still checked",
			input: strings.Join([]string{
				`fn main() {`,
				`  let a = missing`,
				`  let b: Int = "two"`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
			},
		},
		{
			name: "later top-level statements are still checked",
			input: strings.Join([]string{
				`let a = missing`,
				`let b: Int = "two"`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
			},
		},
		{
			name: "every undefined variable is reported",
			input: strings.Join([]string{
				`fn one() {`,
				`  let a = first`,
				`}`,
				`fn two() {`,
				`  let b = second`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: first"},
				{Kind: checker.Error, Message: "Undefined variable: second"},
			},
		},
	})
}

// A struct field value that fails to check is reported once, even though the
// checker retries it for mutable-reference auto-borrowing (#523).
func TestStructFieldUndefinedValueReportedOnce(t *testing.T) {
	run(t, []test{
		{
			name: "explicit field value",
			input: strings.Join([]string{
				`struct Person {`,
				`  name: Str,`,
				`}`,
				`let p = Person{name: missing}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "undefined function call as field value",
			input: strings.Join([]string{
				`struct Person {`,
				`  name: Str,`,
				`}`,
				`let p = Person{name: missing_fn()}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined function: missing_fn"},
			},
		},
		{
			name: "field value of a later struct literal is still checked",
			input: strings.Join([]string{
				`struct Person {`,
				`  name: Str,`,
				`}`,
				`let p = Person{name: missing}`,
				`let q = Person{name: 1}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
	})
}
