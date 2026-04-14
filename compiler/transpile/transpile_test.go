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

func TestBuildBinaryCompilesListSortAndSwap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn reordered() Int {
  mut list = [3, 7, 8, 5, 2, 9, 5, 4]
  list.sort(fn(a: Int, b: Int) Bool { a < b })
  list.swap(0, 7)
  list.at(0)
}

let value = reordered()
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

func TestBuildBinaryCompilesStdlibFsModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/fs

let exists = fs::exists("./demo.txt")
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesStdlibIoModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/io

struct Person {
  name: Str,
}

impl Str::ToString for Person {
  fn to_str() Str {
    self.name
  }
}

io::print("hello")
io::print(42)
io::print(Person{name: "world"})
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesStdlibEncodeModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/encode

let a = encode::json("hello")
let b = encode::json(42)
let c = encode::json(true)
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesStdlibDecodeModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/decode

let int_decoder = decode::int
let string_decoder = decode::string
let flatten_errors = decode::flatten
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesGenericStdlibDecodeCombinators(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/decode

let int_decoder = decode::int
let maybe_int = decode::nullable(int_decoder)
let age = decode::field("age", int_decoder)
let first = decode::one_of(int_decoder, [age])
let _ = maybe_int
let _ = first
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesStdlibDynamicModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/dynamic

let a = dynamic::from("hello")
let b = dynamic::from(42)
let c = dynamic::from(true)
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesStdlibHttpModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/http

let ok = http::Response::new(200, "ok").is_ok()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesVariableShadowing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn run() Str!Str {
  let body = "request"
  let result: Str!Str = Result::ok("response")
  let body = try result
  Result::ok(body)
}

let out = run()
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

func TestBuildBinaryCompilesMaybeCombinators(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

fn mapped() Int {
  let value = maybe::some(21)
  let out = value.map(fn(v) { v * 2 })
  out.or(0)
}

fn mapped_none() Bool {
  let value: Int? = maybe::none()
  value.map(fn(v) { v + 1 }).is_none()
}

fn chained() Str {
  let value = maybe::some(21)
  let out = value.and_then<Str>(fn(v) { maybe::some("{v}") })
  out.or("")
}

fn chained_none() Bool {
  let value: Int? = maybe::none()
  value.and_then<Str>(fn(v) { maybe::some("{v}") }).is_none()
}

let a = mapped()
let b = mapped_none()
let c = chained()
let d = chained_none()
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

func TestBuildBinaryCompilesTemplateStringsAndPanic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn greet(name: Str) Str {
  "Hello, {name}!"
}

fn fail() Int {
  panic("boom")
}

let msg = greet("Ard")
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesBasicResults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn divide(a: Int, b: Int) Int!Str {
  match b == 0 {
    true => Result::err("division by zero"),
    false => Result::ok(a / b),
  }
}

fn fallback() Int {
  let res: Int!Str = Result::err("bad")
  res.or(7)
}

fn check() Bool {
  let res: Int!Str = Result::ok(10)
  res.is_ok() and not res.is_err()
}

fn forced() Int {
  let res: Int!Str = Result::ok(9)
  res.expect("boom")
}

let a = divide(4, 2)
let b = fallback()
let c = check()
let d = forced()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesResultCombinators(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn mapped() Int {
  let res: Int!Str = Result::ok(21)
  let out = res.map(fn(value) { value * 2 })
  out.or(0)
}

fn mapped_err() Int {
  let res: Int!Str = Result::err("bad")
  let out = res.map_err(fn(err) { err.size() })
  match out {
    err(size) => size,
    ok(value) => value,
  }
}

fn chained() Str {
  let res: Int!Str = Result::ok(21)
  let out = res.and_then<Str>(fn(value) { Result::ok("{value}") })
  out.or("")
}

fn chained_err() Bool {
  let res: Int!Str = Result::err("boom")
  let out = res.and_then<Str>(fn(value) { Result::ok("{value}") })
  out.is_err()
}

let a = mapped()
let b = mapped_err()
let c = chained()
let d = chained_err()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesResultMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn divide(a: Int, b: Int) Int!Str {
  match b == 0 {
    true => Result::err("division by zero"),
    false => Result::ok(a / b),
  }
}

fn describe(res: Int!Str) Str {
  match res {
    ok(value) => "ok: {value.to_str()}",
    err(msg) => "err: {msg}",
  }
}

fn classify(res: Int!Str) Str {
  match res {
    ok => ok.to_str(),
    err => err,
  }
}

fn from_call() Int {
  match divide(1, 0) {
    ok(value) => value,
    err(msg) => -1,
  }
}

let a = describe(Result::ok(4))
let b = describe(Result::err("bad"))
let c = classify(Result::ok(9))
let d = classify(Result::err("oops"))
let e = from_call()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestEmitEntrypointNestedTryCatchHoistsSuccessValue(t *testing.T) {
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

	out, err := EmitEntrypoint(module)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	generated := string(out)
	checks := []string{
		"__ardTryValue",
		"value := (__ardTryValue",
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

func TestBuildBinaryCompilesTryExpressions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

fn divide(a: Int, b: Int) Int!Str {
  match b == 0 {
    true => Result::err("division by zero"),
    false => Result::ok(a / b),
  }
}

fn half(n: Int) Int? {
  match n > 0 {
    true => maybe::some(n / 2),
    false => maybe::none(),
  }
}

fn render_division(a: Int, b: Int) Str!Str {
  let value = try divide(a, b)
  Result::ok(value.to_str())
}

fn final_message() Str {
  try render_division(1, 0) -> err {
    "bad: {err}"
  }
}

fn unwrap_local() Int!Str {
  let res: Int!Str = divide(6, 3)
  let value = try res
  Result::ok(value)
}

fn maybe_chain(n: Int) Int? {
  let value = try half(n)
  maybe::some(value + 1)
}

fn maybe_fallback(n: Int) Int {
  let value = try half(n) -> _ {
    0
  }
  value + 1
}

fn ignore_success() Int!Str {
  try divide(4, 2)
  Result::ok(1)
}

fn sum_all(values: [Int]) Int!Str {
  mut total = 0
  for value in values {
    let num = try divide(value, 1)
    total = total + num
  }
  Result::ok(total)
}

fn loop_catch(values: [Int]) Int!Str {
  for value in values {
    let num = try divide(value, value - value) -> err {
      Result::err("loop failed: {err}")
    }
  }
  Result::ok(0)
}

fn sum2(a: Int, b: Int) Int {
  a + b
}

fn nested_binary(a: Int, b: Int) Int!Str {
  let value = (try divide(a, b)) + 1
  Result::ok(value)
}

fn nested_call_arg(a: Int, b: Int) Int!Str {
  Result::ok(sum2(1, try divide(a, b)))
}

fn nested_list(a: Int, b: Int) [Int]!Str {
  let values: [Int] = [(try divide(a, b)), 2]
  Result::ok(values)
}

let a = final_message()
let b = unwrap_local()
let c = maybe_chain(4)
let d = maybe_fallback(0)
let e = ignore_success()
let f = sum_all([1, 2, 3])
let g = loop_catch([1, 2])
let h = nested_binary(4, 2)
let i = nested_call_arg(8, 2)
let j = nested_list(6, 3)
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesTryInMatchExpressions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn divide(a: Int, b: Int) Int!Str {
  match b == 0 {
    true => Result::err("division by zero"),
    false => Result::ok(a / b),
  }
}

fn sum_from_match(flag: Bool, value: Int) Int!Str {
  let next = match flag {
    true => (try divide(value, 1)) + 1,
    false => 0,
  }
  Result::ok(next)
}

fn sum_from_result_match(value: Int!Str) Int!Str {
  let next = match value {
    ok(num) => (try divide(num, 1)) + 2,
    err(msg) => 0,
  }
  Result::ok(next)
}

let a = sum_from_match(true, 3)
let b = sum_from_result_match(Result::ok(4))
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	outputPath := filepath.Join(dir, "demo-bin")
	if _, err := BuildBinary(mainPath, outputPath); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}

func TestBuildBinaryCompilesAnonymousFunctions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utils.ard"), []byte(`
let add_one = fn(x: Int) Int { x + 1 }
`), 0o644); err != nil {
		t.Fatalf("failed to write utils source: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use demo/utils

fn apply_twice(value: Int, callback: fn(Int) Int) Int {
  let first = callback(value)
  callback(first)
}

fn has_text(callback: fn(Str) Bool) Bool {
  callback("hello")
}

let base = 2
let multiply = fn(a: Int, b: Int) Int {
  a * b * base
}
let mapped = apply_twice(3, fn(x) { x + 1 })
let checked = has_text(fn(x) { x.size() > 0 })
let imported = utils::add_one(5)
let local = multiply(3, 4)
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
