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
		"ardtryunwrap0 := __ardTryValue.Expect",
		"value := ardtryunwrap0 + 1",
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

func TestCompileEntrypointNormalizesIfAndMatchWithoutExprClosures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn choose(flag: Bool) Int {
  match flag {
    true => 1,
    false => 2,
  }
}

fn weight(flag: Bool) Int {
  let value = match flag {
    true => 3,
    false => 4,
  }
  value
}

let result = choose(true) + weight(true)
`)

	out, err := CompileEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	checks := []string{
		"func Choose(flag bool) int",
		"if flag {",
		"return 1",
		"return 2",
		"func Weight(flag bool) int",
		"var αardnormalizeExpr0 int",
		"if flag {",
		"αardnormalizeExpr0 = 3",
		"αardnormalizeExpr0 = 4",
		"value := αardnormalizeExpr0",
	}
	for _, check := range checks {
		if !strings.Contains(generated, check) {
			t.Fatalf("expected generated source to contain %q\n%s", check, generated)
		}
	}
	if strings.Contains(generated, "func() int") {
		t.Fatalf("did not expect generated source to use expression closures for normalized if/match\n%s", generated)
	}
}

func TestCompileEntrypointNormalizesUnionMatchReturnWithoutExprClosures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Square { size: Int }
struct Circle { radius: Int }

type Shape = Square | Circle

fn name(shape: Shape) Str {
  match shape {
    Square => "Square",
    Circle => "Circle"
  }
}

let result = name(Square{size: 1})
`)

	out, err := CompileEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	checks := []string{
		"func Name(shape Shape) string",
		"var αardnormalizeExpr0 string",
		"switch unionValue := any(shape).(type) {",
		"αardnormalizeExpr0 = \"Square\"",
		"αardnormalizeExpr0 = \"Circle\"",
		"return αardnormalizeUnionmatch0",
	}
	for _, check := range checks {
		if !strings.Contains(generated, check) {
			t.Fatalf("expected generated source to contain %q\n%s", check, generated)
		}
	}
	if strings.Contains(generated, "func() string") {
		t.Fatalf("did not expect generated source to use expression closures for normalized union match return\n%s", generated)
	}
}

func TestCompileEntrypointHoistsNestedMatchInCallArgs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn id(n: Int) Int {
  n
}

fn choose(flag: Bool) Int {
  id(
    match flag {
      true => 1,
      false => 2,
    },
  )
}

let result = choose(true)
`)

	out, err := CompileEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	checks := []string{
		"func Choose(flag bool) int",
		"var αardnormalizeExpr0 int",
		"if flag {",
		"αardnormalizeExpr0 = 1",
		"αardnormalizeExpr0 = 2",
		"return Id(αardnormalizeExpr0)",
	}
	for _, check := range checks {
		if !strings.Contains(generated, check) {
			t.Fatalf("expected generated source to contain %q\n%s", check, generated)
		}
	}
	if strings.Contains(generated, "func() int") {
		t.Fatalf("did not expect generated source to use expression closures for nested match call arg\n%s", generated)
	}
}

func TestCompileEntrypointNormalizesEnumAndIntMatchAssignmentsWithoutExprClosures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
enum Region {
  North,
  South,
}

fn enum_weight(region: Region) Int {
  let value = match region {
    Region::North => 3,
    Region::South => 4,
  }
  value
}

fn int_label(n: Int) Str {
  let value = match n {
    0 => "zero",
    1..5 => "small",
    _ => "big",
  }
  value
}

let result = enum_weight(Region::North).to_str() + int_label(3)
`)

	out, err := CompileEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	checks := []string{
		"func EnumWeight(region Region) int",
		"var αardnormalizeExpr0 int",
		"if region.Tag == 0 {",
		"αardnormalizeExpr0 = 3",
		"αardnormalizeExpr0 = 4",
		"panic(\"non-exhaustive enum match\")",
		"func IntLabel(n int) string",
		"var αardnormalizeExpr0 string",
		"if n == 0 {",
		"αardnormalizeExpr0 = \"zero\"",
		"αardnormalizeExpr0 = \"small\"",
		"αardnormalizeExpr0 = \"big\"",
	}
	for _, check := range checks {
		if !strings.Contains(generated, check) {
			t.Fatalf("expected generated source to contain %q\n%s", check, generated)
		}
	}
	if strings.Contains(generated, "func() int") || strings.Contains(generated, "func() string") {
		t.Fatalf("did not expect generated source to use expression closures for enum/int match assignments\n%s", generated)
	}
}

