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

func TestBuildBinaryCompilesListLiteralAndMethods(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn get_second() Int {
  let values = [1, 2, 3]
  values.at(1)
}

let size = [1, 2, 3].size()
let second = get_second()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesMutableListMethods(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn update() Int {
  mut values = [1, 2]
  values.push(3)
  values.prepend(0)
  values.set(1, 9)
  values.at(1)
}

let result = update()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesBasicMapMethods(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn update() Bool {
  mut values: [Str: Int] = ["a": 1]
  values.set("b", 2)
  values.drop("a")
  values.has("b")
}

let size = ["a": 1].size()
let has_b = update()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesMaybeValues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

fn from_some() Int {
  let maybe_num = maybe::some(10)
  maybe_num.or(0)
}

fn from_none() Int {
  let maybe_num: Int? = maybe::none()
  maybe_num.or(7)
}

let a = from_some()
let b = from_none()
let has_none = maybe::none<Int>().is_none()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesMapGetAndKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn lookup() Int {
  let values: [Str: Int] = ["a": 1]
  values.get("a").or(0)
}

let keys = ["a": 1].keys()
let found = lookup()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesPrimitiveMethods(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
let a = "hello".contains("ell")
let b = "hello".starts_with("he")
let c = " hello ".trim()
let d = "a,b".split(",").size()
let e = 42.to_str()
let f = 98.6.to_int()
let g = true.to_str()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesRangeAndForInLoops(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn sum_range() Int {
  mut total = 0
  for i in 1..3 {
    total = total + i
  }
  total
}

fn sum_list() Int {
  let values = [1, 2, 3]
  mut total = 0
  for value, idx in values {
    total = total + value + idx
  }
  total
}

fn sum_chars() Int {
  mut total = 0
  for char, idx in "ab" {
    total = total + idx
    let char_copy = char
  }
  total
}

fn sum_map() Int {
  let values: [Str: Int] = ["a": 1, "b": 2]
  mut total = 0
  for key, value in values {
    let key_copy = key
    total = total + value
  }
  total
}

let a = sum_range()
let b = sum_list()
let c = sum_chars()
let d = sum_map()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesEnums(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "colors.ard"), []byte(`
enum Color {
  red,
  green,
}
`), 0o644); err != nil {
		t.Fatalf("failed to write colors source: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use demo/colors

enum Status {
  active,
  inactive,
}

let a = Status::active == Status::inactive
let b = colors::Color::green == colors::Color::red
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesExternStubs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utils.ard"), []byte(`
extern fn meaning() Int = "Meaning"
`), 0o644); err != nil {
		t.Fatalf("failed to write utils source: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use demo/utils

extern fn add(a: Int, b: Int) Int = "Add"

let a = add(1, 2)
let b = utils::meaning()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesStructMethods(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
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

fn run() Int {
  mut box = Box{value: 1}
  box.set(2)
  box.get()
}

let result = run()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesBoolAndIntMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn pick(flag: Bool) Int {
  match flag {
    true => 1,
    false => 2,
  }
}

fn bucket(num: Int) Str {
  match num {
    0 => "zero",
    1..3 => "few",
    _ => "many",
  }
}

let a = pick(true)
let b = bucket(2)
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesEnumMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
enum Status {
  active,
  inactive,
}

fn label(status: Status) Int {
  match status {
    Status::active => 1,
    Status::inactive => 2,
  }
}

let result = label(Status::active)
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesMaybeMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

fn pick(value: Int?) Int {
  match value {
    num => num,
    _ => 0,
  }
}

let a = pick(maybe::some(1))
let b = pick(maybe::none())
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
