package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/compiler/air"
	"github.com/akonwi/ard/compiler/frontend"
)

// Issue #593: a generic named function used as a value is instantiated from
// the expected function type.
func TestGoTargetGenericNamedFunctionValuesInstantiateFromExpectedType(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

fn same(x: $T) $T {
  x
}

fn first(a: $A, b: $B) $A {
  a
}

fn empty() [$T] {
  let items: [$T] = []
  items
}

fn apply(f: fn(Int) Int, x: Int) Int {
  f(x)
}

fn apply_to(x: $A, f: fn($A) $A) $A {
  f(x)
}

fn pick() fn(Str) Str {
  same
}

struct Box<$T> {
  value: $T,
}

fn Box::wrap(value: $T) Box<$T> {
  Box{value: value}
}

struct Holder {
  f: fn(Int) Int,
}

fn through(x: $U) $U {
  let f: fn($U) $U = same
  f(x)
}

fn eq(a: $T, b: $T) Bool {
  a == b
}

// The enclosing $U must become comparable in Go because eq requires it.
fn check(x: $U, y: $U) Bool {
  let f: fn($U, $U) Bool = eq
  f(x, y)
}

fn main() {
  let m: Int? = Maybe::new(4)
  fmt::Println(m.map(same).or(0))
  fmt::Println(apply(same, 4))
  let f: fn(Int) Int = same
  fmt::Println(f(5))

  let r: Int!Str = Result::ok(6)
  fmt::Println(r.map(same).or(0))
  fmt::Println(apply_to("seven", same))
  fmt::Println(pick()("eight"))
  let holder = Holder{f: same}
  fmt::Println(holder.f(9))
  fmt::Println(through(10))
  fmt::Println(check("ten", "ten"))

  let pair: fn(Int, Str) Int = first
  fmt::Println(pair(11, "ignored"))
  let make: fn() [Int] = empty
  fmt::Println(make().size())

  let wrap: fn(Str) Box<Str> = Box::wrap
  fmt::Println(wrap("twelve").value)

  mut g: fn(Bool) Bool = fn(b: Bool) Bool { not b }
  g = same
  fmt::Println(g(true))
}
`)
	want := "4\n4\n5\n6\nseven\neight\n9\n10\ntrue\n11\n0\ntwelve\ntrue\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// Issue #593: every contextually typed position instantiates the reference,
// including nullable destinations, collections, and discarded returns.
func TestGoTargetGenericFunctionValuesInContextualPositions(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

fn same(x: $T) $T {
  x
}

fn take(f: (fn(Int) Int)?) Int {
  match f {
    g => g(1),
    _ => 0,
  }
}

fn run(f: fn(Int)) {
  f(3)
}

struct Opt {
  f: (fn(Str) Str)?,
}

struct Box<$T> {
  value: $T,
}

fn main() {
  fmt::Println(take(same))
  mut opt = Opt{f: same}
  fmt::Println(opt.f.or(fn(s: Str) Str { "none" })("field"))
  opt.f = same
  fmt::Println(opt.f.or(fn(s: Str) Str { "none" })("assigned"))
  let maybe: (fn(Int) Int)? = Maybe::new(same)
  fmt::Println(maybe.or(fn(n: Int) Int { 0 })(2))
  let boxed = Box<fn(Int) Int>{value: same}
  fmt::Println(boxed.value(3))
  let fns: [fn(Int) Int] = [same, same]
  fmt::Println(fns.size())
  let pair: (fn(Int) Int, Int) = (same, 4)
  let (first, n) = pair
  fmt::Println(first(n))
  run(same)
  let discard: fn(Int) Void = same
  discard(5)
  fmt::Println("done")
}
`)
	want := "1\nfield\nassigned\n2\n3\n2\n4\ndone\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// Issue #593: generic functions from another module are instantiated the
// same way.
func TestRunProgramInstantiatesImportedGenericFunctionValues(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "feature.ard"), []byte(`
fn same(x: $T) $T {
  x
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn apply(f: fn(Int) Int, x: Int) Int {
  f(x)
}

fn main() {
  if apply(feature::same, 1) != 1 { panic("imported generic argument") }
  let f: fn(Str) Str = feature::same
  if f("two") != "two" { panic("imported generic binding") }
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
