package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestGoGenericStructLiterals(t *testing.T) {
	root := t.TempDir()
	writeGoGenericStructPackage(t, root)
	resolver := checker.NewGoPackagesResolver(root, nil)

	tests := []test{
		{
			name: "explicit type arg on Go generic struct literal",
			input: `use go:example.com/app/ffi

let box = ffi::Box<Str>{Value: "hello"}`,
		},
		{
			name: "infer type arg from supplied Go struct field",
			input: `use go:example.com/app/ffi

let box = ffi::Box{Value: "hello"}`,
		},
		{
			name: "infer slice element type arg from supplied field",
			input: `use go:example.com/app/ffi

let list_box = ffi::ListBox{Values: [1, 2]}`,
		},
		{
			name: "infer shared type arg from multiple fields",
			input: `use go:example.com/app/ffi

let radio = ffi::Radio{Value: "compact", GroupValue: "cozy"}`,
		},
		{
			name: "reject conflicting inferred type args",
			input: `use go:example.com/app/ffi

let radio = ffi::Radio{Value: "compact", GroupValue: 1}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Conflicting inferred type arguments for T: Str and Int"}},
		},
		{
			name: "require explicit args when fields do not constrain type param",
			input: `use go:example.com/app/ffi

let box = ffi::Box{Label: "empty"}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Could not infer type argument T for Go type ffi::Box"}},
		},
		{
			name: "does not duplicate diagnostics from non-inference fields",
			input: `use go:example.com/app/ffi

let box = ffi::Box{Value: "x", Label: "a" + 1}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot add different types"}},
		},
		{
			name: "enforce comparable constraint",
			input: `use go:example.com/app/ffi

let radio = ffi::Radio<[Int]>{Value: [1], GroupValue: [2]}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type argument [Int] does not satisfy Go constraint comparable"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func writeGoGenericStructPackage(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := `package ffi

type Box[T any] struct {
	Value T
	Label string
}

type Radio[T comparable] struct {
	Value T
	GroupValue T
}

type ListBox[T any] struct {
	Values []T
}
`
	if err := os.WriteFile(filepath.Join(ffiDir, "generic.go"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
