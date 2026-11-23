package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/compiler/air"
	"github.com/akonwi/ard/compiler/frontend"
)

// Issue #607: a static function declared in another module is a function
// value, like a local static function or a module function. A generic one is
// instantiated from the expected function type (issue #593).
func TestRunProgramUsesImportedStaticFunctionsAsValues(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "feature.ard"), []byte(`
struct Box {
  value: Int,
}

fn Box::wrap(value: Int) Box {
  Box{value: value}
}

fn Box::same(x: $T) $T {
  x
}

struct Cell<$T> {
  value: $T,
}

fn Cell::wrap(value: $T) Cell<$T> {
  Cell{value: value}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn apply(f: fn(Int) feature::Box, x: Int) feature::Box {
  f(x)
}

fn apply_str(f: fn(Str) Str, x: Str) Str {
  f(x)
}

fn main() {
  let wrap: fn(Int) feature::Box = feature::Box::wrap
  if wrap(3).value != 3 { panic("imported static binding") }
  if apply(feature::Box::wrap, 4).value != 4 { panic("imported static argument") }
  let inferred = feature::Box::wrap
  if inferred(5).value != 5 { panic("imported static inferred binding") }

  let same: fn(Int) Int = feature::Box::same
  if same(6) != 6 { panic("imported generic static binding") }
  if apply_str(feature::Box::same, "seven") != "seven" { panic("imported generic static argument") }
  let cell: fn(Int) feature::Cell<Int> = feature::Cell::wrap
  if cell(8).value != 8 { panic("imported generic struct static binding") }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

// Issue #607: a static function is reached through any path that names its
// owner type, including a type alias here, an alias in another module, and an
// alias re-exported through a third module. An alias that applies type
// arguments binds the static function's generics of the same name.
func TestRunProgramResolvesStaticFunctionsThroughTypeAliases(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"ard.toml": "name = \"app\"\nard = \">= 0.1.0\"\n",
		"feature.ard": `
struct Box {
  value: Int,
}

fn Box::wrap(value: Int) Box {
  Box{value: value}
}

struct Cell<$T> {
  value: $T,
}

fn Cell::wrap(value: $T) Cell<$T> {
  Cell{value: value}
}

fn Cell::tag(value: $T, label: $L) Cell<$T> {
  Cell{value: value}
}

type Alias = Box
type IntCell = Cell<Int>
`,
		"barrel.ard": `
use app/feature

type Reexport = feature::Box
type StrCell = feature::Cell<Str>
`,
		"main.ard": `
use app/barrel
use app/feature

type Local = feature::Box
type LocalIntCell = feature::Cell<Int>

struct Own {
  value: Int,
}

fn Own::make(value: Int) Own {
  Own{value: value}
}

struct Pair<$T> {
  left: $T,
  right: $T,
}

fn Pair::of(value: $T) Pair<$T> {
  Pair{left: value, right: value}
}

type OwnAlias = Own
type IntPair = Pair<Int>

fn apply(f: fn(Int) feature::Box, x: Int) feature::Box {
  f(x)
}

fn main() {
  if feature::Alias::wrap(1).value != 1 { panic("module alias call") }
  if Local::wrap(2).value != 2 { panic("local alias of imported type call") }
  if barrel::Reexport::wrap(3).value != 3 { panic("re-exported alias call") }
  if OwnAlias::make(4).value != 4 { panic("local alias call") }

  let module_alias: fn(Int) feature::Box = feature::Alias::wrap
  if module_alias(5).value != 5 { panic("module alias value") }
  if apply(Local::wrap, 6).value != 6 { panic("local alias of imported type value") }
  let reexport = barrel::Reexport::wrap
  if reexport(7).value != 7 { panic("re-exported alias value") }
  let own = OwnAlias::make
  if own(8).value != 8 { panic("local alias value") }

  if feature::IntCell::wrap(9).value + 1 != 10 { panic("generic module alias call") }
  if barrel::StrCell::wrap("ten").value != "ten" { panic("generic re-exported alias call") }
  if IntPair::of(11).right + 1 != 12 { panic("generic local alias call") }
  let int_cell = feature::IntCell::wrap
  if int_cell(13).value + 1 != 14 { panic("generic module alias value") }
  let local_int_cell = LocalIntCell::wrap
  if local_int_cell(15).value + 1 != 16 { panic("generic local alias of imported type value") }
  let pair = IntPair::of
  if pair(17).left + 1 != 18 { panic("generic local alias value") }
  let tag: fn(Int, Str) feature::Cell<Int> = feature::IntCell::tag
  if tag(19, "label").value != 19 { panic("partially bound alias value") }
  if feature::IntCell::tag(20, true).value != 20 { panic("partially bound alias call") }
}
`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(root, "main.ard")
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

// Issue #607: a static function declared here under the exact name
// `owner::fn` takes precedence over an imported module named owner, for a
// function value as for a call.
func TestRunProgramLocalStaticFunctionShadowsSameNamedModuleAsValue(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"ard.toml":    "name = \"app\"\nard = \">= 0.1.0\"\n",
		"feature.ard": "fn make(value: Int) Int {\n  value\n}\n",
		"main.ard": `
use app/feature

struct feature {
  value: Int,
}

fn feature::make(value: Int) Int {
  value + 100
}

fn main() {
  if feature::make(1) != 101 { panic("local static call") }
  let make = feature::make
  if make(1) != 101 { panic("local static value") }
}
`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(root, "main.ard")
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
