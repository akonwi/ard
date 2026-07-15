package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestRunProgramBuiltinErrorInterop(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"errorinterop\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module errorinterop\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ffi", "ffi.go"), []byte(`package ffi

func Message(err error) string { return err.Error() }

var remembered error

func Remember(err error) { remembered = err }

func Same(err error) bool { return err == remembered }

type Holder struct { Err error }

func HolderMessage(holder Holder) string { return holder.Err.Error() }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:errorinterop/ffi

struct ValidationError {
  message: Str,
}

impl Error for ValidationError {
  fn error() Str {
    self.message
  }
}

struct MutableError {
  message: Str,
}

impl Error for MutableError {
  fn mut error() Str {
    self.message
  }
}

fn fail() Int!Error {
  Result::err(Error::new("failed"))
}

fn passthrough(result: Int!Error) Int!Error {
  result
}

fn propagate() Int!Error {
  let value = try fail()
  Result::ok(value)
}

fn passthrough_void(result: Void!Error) Void!Error {
  result
}

fn propagate_void(error: Error) Void!Error {
  try passthrough_void(Result::err(error))
  Result::ok(())
}

fn succeed_void() Void!Error {
  Result::ok(())
}

fn main() {
  let custom = ValidationError{message: "custom"}
  if not ffi::Message(custom) == "custom" { panic("custom Error implementation failed") }
  if not ffi::Message(Error::new("simple")) == "simple" { panic("Error::new failed") }
  mut mutable_error = MutableError{message: "mutable"}
  if not ffi::Message(mutable_error) == "mutable" { panic("mutable Error implementation failed") }
  let holder = ffi::Holder{Err: custom}
  if not ffi::HolderMessage(holder) == "custom" { panic("Go error field failed") }
  let message = match passthrough(fail()) {
    ok(_) => "unexpected",
    err(error) => error.error(),
  }
  if not message == "failed" { panic("packed Error result lost value") }
  let propagated = match propagate() {
    ok(_) => "unexpected",
    err(error) => error.error(),
  }
  if not propagated == "failed" { panic("try lost Error value") }
  let identity = Error::new("identity")
  ffi::Remember(identity)
  let same = match propagate_void(identity) {
    ok(_) => false,
    err(error) => ffi::Same(error),
  }
  if not same { panic("Error identity was not preserved") }
  let succeeded = match succeed_void() {
    ok(_) => true,
    err(_) => false,
  }
  if not succeeded { panic("expected void success") }
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
