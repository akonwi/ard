package transpile

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type cliRunResult struct {
	stdout   string
	stderr   string
	err      error
	exitCode int
}

type cliSnippetCase struct {
	name  string
	files map[string]string
}

func TestCLIRunMatchesVMSnippetParity(t *testing.T) {
	ardPath := ensureArdBinary(t)

	cases := []cliSnippetCase{
		{
			name: "entrypoint_main",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  io::print("entrypoint ok")
}
`,
			},
		},
		{
			name: "else_if_preserves_fallback",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  for num in 1..6 {
    if num % 3 == 0 {
      io::print("Fizz")
    } else if num % 5 == 0 {
      io::print("Buzz")
    } else {
      io::print(num)
    }
  }
}
`,
			},
		},
		{
			name: "float_to_str_matches_vm",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  let celsius = (Float::from_int(0) - 32.0) * 5.0 / 9.0
  io::print(celsius.to_str())
}
`,
			},
		},
		{
			name: "user_module_import",
			files: map[string]string{
				"main.ard": `
use ard/io
use demo/maths

fn main() {
  io::print(maths::add(1, 2).to_str())
}
`,
				"maths.ard": `
fn add(a: Int, b: Int) Int {
  a + b
}
`,
			},
		},
		{
			name: "maybe_match",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/maybe

fn main() {
  let name: Str? = maybe::some("kit")
  match name {
    value => io::print(value),
    _ => io::print("none")
  }
}
`,
			},
		},
		{
			name: "result_match",
			files: map[string]string{
				"main.ard": `
use ard/io

fn divide(num: Int) Int!Str {
  match num == 0 {
    true => Result::err("zero"),
    false => Result::ok(10 / num),
  }
}

fn main() {
  match divide(2) {
    ok(value) => io::print(value),
    err(message) => io::print(message)
  }
}
`,
			},
		},
		{
			name: "enum_match",
			files: map[string]string{
				"main.ard": `
use ard/io

enum Color {
  Red,
  Yellow,
}

fn label(color: Color) Str {
  match color {
    Color::Red => "stop",
    Color::Yellow => "wait"
  }
}

fn main() {
  io::print(label(Color::Yellow))
}
`,
			},
		},
		{
			name: "union_match",
			files: map[string]string{
				"main.ard": `
use ard/io

struct Square {
  size: Int,
}

struct Circle {
  radius: Int,
}

type Shape = Square | Circle

fn label(shape: Shape) Str {
  match shape {
    Square => "square {it.size}",
    Circle => "circle {it.radius}"
  }
}

fn main() {
  let shapes: [Shape] = [Square { size: 2 }, Circle { radius: 3 }]
  for shape in shapes {
    io::print(label(shape))
  }
}
`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot := writeSnippetProject(t, tc.files)

			vmResult := runArdCLI(t, ardPath, projectRoot, "run", "main.ard")
			if vmResult.err != nil {
				t.Fatalf("vm snippet run failed: %s", formatCLIRunFailure(vmResult))
			}

			goResult := runArdCLI(t, ardPath, projectRoot, "run", "--target", "go", "main.ard")
			if goResult.err != nil {
				t.Fatalf("go snippet run failed: %s", formatCLIRunFailure(goResult))
			}

			if vmResult.exitCode != goResult.exitCode || vmResult.stdout != goResult.stdout || vmResult.stderr != goResult.stderr {
				t.Fatalf("snippet parity mismatch\nvm: %s\ngo: %s", formatCLIRunFailure(vmResult), formatCLIRunFailure(goResult))
			}
		})
	}
}

func ensureArdBinary(t *testing.T) string {
	t.Helper()
	compilerRoot, err := compilerModuleRoot()
	if err != nil {
		t.Fatalf("failed to determine compiler root: %v", err)
	}
	ardPath := filepath.Join(t.TempDir(), "ard")
	cmd := exec.Command("go", "build", "-o", ardPath, ".")
	cmd.Dir = compilerRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build ard CLI: %v\n%s", err, string(output))
	}
	return ardPath
}

func writeSnippetProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\ntarget = \"bytecode\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	for path, content := range files {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("failed to create directory for %s: %v", path, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}
	return root
}

func runArdCLI(t *testing.T, ardPath, dir string, args ...string) cliRunResult {
	t.Helper()
	cmd := exec.Command(ardPath, args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := cliRunResult{
		stdout: normalizeOutput(stdout.String()),
		stderr: normalizeOutput(stderr.String()),
		err:    err,
	}
	if err == nil {
		return result
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.exitCode = exitErr.ExitCode()
		return result
	}
	result.exitCode = -1
	return result
}

func formatCLIRunFailure(result cliRunResult) string {
	return fmt.Sprintf("exit=%d err=%v\nstdout:\n%s\nstderr:\n%s", result.exitCode, result.err, result.stdout, result.stderr)
}
