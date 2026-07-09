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
