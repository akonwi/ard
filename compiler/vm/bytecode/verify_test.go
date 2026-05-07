package bytecode

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestLowerDiscardedIfWithListPushVerifies(t *testing.T) {
	result := parse.Parse([]byte(`
		fn build_shapes(count: Int) [Int] {
			mut shapes: [Int] = []
			for i in 0..count {
				if i % 4 == 0 {
					shapes.push(i)
				} else if i % 4 == 1 {
					shapes.push(i + 1)
				} else {
					shapes.push(i + 2)
				}
			}
			shapes
		}
		fn main() Int {
			build_shapes(10).size()
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("air lower: %v", err)
	}
	bytecodeProgram, err := Lower(program)
	if err != nil {
		t.Fatalf("bytecode lower: %v", err)
	}
	if err := Verify(bytecodeProgram); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyAcceptsStraightLineStackUsage(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 0,
		Code: []Instruction{
			{Op: OpConstInt},
			{Op: OpReturn},
		},
	})

	if err := Verify(program); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyAcceptsCompatibleStackHeightAtJoin(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 0,
		Code: []Instruction{
			{Op: OpConstBool},         // stack: 1
			{Op: OpJumpIfFalse, A: 4}, // stack: 0 on both edges
			{Op: OpConstInt},          // true branch stack: 1
			{Op: OpJump, A: 5},
			{Op: OpConstInt}, // false branch stack: 1
			{Op: OpReturn},
		},
	})

	if err := Verify(program); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsInconsistentStackHeightAtJoin(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 0,
		Code: []Instruction{
			{Op: OpConstBool},         // stack: 1
			{Op: OpJumpIfFalse, A: 4}, // stack: 0 on both edges
			{Op: OpConstInt},          // true branch stack: 1
			{Op: OpJump, A: 5},
			{Op: OpNoop},   // false branch stack: 0
			{Op: OpReturn}, // join reached with stack 1 or 0
		},
	})

	err := Verify(program)
	if err == nil {
		t.Fatalf("Verify() error = nil, want stack mismatch")
	}
	if !strings.Contains(err.Error(), "stack") {
		t.Fatalf("Verify() error = %q, want stack-related message", err)
	}
}

func TestVerifyAcceptsLoopBackedgeWithStableStackHeight(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 2,
		Code: []Instruction{
			{Op: OpJumpIfIntGtLocal, A: 3, B: 0, C: 1},
			{Op: OpJump, A: 0},
			{Op: OpNoop},
			{Op: OpConstInt},
			{Op: OpReturn},
		},
	})

	if err := Verify(program); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyAcceptsTryResultWithCatch(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 1,
		Code: []Instruction{
			{Op: OpConstInt},
			{Op: OpMakeResultErr},
			{Op: OpTryResult, B: 4, C: 0},
			{Op: OpReturn},
			{Op: OpLoadLocal, A: 0},
			{Op: OpReturn},
		},
	})

	if err := Verify(program); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsTryResultNoCatchWhenSuccessPathUnderflows(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 0,
		Code: []Instruction{
			{Op: OpConstInt},
			{Op: OpMakeResultOk},
			{Op: OpTryResult, B: -1, C: -1},
			{Op: OpPop},
			{Op: OpReturn},
		},
	})

	err := Verify(program)
	if err == nil {
		t.Fatalf("Verify() error = nil, want success-path underflow")
	}
	if !strings.Contains(err.Error(), "underflow") {
		t.Fatalf("Verify() error = %q, want underflow message", err)
	}
}

func TestVerifyAcceptsTryMaybeWithCatch(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 0,
		Code: []Instruction{
			{Op: OpMakeMaybeNone},
			{Op: OpTryMaybe, B: 3, C: -1},
			{Op: OpReturn},
			{Op: OpConstInt},
			{Op: OpReturn},
		},
	})

	if err := Verify(program); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsTryMaybeNoCatchWhenSuccessPathUnderflows(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 0,
		Code: []Instruction{
			{Op: OpConstInt},
			{Op: OpMakeMaybeSome},
			{Op: OpTryMaybe, B: -1, C: -1},
			{Op: OpPop},
			{Op: OpReturn},
		},
	})

	err := Verify(program)
	if err == nil {
		t.Fatalf("Verify() error = nil, want success-path underflow")
	}
	if !strings.Contains(err.Error(), "underflow") {
		t.Fatalf("Verify() error = %q, want underflow message", err)
	}
}

func TestVerifyAcceptsMapGetLocalTryMaybeWithCatch(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 2,
		Code: []Instruction{
			{Op: OpMapGetLocalTryMaybe, B: 0, C: 1, Imm: 2},
			{Op: OpReturn},
			{Op: OpConstInt},
			{Op: OpReturn},
		},
	})

	if err := Verify(program); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsMapGetLocalTryMaybeNoCatchWhenSuccessPathUnderflows(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 2,
		Code: []Instruction{
			{Op: OpMapGetLocalTryMaybe, B: 0, C: 1, Imm: -1},
			{Op: OpPop},
			{Op: OpReturn},
		},
	})

	err := Verify(program)
	if err == nil {
		t.Fatalf("Verify() error = nil, want success-path underflow")
	}
	if !strings.Contains(err.Error(), "underflow") {
		t.Fatalf("Verify() error = %q, want underflow message", err)
	}
}

func TestVerifyRejectsReachableStackUnderflow(t *testing.T) {
	program := verifierTestProgram(Function{
		Name: "main",
		Code: []Instruction{
			{Op: OpPop},
		},
	})

	err := Verify(program)
	if err == nil {
		t.Fatalf("Verify() error = nil, want stack underflow")
	}
	if !strings.Contains(err.Error(), "underflow") {
		t.Fatalf("Verify() error = %q, want underflow message", err)
	}
}

func TestVerifyRejectsBranchTargetOutOfRange(t *testing.T) {
	program := verifierTestProgram(Function{
		Name: "main",
		Code: []Instruction{
			{Op: OpJump, A: 1},
		},
	})

	err := Verify(program)
	if err == nil {
		t.Fatalf("Verify() error = nil, want jump target rejection")
	}
	if !strings.Contains(err.Error(), "jump target") {
		t.Fatalf("Verify() error = %q, want jump target message", err)
	}
}

func TestVerifyRejectsTryCatchTargetOutOfRange(t *testing.T) {
	program := verifierTestProgram(Function{
		Name: "main",
		Code: []Instruction{
			{Op: OpConstInt},
			{Op: OpTryMaybe, B: 2, C: -1},
		},
	})

	err := Verify(program)
	if err == nil {
		t.Fatalf("Verify() error = nil, want try target rejection")
	}
	if !strings.Contains(err.Error(), "try jump target") {
		t.Fatalf("Verify() error = %q, want try jump target message", err)
	}
}

func TestVerifyIgnoresUnreachableCodeForStackValidation(t *testing.T) {
	program := verifierTestProgram(Function{
		Name: "main",
		Code: []Instruction{
			{Op: OpConstInt},
			{Op: OpReturn},
			{Op: OpPop},
		},
	})

	if err := Verify(program); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsReachableFallthroughOffEnd(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 0,
		Code: []Instruction{
			{Op: OpConstInt},          // return value
			{Op: OpConstBool},         // condition
			{Op: OpJumpIfFalse, A: 4}, // false branch skips return
			{Op: OpReturn},
			{Op: OpNoop}, // reachable fallthrough off end
		},
	})

	err := Verify(program)
	if err == nil {
		t.Fatalf("Verify() error = nil, want fallthrough rejection")
	}
	if !strings.Contains(err.Error(), "fall") && !strings.Contains(err.Error(), "terminate") {
		t.Fatalf("Verify() error = %q, want termination-related message", err)
	}
}

func TestVerifyTreatsPanicAsTerminating(t *testing.T) {
	program := verifierTestProgram(Function{
		Name:   "main",
		Locals: 0,
		Code: []Instruction{
			{Op: OpConstBool},         // stack: 1
			{Op: OpJumpIfFalse, A: 4}, // stack: 0 on both edges
			{Op: OpConstStr, B: 0},    // true branch stack: 1
			{Op: OpPanic},             // true branch terminates
			{Op: OpReturn},            // false branch underflows unless panic is terminating
		},
	}, Constant{Kind: ConstStr, Str: "boom"})

	err := Verify(program)
	if err == nil {
		t.Fatalf("Verify() error = nil, want stack underflow on false branch")
	}
	if !strings.Contains(err.Error(), "stack") {
		t.Fatalf("Verify() error = %q, want stack-related message", err)
	}
}

func verifierTestProgram(fn Function, constants ...Constant) *Program {
	return &Program{
		Constants: constants,
		Functions: []Function{fn},
	}
}
