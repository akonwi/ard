package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

// TestGoFunctionsAsValues pins that imported Go functions are first-class
// values with their Ard-facing signatures: unadapted shapes reference the Go
// function directly, while adapted shapes (variadic, error or comma-ok
// results) carry the same adapted signature they have in call position.
// Generic functions still report an actionable diagnostic.
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
			name: "variadic Go function value takes a trailing Maybe parameter",
			input: `use go:fmt
use ard/maybe

fn main() {
  let print: fn(Any?) Int!Str = fmt::Println
  print("hello")
  print(maybe::none())
}`,
		},
		{
			name: "error-adapted Go function value carries its Result signature",
			input: `use go:os

fn main() {
  let read: fn(Str) [Byte]!Str = os::ReadFile
  let failed = read("missing.txt").is_err()
}`,
		},
		{
			name: "comma-ok-adapted Go function value carries its Maybe signature",
			input: `use go:os

fn main() {
  let lookup: fn(Str) Str? = os::LookupEnv
  let home = lookup("HOME").or("")
}`,
		},
		{
			name: "error-only Go function value carries a Void Result signature",
			input: `use go:os

fn main() Void!Str {
  let chdir: fn(Str) Void!Str = os::Chdir
  try chdir(".")
  Result::ok(())
}`,
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
