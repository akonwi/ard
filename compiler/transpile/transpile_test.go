package transpile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func checkedModuleFromSource(t *testing.T, dir, fileName, source string) checker.Module {
	t.Helper()
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	result := parse.Parse([]byte(source), path)
	if len(result.Errors) > 0 {
		result.PrintErrors()
		t.Fatalf("unexpected parse errors")
	}

	resolver, err := checker.NewModuleResolver(dir)
	if err != nil {
		t.Fatalf("failed to create module resolver: %v", err)
	}

	c := checker.New(fileName, result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		for _, diagnostic := range c.Diagnostics() {
			t.Log(diagnostic)
		}
		t.Fatalf("unexpected checker errors")
	}

	return c.Module()
}

func TestEmitEntrypoint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn add(a: Int, b: Int) Int {
  let sum = a + b
  sum
}

let result = add(1, 2)
`)

	out, err := EmitEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	checks := []string{
		"package main",
		"func Add(a int, b int) int",
		"sum := (a + b)",
		"return sum",
		"func main()",
		"result := Add(1, 2)",
		"_ = result",
	}
	for _, check := range checks {
		if !strings.Contains(generated, check) {
			t.Fatalf("expected generated source to contain %q\n%s", check, generated)
		}
	}
	if strings.Contains(generated, "_ = sum") {
		t.Fatalf("did not expect generated source to contain redundant discard for used local\n%s", generated)
	}
}

func TestEmitEntrypointUnusedLocalGetsDiscard(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn noop() {
  let sum = 1
}

noop()
`)

	out, err := EmitEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	if !strings.Contains(generated, "_ = sum") {
		t.Fatalf("expected generated source to contain discard for unused local\n%s", generated)
	}
}

func TestBuildBinaryCompilesUserModuleImport(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utils.ard"), []byte(`
fn add(a: Int, b: Int) Int {
  a + b
}
`), 0o644); err != nil {
		t.Fatalf("failed to write utils source: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use demo/utils

let result = utils::add(1, 2)
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	builtPath, err := BuildBinary(mainPath, outputPath)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("expected built path %q, got %q", outputPath, builtPath)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected output binary to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "generated", "utils", "utils.go")); err != nil {
		t.Fatalf("expected generated module file to exist: %v", err)
	}
}

func TestBuildBinaryCompilesIfReturn(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn choose(num: Int) Int {
  if num > 1 {
    10
  } else {
    20
  }
}

let result = choose(2)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesWhileLoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn count(limit: Int) Int {
  mut total = 0
  while total < limit {
    total = total + 1
  }
  total
}

let result = count(3)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesForLoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn count(limit: Int) Int {
  mut total = 0
  for mut i = 0; i < limit; i = i + 1 {
    total = total + 1
  }
  total
}

let result = count(3)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesBreakInWhileLoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn count(limit: Int) Int {
  mut total = 0
  while total < limit {
    total = total + 1
    break
  }
  total
}

let result = count(3)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesStructLiteralAndFieldAccess(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
struct Person {
  age: Int,
}

fn get_age() Int {
  let person = Person{age: 30}
  person.age
}

let result = get_age()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesImportedModuleStructLiteral(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models.ard"), []byte(`
struct Person {
  age: Int,
}
`), 0o644); err != nil {
		t.Fatalf("failed to write models source: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use demo/models

fn get_age() Int {
  let person = models::Person{age: 30}
  person.age
}

let result = get_age()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesImportedModuleSymbol(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utils.ard"), []byte(`
let answer = 42
`), 0o644); err != nil {
		t.Fatalf("failed to write utils source: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use demo/utils

let result = utils::answer
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	builtPath, err := BuildBinary(mainPath, outputPath)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("expected built path %q, got %q", outputPath, builtPath)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected output binary to exist: %v", err)
	}
}

func TestBuildBinaryCompilesSimpleProgram(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn add(a: Int, b: Int) Int {
  a + b
}

let result = add(1, 2)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	builtPath, err := BuildBinary(mainPath, outputPath)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("expected built path %q, got %q", outputPath, builtPath)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected output binary to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "generated", "go.mod")); err != nil {
		t.Fatalf("expected generated go.mod to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "generated", "main.go")); err != nil {
		t.Fatalf("expected generated main.go to exist: %v", err)
	}
}
