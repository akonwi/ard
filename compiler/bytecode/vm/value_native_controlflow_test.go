package vm

import (
	"testing"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

func TestValueNativeMaybeUnwrapOpcode(t *testing.T) {
	program := bytecode.Program{
		Types: []bytecode.TypeEntry{{ID: 1, Name: "Int"}},
		Functions: []bytecode.Function{{
			Name:     "main",
			Locals:   0,
			MaxStack: 1,
			Code: []bytecode.Instruction{
				{Op: bytecode.OpMaybeUnwrap, A: 1},
				{Op: bytecode.OpReturn},
			},
		}},
	}

	vm := New(program)
	frame := &Frame{
		Fn:         &program.Functions[0],
		Locals:     []any{},
		Stack:      []any{runtime.SomeValue(7)},
		StackTop:   1,
		ReturnType: checker.Int,
	}
	vm.Frames = []*Frame{frame}

	res, err := vm.run()
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got := res.AsInt(); got != 7 {
		t.Fatalf("expected unwrapped maybe value 7, got %d", got)
	}
}

func TestValueNativeResultTryOpcode(t *testing.T) {
	program := bytecode.Program{
		Types: []bytecode.TypeEntry{{ID: 1, Name: "Int"}, {ID: 2, Name: "Str"}},
		Functions: []bytecode.Function{{
			Name:     "main",
			Locals:   0,
			MaxStack: 1,
			Code: []bytecode.Instruction{
				{Op: bytecode.OpTryResult, Imm: 1, C: 2},
				{Op: bytecode.OpReturn},
			},
		}},
	}

	vm := New(program)
	frame := &Frame{
		Fn:         &program.Functions[0],
		Locals:     []any{},
		Stack:      []any{runtime.OkValue(11)},
		StackTop:   1,
		ReturnType: checker.Int,
	}
	vm.Frames = []*Frame{frame}

	res, err := vm.run()
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got := res.AsInt(); got != 11 {
		t.Fatalf("expected unwrapped result value 11, got %d", got)
	}
}
