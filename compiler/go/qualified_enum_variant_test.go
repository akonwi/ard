package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestRunProgramResolvesQualifiedEnumVariantsThroughAliases(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"enumrun\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "types.ard"), []byte(`enum Command {
  help,
  run = 7,
}
type Alias = Command
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "barrel.ard"), []byte(`use enumrun/types

type Reexport = types::Command
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use enumrun/types
use enumrun/barrel

fn rank(command: types::Command) Int {
  match command {
    types::Command::help => 1,
    types::Command::run => 2,
  }
}

fn is_run(value: Int) Bool {
  match value {
    types::Command::run => true,
    _ => false,
  }
}

fn main() {
  if rank(types::Command::help) != 1 {
    panic("direct qualified variant failed")
  }
  if rank(types::Alias::run) != 2 {
    panic("enum alias variant failed")
  }
  if rank(barrel::Reexport::help) != 1 {
    panic("re-exported enum variant failed")
  }
  if not is_run(7) {
    panic("qualified custom discriminant match failed")
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
