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
