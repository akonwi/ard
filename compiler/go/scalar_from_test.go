package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// #284: `T::from(x)` is a truncating numeric conversion (like Go's T(x))
// into a bare sized scalar or a foreign named scalar type.
func TestGoTargetScalarFromBareScalars(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "Int to Int64 is lossless",
			input: `fn main() Bool {
  let x: Int = 5
  Int64::from(x) == 5
}`,
			want: "true",
		},
		{
			name: "runtime narrowing truncates like Go",
			input: `fn main() Bool {
  let n: Int = 300
  Uint8::from(n) == 44
}`,
			want: "true",
		},
		{
			name: "runtime Int narrows to Uint32",
			input: `fn main() Bool {
  let n: Int = 4294967297
  Uint32::from(n) == 1
}`,
			want: "true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			if got := runGoTargetParityJSON(t, program); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// The motivating case: converting a runtime Int to a foreign named scalar so
// it composes with idiomatic duration arithmetic.
func TestRunProgramScalarFromForeignNamed(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"durations\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module durations\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:time

fn ms(n: Int) time::Duration {
  time::Duration::from(n) * time::Millisecond
}

fn main() {
  let d = ms(2)
  time::Sleep(d)
  if not d == 2 * time::Millisecond { panic("from() duration mismatch") }
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
