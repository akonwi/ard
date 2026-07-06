package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

// writeGoInterfaceABIPackage provides Go interfaces whose methods take a
// struct by value and by pointer, to pin the mut-parameter ABI rules.
func writeGoInterfaceABIPackage(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := `package ffi

type Payload struct {
	N int
}

type ValueTaker interface {
	Take(p Payload)
}

type PointerTaker interface {
	Take(p *Payload)
}
`
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGoInterfaceMutParameterABI pins how `mut` parameters interact with Go
// interface method ABIs after mutability became type syntax:
//
//   - a Go method expecting value Payload rejects `mut ffi::Payload` (the
//     pointer form changes the ABI) — reported as a type mismatch;
//   - a Go method expecting *Payload accepts `mut ffi::Payload` (mutation
//     flows through the pointer the ABI already carries);
//   - native mut parameters still trip the dedicated ABI diagnostic.
func TestGoInterfaceMutParameterABI(t *testing.T) {
	root := t.TempDir()
	writeGoInterfaceABIPackage(t, root)
	resolver := checker.NewGoPackagesResolver(root, nil)

	tests := []struct {
		name        string
		input       string
		diagnostics []checker.Diagnostic
	}{
		{
			name: "value-taking Go method rejects mut foreign struct param",
			input: `use go:example.com/app/ffi

struct Impl {}

impl ffi::ValueTaker for Impl {
  fn take(p: mut ffi::Payload) {
  }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected ffi::Payload, got mut ffi::Payload"}},
		},
		{
			name: "pointer-taking Go method accepts mut foreign struct param",
			input: `use go:example.com/app/ffi

struct Impl {}

impl ffi::PointerTaker for Impl {
  fn take(p: mut ffi::Payload) {
    p.N = 1
  }
}`,
		},
		{
			name: "native mut parameter still trips the ABI diagnostic",
			input: `use go:example.com/app/ffi

struct Impl {}

impl ffi::ValueTaker for Impl {
  fn take(p: mut Int) {
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected ffi::Payload, got Int"},
				{Kind: checker.Error, Message: "Go interface method 'take' parameter 'p' cannot be mutable because it would change the Go ABI"},
			},
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
