package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestRunProgramImportedGenericMethodClosureInheritsReceiverGeneric(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"closuretest\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	libDir := filepath.Join(projectDir, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "box.ard"), []byte(`fn identity(value: $T) $T { value }

struct Box<$T> { value: $T }

impl Box {
  fn callback() fn() Int {
    fn() Int {
      let copy = identity<$T>(self.value)
      let _ = copy
      42
    }
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use closuretest/lib/box

fn main() {
  let int_box = box::Box<Int>{value: 1}
  let str_box = box::Box<Str>{value: "one"}
  if int_box.callback()() != 42 { panic("unexpected int callback") }
  if str_box.callback()() != 42 { panic("unexpected str callback") }
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

func TestRunProgramLowersPrivateGenericCalledThroughImportedClosure(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"dectest\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	libDir := filepath.Join(projectDir, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "combo.ard"), []byte(`fn wrap(decoder: fn(Str) $T!Str) fn(Str) $T!Str {
  fn(s: Str) $T!Str {
    helper(s, 0, decoder)
  }
}

private fn helper(s: Str, depth: Int, decoder: fn(Str) $T!Str) $T!Str {
  match depth > 1 {
    true => decoder(s),
    false => helper(s, depth + 1, decoder),
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use dectest/lib/combo

fn main() {
  let decoder = combo::wrap(fn(s: Str) Int!Str { Result::ok(s.size()) })
  let result = decoder("abc")
  if result.or(0) != 3 { panic("unexpected result") }
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
	files := lowerProgramAST(t, program, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo})
	if astFilesHaveFuncContaining(files, "anon_func") {
		t.Fatal("generated AST should inline cross-module capturing closure bodies")
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
