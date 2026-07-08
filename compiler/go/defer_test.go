package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// TestRunProgramFunctionScopedDefer pins ADR 0046 end-to-end: Ard defer is
// function-scoped and LIFO like Go, but both call and block forms execute
// later via closure capture rather than evaluating call arguments at
// registration time.
func TestRunProgramFunctionScopedDefer(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"defercase\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module defercase\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ffi", "ffi.go"), []byte(`package ffi

import "errors"

var Log string

func Reset() { Log = "" }
func Append(s string) { Log += s }
func Value() string { return Log }
func Fail(msg string) error { return errors.New(msg) }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:defercase/ffi

fn exercise() Void!Str {
  ffi::Reset()

  mut label = "first"
  defer ffi::Append(label)
  label = "late"

  defer {
    ffi::Append("-block")
  }

  for i in 0..2 {
    defer ffi::Append("-loop{i}")
  }

  defer {
    ffi::Fail("ignored cleanup failure")
  }

  try ffi::Fail("stop")
  Result::ok(())
}

fn main() {
  let result = exercise()
  if not result.is_err() { panic("expected exercise to fail") }
  let got = ffi::Value()
  if not got == "-loop2-loop1-loop0-blocklate" { panic("bad defer order/capture: {got}") }
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
