package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// TestRunProgramAdaptedGoFunctionValues pins that adapted Go functions are
// first-class values: the compiler synthesizes the boundary adapter, so the
// stored value carries the Ard-facing signature (error results become
// Results, comma-ok becomes Maybe, a variadic tail becomes a trailing Maybe
// parameter).
func TestRunProgramAdaptedGoFunctionValues(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(projectDir, "ffi", "ffi.go"), []byte(`package ffi

import "errors"

func Parse(s string) (int, error) {
	if s == "bad" {
		return 0, errors.New("bad input")
	}
	return len(s), nil
}

func Validate(s string) error {
	if s == "bad" {
		return errors.New("invalid")
	}
	return nil
}

func Find(key string) (string, bool) {
	if key == "hit" {
		return "found", true
	}
	return "", false
}

func Join(prefix string, parts ...string) string {
	out := prefix
	for _, p := range parts {
		out += ":" + p
	}
	return out
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:fnvalues/ffi
use ard/maybe

fn apply(f: fn(Str) Int!Str, input: Str) Int {
  f(input).or(-1)
}

fn main() {
  // (T, error) adaptation
  let parse = ffi::Parse
  if not parse("four").or(-1) == 4 { panic("parse ok case failed") }
  if not parse("bad").or(-1) == -1 { panic("parse err case failed") }
  // adapted value flows as a function argument
  if not apply(ffi::Parse, "12345") == 5 { panic("adapted value as argument failed") }

  // error-only adaptation
  let validate = ffi::Validate
  if validate("ok").is_err() { panic("validate ok case failed") }
  if not validate("bad").is_err() { panic("validate err case failed") }

  // comma-ok adaptation
  let find = ffi::Find
  if not find("hit").or("") == "found" { panic("find hit case failed") }
  if find("miss").is_some() { panic("find miss case failed") }

  // variadic tail becomes a trailing Maybe parameter
  let join = ffi::Join
  if not join("a", "b") == "a:b" { panic("variadic some case failed") }
  if not join("a", maybe::none()) == "a" { panic("variadic none case failed") }
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
