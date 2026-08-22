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
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module errorinterop\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ffi", "ffi.go"), []byte(`package ffi

import (
	"errors"
	"fmt"
)

func Message(err error) string { return err.Error() }

var remembered error
var Sentinel error = errors.New("sentinel")

func Fail() error { return Sentinel }

func Load() (int, error) { return 0, fmt.Errorf("load failed: %w", Sentinel) }

func IsSentinel(err error) bool { return errors.Is(err, Sentinel) }

type Worker struct{}

type Failer interface {
	Fail() error
}

func (Worker) Fail() error { return Sentinel }

func AsFailer() Failer { return Worker{} }

func GenericFail[T any](value T) (T, error) { return value, Sentinel }

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
  calls: Int,
}

struct BothStringAndError {
  label: Str,
}

struct GenericError<$T> {
  value: $T,
}

impl Error for GenericError {
  fn error() Str { "generic" }
}

struct GenericMutableError<$T> {
  value: $T,
  calls: Int,
}

impl Error for GenericMutableError {
  fn error() Str {
    "generic mutable"
  }
}

impl BothStringAndError {
  fn to_str() Str { "string" }
}

impl Error for BothStringAndError {
  fn error() Str { "error" }
}

impl Error for MutableError {
  fn error() Str {
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
  let mutable_error = MutableError{message: "mutable", calls: 0}
  if not ffi::Message(mutable_error) == "mutable" { panic("Error implementation failed") }
  let interpolation_error: mut MutableError = mut MutableError{message: "interpolated", calls: 0}
  if not "{interpolation_error}" == "interpolated" { panic("referenced Error interpolation failed") }
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

  let failed = ffi::Fail()
  match failed {
    ok(_) => panic("expected imported error-only failure"),
    err(error) => {
      if not ffi::IsSentinel(error) { panic("error-only import lost identity") }
      if not "failure: {error}" == "failure: sentinel" { panic("Error interpolation failed") }
      if not ffi::IsSentinel(error) { panic("interpolation changed error identity") }
    },
  }

  let loaded = ffi::Load()
  match loaded {
    ok(_) => panic("expected imported value-error failure"),
    err(error) => {
      if not ffi::IsSentinel(error) { panic("wrapped import lost its error chain") }
      if not "{error}" == "load failed: sentinel" { panic("wrapped Error interpolation failed") }
    },
  }

  let fail: fn() Void!Error = ffi::Fail
  match fail() {
    ok(_) => panic("expected imported function-value failure"),
    err(error) => {
      if not ffi::IsSentinel(error) { panic("function value lost identity") }
    },
  }

  let worker = ffi::Worker{}
  match worker.Fail() {
    ok(_) => panic("expected imported method failure"),
    err(error) => {
      if not ffi::IsSentinel(error) { panic("method call lost identity") }
    },
  }
  let fail_method: fn() Void!Error = worker.Fail
  match fail_method() {
    ok(_) => panic("expected imported method-value failure"),
    err(error) => {
      if not ffi::IsSentinel(error) { panic("method value lost identity") }
    },
  }

  let failer = ffi::AsFailer()
  match failer.Fail() {
    ok(_) => panic("expected imported interface failure"),
    err(error) => {
      if not ffi::IsSentinel(error) { panic("interface method lost identity") }
    },
  }

  match ffi::GenericFail(7) {
    ok(_) => panic("expected imported generic failure"),
    err(error) => {
      if not ffi::IsSentinel(error) { panic("generic call lost identity") }
    },
  }

  let concrete_message = "{ValidationError{message: "concrete"}}"
  if not concrete_message == "concrete" { panic("concrete Error interpolation failed") }
  if not "{BothStringAndError{label: "both"}}" == "string" { panic("to_str should take interpolation precedence") }
  let widened_error: Error = BothStringAndError{label: "both"}
  if not "{widened_error}" == "error" { panic("Error fallback should use static Error contract") }
  let generic_error = GenericError<Int>{value: 1}
  if not "{generic_error}" == "generic" { panic("generic Error interpolation failed") }
  let generic_as_error: Error = generic_error
  if not "{generic_as_error}" == "generic" { panic("generic Error upcast failed") }
  let generic_mutable: mut GenericMutableError<Int> = mut GenericMutableError<Int>{value: 1, calls: 0}
  if not "{generic_mutable}" == "generic mutable" { panic("generic referenced Error interpolation failed") }
  let generic_mutable_as_error: Error = generic_mutable
  if not "{generic_mutable_as_error}" == "generic mutable" { panic("generic referenced Error upcast failed") }
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
