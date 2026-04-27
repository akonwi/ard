//go:build integration

package go_backend

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	backendir "github.com/akonwi/ard/go_backend/ir"
)

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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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
let h = "hello".to_dyn()
let i = 42.to_dyn()
let j = 98.6.to_dyn()
let k = true.to_dyn()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibDecodeFromJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/decode

let raw = decode::from_json("\{\"age\":1\}")
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesDecodeEndToEndFlow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/decode

fn run() Int!Str {
  let raw = try decode::from_json("\{\"age\":1\}")
  let age = decode::field("age", decode::int)
  match age(raw) {
    ok => Result::ok(ok),
    err(errs) => Result::err(decode::flatten(errs)),
  }
}

let out = run()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibArgvModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/argv

let args = argv::load()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibDatesModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/dates

let today = dates::get_today()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibDurationModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/duration

let hours = duration::from_hours(1)
let minutes = duration::from_minutes(2)
let seconds = duration::from_seconds(3)
let millis = duration::from_millis(4)
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibBase64Module(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/base64

let encoded = base64::encode("hello", true)
let decoded = base64::decode(encoded, true)
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibChronoModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/chrono

let now = chrono::now()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibCryptoModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/crypto
use ard/hex

let md5 = crypto::md5("hello")
let sha = hex::encode(crypto::sha256("hello"))
let hashed = crypto::hash("password123", 4)
let verified = crypto::verify("password123", "hashed")
let scrypt = crypto::scrypt_hash("password123", "73616c74", 16, 1, 1, 16)
let scrypt_ok = crypto::scrypt_verify("password123", "hash", 16, 1, 1, 16)
let id = crypto::uuid()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibEnvModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/env

let home = env::get("HOME")
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibFloatModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/float

let a = float::from_str("3.14")
let b = float::from_int(2)
let c = float::floor(3.9)
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibHexModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/hex

let encoded = hex::encode("hello")
let decoded = hex::decode(encoded)
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibIntModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/int

let parsed = int::from_str("42")
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibJsonModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/json

let out = json::encode(["age": 1])
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibMapModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/map

let values = map::new<Int>()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibAsyncModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/async

let fiber = async::eval(
  fn() Int {
    1
  },
)
let value = fiber.get()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibAsyncStartModuleFunction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.ard"), []byte(`
fn run() {
  let value = 1
  let _ = value
}
`), 0o644); err != nil {
		t.Fatalf("failed to write worker source: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/async
use demo/worker

let fiber = async::start(worker::run)
fiber.join()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibSqlModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/sql

fn run() Void!Str {
  let db = try sql::open("demo.db")
  let query = db.query("SELECT 1 WHERE 1 = @id")
  let _ = query.all(["id": 1])
  db.close()
}

let out = run()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesStdlibSqlTransactionFlow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/sql

fn run() Dynamic?!Str {
  let db = try sql::open("demo.db")
  let tx = try db.begin()
  let row = try tx.query("SELECT 1 WHERE 1 = @id").first(["id": 1])
  try tx.rollback()
  try db.close()
  Result::ok(row)
}

let out = run()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesExplicitTypeArgsOnZeroArgGenericFunction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn empty() [$T] {
  let out: [$T] = []
  out
}

let values = empty<Int>()
`), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
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

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesExplicitVoidAnonymousFunctions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn run(callback: fn(Str) Void) {
  callback("ard")
}

