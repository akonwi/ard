package transpile

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	env   map[string]string
	args  []string
	stdin string
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
			name: "try_fallback",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/maybe

fn half(n: Int) Int? {
  match n > 0 {
    true => maybe::some(n / 2),
    false => maybe::none(),
  }
}

fn maybe_fallback(n: Int) Int {
  let value = try half(n) -> _ {
    0
  }
  value + 1
}

fn main() {
  io::print(maybe_fallback(0))
  io::print(maybe_fallback(8))
}
`,
			},
		},
		{
			name: "map_operations",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  mut values: [Str: Int] = ["a": 1]
  values.set("b", 2)
  io::print(values.has("a").to_str())
  io::print(values.get("b").or(0))
  values.drop("a")
  io::print(values.has("a").to_str())
}
`,
			},
		},
		{
			name: "struct_methods",
			files: map[string]string{
				"main.ard": `
use ard/io

struct Box {
  value: Int,
}

impl Box {
  fn get() Int {
    self.value
  }

  fn mut set(value: Int) {
    self.value = value
  }
}

fn main() {
  mut box = Box{value: 1}
  box.set(2)
  io::print(box.get())
}
`,
			},
		},
		{
			name: "trait_dispatch",
			files: map[string]string{
				"main.ard": `
use ard/io

struct Book {
  title: Str,
  author: Str,
}

impl Str::ToString for Book {
  fn to_str() Str {
    "Book: " + self.title + " by " + self.author
  }
}

fn show(item: Str::ToString) {
  io::print(item)
}

fn main() {
  let book = Book{title: "The Hobbit", author: "J.R.R. Tolkien"}
  show(book)
}
`,
			},
		},
		{
			name: "env_lookup",
			env: map[string]string{
				"ARD_PARITY_VALUE": "parity-ok",
			},
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/env

fn main() {
  match env::get("ARD_PARITY_VALUE") {
    value => io::print(value),
    _ => io::print("missing")
  }
}
`,
			},
		},
		{
			name: "argv_load",
			args: []string{"alpha", "beta"},
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/argv

fn main() {
  let args = argv::load()
  io::print(args.program)
  io::print(args.arguments.size())
  io::print(args.arguments.at(0))
  io::print(args.arguments.at(1))
}
`,
			},
		},
		{
			name:  "stdin_read_line",
			stdin: "kit\n",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  match io::read_line() {
    ok(line) => io::print(line.trim()),
    err(message) => io::print(message),
  }
}
`,
			},
		},
		{
			name: "int_from_str",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  io::print(Int::from_str("42").or(-1))
  io::print(Int::from_str("oops").or(-1))
}
`,
			},
		},
		{
			name: "float_helpers",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  let parsed = Float::from_str("3.75").or(0.0)
  io::print(Float::floor(parsed))
  io::print(Float::from_str("oops").or(1.25))
}
`,
			},
		},
		{
			name: "hex_codec",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/hex

fn main() {
  let encoded = hex::encode("abc")
  io::print(encoded)
  match hex::decode(encoded) {
    ok(decoded) => io::print(decoded),
    err(message) => io::print(message),
  }
  io::print(hex::decode("zz").is_err().to_str())
}
`,
			},
		},
		{
			name: "base64_codec",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/base64

fn main() {
  let encoded = base64::encode("hello")
  io::print(encoded)
  match base64::decode(encoded) {
    ok(decoded) => io::print(decoded),
    err(message) => io::print(message),
  }
  let encoded_url = base64::encode_url("hello world", true)
  io::print(encoded_url)
  match base64::decode_url(encoded_url, true) {
    ok(decoded) => io::print(decoded),
    err(message) => io::print(message),
  }
  io::print(base64::decode("not!valid!base64").is_err().to_str())
}
`,
			},
		},
		{
			name: "json_encode",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/encode

fn main() {
  io::print(encode::json("hello").or("err"))
  io::print(encode::json(42).or("err"))
  io::print(encode::json(true).or("err"))
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

			vmArgs := append([]string{"run", "main.ard"}, tc.args...)
			vmResult := runArdCLI(t, ardPath, projectRoot, tc.env, tc.stdin, vmArgs...)
			if vmResult.err != nil {
				t.Fatalf("vm snippet run failed: %s", formatCLIRunFailure(vmResult))
			}

			goArgs := append([]string{"run", "--target", "go", "main.ard"}, tc.args...)
			goResult := runArdCLI(t, ardPath, projectRoot, tc.env, tc.stdin, goArgs...)
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

func runArdCLI(t *testing.T, ardPath, dir string, env map[string]string, stdin string, args ...string) cliRunResult {
	t.Helper()
	cmd := exec.Command(ardPath, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
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
