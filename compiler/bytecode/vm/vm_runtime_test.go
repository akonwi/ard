package vm

import "testing"

func TestBytecodeVMParityPanicVoidAndFunctionVariables(t *testing.T) {
	expectBytecodeRuntimeError(t, "This is an error", `
		fn speak() {
		  panic("This is an error")
		}
		speak()
		1 + 1
	`)

	runBytecodeRaw(t, `
		let unit = ()
		unit

		fn void() Void!Str {
			if not 42 == 42 {
				Result::err("42 should equal 42")
			}
			Result::ok(())
		}
		void()
	`)

	if got := runBytecode(t, `
		let multiply = fn(a: Int, b: Int) Int {
			a * b
		}
		multiply(3, 4)
	`); got != 12 {
		t.Fatalf("Expected 12, got %v", got)
	}
}
