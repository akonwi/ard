package gotarget

import "testing"

func TestGoTargetRepeatedGoVariadicArguments(t *testing.T) {
	program := lowerParitySource(t, `use go:fmt

fn main() Bool {
  fmt::Sprint("a", "b", 3) == "ab3"
}`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}
