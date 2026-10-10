package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// Go values that implement Go's `error` flow into builtin Error as native Go
// error values: a pointer keeps its identity and concrete type, so message,
// unwrap chain, and errors.As all observe the original value (#513).
func TestRunProgramGoErrorTypesConvertToBuiltinError(t *testing.T) {
	projectDir := t.TempDir()
	files := map[string]string{
		"ard.toml": "name = \"goerrors\"\nard = \">= 0.1.0\"\n",
		"go.mod":   "module goerrors\n\ngo 1.27\n",
		"ffi/ffi.go": `package ffi

import (
	"errors"
	"fmt"
)

type ServerError struct{ Code int }

func (e *ServerError) Error() string { return fmt.Sprintf("server error %d", e.Code) }

type ValueError struct{ Code int }

func (e ValueError) Error() string { return fmt.Sprintf("value error %d", e.Code) }

func IsServerError(err error) bool {
	var server *ServerError
	return errors.As(err, &server)
}
`,
		"main.ard": `use go:errors
use go:goerrors/ffi
use go:io/fs
use go:os

fn path_error() Error {
  &mut os::PathError{Op: "open", Path: "/missing", Err: fs::ErrNotExist}
}

fn main() {
  let err = path_error()
  if err.error() != "open /missing: file does not exist" { panic("message = {err.error()}") }
  if not errors::Is(err, fs::ErrNotExist) { panic("unwrap chain lost") }

  let server = &mut ffi::ServerError{Code: 500}
  let as_error: Error = server
  server.Code = 503
  if as_error.error() != "server error 503" { panic("pointer identity lost: {as_error.error()}") }
  if not ffi::IsServerError(as_error) { panic("concrete type lost") }

  let value: Error = ffi::ValueError{Code: 7}
  if value.error() != "value error 7" { panic("value error = {value.error()}") }
}
`,
	}
	for name, contents := range files {
		path := filepath.Join(projectDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(projectDir, "main.ard")
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
