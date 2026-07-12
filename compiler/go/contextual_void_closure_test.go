package gotarget

import "testing"

func TestGoTargetContextualVoidClosureDiscardsValue(t *testing.T) {
	program := lowerParitySource(t, `fn each(callback: fn(Int)) {
  callback(1)
}

fn main() Bool {
  mut seen = 0
  each(fn(value) {
    seen = value
    value + 1
  })
  seen == 1
}`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}

func TestGoTargetStoredValueReturningFunctionCoercesToVoid(t *testing.T) {
	program := lowerParitySource(t, `struct Handler {
  callback: fn(Int),
}

mut adaptations = 0
mut observed = 0

fn make_callback() fn(Int) Int {
  adaptations =+ 1
  fn(value: Int) Int {
    observed = value
    value + 1
  }
}

fn run(callback: fn(Int)) {
  callback(7)
}

fn main() Bool {
  let callback: fn(Int) = make_callback()
  let handler = Handler{callback: make_callback()}
  run(make_callback())
  callback(3)
  handler.callback(5)
  adaptations == 3 and observed == 5
}`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}
