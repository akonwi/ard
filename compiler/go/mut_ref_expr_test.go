package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// TestRunProgramMutRefExpressions exercises ADR 0057 end to end: explicit
// references can target let storage, flow as first-class values, mutate the
// pointee, materialize shallow values with deref, and use fresh storage.
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
  // A let binding is stable addressable storage.
  let alice = Person{age: 30}
  let alice_ref = mut alice
  grow(alice_ref)
  if not alice.age == 31 { panic("explicit reference write lost") }

  // Copying a reference preserves pointee identity.
  let alias = alice_ref
  alias.age =+ 1
  if not alice.age == 32 { panic("reference copy lost pointee identity") }

  // Value materialization is explicit and shallow.
  let snapshot: Person = deref alice_ref
  alias.age =+ 1
  if not snapshot.age == 32 { panic("deref snapshot tracked later mutation") }

  // Fresh storage from value expressions.
  let fresh = mut Person{age: 30}
  fresh.age = 99
  if not fresh.age == 99 { panic("fresh literal storage write lost") }
  grow(mut Person{age: 1})

  let made = mut make_person()
  made.age =+ 5
  if not made.age == 15 { panic("fresh call storage write lost") }

  // Descriptor-backed references update the referenced descriptor/storage.
  let items = [1, 2]
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
// issue #257 and ADR 0057: `mut <addressable value>` produces the pointer form
// independently of whether the source binding slot is writable.
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
  let counter = ffi::Counter{N: 0}
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