func TestCompileEntrypointNormalizesOptionAndResultMatchAssignmentsWithoutExprClosures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/maybe

fn maybe_label(flag: Bool) Str {
  let value: Int? = match flag {
    true => maybe::some(1),
    false => maybe::none(),
  }
  match value {
    n => "some {n}",
    _ => "none",
  }
}

fn result_label(flag: Bool) Str {
  let value: Int!Str = match flag {
    true => Result::ok(1),
    false => Result::err("bad"),
  }
  match value {
    ok(n) => "ok {n}",
    err(msg) => msg,
  }
}

let result = maybe_label(true) + result_label(false)
`)

	out, err := CompileEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	checks := []string{
		"func MaybeLabel(flag bool) string",
		"func ResultLabel(flag bool) string",
		"var αardnormalizeExpr0 ardgo.Maybe[int]",
		"var αardnormalizeExpr0 ardgo.Result[int, string]",
	}
	for _, check := range checks {
		if !strings.Contains(generated, check) {
			t.Fatalf("expected generated source to contain %q\n%s", check, generated)
		}
	}
	if strings.Contains(generated, "func() string") {
		t.Fatalf("did not expect generated source to use expression closures for option/result match assignments\n%s", generated)
	}
}

func TestCompileEntrypointHoistsNestedOptionAndResultMatchesInCallArgs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/maybe

fn label(value: Str) Str {
  value
}

fn maybe_label(flag: Bool) Str {
  let value: Int? = match flag {
    true => maybe::some(1),
    false => maybe::none(),
  }
  label(
    match value {
      n => "some {n}",
      _ => "none",
    },
  )
}

fn result_label(flag: Bool) Str {
  let value: Int!Str = match flag {
    true => Result::ok(1),
    false => Result::err("bad"),
  }
  label(
    match value {
      ok(n) => "ok {n}",
      err(msg) => msg,
    },
  )
}

let result = maybe_label(true) + result_label(false)
`)

	out, err := CompileEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	checks := []string{
		"func MaybeLabel(flag bool) string",
		"func ResultLabel(flag bool) string",
		"return Label(αardnormalizeExpr",
	}
	for _, check := range checks {
		if !strings.Contains(generated, check) {
			t.Fatalf("expected generated source to contain %q\n%s", check, generated)
		}
	}
	if strings.Contains(generated, "func() string") {
		t.Fatalf("did not expect generated source to use expression closures for nested option/result matches\n%s", generated)
	}
}

func TestCompileEntrypointNormalizesConditionalMatchAssignmentsWithoutExprClosures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn classify(n: Int) Str {
  let value = match {
    n == 0 => "zero",
    n > 0 => "positive",
    _ => "negative",
  }
  value
}

let result = classify(1)
`)

	out, err := CompileEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	checks := []string{
		"func Classify(n int) string",
		"var αardnormalizeExpr0 string",
		"if n == 0 {",
		"αardnormalizeExpr0 = \"zero\"",
		"αardnormalizeExpr0 = \"positive\"",
		"αardnormalizeExpr0 = \"negative\"",
	}
	for _, check := range checks {
		if !strings.Contains(generated, check) {
			t.Fatalf("expected generated source to contain %q\n%s", check, generated)
		}
	}
	if strings.Contains(generated, "func() string") {
		t.Fatalf("did not expect generated source to use expression closures for conditional match assignments\n%s", generated)
	}
}
