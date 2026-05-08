package javascript

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/frontend"
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
	if !strings.Contains(source, `import * as demo_utils from "./demo/utils.mjs";`) {
		t.Fatalf("expected imported module file import, got:\n%s", source)
	}
	if !strings.Contains(source, "const result = demo_utils.add(1, 2);") {
		t.Fatalf("expected imported module call, got:\n%s", source)
	}
	importedPath := filepath.Join(dir, "demo", "utils.mjs")
	importedOut, err := os.ReadFile(importedPath)
	if err != nil {
		t.Fatalf("expected emitted imported module file: %v", err)
	}
	if !strings.Contains(string(importedOut), "function add(a, b) {") || !strings.Contains(string(importedOut), "export { add };") {
		t.Fatalf("expected emitted imported module contents, got:\n%s", string(importedOut))
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
	if !strings.Contains(source, `import * as demo_models from "./demo/models.mjs";`) {
		t.Fatalf("expected imported struct module import, got:\n%s", source)
	}
	if !strings.Contains(source, "const person = new demo_models.Person(30);") {
		t.Fatalf("expected imported struct instantiation, got:\n%s", source)
	}
	importedPath := filepath.Join(dir, "demo", "models.mjs")
	importedOut, err := os.ReadFile(importedPath)
	if err != nil {
		t.Fatalf("expected emitted imported struct module file: %v", err)
	}
	importedSource := string(importedOut)
	if !strings.Contains(importedSource, "class Person {") || !strings.Contains(importedSource, "export { Person };") {
		t.Fatalf("expected imported struct module contents, got:\n%s", importedSource)
	}
}

