package bytecode

import "fmt"

func Verify(program *Program) error {
	if program == nil {
		return fmt.Errorf("bytecode program is nil")
	}
	for i := range program.Functions {
		if err := verifyFunction(program, &program.Functions[i]); err != nil {
			return err
		}
	}
	return nil
}

func verifyFunction(program *Program, fn *Function) error {
	if fn == nil {
		return fmt.Errorf("function is nil")
	}
	stack := 0
	for ip, inst := range fn.Code {
		pop, push, err := stackEffect(inst)
		if err != nil {
			return fmt.Errorf("%s ip=%d: %w", fn.Name, ip, err)
		}
		if stack < pop {
			return fmt.Errorf("%s ip=%d (%s): stack underflow", fn.Name, ip, inst.Op)
		}
		stack -= pop
		stack += push
		switch inst.Op {
		case OpConstFloat, OpConstStr:
			if inst.B < 0 || inst.B >= len(program.Constants) {
				return fmt.Errorf("%s ip=%d: constant index out of range", fn.Name, ip)
			}
		case OpLoadLocal, OpStoreLocal:
			if inst.A < 0 || inst.A >= fn.Locals {
				return fmt.Errorf("%s ip=%d: local index out of range", fn.Name, ip)
			}
		case OpJump, OpJumpIfFalse:
			if inst.A < 0 || inst.A > len(fn.Code) {
				return fmt.Errorf("%s ip=%d: jump target out of range", fn.Name, ip)
			}
		case OpCall:
			if inst.A < 0 || inst.A >= len(program.Functions) {
				return fmt.Errorf("%s ip=%d: call target out of range", fn.Name, ip)
			}
			if program.Functions[inst.A].Arity != inst.B {
				return fmt.Errorf("%s ip=%d: arity mismatch for %s", fn.Name, ip, program.Functions[inst.A].Name)
			}
		case OpGetField:
			if inst.B < 0 {
				return fmt.Errorf("%s ip=%d: invalid field index", fn.Name, ip)
			}
		}
	}
	return nil
}

func stackEffect(inst Instruction) (pop int, push int, err error) {
	switch inst.Op {
	case OpNoop:
		return 0, 0, nil
	case OpConstVoid, OpConstInt, OpConstFloat, OpConstBool, OpConstStr, OpLoadLocal:
		return 0, 1, nil
	case OpStoreLocal, OpPop:
		return 1, 0, nil
	case OpJump:
		return 0, 0, nil
	case OpJumpIfFalse:
		return 1, 0, nil
	case OpCall:
		return inst.B, 1, nil
	case OpReturn:
		return 1, 0, nil
	case OpIntAdd, OpIntSub, OpIntMul, OpIntDiv, OpIntMod,
		OpFloatAdd, OpFloatSub, OpFloatMul, OpFloatDiv,
		OpStrConcat, OpEq, OpNotEq, OpLt, OpLte, OpGt, OpGte, OpAnd, OpOr:
		return 2, 1, nil
	case OpNot, OpNeg, OpToStr, OpGetField:
		return 1, 1, nil
	case OpBlock:
		return 0, 0, nil
	case OpMakeStruct:
		return inst.B, 1, nil
	default:
		return 0, 0, fmt.Errorf("unsupported opcode %d", inst.Op)
	}
}
