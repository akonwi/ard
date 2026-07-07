package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// TestRunProgramForeignScalarLiteralArithmetic pins Go-style untyped-literal
// arithmetic against foreign named scalars: `5 * time::Second` types as
// time::Duration and flows into Go APIs expecting it.
func TestRunProgramForeignScalarLiteralArithmetic(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"durations\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module durations\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:time

let five_millis: time::Duration = 5 * time::Millisecond

fn main() {
  time::Sleep(five_millis)
  time::Sleep(time::Millisecond * 2)
  if not five_millis > 0 { panic("duration comparison failed") }
  let total = five_millis + five_millis
  if not total == 10 * time::Millisecond { panic("duration addition failed") }
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