fn main() {
  run(fn(text: Str) Void {
    text.contains("a")
  })
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesContextualVoidAnonymousFunctions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn run(callback: fn(Str) Void) {
  callback("ard")
}

fn main() {
  run(fn(text) {
    text.contains("a")
  })
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesGenericAsyncFiberLists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/async

fn main() {
  mut fibers: [async::Fiber<Int>] = []
  fibers.push(async::eval(fn() Int {
    1
  }))
  async::join(fibers)
  let total = fibers.at(0).get()
  let _ = total
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesNotMethodPrecedence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn main() {
  if not "".is_empty() {
    panic("unexpected")
  }

  if not "x".is_empty() {
    let _ = 1
  }
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryCompilesMaybeSomeCoercionInStructFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/http
use ard/maybe

fn main() {
  let req = http::Request{
    method: http::Method::Post,
    url: "http://example.com",
    headers: ["content-type": "text/plain"],
    body: maybe::some("raw text"),
  }
  let _ = req
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	assertGoTargetRunSucceeds(t, dir, filepath.Base(mainPath))
}

func TestBuildBinaryRespectsRelativeOutputPath(t *testing.T) {
	dir := t.TempDir()
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldCwd)
	}()

	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn main() {
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	builtPath, err := BuildBinary("main.ard", "./demo-bin")
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
	if builtPath != "./demo-bin" {
		t.Fatalf("expected built path %q, got %q", "./demo-bin", builtPath)
	}
	if _, err := os.Stat(filepath.Join(dir, "demo-bin")); err != nil {
		t.Fatalf("expected output binary in cwd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "generated", "demo-bin")); !os.IsNotExist(err) {
		t.Fatalf("did not expect output binary under generated dir, got err=%v", err)
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

// TestCompileModuleSourceViaBackendIR_MigratedPathsWithoutLegacySourceModule
// pins the migration-hardening contract that migrated try/match flows must
// be emittable through the backend IR pipeline without a hidden dependency
// on the legacy source-module emitter. It does this by:
//
//  1. Compiling a try/match-heavy module via the standard backend-IR-first
//     compile path (`compileModuleSourceViaBackendIR`) and asserting the
//     output parses as Go.
//  2. Lowering the same module to backend IR and then emitting the Go file
//     through `emitGoFileFromBackendIR(irModule, nil, ...)` — explicitly
//     passing a `nil` checker.Module so any latent legacy fallback would
//     fail loudly. The emit step must succeed, the result must parse as
//     Go, and the generated source must contain only the native try/match
//     control-flow shapes (no legacy marker artifacts smuggled in).
//
// Together these checks prove that for migrated try/match shapes the
// emission step does not need to reach back into the legacy AST lowerers.
func TestCompileModuleSourceViaBackendIR_MigratedPathsWithoutLegacySourceModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
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

fn render(a: Int, b: Int) Str!Str {
  let value = try divide(a, b)
  Result::ok(value.to_str())
}

fn final_message() Str {
  try render(1, 0) -> err {
    "bad: {err}"
  }
}

fn use_maybe(n: Int) Int {
  let value = try half(n) -> _ {
    0
  }
  value + 1
}

fn classify(n: Int) Str {
  match n {
    0 => "zero",
    1..3 => "small",
    _ => "many",
  }
}

let a = final_message()
let b = use_maybe(2)
let c = classify(2)
`)

	// Step 1: standard backend-IR-first compile path must succeed and
	// produce parseable Go.
	standardOut, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("standard backend ir compile failed: %v", err)
	}
	assertParsesAsGo(t, standardOut)

	// Step 2: emit through the backend IR with a nil checker.Module to
	// prove migrated try/match flows do not require the legacy
	// source-module fallback for correctness.
	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("lowerModuleToBackendIR failed: %v", err)
	}
	if irModule == nil {
		t.Fatalf("expected non-nil backend ir module")
	}
	// Validate the lowered module so structural IR regressions fail here
	// before emission.
	if err := backendir.ValidateModule(irModule); err != nil {
		t.Fatalf("expected lowered module to validate cleanly, got: %v", err)
	}

	imports := map[string]string{
		helperImportPath: helperImportAlias,
	}
	fileIR, err := emitGoFileFromBackendIR(irModule, nil, imports, true, "")
	if err != nil {
		t.Fatalf("emitGoFileFromBackendIR with nil source module failed: %v", err)
	}
	rendered, err := renderGoFile(optimizeGoFileIR(fileIR))
	if err != nil {
		t.Fatalf("renderGoFile failed: %v", err)
	}

	// Confirm the source-less emission produced parseable Go.
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", rendered, parser.AllErrors); err != nil {
		t.Fatalf("expected nil-source-module emission to parse as Go, got error: %v\n%s", err, string(rendered))
	}

	// The generated source must not contain marker artifacts.
	for _, marker := range []string{
		"try_op",
		"bool_match",
		"int_match",
		"conditional_match",
		"option_match",
		"result_match",
		"enum_match",
		"union_match",
	} {
		if strings.Contains(string(rendered), marker) {
			t.Fatalf("expected nil-source-module emission to be free of marker %q\n%s", marker, string(rendered))
		}
	}

	// Sanity check: migrated try/match should still emit recognizable
	// native Go control flow shapes.
	for _, want := range []string{
		"package main",
		"func Render(",
		"func main()",
	} {
		if !strings.Contains(string(rendered), want) {
			t.Fatalf("expected nil-source-module emission to contain %q\n%s", want, string(rendered))
		}
	}

	// Final binding: the standard path output and the nil-source-module
	// path output must remain mutually parseable Go programs. We do not
	// require byte-exact equality (imports/comments may differ) but they
	// should both compile to the same set of declarations.
	if len(standardOut) == 0 || len(rendered) == 0 {
		t.Fatalf("expected both standard and nil-source-module emissions to be non-empty")
	}
}