func TestBuildWritesLoopStatements(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn countdown() Int {
  mut n = 3
  while n > 0 {
    n = n - 1
  }
  n
}

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
    let char_copy = char
    total = total + idx
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

let a = countdown()
let b = sum_range()
let c = sum_list()
let d = sum_chars()
let e = sum_map()
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
	if !strings.Contains(source, "while ((n > 0)) {") {
		t.Fatalf("expected while lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "const __range_start = 1;") || !strings.Contains(source, "for (let i = __range_start; i <= __range_end; i++) {") {
		t.Fatalf("expected int-range lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "for (const [idx, value] of __list_value.entries()) {") {
		t.Fatalf("expected list iteration lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "const __string_value = Array.from(\"ab\");") || !strings.Contains(source, "for (const [idx, char] of __string_value.entries()) {") {
		t.Fatalf("expected string iteration lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "for (const [key, value] of __map_value.entries()) {") {
		t.Fatalf("expected map iteration lowering, got:\n%s", source)
	}
}

func TestRunExecutesCoreLoopProgram(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn countdown() Int {
  mut n = 3
  while n > 0 {
    n = n - 1
  }
  n
}

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
    let char_copy = char
    total = total + idx
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

let a = countdown()
let b = sum_range()
let c = sum_list()
let d = sum_chars()
let e = sum_map()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect build error: %v", err)
	}

	cmd := exec.Command("node", "--input-type=module", "-e", `
import { pathToFileURL } from "node:url";
const mod = await import(pathToFileURL(process.argv[1]).href);
if (mod.a !== 0) throw new Error("countdown");
if (mod.b !== 6) throw new Error("range");
if (mod.c !== 9) throw new Error("list");
if (mod.d !== 1) throw new Error("chars");
if (mod.e !== 3) throw new Error("map");
`, outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("did not expect node assertion error: %v", err)
	}
}

func TestRunExecutesLoopBreakInsideMatch(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn find() Int {
  mut total = 0
  while true {
    match (true) {
      true => {
        total = total + 1
        break
      },
      false => {}
    }
  }
  total
}

let result = find()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect build error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "throw makeBreakSignal();") {
		t.Fatalf("expected break signal lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "if (__ard_break && __ard_break.__ard_break) break;") {
		t.Fatalf("expected loop break catch lowering, got:\n%s", source)
	}

	cmd := exec.Command("node", "--input-type=module", "-e", `
import { pathToFileURL } from "node:url";
const mod = await import(pathToFileURL(process.argv[1]).href);
if (mod.result !== 1) throw new Error("break-in-match");
`, outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("did not expect node assertion error: %v", err)
	}
}

func TestBuildLowersStaticStructFunctionsWithoutRedeclaration(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
struct Book {
  title: Str
}

impl Book {
  fn get_title() Str {
    self.title
  }
}

fn Book::new(title: Str) Book {
  Book { title: title }
}

let title = Book::new("Ard").get_title()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect build error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if strings.Count(source, "class Book {") != 1 {
		t.Fatalf("expected one Book class, got:\n%s", source)
	}
	if !strings.Contains(source, "function Book__new(title) {") {
		t.Fatalf("expected mangled static function name, got:\n%s", source)
	}
	if !strings.Contains(source, "const title = Book__new(\"Ard\").get_title();") {
		t.Fatalf("expected mangled static function call, got:\n%s", source)
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
	if !strings.Contains(source, "__value.sort(") || !strings.Contains(source, "(a, b) => function(a, b)") {
		t.Fatalf("expected sort comparator lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "return list[0];") || !strings.Contains(source, "return values[1];") {
		t.Fatalf("expected at() lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "const __tmp = __value[0];") {
		t.Fatalf("expected swap lowering, got:\n%s", source)
	}
}

func TestBuildWritesEnumAwareUnionMatchAndComparisonLowering(t *testing.T) {
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
 
 type Value = Status | Int | Bool
 
 fn label(value: Value) Str {
   match value {
     Status(status) => "enum",
     Int(int) => int.to_str(),
     _ => "boolean value",
   }
 }
 
 fn compare(status: Status) Bool {
   status == 0 and status < 1
 }
 
 let a = label(Status::active)
 let b = label(20)
 let c = label(true)
 let d = compare(Status::inactive)
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
	if !strings.Contains(source, `import { Maybe, Result, ardEnumValue, ardEq, ardToString, isArdEnum, isArdMaybe, isEnumOf, makeArdError, makeBreakSignal, makeEnum } from "./ard.prelude.mjs";`) {
		t.Fatalf("expected prelude import, got:\n%s", source)
	}
	if !strings.Contains(source, `const Status = Object.freeze(`) || !strings.Contains(source, `active: makeEnum("Status", "active", 0)`) || !strings.Contains(source, `inactive: makeEnum("Status", "inactive", 1)`) {
		t.Fatalf("expected branded enum object lowering, got:\n%s", source)
	}
	if !strings.Contains(source, `if (isEnumOf(__match, "Status")) return`) {
		t.Fatalf("expected enum-specific union predicate, got:\n%s", source)
	}
	if !strings.Contains(source, `typeof __match === "number"`) {
		t.Fatalf("expected int union predicate, got:\n%s", source)
	}
	if !strings.Contains(source, `ardEq(status, 0)`) {
		t.Fatalf("expected enum-aware equality lowering, got:\n%s", source)
	}
	if !strings.Contains(source, `(ardEnumValue(status) < ardEnumValue(1))`) {
		t.Fatalf("expected enum-aware ordering lowering, got:\n%s", source)
	}
}

func TestBuildWritesUnionMatchLowering(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
 type Printable = Str | Int | Bool
 
 fn print(p: Printable) Str {
   match p {
     Str(str) => str,
     Int(int) => int.to_str(),
     _ => "boolean value",
   }
 }
 
 let a = print(20)
 let b = print("hi")
 let c = print(true)
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
	if !strings.Contains(source, "typeof __match === \"string\"") {
		t.Fatalf("expected string union predicate, got:\n%s", source)
	}
	if !strings.Contains(source, "typeof __match === \"number\"") {
		t.Fatalf("expected number union predicate, got:\n%s", source)
	}
	if !strings.Contains(source, "const str = __match;") || !strings.Contains(source, "const int = __match;") {
		t.Fatalf("expected union case bindings, got:\n%s", source)
	}
	if !strings.Contains(source, "return (() => {") || !strings.Contains(source, "return \"boolean value\";") {
		t.Fatalf("expected catch-all union lowering, got:\n%s", source)
	}
}

func TestBuildWritesMapLiteralsAndMethods(t *testing.T) {
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

fn lookup() Int {
  let values: [Str: Int] = ["a": 1]
  values.get("a").or(0)
}

let size = ["a": 1].size()
let keys = ["a": 1].keys()
let has_b = update()
let found = lookup()
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
	if !strings.Contains(source, "new Map([[\"a\", 1]])") {
		t.Fatalf("expected map literal lowering, got:\n%s", source)
	}
	if !strings.Contains(source, ".size") || !strings.Contains(source, "Array.from(new Map([[\"a\", 1]]).keys())") {
		t.Fatalf("expected size/keys lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "__value.set(\"b\", 2);") || !strings.Contains(source, "__value.delete(\"a\");") {
		t.Fatalf("expected set/drop lowering, got:\n%s", source)
	}
	if !strings.Contains(source, ".has(\"b\")") {
		t.Fatalf("expected has lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "Maybe.some(values.get(\"a\"))") || !strings.Contains(source, "values.has(\"a\")") {
		t.Fatalf("expected get lowering, got:\n%s", source)
	}
}

func TestBuildWritesJSStdlibExternImports(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/io

fn main() {
  io::print("hello")
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	modulePath := filepath.Join(dir, "ard", "io.mjs")
	out, err := os.ReadFile(modulePath)
	if err != nil {
		t.Fatalf("failed to read generated io module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, `import * as stdlib from "../ffi.stdlib.js-server.mjs";`) {
		t.Fatalf("expected stdlib ffi import in io module, got:\n%s", source)
	}
	if !strings.Contains(source, `return stdlib.printLine(string);`) {
		t.Fatalf("expected stdlib extern wrapper call, got:\n%s", source)
	}
	if _, err := os.Stat(filepath.Join(dir, "ffi.stdlib.js-server.mjs")); err != nil {
		t.Fatalf("expected copied stdlib ffi companion: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ard.prelude.mjs")); err != nil {
		t.Fatalf("expected copied js prelude companion: %v", err)
	}
}

func TestRunExecutesJSStdlibIOProgram(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/io

fn main() {
  io::print("hello")
  let line = io::read_line().or("missing")
  io::print(line)
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	cmd := exec.Command("go", "run", "-tags=goexperiment.jsonv2", ".", "run", "--target", "js-server", mainPath)
	cmd.Dir = ".."
	cmd.Stdin = strings.NewReader("world\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("did not expect js-server io run error: %v\n%s", err, string(out))
	}
	if string(out) != "hello\nworld\n" {
		t.Fatalf("unexpected output:\n%s", string(out))
	}
}

func TestRunExecutesDecodeAndJSONStdlibProgram(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/io
use ard/decode
use ard/json
use ard/maybe

struct Person {
  name: Str,
  employed: Bool?,
}

fn main() {
  let raw = decode::from_json("\{\"name\":\"kit\",\"nums\":[1,2,3],\"counts\":\{\"a\":1\}\}").expect("parse")
  io::print(decode::run(raw, decode::field("name", decode::string)).expect("name"))
  io::print(decode::run(raw, decode::field("nums", decode::list(decode::int))).is_ok().to_str())
  io::print(decode::run(raw, decode::field("counts", decode::map(decode::string, decode::int))).is_ok().to_str())
  match decode::run(raw, decode::field("name", decode::int)) {
    ok => io::print("unexpected"),
    err(errs) => io::print(decode::flatten(errs)),
  }

  let person = Person{ name: "kit", employed: maybe::none() }
  let encoded = json::encode(person).expect("json")
  io::print(encoded.contains("\"name\":\"kit\"").to_str())
  io::print(encoded.contains("\"employed\":null").to_str())
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	js := exec.Command("go", "run", "-tags=goexperiment.jsonv2", ".", "run", "--target", "js-server", mainPath)
	js.Dir = ".."
	jsOut, err := js.CombinedOutput()
	if err != nil {
		t.Fatalf("did not expect js-server decode/json run error: %v\n%s", err, string(jsOut))
	}

	base := exec.Command("go", "run", "-tags=goexperiment.jsonv2", ".", "run", mainPath)
	base.Dir = ".."
	baseOut, err := base.CombinedOutput()
	if err != nil {
		t.Fatalf("did not expect baseline decode/json run error: %v\n%s", err, string(baseOut))
	}

	if string(jsOut) != string(baseOut) {
		t.Fatalf("unexpected decode/json output mismatch\njs:\n%s\nbase:\n%s", string(jsOut), string(baseOut))
	}
}

func TestBuildWritesJSBrowserPromiseStdlibCompanion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/dynamic as Dynamic
use ard/js/promise as promise

fn main() {
  promise::resolve(Dynamic::from_int(1))
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSBrowser); err != nil {
		t.Fatalf("did not expect browser build error: %v", err)
	}

	modulePath := filepath.Join(dir, "ard", "js", "promise.mjs")
	out, err := os.ReadFile(modulePath)
	if err != nil {
		t.Fatalf("failed to read generated promise module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, `import * as prelude from "../../ard.prelude.mjs";`) {
		t.Fatalf("expected browser promise module to import prelude namespace, got:\n%s", source)
	}
	if !strings.Contains(source, `return prelude.promiseResolve(value);`) {
		t.Fatalf("expected browser promise module to call prelude directly, got:\n%s", source)
	}
	if _, err := os.Stat(filepath.Join(dir, "ffi.stdlib.js-browser.mjs")); !os.IsNotExist(err) {
		t.Fatalf("did not expect copied browser stdlib ffi companion, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ard.prelude.mjs")); err != nil {
		t.Fatalf("expected copied js prelude companion: %v", err)
	}
}

func TestBuildWritesJSBrowserFetchStdlibCompanion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/js/fetch

fn main() {
  fetch::fetch("https://example.com")
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSBrowser); err != nil {
		t.Fatalf("did not expect browser build error: %v", err)
	}

	modulePath := filepath.Join(dir, "ard", "js", "fetch.mjs")
	out, err := os.ReadFile(modulePath)
	if err != nil {
		t.Fatalf("failed to read generated fetch module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, `import * as prelude from "../../ard.prelude.mjs";`) {
		t.Fatalf("expected browser fetch module to import prelude namespace, got:\n%s", source)
	}
	if !strings.Contains(source, `return prelude.fetchNative(method, url, body, headers, timeout);`) {
		t.Fatalf("expected browser fetch module to call prelude directly, got:\n%s", source)
	}
	if _, err := os.Stat(filepath.Join(dir, "ffi.stdlib.js-browser.mjs")); !os.IsNotExist(err) {
		t.Fatalf("did not expect copied browser stdlib ffi companion, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ard.prelude.mjs")); err != nil {
		t.Fatalf("expected copied js prelude companion: %v", err)
	}
}

func TestRunExecutesJSPromiseProgram(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/io
use ard/decode
use ard/dynamic as Dynamic
use ard/js/promise as promise

fn main() {
  let pair = promise::all([
    promise::resolve(Dynamic::from_int(20)),
    promise::delay(1, Dynamic::from_int(22)),
  ])

  let summed = promise::map(pair, fn(values: [Dynamic]) Dynamic {
    let a = decode::int(values.at(0)).expect("a")
    let b = decode::int(values.at(1)).expect("b")
    Dynamic::from_int(a + b)
  })

  let chained = promise::then(summed, fn(sum: Dynamic) {
    io::print(decode::int(sum).expect("sum"))
    let recovered = promise::rescue(
      promise::reject(Dynamic::from_str("boom")),
      fn(reason: Dynamic) {
        io::print(decode::string(reason).expect("reason"))
        Dynamic::from_str("recovered")
      },
    )
    promise::inspect(recovered, fn(value: Dynamic) {
      io::print(decode::string(value).expect("value"))
    })
  })

  promise::finally(chained, fn() {
    io::print("done")
  })
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	cmd := exec.Command("go", "run", "-tags=goexperiment.jsonv2", ".", "run", "--target", "js-server", mainPath)
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("did not expect js promise run error: %v\n%s", err, string(out))
	}
	if string(out) != "42\nboom\nrecovered\ndone\n" {
		t.Fatalf("unexpected promise output:\n%s", string(out))
	}
}

func TestRunExecutesJSFetchProgram(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		_ = r.Body.Close()
		w.Header().Set("X-Echo-Method", r.Method)
		w.Header().Set("X-Echo-Query", r.URL.RawQuery)
		w.Header().Set("X-Echo-Header", r.Header.Get("X-Demo"))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(fmt.Sprintf(`
use ard/decode
use ard/dynamic as Dynamic
use ard/io
use ard/js/fetch
use ard/js/promise as promise
use ard/maybe

fn main() {
  let sent = promise::inspect(
    fetch::fetch(
      %q,
      fetch::Options{
        method: maybe::some(fetch::Method::Post),
        headers: maybe::some(["content-type": "text/plain", "x-demo": "kit"]),
        body: maybe::some(Dynamic::from_str("hello")),
        timeout: maybe::some(5),
      },
    ),
    fn(res: fetch::Response) {
      io::print(res.status)
      io::print(res.is_ok().to_str())
      io::print(res.headers.get("x-echo-method").or("missing"))
      io::print(res.headers.get("x-echo-query").or("missing"))
      io::print(res.headers.get("x-echo-header").or("missing"))
      io::print(res.text())
    },
  )

  promise::then(sent, fn(_: fetch::Response) {
    promise::rescue(
      fetch::fetch("http://127.0.0.1:1/unreachable"),
      fn(reason: Dynamic) {
        io::print((decode::string(reason).expect("reason").size() > 0).to_str())
        fetch::Response{url: "", status: 0, headers: [:], body: ""}
      },
    )
  })
}
`, server.URL+"/echo?lang=ard")), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	cmd := exec.Command("go", "run", "-tags=goexperiment.jsonv2", ".", "run", "--target", "js-server", mainPath)
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("did not expect js fetch run error: %v\n%s", err, string(out))
	}
	if string(out) != "201\ntrue\nPOST\nlang=ard\nkit\nhello\ntrue\n" {
		t.Fatalf("unexpected js fetch output:\n%s", string(out))
	}
}

func TestRunExecutesJSStdlibFSProgram(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	workspace := filepath.Join(dir, "workspace")
	root := filepath.Join(workspace, "root")
	nested := filepath.Join(root, "nested")
	note := filepath.Join(root, "note.txt")
	copyPath := filepath.Join(root, "copy.txt")
	renamed := filepath.Join(root, "renamed.txt")
	mainPath := filepath.Join(dir, "main.ard")
	source := fmt.Sprintf(`
use ard/io
use ard/fs

fn main() {
  let root = %q
  let nested = %q
  let note = %q
  let copy_path = %q
  let renamed = %q

  if fs::exists(root) {
    fs::delete_dir(root).expect("cleanup root")
  }

  fs::create_dir(root).expect("create root")
  fs::create_dir(nested).expect("create nested")
  io::print(fs::exists(root).to_str())
  io::print(fs::is_dir(root).to_str())
  io::print(fs::create_file(note).is_ok().to_str())
  io::print(fs::is_file(note).to_str())
  io::print(fs::write(note, "hello").is_ok().to_str())
  io::print(fs::append(note, " world").is_ok().to_str())
  io::print(fs::read(note).or("bad"))
  io::print(fs::copy(note, copy_path).is_ok().to_str())
  io::print(fs::rename(copy_path, renamed).is_ok().to_str())
  io::print(fs::exists(renamed).to_str())
  io::print(fs::cwd().is_ok().to_str())
  io::print((fs::abs(note).or("bad") == note).to_str())

  let entries = fs::list_dir(root).expect("list root")
  mut saw_note = false
  mut saw_nested = false
  mut saw_renamed = false
  for entry in entries {
    if entry.name == "note.txt" and entry.is_file {
      saw_note = true
    } else if entry.name == "nested" and entry.is_file == false {
      saw_nested = true
    } else if entry.name == "renamed.txt" and entry.is_file {
      saw_renamed = true
    }
  }
  io::print(saw_note.to_str())
  io::print(saw_nested.to_str())
  io::print(saw_renamed.to_str())

  fs::delete_dir(root).expect("delete root")
  io::print(fs::exists(root).to_str())
}
`, root, nested, note, copyPath, renamed)
	if err := os.WriteFile(mainPath, []byte(source), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	js := exec.Command("go", "run", "-tags=goexperiment.jsonv2", ".", "run", "--target", "js-server", mainPath)
	js.Dir = ".."
	jsOut, err := js.CombinedOutput()
	if err != nil {
		t.Fatalf("did not expect js-server fs run error: %v\n%s", err, string(jsOut))
	}

	base := exec.Command("go", "run", "-tags=goexperiment.jsonv2", ".", "run", mainPath)
	base.Dir = ".."
	baseOut, err := base.CombinedOutput()
	if err != nil {
		t.Fatalf("did not expect baseline fs run error: %v\n%s", err, string(baseOut))
	}

	if string(jsOut) != string(baseOut) {
		t.Fatalf("unexpected fs output mismatch\njs:\n%s\nbase:\n%s", string(jsOut), string(baseOut))
	}
}

func TestBuildWritesPrimitiveModuleLowering(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
let a = Int::from_str("42").or(-1)
let b = Int::from_str("oops").or(-1)
let c = Float::from_int(100)
let d = Float::floor(3.75)
let e = Float::from_str("3.5").or(0.0)
let f = Float::from_str("oops").or(1.25)
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
	if !strings.Contains(source, `Number.parseInt(__input, 10)`) {
		t.Fatalf("expected Int::from_str lowering, got:\n%s", source)
	}
	if !strings.Contains(source, `const c = Number(100);`) {
		t.Fatalf("expected Float::from_int lowering, got:\n%s", source)
	}
	if !strings.Contains(source, `const d = Math.floor(3.75);`) {
		t.Fatalf("expected Float::floor lowering, got:\n%s", source)
	}
	if !strings.Contains(source, `const __value = Number(__input);`) {
		t.Fatalf("expected Float::from_str lowering, got:\n%s", source)
	}
}

func TestRunExecutesPrimitiveModuleProgram(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
let a = Int::from_str("42").or(-1)
let b = Int::from_str("oops").or(-1)
let c = Float::from_int(100)
let d = Float::floor(3.75)
let e = Float::from_str("3.5").or(0.0)
let f = Float::from_str("oops").or(1.25)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect build error: %v", err)
	}

	cmd := exec.Command("node", "--input-type=module", "-e", `
import { pathToFileURL } from "node:url";
const mod = await import(pathToFileURL(process.argv[1]).href);
if (mod.a !== 42) throw new Error("int parse success");
if (mod.b !== -1) throw new Error("int parse fallback");
if (mod.c !== 100) throw new Error("float from int");
if (mod.d !== 3) throw new Error("float floor");
if (mod.e !== 3.5) throw new Error("float parse success");
if (mod.f !== 1.25) throw new Error("float parse fallback");
`, outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("did not expect node assertion error: %v", err)
	}
}

func TestRunExecutesNumericSemanticsParity(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
let a = 265 / 3
let b = 1.5.to_str()
let c = 60.0.to_str()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect build error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, `Math.trunc((265) / (3))`) {
		t.Fatalf("expected int division truncation lowering, got:\n%s", source)
	}
	if !strings.Contains(source, `(1.5).toFixed(2)`) || !strings.Contains(source, `(60).toFixed(2)`) {
		t.Fatalf("expected float to_str fixed formatting lowering, got:\n%s", source)
	}

	cmd := exec.Command("node", "--input-type=module", "-e", `
import { pathToFileURL } from "node:url";
const mod = await import(pathToFileURL(process.argv[1]).href);
if (mod.a !== 88) throw new Error("int-division");
if (mod.b !== "1.50") throw new Error("float-to-str-1");
if (mod.c !== "60.00") throw new Error("float-to-str-2");
`, outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("did not expect node assertion error: %v", err)
	}
}

func TestBuildDoesNotEmitUnusedImportedEnumMethodHelper(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fetchish.ard"), []byte(`
enum Method {
  Get,
  Post,
}

impl Method {
  fn to_str() Str {
    match self {
      Method::Get => "GET",
      Method::Post => "POST",
    }
  }
}

struct Options {
  method: Method?,
}
`), 0o644); err != nil {
		t.Fatalf("failed to write fetchish source: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use demo/fetchish
use ard/maybe

fn main() {
  let _ = fetchish::Options{ method: maybe::some(fetchish::Method::Post) }
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect build error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if strings.Contains(source, "function __enum_method__Method__to_str(__enum_self) {") {
		t.Fatalf("did not expect unused enum method helper in root module, got:\n%s", source)
	}
	if strings.Contains(source, "const Method = Object.freeze(") {
		t.Fatalf("did not expect imported enum re-declaration in root module, got:\n%s", source)
	}
	if !strings.Contains(source, "demo_fetchish.Method.Post") {
		t.Fatalf("expected imported enum variant to reference imported module export, got:\n%s", source)
	}
}

func TestRunExecutesEnumMethods(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
enum Region {
  North,
  South,
}

impl Region {
  fn weight() Int {
    match self {
      Region::North => 3,
      Region::South => 2,
    }
  }
}

let a = Region::North.weight()
let b = Region::South.weight()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect build error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "function __enum_method__Region__weight(__enum_self) {") {
		t.Fatalf("expected enum method helper lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "const a = __enum_method__Region__weight(Region.North);") {
		t.Fatalf("expected enum method call lowering, got:\n%s", source)
	}

	cmd := exec.Command("node", "--input-type=module", "-e", `
import { pathToFileURL } from "node:url";
const mod = await import(pathToFileURL(process.argv[1]).href);
if (mod.a !== 3) throw new Error("enum-method-a");
if (mod.b !== 2) throw new Error("enum-method-b");
`, outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("did not expect node assertion error: %v", err)
	}
}

func TestBuildWritesMaybeEqualityLowering(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

let a = maybe::some("hello") == maybe::some("hello")
let b = maybe::some("hello") == maybe::none()
let c = maybe::none<Str>() == maybe::none()
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
	if !strings.Contains(source, `import { Maybe, Result, ardEnumValue, ardEq, ardToString, isArdEnum, isArdMaybe, isEnumOf, makeArdError, makeBreakSignal, makeEnum } from "./ard.prelude.mjs";`) {
		t.Fatalf("expected prelude import for maybe equality helper, got:\n%s", source)
	}
	if strings.Count(source, "ardEq(") < 3 {
		t.Fatalf("expected ardEq call sites for maybe equality, got:\n%s", source)
	}
	if !strings.Contains(source, `const a = ardEq(Maybe.some("hello"), Maybe.some("hello"));`) {
		t.Fatalf("expected some/some equality lowering, got:\n%s", source)
	}
	if !strings.Contains(source, `const b = ardEq(Maybe.some("hello"), Maybe.none());`) {
		t.Fatalf("expected some/none equality lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "const c = ardEq(Maybe.none(), Maybe.none());") {
		t.Fatalf("expected none/none equality lowering, got:\n%s", source)
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
	if !strings.Contains(source, `import { Maybe, Result, ardEnumValue, ardEq, ardToString, isArdEnum, isArdMaybe, isEnumOf, makeArdError, makeBreakSignal, makeEnum } from "./ard.prelude.mjs";`) {
		t.Fatalf("expected prelude import for Maybe/Result runtime helpers, got:\n%s", source)
	}
	if !strings.Contains(source, "const a = Maybe.none().isNone();") {
		t.Fatalf("expected maybe none/is_none lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "const b = Result.err(\"boom\").isErr();") {
		t.Fatalf("expected result err/is_err lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "value.andThen(") || !strings.Contains(source, "function(v) {") {
		t.Fatalf("expected maybe and_then lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "res.map(") || !strings.Contains(source, "function(value) {") || !strings.Contains(source, ").andThen(") {
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
	if !strings.Contains(source, `const Status = Object.freeze(`) || !strings.Contains(source, `active: makeEnum("Status", "active", 0)`) || !strings.Contains(source, `inactive: makeEnum("Status", "inactive", 1)`) {
		t.Fatalf("expected branded enum object lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "let d = label(Status.active)") && !strings.Contains(source, "const d = label(Status.active)") {
		t.Fatalf("expected enum variant lowering, got:\n%s", source)
	}
	if !strings.Contains(source, `if (isEnumOf(__match, "Status") && __match.value === 0) return`) || !strings.Contains(source, `if (isEnumOf(__match, "Status") && __match.value === 1) return`) {
		t.Fatalf("expected branded enum match lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "if (__match) return") {
		t.Fatalf("expected bool match lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "if (__match >= 1 && __match <= 3) return") {
		t.Fatalf("expected int/range match lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "if ((score >= 90)) return") || !strings.Contains(source, "if ((score >= 80)) return") {
		t.Fatalf("expected conditional match lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "if (__match.isSome()) return") || !strings.Contains(source, "const num = __match.value;") {
		t.Fatalf("expected maybe match lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "if (__match.isOk()) return") || !strings.Contains(source, "const msg = __match.error;") {
		t.Fatalf("expected result match lowering, got:\n%s", source)
	}
}

func TestBuildWritesConditionalMatchWithTryConditions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn describe(value: Int!Str) Str!Str {
  match {
    (try value) > 10 => Result::ok("big"),
    (try value) > 0 => Result::ok("small"),
    _ => Result::ok("zero"),
  }
}
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
	if !strings.Contains(source, "const __try") || !strings.Contains(source, "if (__try") || !strings.Contains(source, "} else {") || strings.Count(source, "const __try") < 2 {
		t.Fatalf("expected nested conditional match lowering for try conditions, got:\n%s", source)
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
	if !strings.Contains(source, `import { Maybe, Result, ardEnumValue, ardEq, ardToString, isArdEnum, isArdMaybe, isEnumOf, makeArdError, makeBreakSignal, makeEnum } from "./ard.prelude.mjs";`) {
		t.Fatalf("expected prelude import for try lowering, got:\n%s", source)
	}
	if strings.Contains(source, "catch (__ard_try) {") || strings.Contains(source, "makeTryReturn(") {
		t.Fatalf("did not expect try sentinel lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "const __try0 = value;") || !strings.Contains(source, "if (__try0.isErr()) {") {
		t.Fatalf("expected result try guard lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "return Result.err(__try0.error);") {
		t.Fatalf("expected result early return lowering, got:\n%s", source)
	}
	if !strings.Contains(source, "const err = __try1.error;") {
		t.Fatalf("expected catch var binding, got:\n%s", source)
	}
	if !strings.Contains(source, "if (__try2.isNone()) {") || !strings.Contains(source, "return Maybe.none();") {
		t.Fatalf("expected maybe try propagation, got:\n%s", source)
	}
	if !strings.Contains(source, "const out = __try0.ok;") || !strings.Contains(source, "const out = __try2.value;") {
		t.Fatalf("expected try success unwrapping, got:\n%s", source)
	}
	if !strings.Contains(source, "const out = (__try3.ok + 1);") {
		t.Fatalf("expected nested try lowering into statement flow, got:\n%s", source)
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

func TestRunDoesNotDoubleInvokeMainWhenCalledAtTopLevel(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/io

fn main() {
  io::print("once")
}

main()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	cmd := exec.Command("go", "run", "-tags=goexperiment.jsonv2", ".", "run", "--target", "js-server", mainPath)
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("did not expect js-server run error: %v\n%s", err, string(out))
	}
	if string(out) != "once\n" {
		t.Fatalf("expected single main invocation, got:\n%s", string(out))
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

func TestGenerateSourcesFromAIRSimpleModule(t *testing.T) {
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
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := string(files["main.mjs"])
	if !strings.Contains(source, "function add(a, b) {") {
		t.Fatalf("expected function definition in AIR JS output, got:\n%s", source)
	}
	if !strings.Contains(source, "const result = add(1, 2);") {
		t.Fatalf("expected script let in AIR JS output, got:\n%s", source)
	}
	if !strings.Contains(source, "export { add };") {
		t.Fatalf("expected function export in AIR JS output, got:\n%s", source)
	}
}

func TestBuildProgramFromAIRWritesModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn main() {
  "ok"
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSBrowser)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	outputPath := filepath.Join(dir, "main.mjs")
	builtPath, err := BuildProgram(program, outputPath, backend.TargetJSBrowser, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build AIR JS program: %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("expected built path %q, got %q", outputPath, builtPath)
	}
	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(out), "function main() {") {
		t.Fatalf("expected main function in output, got:\n%s", string(out))
	}
	if _, err := os.Stat(filepath.Join(dir, "ard.prelude.mjs")); err != nil {
		t.Fatalf("expected prelude companion: %v", err)
	}
}

func TestGenerateSourcesFromAIRCollectionsMaybeResult(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

let size = [1, 2, 3].size()
let first = [1, 2, 3].at(0)
let present = ["a": 1].get("a").or(0)
let some = maybe::some(2).or(0)
let ok: Int!Str = Result::ok(1)
let ok_value = ok.or(0)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := string(files["main.mjs"])
	for _, expected := range []string{
		"const size = [1, 2, 3].length;",
		"const first = [1, 2, 3][0];",
		"Maybe.some",
		"Maybe.some(2).or(0)",
		"Result.ok(1)",
		"ok.or(0)",
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("expected %q in AIR JS output, got:\n%s", expected, source)
		}
	}
}

func TestGenerateSourcesFromAIRImportedModuleCalls(t *testing.T) {
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
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := string(files["main.mjs"])
	if !strings.Contains(source, `import * as demo_utils from "./demo/utils.mjs";`) {
		t.Fatalf("expected imported module import, got:\n%s", source)
	}
	if !strings.Contains(source, "const result = demo_utils.add(1, 2);") {
		t.Fatalf("expected imported module call, got:\n%s", source)
	}
	if !strings.Contains(string(files["demo/utils.mjs"]), "function add(a, b) {") {
		t.Fatalf("expected imported module source, got:\n%s", string(files["demo/utils.mjs"]))
	}
}

func TestGenerateSourcesFromAIRMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

enum Status { active, inactive }

fn label(status: Status) Int {
  match status {
    Status::active => 1,
    Status::inactive => 2,
  }
}

fn bucket(num: Int) Str {
  match num {
    0 => "zero",
    1..3 => "few",
    _ => "many",
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
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := ""
	for _, content := range files {
		source += string(content)
	}
	for _, expected := range []string{
		`if (isEnumOf(__match`,
		`.value === 0)`,
		`if (__match`,
		`>= 1 && __match`,
		`.isSome()`,
		`.isOk()`,
		`.error;`,
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("expected %q in AIR JS output, got:\n%s", expected, source)
		}
	}
}

func TestRunProgramFromAIRServerPrimitive(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node not available: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn main() Int {
  1 + 2
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := RunProgram(program, backend.TargetJSServer, nil, loaded.ProjectInfo); err != nil {
		t.Fatalf("run AIR JS program: %v", err)
	}
}

func TestGenerateSourcesFromAIRMapForIn(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn sum(values: [Str: Int]) Int {
  mut total = 0
  for key, value in values {
    total = total + value
  }
  total
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := ""
	for _, content := range files {
		source += string(content)
	}
	if !strings.Contains(source, ".keys())[") || !strings.Contains(source, ".values())[") {
		t.Fatalf("expected map key/value index helpers in AIR JS output, got:\n%s", source)
	}
}

func TestGenerateSourcesFromAIRClosures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn create_adder(base: Int) fn(Int) Int {
  fn(value: Int) Int { base + value }
}

let add_two = create_adder(2)
let result = add_two(40)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := ""
	for _, content := range files {
		source += string(content)
	}
	if !strings.Contains(source, "function(value) {") || !strings.Contains(source, "return add_two(40);") && !strings.Contains(source, "const result = add_two(40);") {
		t.Fatalf("expected closure literal and call in AIR JS output, got:\n%s", source)
	}
}

func TestGenerateSourcesFromAIRUnionMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
type Printable = Str | Int | Bool

fn print(p: Printable) Str {
  match p {
    Str(str) => str,
    Int(num) => num.to_str(),
    _ => "boolean value",
  }
}

let value = print(20)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := ""
	for _, content := range files {
		source += string(content)
	}
	if !strings.Contains(source, "__ard_union_tag") || !strings.Contains(source, ".value;") {
		t.Fatalf("expected union wrap/match in AIR JS output, got:\n%s", source)
	}
}

func TestGenerateSourcesFromAIRTryLet(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

fn render(value: Int!Str) Str!Str {
  let out = try value
  Result::ok(out.to_str())
}

fn maybe_chain(value: Int?) Int? {
  let out = try value
  maybe::some(out + 1)
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := ""
	for _, content := range files {
		source += string(content)
	}
	for _, expected := range []string{".isErr()) return Result.err", ".isNone()) return Maybe.none()", ".ok;", ".value;"} {
		if !strings.Contains(source, expected) {
			t.Fatalf("expected %q in AIR JS output, got:\n%s", expected, source)
		}
	}
}

func TestGenerateSourcesFromAIRTraitToStringAndExternAdapters(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
extern fn maybe_value() Int? = {
  js-server = "maybeValue"
}
extern fn result_value() Int!Str = {
  js-server = "resultValue"
}

let label = 42.to_str()
let maybe = maybe_value()
let result = result_value()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, ffi, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	if !ffi.useProject {
		t.Fatalf("expected project ffi artifact use")
	}
	source := string(files["main.mjs"])
	for _, expected := range []string{"ardToString(42)", "Maybe.none()", "Maybe.some(__extern)", "Result.ok(__extern.ok)", "Result.err(__extern.error)"} {
		if !strings.Contains(source, expected) {
			t.Fatalf("expected %q in AIR JS output, got:\n%s", expected, source)
		}
	}
}

func TestGenerateSourcesFromAIRFibers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/async

fn value() Int { 42 }

let fiber = async::eval(fn() { value() })
let out = fiber.get()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := string(files["main.mjs"])
	if !strings.Contains(source, "done: true") || !strings.Contains(source, "const out = fiber.value;") {
		t.Fatalf("expected synchronous fiber lowering in AIR JS output, got:\n%s", source)
	}
}

func TestGenerateSourcesFromAIRExternStructListMapAdapters(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
struct Person { age: Int }

extern fn get_person() Person = {
  js-server = "getPerson"
}
extern fn get_people() [Person] = {
  js-server = "getPeople"
}
extern fn get_scores() [Str: Person] = {
  js-server = "getScores"
}

let person = get_person()
let people = get_people()
let scores = get_scores()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := string(files["main.mjs"])
	for _, expected := range []string{
		"new Person",
		"Array.isArray(project.getPeople())",
		"new Map(Object.entries",
		"__map instanceof Map",
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("expected %q in AIR JS output, got:\n%s", expected, source)
		}
	}
}

func TestBuildProgramFromAIRBrowserProjectFFI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.js-browser.mjs"), []byte("export function value() { return 42; }\n"), 0o644); err != nil {
		t.Fatalf("failed to write browser ffi: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
extern fn value() Int = {
  js-browser = "value"
}

let answer = value()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSBrowser)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := BuildProgram(program, outputPath, backend.TargetJSBrowser, loaded.ProjectInfo); err != nil {
		t.Fatalf("build AIR browser JS: %v", err)
	}
	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(out), `import * as project from "./ffi.project.js-browser.mjs";`) || !strings.Contains(string(out), "project.value()") {
		t.Fatalf("expected browser project ffi import/call, got:\n%s", string(out))
	}
	if _, err := os.Stat(filepath.Join(dir, "ffi.project.js-browser.mjs")); err != nil {
		t.Fatalf("expected copied browser ffi companion: %v", err)
	}
}

func TestGenerateSourcesFromAIRImportedStructConstruction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models.ard"), []byte(`
struct Person { age: Int }
`), 0o644); err != nil {
		t.Fatalf("failed to write models module: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use demo/models

let person = models::Person{age: 42}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := string(files["main.mjs"])
	if !strings.Contains(source, "class Person") || !strings.Contains(source, "new Person(42)") {
		t.Fatalf("expected imported struct type declaration and constructor, got:\n%s", source)
	}
}

func TestBuildProgramFromAIRRunsStructEnumListMapParity(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node not available: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
struct Person { age: Int }

enum Status { active, inactive }

fn person_age() Int {
  let person = Person{age: 41}
  person.age + 1
}

fn status_label(status: Status) Str {
  match status {
    Status::active => "active",
    Status::inactive => "inactive",
  }
}

fn list_score() Int {
  mut values = [1, 2]
  values.push(3)
  values.at(0) + values.size()
}

fn map_score() Int {
  ["a": 2].get("a").or(0)
}

let keep_person = person_age()
let keep_status = status_label(Status::active)
let keep_list = list_score()
let keep_map = map_score()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := BuildProgram(program, outputPath, backend.TargetJSServer, loaded.ProjectInfo); err != nil {
		t.Fatalf("build AIR JS program: %v", err)
	}
	script := `
import { pathToFileURL } from 'node:url';
const m = await import(pathToFileURL(process.argv[1]).href);
const got = [m.person_age(), m.status_label(m.Status.active), m.list_score(), m.map_score()];
console.log(JSON.stringify(got));
`
	cmd := exec.Command("node", "--input-type=module", "-e", script, outputPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run AIR JS module: %v\n%s", err, string(out))
	}
	if strings.TrimSpace(string(out)) != `[42,"active",4,2]` {
		t.Fatalf("unexpected AIR JS runtime output: %s", string(out))
	}
}

func TestBuildProgramFromAIRRunsMaybeResultTryMatchParity(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node not available: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/maybe

fn bucket(num: Int) Str {
  match num {
    0 => "zero",
    1..3 => "few",
    _ => "many",
  }
}

fn maybe_some() Int {
  match maybe::some(7) {
    num => num,
    _ => 0,
  }
}

fn maybe_none() Int {
  let empty: Int? = maybe::none()
  match empty {
    num => num,
    _ => 11,
  }
}

fn result_ok() Str {
  let res: Int!Str = Result::ok(4)
  match res {
    ok(num) => num.to_str(),
    err(msg) => msg,
  }
}

fn result_err() Str {
  let res: Int!Str = Result::err("no")
  match res {
    ok(num) => num.to_str(),
    err(msg) => msg,
  }
}

fn stringify(value: Int!Str) Str!Str {
  let num = try value
  Result::ok(num.to_str())
}

fn try_ok() Str {
  stringify(Result::ok(5)).or("bad")
}

fn try_err() Str {
  stringify(Result::err("boom")).or("fallback")
}

let keep_bucket = bucket(2)
let keep_some = maybe_some()
let keep_none = maybe_none()
let keep_ok = result_ok()
let keep_err = result_err()
let keep_try_ok = try_ok()
let keep_try_err = try_err()
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := BuildProgram(program, outputPath, backend.TargetJSServer, loaded.ProjectInfo); err != nil {
		t.Fatalf("build AIR JS program: %v", err)
	}
	script := `
import { pathToFileURL } from 'node:url';
const m = await import(pathToFileURL(process.argv[1]).href);
const got = [m.bucket(0), m.bucket(2), m.bucket(9), m.maybe_some(), m.maybe_none(), m.result_ok(), m.result_err(), m.try_ok(), m.try_err()];
console.log(JSON.stringify(got));
`
	cmd := exec.Command("node", "--input-type=module", "-e", script, outputPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run AIR JS module: %v\n%s", err, string(out))
	}
	if strings.TrimSpace(string(out)) != `["zero","few","many",7,11,"4","no","5","fallback"]` {
		t.Fatalf("unexpected AIR JS runtime output: %s", string(out))
	}
}

func TestGenerateSourcesFromAIRSpecializedGenericNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn id<$T>(value: $T) $T { value }

let a = id(1)
let b = id("x")
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetJSServer)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	files, _, err := GenerateSources(program, Options{Target: backend.TargetJSServer, RootFileName: "main.mjs"})
	if err != nil {
		t.Fatalf("generate AIR JS sources: %v", err)
	}
	source := string(files["main.mjs"])
	if strings.Count(source, "function id(") != 0 || !strings.Contains(source, "function id__") {
		t.Fatalf("expected specialized generic functions to be uniquely named, got:\n%s", source)
	}
	if !strings.Contains(source, "const a = id__") || !strings.Contains(source, "const b = id__") {
		t.Fatalf("expected calls to specialized generic functions, got:\n%s", source)
	}
}
