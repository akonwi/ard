package gotarget

import "testing"

func TestGoTargetRepeatedGoVariadicArguments(t *testing.T) {
	program := lowerParitySource(t, `use go:fmt

fn main() Bool {
  let sprint = fmt::Sprint
  fmt::Sprint("a", "b", 3) == "ab3" and sprint("c", "d", 4) == "cd4" and sprint() == ""
}`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}
