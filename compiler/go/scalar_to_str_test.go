package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestRunProgramGoInt64ToStr(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"scalartostr\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module scalartostr\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:time

fn render_duration(value: mut time::Duration) Str {
  value.to_str()
}

fn main() {
  let value: Int64 = time::Second.Nanoseconds()
  if value.to_str() != "1000000000" {
    panic("Int64.to_str mismatch")
  }
  if time::Second.to_str() != "1000000000" {
    panic("time.Duration.to_str mismatch")
  }
  let duration: time::Duration = time::Second
  if render_duration(mut duration) != "1000000000" {
    panic("mut time.Duration.to_str mismatch")
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

func TestGoTargetWholeValueFloat64LiteralKeepsFloatType(t *testing.T) {
	program := lowerParitySource(t, `fn main() Str {
  let value = 3.0
  value.to_str()
}`)
	if got := runGoTargetParityJSON(t, program); got != `"3.00"` {
		t.Fatalf("whole-value Float64.to_str() result mismatch: got %s", got)
	}
}

func TestGoTargetSizedScalarToStrMutableReference(t *testing.T) {
	program := lowerParitySource(t, `fn render(value: mut Int64) Str {
  value.to_str()
}

fn main() Bool {
  let value: Int64 = 42
  render(mut value) == "42"
}`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("mut Int64.to_str() result mismatch: got %s", got)
	}
}

func TestGoTargetSizedScalarToStr(t *testing.T) {
	cases := []struct {
		name       string
		scalarType string
		literal    string
		want       string
	}{
		{name: "int8", scalarType: "Int8", literal: "-8", want: "-8"},
		{name: "int16", scalarType: "Int16", literal: "-16", want: "-16"},
		{name: "int32", scalarType: "Int32", literal: "-32", want: "-32"},
		{name: "int64", scalarType: "Int64", literal: "-64", want: "-64"},
		{name: "uint", scalarType: "Uint", literal: "10", want: "10"},
		{name: "uint8", scalarType: "Uint8", literal: "8", want: "8"},
		{name: "uint16", scalarType: "Uint16", literal: "16", want: "16"},
		{name: "uint32", scalarType: "Uint32", literal: "32", want: "32"},
		{name: "uint64", scalarType: "Uint64", literal: "64", want: "64"},
		{name: "uintptr", scalarType: "Uintptr", literal: "128", want: "128"},
		{name: "float32", scalarType: "Float32", literal: "1.5", want: "1.5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := "fn main() Bool {\n" +
				"  let value: " + tc.scalarType + " = " + tc.literal + "\n" +
				"  value.to_str() == \"" + tc.want + "\"\n" +
				"}"
			program := lowerParitySource(t, source)
			if got := runGoTargetParityJSON(t, program); got != "true" {
				t.Fatalf("%s.to_str() result mismatch: got %s", tc.scalarType, got)
			}
		})
	}
}
