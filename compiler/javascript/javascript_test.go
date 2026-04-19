package javascript

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/backend"
)

func TestBuildWritesSimpleJavaScriptModule(t *testing.T) {
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

	outputPath := filepath.Join(dir, "main.mjs")
	builtPath, err := Build(mainPath, outputPath, backend.TargetJSBrowser)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("expected built path %q, got %q", outputPath, builtPath)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "function choose(num) {") {
		t.Fatalf("expected function definition in output, got:\n%s", source)
	}
	if !strings.Contains(source, "const result = choose(2);") {
		t.Fatalf("expected top-level let emission in output, got:\n%s", source)
	}
	if !strings.Contains(source, "export { choose, result };") {
		t.Fatalf("expected exports in output, got:\n%s", source)
	}
}

func TestBuildWritesImportedUserModules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utils.ard"), []byte(`
fn add(a: Int, b: Int) Int {
  a + b
}
`), 0o644); err != nil {
		t.Fatalf("failed to write utils module: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use demo/utils

let result = utils::add(1, 2)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "const __module_demo_utils = (() => {") {
		t.Fatalf("expected imported module wrapper, got:\n%s", source)
	}
	if !strings.Contains(source, "const result = __module_demo_utils.add(1, 2);") {
		t.Fatalf("expected imported module call, got:\n%s", source)
	}
}

func TestBuildWritesStructLiteralAndFieldAccess(t *testing.T) {
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

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSBrowser); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "class Person {") {
		t.Fatalf("expected struct class definition, got:\n%s", source)
	}
	if !strings.Contains(source, "const person = new Person(30);") {
		t.Fatalf("expected struct instantiation, got:\n%s", source)
	}
	if !strings.Contains(source, "return person.age;") {
		t.Fatalf("expected field access, got:\n%s", source)
	}
}

func TestBuildWritesImportedModuleStructLiteral(t *testing.T) {
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
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "class Person {") {
		t.Fatalf("expected imported struct class definition, got:\n%s", source)
	}
	if !strings.Contains(source, "return { Person: Person };") {
		t.Fatalf("expected imported struct export, got:\n%s", source)
	}
	if !strings.Contains(source, "const person = new __module_demo_models.Person(30);") {
		t.Fatalf("expected imported struct instantiation, got:\n%s", source)
	}
}

func TestBuildWritesListLiteralsAndMethods(t *testing.T) {
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

fn update() Int {
  mut values = [1, 2]
  values.push(3)
  values.prepend(0)
  values.set(1, 9)
  values.at(1)
}

let size = [1, 2, 3].size()
let first = reordered()
let result = update()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "const size = [1, 2, 3].length;") {
		t.Fatalf("expected list size lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "__value.push(3);") || !strings.Contains(source, "__value.unshift(0);") {
		t.Fatalf("expected push/prepend lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "__value[1] = 9;") {
		t.Fatalf("expected set lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "__value.sort((a, b) => function(a, b)") {
		t.Fatalf("expected sort comparator lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "return list[0];") || !strings.Contains(source, "return values[1];") {
		t.Fatalf("expected at() lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "const __tmp = __value[0];") {
		t.Fatalf("expected swap lowering, got:\n%s", source)
	}
}

func TestBuildWritesMaybeAndResultMethods(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

fn maybe_flow() Str {
  let value = maybe::some(21)
  let out = value.and_then<Str>(fn(v) { maybe::some("{v}") })
  out.or("")
}

fn result_flow() Str {
  let res: Int!Str = Result::ok(21)
  let out = res.map(fn(value) { value * 2 }).and_then<Str>(fn(value) { Result::ok("{value}") })
  out.or("")
}

let a = maybe::none<Int>().is_none()
let b = Result::err("boom").is_err()
let c = maybe_flow()
let d = result_flow()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "class Maybe {") || !strings.Contains(source, "class Result {") {
		t.Fatalf("expected Maybe/Result runtime classes, got:\n%s", source)
	}
	if !strings.Contains(source, "const a = Maybe.none().isNone();") {
		t.Fatalf("expected maybe none/is_none lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "const b = Result.err(\"boom\").isErr();") {
		t.Fatalf("expected result err/is_err lowering, got:\n%s", source)
	}
	if !strings.Contains(source, ".andThen(function(v)") {
		t.Fatalf("expected maybe and_then lowering, got:\n%s", source)
	}
	if !strings.Contains(source, ".map(function(value)") || !strings.Contains(source, ".andThen(function(value)") {
		t.Fatalf("expected result combinator lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "return out.or(\"\");") {
		t.Fatalf("expected final .or lowering, got:\n%s", source)
	}
}

func TestBuildWritesMatchLowering(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

enum Status {
  active,
  inactive,
}

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

fn classify(score: Int) Str {
  match {
    score >= 90 => "A",
    score >= 80 => "B",
    _ => "F",
  }
}

fn label(status: Status) Int {
  match status {
    Status::active => 1,
    Status::inactive => 2,
  }
}

fn maybe_pick(value: Int?) Int {
  match value {
    num => num,
    _ => 0,
  }
}

fn result_pick(value: Int!Str) Str {
  match value {
    ok(num) => num.to_str(),
    err(msg) => msg,
  }
}

let a = pick(true)
let b = bucket(2)
let c = classify(85)
let d = label(Status::active)
let e = maybe_pick(maybe::some(1))
let f = result_pick(Result::err("bad"))
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "const Status = { active: 0, inactive: 1 };") {
		t.Fatalf("expected enum object lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "let d = label(Status.active)") && !strings.Contains(source, "const d = label(Status.active)") {
		t.Fatalf("expected enum variant lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "if (__match === 0) return") || !strings.Contains(source, "if (__match === 1) return") {
		t.Fatalf("expected enum/int exact match lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "return __match ?") {
		t.Fatalf("expected bool match lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "if (__match >= 1 && __match <= 3) return") {
		t.Fatalf("expected int/range match lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "if ((score >= 90)) return") || !strings.Contains(source, "if ((score >= 80)) return") {
		t.Fatalf("expected conditional match lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "return __match.isSome() ?") || !strings.Contains(source, "const num = __match.value;") {
		t.Fatalf("expected maybe match lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "return __match.isOk() ?") || !strings.Contains(source, "const msg = __match.error;") {
		t.Fatalf("expected result match lowering, got:\n%s", source)
	}
}

func TestBuildWritesTryLowering(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

fn render_result(value: Int!Str) Str!Str {
  let out = try value
  Result::ok(out.to_str())
}

fn final_message() Str {
  try render_result(Result::err("division by zero")) -> err {
    "bad: " + err
  }
}

fn maybe_chain(value: Int?) Int? {
  let out = try value
  maybe::some(out + 1)
}

fn nested_binary(value: Int!Str) Int!Str {
  let out = (try value) + 1
  Result::ok(out)
}

let a = final_message()
let b = maybe_chain(maybe::some(4))
let c = nested_binary(Result::ok(2))
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "function makeTryReturn(value) {") {
		t.Fatalf("expected try sentinel helper, got:\n%s", source)
	}
	if !strings.Contains(source, "catch (__ard_try) {") || !strings.Contains(source, "if (__ard_try && __ard_try.__ard_try_return) return __ard_try.value;") {
		t.Fatalf("expected function try boundary, got:\n%s", source)
	}
	if !strings.Contains(source, "if (__try.isErr()) throw makeTryReturn(Result.err(__try.error));") {
		t.Fatalf("expected result try propagation, got:\n%s", source)
	}
	if !strings.Contains(source, "if (__try.isErr()) throw makeTryReturn((() => {") {
		t.Fatalf("expected result try catch lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "const err = __try.error;") {
		t.Fatalf("expected catch var binding, got:\n%s", source)
	}
	if !strings.Contains(source, "if (__try.isNone()) throw makeTryReturn(Maybe.none());") {
		t.Fatalf("expected maybe try propagation, got:\n%s", source)
	}
	if !strings.Contains(source, "return __try.ok;") || !strings.Contains(source, "return __try.value;") {
		t.Fatalf("expected try success unwrapping, got:\n%s", source)
	}
	if !strings.Contains(source, "const out = (() => {") {
		t.Fatalf("expected try expression lowering in let binding, got:\n%s", source)
	}
}

func TestBuildWritesStructMethods(t *testing.T) {
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
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "get() {") {
		t.Fatalf("expected getter method, got:\n%s", source)
	}
	if !strings.Contains(source, "return this.value;") {
		t.Fatalf("expected self access lowered to this, got:\n%s", source)
	}
	if !strings.Contains(source, "set(value) {") {
		t.Fatalf("expected setter method, got:\n%s", source)
	}
	if !strings.Contains(source, "this.value = value;") {
		t.Fatalf("expected mutating method body, got:\n%s", source)
	}
	if !strings.Contains(source, "box.set(2);") || !strings.Contains(source, "return box.get();") {
		t.Fatalf("expected instance method calls, got:\n%s", source)
	}
}

func TestRunRejectsBrowserTarget(t *testing.T) {
	err := Run("main.ard", backend.TargetJSBrowser, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "js-browser cannot be run directly") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunExecutesSimpleServerProgram(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn main() Int {
  1
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	if err := Run(mainPath, backend.TargetJSServer, []string{"ard", "run", mainPath, "--target", "js-server"}); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}
