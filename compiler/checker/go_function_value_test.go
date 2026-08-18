package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

// TestGoFunctionsAsValues pins that imported Go functions are first-class
// values with their Ard-facing signatures. Variadicity is part of the callable
// type and survives assignment and explicitly typed function boundaries.
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
			name: "variadic Go function value accepts zero and repeated arguments",
			input: `use go:fmt

fn invoke(print: fn(...Any) Int!Error) Int!Error {
  print("hello", 42, true)
}

fn main() {
  let print: fn(...Any) Int!Error = fmt::Println
  let rebound = print
  print()
  rebound("hello")
  invoke(rebound)
}`,
		},
		{
			name: "variadic Go function value accepts referenced list spread",
			input: `use go:fmt

fn invoke(print: fn(...Any) Int!Error, values: mut [Any]) Int!Error {
  print(values...)
}

fn main() {
  let print: fn(...Any) Int!Error = fmt::Println
  let values: [Any] = ["hello", 42, true]
  let reference = mut values
  print(reference...)
  invoke(print, reference)
}`,
		},
		{
			name: "generic Ard forwarding cannot hide descriptor element ABI",
			input: `fn forward(call: fn(...$T), values: mut [$T]) {
  call(values...)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Variadic spread requires a concrete element type, got $T"}},
		},
		{
			name: "generic spread source must also have a concrete element",
			input: `fn forward(call: fn(...Int), values: mut [$T]) {
  call(values...)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Variadic spread requires a concrete element type, got $T"}},
		},
		{
			name: "fixed and variadic callable types are distinct",
			input: `use go:fmt

fn main() {
  let print: fn(Any) Int!Error = fmt::Println
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected fn(Any) Int!Error, got fn(...Any) Int!Error"}},
		},
		{
			name: "Ard closure cannot acquire variadicity from context",
			input: `fn main() {
  let join: fn(...Str) Str = fn(value: Str) Str { value }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected fn(...Str) Str, got fn(Str) Str"}},
		},
		{
			name: "error-adapted Go function value carries its Result signature",
			input: `use go:os

fn main() {
  let read: fn(Str) [Byte]!Error = os::ReadFile
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

fn main() Void!Error {
  let chdir: fn(Str) Void!Error = os::Chdir
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
