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
