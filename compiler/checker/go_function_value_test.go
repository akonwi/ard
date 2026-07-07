package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

// TestGoFunctionsAsValues pins that imported Go functions whose signatures
// need no boundary adaptation are first-class values, while adapted shapes
// (variadic, error or comma-ok results) and generic functions report an
// actionable diagnostic instead.
func TestGoFunctionsAsValues(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		diagnostics []checker.Diagnostic
	}{
		{
			name: "plain Go function binds and calls as a value",
			input: `use go:strings

fn main() {
  let upper = strings::ToUpper
  let shout: fn(Str) Str = strings::ToUpper
  let loud = upper("hi")
  let louder = shout(loud)
}`,
		},
		{
			name: "Go function reference satisfies a Go callback parameter",
			// strings.IndexFunc(s string, f func(rune) bool) int and
			// unicode.IsUpper(r rune) bool.
			input: `use go:strings
use go:unicode

fn main() {
  let at = strings::IndexFunc("aB", unicode::IsUpper)
}`,
		},
		{
			name: "variadic Go function cannot be referenced as a value",
			input: `use go:fmt

fn main() {
  let print = fmt::Println
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Go function fmt::Println cannot be referenced as a value: it is variadic; wrap it in a closure"}},
		},
		{
			name: "error-adapted Go function cannot be referenced as a value",
			input: `use go:os

fn main() {
  let read = os::ReadFile
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Go function os::ReadFile cannot be referenced as a value: it returns a Go error, which Ard adapts to a Result at call sites; wrap it in a closure"}},
		},
		{
			name: "comma-ok-adapted Go function cannot be referenced as a value",
			input: `use go:os

fn main() {
  let lookup = os::LookupEnv
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Go function os::LookupEnv cannot be referenced as a value: its comma-ok result is adapted to a Maybe at call sites; wrap it in a closure"}},
		},
		{
			name: "generic Go function cannot be referenced as a value",
			input: `use go:slices

fn main() {
  let sorted = slices::Sort
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Generic Go function slices::Sort cannot be referenced as a value; wrap it in a closure so its type parameters are fixed"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := checker.NewGoPackagesResolver(t.TempDir(), nil)
			result := parse.Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse error: %s", result.Errors[0].Message)
			}
			c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
			c.Check()
			if len(tt.diagnostics) > 0 || c.HasErrors() {
				if diff := cmp.Diff(tt.diagnostics, c.Diagnostics(), compareOptions); diff != "" {
					t.Fatalf("diagnostics mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}
