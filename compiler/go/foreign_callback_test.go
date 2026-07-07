package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// TestRunProgramDiscardsClosureValueForVoidGoCallback pins that a closure
// whose body ends in a value-producing expression still lowers to a Go
// function with no results when the expected Go callback type has none.
// The closure's Go signature must come from the expected callback type, not
// from the body's final expression (which the checker already treats as
// discarded).
func TestRunProgramDiscardsClosureValueForVoidGoCallback(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"callbackvoid\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module callbackvoid\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ffi", "ffi.go"), []byte(`package ffi

type Callback func(x int)

var Ran = false

func Run(cb Callback) {
	cb(1)
	Ran = true
}

func RunPlain(cb func(x int)) {
	cb(2)
}

func DidRun() bool { return Ran }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:callbackvoid/ffi

fn main() {
  // The closure body's final expression produces a value (Int); the Go
  // callback returns nothing, so the value is discarded.
  ffi::Run(fn(x: Int) {
    "count: {x}".size()
  })
  if not ffi::DidRun() { panic("named callback did not run") }
  ffi::RunPlain(fn(x: Int) {
    "count: {x}".size()
  })
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

// TestRunProgramInfersClosureReturnFromBody pins that a stand-alone closure
// with no return annotation adopts its body's final expression type and
// compiles to a value-returning Go function (issue #266).
func TestRunProgramInfersClosureReturnFromBody(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"closureinfer\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`fn main() {
  let double = fn(x: Int) { x * 2 }
  let total = double(2) + double(3)
  if not total == 10 { panic("inferred closure returned {total}") }

  let describe = fn(flag: Bool) {
    match flag {
      true => "on",
      false => "off",
    }
  }
  if not describe(true) == "on" { panic("inferred match closure failed") }
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
