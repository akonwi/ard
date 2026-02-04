package bytecode

import "fmt"

// VerifyProgram performs basic validation of a bytecode program.
// It checks jump bounds, stack underflows, and arity consistency.
func VerifyProgram(program Program) error {
	for i := range program.Functions {
		fn := program.Functions[i]
		if err := verifyFunction(program, fn); err != nil {
			return err
		}
	}
	return nil
}

func verifyFunction(program Program, fn Function) error {
	stack := 0
	for ip, inst := range fn.Code {
		pop, push, err := stackEffect(program, inst)
		if err != nil {
			return fmt.Errorf("%s ip=%d: %w", fn.Name, ip, err)
		}
		if (inst.Op == OpReturn || inst.Op == OpPanic) && stack < pop {
			stack = 0
			continue
		}
		stack -= pop
		if stack < 0 {
			return fmt.Errorf("%s ip=%d (%s): stack underflow", fn.Name, ip, inst.Op)
		}
		stack += push
		switch inst.Op {
		case OpJump, OpJumpIfFalse, OpJumpIfTrue:
			if inst.A < 0 || inst.A >= len(fn.Code) {
				return fmt.Errorf("%s ip=%d: jump target out of range", fn.Name, ip)
			}
		case OpTryResult, OpTryMaybe:
			if inst.A >= 0 && inst.A >= len(fn.Code) {
				return fmt.Errorf("%s ip=%d: try catch target out of range", fn.Name, ip)
			}
		case OpCall:
			if inst.A < 0 || inst.A >= len(program.Functions) {
				return fmt.Errorf("%s ip=%d: call target out of range", fn.Name, ip)
			}
			target := program.Functions[inst.A]
			if target.Arity != inst.B {
				return fmt.Errorf("%s ip=%d: arity mismatch for %s", fn.Name, ip, target.Name)
			}
		case OpMakeClosure:
			if inst.A < 0 || inst.A >= len(program.Functions) {
				return fmt.Errorf("%s ip=%d: closure target out of range", fn.Name, ip)
			}
			if inst.B < 0 {
				return fmt.Errorf("%s ip=%d: invalid capture count", fn.Name, ip)
			}
		}
	}
	return nil
}

func stackEffect(program Program, inst Instruction) (int, int, error) {
	switch inst.Op {
	case OpCall:
		return inst.B, 1, nil
	case OpCallExtern:
		return inst.Imm, 1, nil
	case OpCallModule:
		return inst.Imm, 1, nil
	case OpCallClosure:
		return inst.B + 1, 1, nil
	case OpCallMethod:
		return inst.B + 1, 1, nil
	case OpMakeList:
		return inst.B, 1, nil
	case OpMakeMap:
		return inst.B * 2, 1, nil
	case OpMakeStruct:
		return inst.B * 2, 1, nil
	case OpMakeEnum:
		return 1, 1, nil
	case OpMakeNone:
		return 0, 1, nil
	case OpStrMethod:
		return inst.B + 1, 1, nil
	case OpIntMethod, OpFloatMethod, OpBoolMethod:
		return 1, 1, nil
	case OpMaybeMethod, OpResultMethod:
		return inst.B + 1, 1, nil
	case OpMaybeUnwrap, OpResultUnwrap, OpTypeName:
		return 1, 1, nil
	case OpTryResult, OpTryMaybe:
		return 1, 1, nil
	case OpMakeClosure:
		return inst.B, 1, nil
	case OpAsyncStart, OpAsyncEval:
		return 1, 1, nil
	case OpSetField:
		return 2, 0, nil
	case OpGetField:
		return 1, 1, nil
	case OpListPush, OpListPrepend:
		return 2, 1, nil
	case OpListSet:
		return 3, 1, nil
	case OpMapSet:
		return 3, 1, nil
	case OpMapDrop, OpMapHas, OpMapGet, OpMapGetValue:
		return 2, 1, nil
	}
	effect := inst.Op.StackEffect()
	return effect.Pop, effect.Push, nil
}
