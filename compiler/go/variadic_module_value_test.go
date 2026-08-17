package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestRunProgramCallsImportedVariadicFunctionValue(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"variadicmodule\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module variadicmodule\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	libDir := filepath.Join(projectDir, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "join.ard"), []byte(`use go:variadicmodule/ffi

let join: fn(Str, ...Str) Str = ffi::Join
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(projectDir, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte(`package ffi

func Join(prefix string, parts ...string) string {
	out := prefix
	for _, part := range parts { out += ":" + part }
	return out
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use variadicmodule/lib/join

fn main() {
  if not join::join("a") == "a" { panic("zero tail failed") }
  if not join::join("a", "b", "c") == "a:b:c" { panic("repeated tail failed") }
  let values = ["b", "c"]
  if not join::join("a", values...) == "a:b:c" { panic("spread tail failed") }
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
