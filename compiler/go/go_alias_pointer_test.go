package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// A pointer to a Go alias is the same Go pointer as one to the alias target:
// writes through it reach the shared Go value (#512).
func TestRunProgramGoPointerToAliasSharesTargetValue(t *testing.T) {
	projectDir := t.TempDir()
	files := map[string]string{
		"ard.toml": "name = \"aliases\"\nard = \">= 0.1.0\"\n",
		"go.mod":   "module aliases\n\ngo 1.27\n",
		"ffi/ffi.go": `package ffi

type Value struct{ N int }

type Alias = Value

type Update struct{ Snapshot *Alias }

var shared = &Value{N: 1}

func Current() Update { return Update{Snapshot: shared} }

func SharedN() int { return shared.N }

func Take(value *Alias) int { return value.N }

func All() []*Alias { return []*Alias{shared} }
`,
		"main.ard": `use go:aliases/ffi

fn snapshot(update: ffi::Update) &mut ffi::Alias {
  update.Snapshot
}

fn main() {
  let value: &mut ffi::Value = snapshot(ffi::Current())
  value.N = 42
  if ffi::SharedN() != 42 { panic("write through alias pointer lost: {ffi::SharedN()}") }
  if ffi::Take(value) != 42 { panic("alias pointer parameter") }
  for item in ffi::All() {
    item.N = item.N + 1
  }
  if ffi::SharedN() != 43 { panic("slice of alias pointers: {ffi::SharedN()}") }
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
