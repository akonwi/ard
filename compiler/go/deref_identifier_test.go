package gotarget

import "testing"

func TestRunProgramTreatsDerefAsOrdinaryIdentifier(t *testing.T) {
	program := lowerSource(t, `
		struct Value {
			deref: Int,
		}

		impl Value {
			fn deref(deref: Int) Int {
				self.deref + deref
			}
		}

		fn deref(deref: Int) Int {
			deref + 1
		}

		fn call_deref() Int {
			deref(2)
		}

		fn main() {
			let deref = 2
			let value = Value{deref: 3}
			if not value.deref(deref) == 5 {
				panic("method, parameter, binding, or property named deref failed")
			}
			if not call_deref() == 3 {
				panic("function named deref failed")
			}
		}
	`)
	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
