package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// Builtin Error is Go's predeclared `error`, so a Go value whose method set
// implements `error` is assignable to it under Go's rules, including the
// pointer method set of a `&T` / `&mut T` pointer (#513).
func TestGoErrorTypesAreAssignableToBuiltinError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte(`package ffi

type PointerError struct{ Code int }

func (e *PointerError) Error() string { return "pointer error" }

type ValueError struct{ Code int }

func (e ValueError) Error() string { return "value error" }

type Temporary interface {
	error
	Temporary() bool
}

type Plain struct{ N int }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		source  string
		wantErr bool
	}{
		{name: "writable pointer to pointer-receiver error", source: "let err: Error = &mut ffi::PointerError{Code: 1}"},
		{name: "pointer returned as Error", source: "fn make() Error {\n  &mut ffi::PointerError{Code: 1}\n}"},
		{name: "read-only pointer parameter", source: "fn widen(err: &ffi::PointerError) Error {\n  err\n}"},
		{name: "value-receiver error value", source: "let err: Error = ffi::ValueError{Code: 1}"},
		{name: "pointer to value-receiver error", source: "let err: Error = &mut ffi::ValueError{Code: 1}"},
		{name: "Go interface embedding error", source: "fn widen(err: ffi::Temporary) Error {\n  err\n}"},
		{name: "standard library pointer error to Go error parameter", source: "use go:errors\nuse go:io/fs\nuse go:os\n\nfn missing() Bool {\n  errors::Is(&mut os::PathError{Op: \"open\", Path: \"/missing\", Err: fs::ErrNotExist}, fs::ErrNotExist)\n}"},
		{name: "pointer-receiver error as a value", source: "let err: Error = ffi::PointerError{Code: 1}", wantErr: true},
		{name: "pointer to a non-error type", source: "let err: Error = &mut ffi::Plain{N: 1}", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "use go:example.com/app/ffi\n" + tt.source + "\n"
			result := parse.Parse([]byte(source), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			// Each source primes its own Go import set (ADR 0044).
			resolver := checker.NewGoPackagesResolver(root, nil)
			c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
			c.Check()
			if tt.wantErr {
				diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeTypeMismatch)
				if diagnostic.Kind != checker.Error {
					t.Fatalf("diagnostic = %#v, want type mismatch error", diagnostic)
				}
				return
			}
			if c.HasErrors() {
				t.Fatalf("checker diagnostics: %v", c.Diagnostics())
			}
		})
	}
}
