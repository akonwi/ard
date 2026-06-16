package gotarget

import "testing"

func TestGoTargetParityFloatNumerics(t *testing.T) {
	program := lowerParitySource(t, `
use ard/float

fn main() Str {
  Float::format(Float::round(-2.5), 1) + "," + Float::format(Float::from_int(7) / 2.0, 1)
}
`)
	if got := runGoTargetParityJSON(t, program); got != `"-3.0,3.5"` {
		t.Fatalf("got %s, want %s", got, `"-3.0,3.5"`)
	}
}
