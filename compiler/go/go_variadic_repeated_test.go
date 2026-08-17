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

func TestGoTargetSpreadsListsIntoGoVariadics(t *testing.T) {
	program := lowerParitySource(t, `use go:fmt

fn call_sprint(sprint: fn(...Any) Str, values: [Any]) Str {
  sprint(values...)
}

fn main() Bool {
  let direct_values: [Any] = ["a", "b", 3]
  let captured_values: [Any] = ["c", "d", 4]
  let captured_reference = mut captured_values
  let sprint = fmt::Sprint
  fmt::Sprint(direct_values...) == "ab3" and sprint(captured_reference...) == "cd4" and call_sprint(sprint, captured_values) == "cd4"
}`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}
