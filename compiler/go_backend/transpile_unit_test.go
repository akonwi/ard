package go_backend

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

func TestCompileEntrypoint(t *testing.T) {
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

	out, err := CompileEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	checks := []string{
		"package main",
		"func Add(a int, b int) int",
		"sum := a + b",
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

func TestCompileEntrypointNestedTryCatchHoistsSuccessValue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/maybe

fn half(n: Int) Int? {
  match n > 0 {
    true => maybe::some(n / 2),
    false => maybe::none(),
  }
}

fn maybe_fallback(n: Int) Int {
  let value = (try half(n) -> _ { 0 }) + 1
  value
}

let result = maybe_fallback(0)
`)

	out, err := CompileEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	checks := []string{
		"__ardTryValue",
		"return __ardTryValue.Expect",
	}
	for _, check := range checks {
		if !strings.Contains(generated, check) {
			t.Fatalf("expected generated source to contain %q\n%s", check, generated)
		}
	}
	if !strings.Contains(generated, "return 0") {
		t.Fatalf("expected nested try catch to early-return catch value from function\n%s", generated)
	}
}

func TestCompileEntrypointUnusedLocalGetsDiscard(t *testing.T) {
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

	out, err := CompileEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	if !strings.Contains(generated, "_ = sum") {
		t.Fatalf("expected generated source to contain discard for unused local\n%s", generated)
	}
}
