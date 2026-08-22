package gotarget

import "testing"

func TestRunProgramBoxesVoidAsAny(t *testing.T) {
	program := lowerSource(t, `
		fn action() {}

		fn erase(value: Any) {}

		fn main() {
			erase(action())
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
