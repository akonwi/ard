package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestScalarFrom(t *testing.T) {
	run(t, []test{
		{
			name:  "Int64::from types as Int64",
			input: `let x: Int64 = Int64::from(5)`,
		},
		{
			name: "from a runtime Int into a sized scalar",
			input: `fn f(n: Int) Uint32 {
  Uint32::from(n)
}`,
		},
		{
			name:  "fitting literal adopts the target",
			input: `let b: Uint8 = Uint8::from(200)`,
		},
		{
			name:  "overflowing constant is rejected like Go",
			input: `let b = Uint8::from(300)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Integer literal 300 overflows Uint8"},
			},
		},
		{
			name:  "non-numeric argument is rejected",
			input: `let x = Int64::from("nope")`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Int64::from expects a numeric value, got Str"},
			},
		},
		{
			name:  "wrong argument count is rejected",
			input: `let x = Int64::from(1, 2)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 1, got 2"},
			},
		},
		{
			// A foreign named type over an integer underlying (time.Month is an
			// int) is a valid numeric conversion target.
			name: "foreign named integer type accepts from",
			input: `use go:time
fn main() {
  let m = time::Month::from(3)
}`,
		},
		{
			// A foreign named type over a Str underlying (html/template.HTML is a
			// named string) is NOT numeric, so ::from is not a conversion here.
			name: "foreign named string type rejects from",
			input: `use go:html/template
fn main() {
  let x = template::HTML::from(1)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined Go function: template::HTML::from"},
			},
		},
	})
}
