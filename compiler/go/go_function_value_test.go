package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// TestRunProgramUsesGoFunctionsAsValues pins that imported Go functions with
// unadapted signatures are first-class values: bindable, callable through
// the binding, passable to Go higher-order functions, and passable to
// Ard functions expecting a function type.
func TestRunProgramUsesGoFunctionsAsValues(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"fnvalues\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module fnvalues\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A chi-middleware-shaped higher-order function: takes and returns a
	// named func type.
	if err := os.WriteFile(filepath.Join(projectDir, "ffi", "ffi.go"), []byte(`package ffi

type Transform func(s string) string

func Twice(t Transform) Transform {
	return func(s string) string { return t(t(s)) }
}

func Bang(s string) string { return s + "!" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:strings
use go:fnvalues/ffi

fn apply(text: Str, transform: fn(Str) Str) Str {
  transform(text)
}

fn main() {
  // bind and call through the binding
  let upper = strings::ToUpper
  if not upper("hi") == "HI" { panic("call through binding failed") }

  // pass to an Ard function expecting a function type
  if not apply("go", strings::ToUpper) == "GO" { panic("ard higher-order failed") }

  // pass to a Go higher-order function taking a named func type
  let double_bang = ffi::Twice(ffi::Bang)
  if not double_bang("ok") == "ok!!" { panic("go higher-order failed") }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
