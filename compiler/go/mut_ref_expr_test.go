package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// TestRunProgramMutRefExpressions exercises ADR 0045 end to end: explicit
// `mut` expressions create references whose writes are visible through the
// original storage, fresh storage binds, aliases chain, descriptor-backed
// referents share storage without pointers, and explicit `mut` arguments
// reach `mut T` parameters.
func TestRunProgramMutRefExpressions(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"mutref\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`struct Person {
  age: Int,
}

fn grow(person: mut Person) {
  person.age =+ 1
}

fn make_person() Person {
  Person{age: 10}
}

fn main() {
  // Alias to a scalar binding: writes flow both ways.
  mut counter = 0
  let r = mut counter
  r =+ 1
  if not counter == 1 { panic("write through alias lost") }
  counter =+ 1
  let seen: Int = r
  if not seen == 2 { panic("read through alias stale") }

  // Alias of an alias reaches the same storage.
  let rr = mut r
  rr =+ 1
  if not counter == 3 { panic("chained alias write lost") }

  // Binding a reference without mut copies the referent.
  let copy: Int = r
  counter =+ 1
  if not copy == 3 { panic("copy should not track referent") }

  // Fresh storage from a value expression.
  let fresh = mut Person{age: 30}
  fresh.age = 99
  if not fresh.age == 99 { panic("fresh storage write lost") }

  // Explicit mut argument to a mut parameter.
  mut alice = Person{age: 30}
  grow(mut alice)
  if not alice.age == 31 { panic("explicit mut arg write lost") }

  // Explicit mut argument of fresh storage.
  grow(mut Person{age: 1})

  // Fresh storage from a call result (temporary + address-of path).
  let made = mut make_person()
  made.age =+ 5
  if not made.age == 15 { panic("fresh call storage write lost") }

  // Descriptor-backed referent: element writes share storage by value,
  // matching mut-parameter semantics.
  mut items = [1, 2]
  let list_ref = mut items
  list_ref.set(0, 9)
  if not items.at(0).or(0) == 9 { panic("descriptor alias element write lost") }
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

// TestRunProgramMutRefSatisfiesGoInterface pins the motivating case from
// issue #257: `mut <value>` produces the pointer form, so a Go interface
// whose methods have pointer receivers is satisfied.
func TestRunProgramMutRefSatisfiesGoInterface(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"mutrefiface\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module mutrefiface\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ffi", "ffi.go"), []byte(`package ffi

type Counter struct {
	N int
}

// Bump has a pointer receiver, so only *Counter satisfies Bumper.
func (c *Counter) Bump() { c.N++ }

type Bumper interface {
	Bump()
}

func BumpTwice(b Bumper) {
	b.Bump()
	b.Bump()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:mutrefiface/ffi

fn main() {
  mut counter = ffi::Counter{N: 0}
  ffi::BumpTwice(mut counter)
  if not counter.N == 2 { panic("pointer-receiver interface writes lost") }
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
