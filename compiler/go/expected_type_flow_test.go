package gotarget

import "testing"

// A list literal that is a function body's final expression converts its
// elements to the declared element type.
func TestGoTargetReturnTypeTypesFinalCollectionLiteral(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
use go:os
use go:syscall

type Val = Int | Str

fn signals() [os::Signal] {
  [os::Interrupt, syscall::SIGTERM]
}

fn values() [Val] {
  [1, "two"]
}

fn named() [Str: Val] {
  ["a": 1, "b": "two"]
}

fn show(value: Val) Str {
  match value {
    Int => "int {it}",
    Str => "str {it}",
  }
}

fn main() {
  for signal in signals() {
    fmt::Println(signal.String())
  }
  for value in values() {
    fmt::Println(show(value))
  }
  fmt::Println(show(named().get("a").expect("a")), show(named().get("b").expect("b")))
}
`)
	want := "interrupt\nterminated\nint 1\nstr two\nint 1 str two\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// if/else branches returning different members of the declared return type
// produce values of that type.
func TestGoTargetIfBranchesUnifyToExpectedType(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

struct Square { size: Int }
struct Circle { radius: Int }
type Shape = Square | Circle
type Val = Int | Str

trait Named {
  fn name() Str
}

struct A {}
impl Named for A {
  fn name() Str {
    "a"
  }
}

struct B {}
impl Named for B {
  fn name() Str {
    "b"
  }
}

fn pick(square: Bool) Shape {
  if square {
    Square{size: 1}
  } else {
    Circle{radius: 2}
  }
}

fn describe(n: Int) Val {
  if n == 0 {
    0
  } else if n == 1 {
    "one"
  } else {
    n
  }
}

fn named(a: Bool) Named {
  if a {
    A{}
  } else {
    B{}
  }
}

fn shape(s: Shape) Str {
  match s {
    Square => "square {it.size}",
    Circle => "circle {it.radius}",
  }
}

fn show(value: Val) Str {
  match value {
    Int => "int {it}",
    Str => "str {it}",
  }
}

fn main() {
  fmt::Println(shape(pick(true)), shape(pick(false)))
  fmt::Println(show(describe(0)), show(describe(1)), show(describe(7)))
  fmt::Println(named(true).name(), named(false).name())
}
`)
	want := "square 1 circle 2\nint 0 str one int 7\na b\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// A tuple literal argument converts its elements to the parameter's element
// types.
func TestGoTargetParameterTypesTupleLiteralArgument(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

type Val = Int | Str

struct Holder { pair: (Val, Val) }

impl Holder {
  fn show_with(pair: (Val, Val)) Str {
    let first, _ = self.pair
    let _, second = pair
    "{show(first)} {show(second)}"
  }
}

fn show(value: Val) Str {
  match value {
    Int => "int {it}",
    Str => "str {it}",
  }
}

fn take(pair: (Val, Val)) Str {
  let first, second = pair
  "{show(first)} {show(second)}"
}

fn maybe_take(pair: (Val, Val)?) Str {
  match pair {
    p => take(p),
    _ => "none",
  }
}

fn main() {
  fmt::Println(take((1, 2)))
  fmt::Println(take((1, "x")))
  fmt::Println(maybe_take(("z", 9)))
  let holder = Holder{pair: (3, 4)}
  fmt::Println(holder.show_with((5, "y")))
}
`)
	want := "int 1 int 2\nint 1 str x\nstr z int 9\nint 3 str y\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
