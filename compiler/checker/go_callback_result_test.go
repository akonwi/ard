package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

// TestGoCallbackResultAdaptation pins the Ard-facing types of Go callback
// parameters whose signatures carry adapted result shapes: error results
// become Result returns, comma-ok pairs become Maybe returns, and shapes
// beyond those stay rejected with an actionable diagnostic.
func TestGoCallbackResultAdaptation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module cbshapes\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ffi", "ffi.go"), []byte(`package ffi

func RunErr(cb func(n int) error)                {}
func RunPair(cb func(n int) (string, error))     {}
func RunOk(cb func(key string) (int, bool))      {}
func RunTuple(cb func(n int) (string, int))      {}

type WalkFunc func(n int) error

func RunNamed(cb WalkFunc) {}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		input       string
		diagnostics []checker.Diagnostic
	}{
		{
			name: "error-returning callback takes a Void Result closure",
			input: `use go:cbshapes/ffi

fn main() {
  ffi::RunErr(fn(n: Int) Void!Str { Result::ok(()) })
}`,
		},
		{
			name: "value-and-error callback takes a Result closure",
			input: `use go:cbshapes/ffi

fn main() {
  ffi::RunPair(fn(n: Int) Str!Str { Result::ok("{n}") })
}`,
		},
		{
			name: "comma-ok callback takes a Maybe closure",
			input: `use go:cbshapes/ffi

fn main() {
  ffi::RunOk(fn(key: Str) Int? { Maybe::new() })
}`,
		},
		{
			name: "named func type with error return takes a Result closure",
			input: `use go:cbshapes/ffi

fn main() {
  ffi::RunNamed(fn(n: Int) Void!Str { Result::ok(()) })
}`,
		},
		{
			name: "mismatched closure return is rejected",
			input: `use go:cbshapes/ffi

fn main() {
  ffi::RunErr(fn(n: Int) Int { n })
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected fn(Int) Void!Str, got fn(Int) Int"}},
		},
		{
			name: "non-error non-bool tuple callback stays rejected",
			input: `use go:cbshapes/ffi

fn main() {
  ffi::RunTuple(fn(n: Int) Str { "{n}" })
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported Go function ffi::RunTuple: parameter 1 has unsupported type func(n int) (string, int): callback multi-result shape (string, int) is not supported yet"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := checker.NewGoPackagesResolver(root, nil)
			result := parse.Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse error: %s", result.Errors[0].Message)
			}
			c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
			c.Check()
			if tt.name == "non-error non-bool tuple callback stays rejected" && (len(c.Diagnostics()) != 1 || c.Diagnostics()[0].Code != checker.DiagnosticCodeUnsupportedGoEntity) {
				t.Fatalf("structured unsupported function diagnostic = %#v", c.Diagnostics())
			}
			if len(tt.diagnostics) > 0 || c.HasErrors() {
				if diff := cmp.Diff(tt.diagnostics, c.Diagnostics(), compareOptions); diff != "" {
					t.Fatalf("diagnostics mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}
