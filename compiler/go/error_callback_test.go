package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// TestRunProgramErrorReturningGoCallbacks pins that Go callback parameters
// whose signatures return an error or comma-ok pair adapt to Ard closures,
// mirroring the call-boundary adaptation in reverse: `func(...) error` takes
// an Ard `fn(...) Void!Str`, `func(...) (T, error)` takes `fn(...) T!Str`,
// and `func(...) (T, bool)` takes `fn(...) T?`. The Ard returns already
// lower to those Go ABI shapes (ADR 0038), so no wrapper is generated.
func TestRunProgramErrorReturningGoCallbacks(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"errcb\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module errcb\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ffi", "ffi.go"), []byte(`package ffi

// Walk visits values until the callback fails, mirroring the
// filepath.WalkDir / errgroup.Go callback convention.
func Walk(cb func(n int) error) string {
	for n := 1; n <= 3; n++ {
		if err := cb(n); err != nil {
			return err.Error()
		}
	}
	return "done"
}

// Transform mirrors the (T, error) callback convention.
func Transform(n int, cb func(n int) (string, error)) string {
	out, err := cb(n)
	if err != nil {
		return "error: " + err.Error()
	}
	return out
}

// Lookup mirrors the comma-ok callback convention.
func Lookup(cb func(key string) (int, bool)) string {
	if v, ok := cb("hit"); ok && v == 7 {
		if _, miss := cb("miss"); !miss {
			return "found"
		}
	}
	return "wrong"
}

// WalkFunc pins the named-func-type variant of the error convention.
type WalkFunc func(n int) error

func Named(cb WalkFunc) string {
	if err := cb(5); err != nil {
		return err.Error()
	}
	return "ok"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:errcb/ffi

fn main() {
  // error-only callback: succeed for every value
  let all = ffi::Walk(fn(n: Int) Void!Str {
    Result::ok(())
  })
  if not all == "done" { panic("expected full walk, got {all}") }

  // error-only callback: fail midway and surface the message through Go
  let stopped = ffi::Walk(fn(n: Int) Void!Str {
    match n == 2 {
      true => Result::err("stop at {n}"),
      false => Result::ok(()),
    }
  })
  if not stopped == "stop at 2" { panic("expected early stop, got {stopped}") }

  // (T, error) callback: ok case
  let doubled = ffi::Transform(21, fn(n: Int) Str!Str {
    Result::ok("{n * 2}")
  })
  if not doubled == "42" { panic("expected 42, got {doubled}") }

  // (T, error) callback: err case
  let failed = ffi::Transform(1, fn(n: Int) Str!Str {
    Result::err("bad input")
  })
  if not failed == "error: bad input" { panic("expected error passthrough, got {failed}") }

  // comma-ok callback: Maybe return maps to (T, bool)
  let looked = ffi::Lookup(fn(key: Str) Int? {
    match key == "hit" {
      true => Maybe::new(7),
      false => Maybe::new(),
    }
  })
  if not looked == "found" { panic("expected comma-ok lookup, got {looked}") }

  // named Go func type with an error return
  let named = ffi::Named(fn(n: Int) Void!Str {
    Result::err("n was {n}")
  })
  if not named == "n was 5" { panic("expected named callback error, got {named}") }

  // a named Ard function (not a literal) as the callback
  let checked = ffi::Walk(check)
  if not checked == "stop at 3" { panic("expected named fn callback, got {checked}") }

  // a capturing closure as the callback
  let limit = 1
  let captured = ffi::Walk(fn(n: Int) Void!Str {
    match n > limit {
      true => Result::err("over {limit}"),
      false => Result::ok(()),
    }
  })
  if not captured == "over 1" { panic("expected capturing callback, got {captured}") }
}

fn check(n: Int) Void!Str {
  match n == 3 {
    true => Result::err("stop at {n}"),
    false => Result::ok(()),
  }
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
