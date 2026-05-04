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
		case OpIntAddConstLocal:
			if inst.B < 0 || inst.B >= fn.Locals {
				return fmt.Errorf("%s ip=%d: int local index out of range", fn.Name, ip)
			}
		case OpCallClosureLocal:
			if inst.B < 0 || inst.B >= fn.Locals {
				return fmt.Errorf("%s ip=%d: closure local index out of range", fn.Name, ip)
			}
		case OpResultExpectLocal, OpResultErrValueLocal, OpResultIsOkLocal:
			if inst.B < 0 || inst.B >= fn.Locals {
				return fmt.Errorf("%s ip=%d: result local index out of range", fn.Name, ip)
			}
		case OpUnionTagLocal, OpUnionValueLocal:
			if inst.B < 0 || inst.B >= fn.Locals {
				return fmt.Errorf("%s ip=%d: union local index out of range", fn.Name, ip)
			}
		case OpGetFieldLocal:
			if inst.B < 0 || inst.B >= fn.Locals {
				return fmt.Errorf("%s ip=%d: local index out of range", fn.Name, ip)
			}
		case OpListSizeLocal:
			if inst.B < 0 || inst.B >= fn.Locals {
				return fmt.Errorf("%s ip=%d: list local index out of range", fn.Name, ip)
			}
		case OpListAtLocal, OpListAtModLocal, OpListIndexLtLocal:
			if inst.B < 0 || inst.B >= fn.Locals || inst.C < 0 || inst.C >= fn.Locals {
				return fmt.Errorf("%s ip=%d: list/index local out of range", fn.Name, ip)
			}
		case OpListPushLocal, OpListPushLocalDrop:
			if inst.B < 0 || inst.B >= fn.Locals {
				return fmt.Errorf("%s ip=%d: list local index out of range", fn.Name, ip)
			}
		case OpMapSizeLocal:
			if inst.B < 0 || inst.B >= fn.Locals {
				return fmt.Errorf("%s ip=%d: map local index out of range", fn.Name, ip)
			}
		case OpMapIndexLtLocal, OpMapKeyAtLocal, OpMapValueAtLocal, OpMapGetLocal, OpMapGetLocalTryMaybe, OpMapGetOrConstIntLocal, OpMapSetLocal, OpMapSetLocalDrop, OpMapIncrementIntLocal, OpMapIncrementIntLocalDrop:
			if inst.B < 0 || inst.B >= fn.Locals || inst.C < 0 || inst.C >= fn.Locals {
				return fmt.Errorf("%s ip=%d: map/index local out of range", fn.Name, ip)
			}
			if inst.Op == OpMapGetLocalTryMaybe && inst.Imm >= 0 && inst.Imm > len(fn.Code) {
				return fmt.Errorf("%s ip=%d: try maybe jump target out of range", fn.Name, ip)
			}
		case OpMapGetOrConstIntLocalKey, OpMapSetLocalStackKeyDrop:
			if inst.B < 0 || inst.B >= fn.Locals {
				return fmt.Errorf("%s ip=%d: map local index out of range", fn.Name, ip)
			}
		case OpJump, OpJumpIfFalse:
			if inst.A < 0 || inst.A > len(fn.Code) {
				return fmt.Errorf("%s ip=%d: jump target out of range", fn.Name, ip)
			}
		case OpStoreIntAddConstLocalJump:
			if inst.A < 0 || inst.A > len(fn.Code) {
				return fmt.Errorf("%s ip=%d: jump target out of range", fn.Name, ip)
			}
			if inst.B < 0 || inst.B >= fn.Locals || inst.C < 0 || inst.C >= fn.Locals {
				return fmt.Errorf("%s ip=%d: int local index out of range", fn.Name, ip)
			}
		case OpJumpIfListIndexGeLocal, OpJumpIfMapIndexGeLocal:
			if inst.A < 0 || inst.A > len(fn.Code) {
				return fmt.Errorf("%s ip=%d: jump target out of range", fn.Name, ip)
			}
			if inst.B < 0 || inst.B >= fn.Locals || inst.C < 0 || inst.C >= fn.Locals {
				return fmt.Errorf("%s ip=%d: jump index/collection local out of range", fn.Name, ip)
			}
		case OpJumpIfIntGtLocal:
			if inst.A < 0 || inst.A > len(fn.Code) {
				return fmt.Errorf("%s ip=%d: jump target out of range", fn.Name, ip)
			}
			if inst.B < 0 || inst.B >= fn.Locals || inst.C < 0 || inst.C >= fn.Locals {
				return fmt.Errorf("%s ip=%d: int local index out of range", fn.Name, ip)
			}
		case OpJumpIfIntModConstNotEqConstLocal:
			if inst.A < 0 || inst.A > len(fn.Code) {
				return fmt.Errorf("%s ip=%d: jump target out of range", fn.Name, ip)
			}
			if inst.B < 0 || inst.B >= fn.Locals {
				return fmt.Errorf("%s ip=%d: int local index out of range", fn.Name, ip)
			}
		case OpCall:
			if inst.A < 0 || inst.A >= len(program.Functions) {
				return fmt.Errorf("%s ip=%d: call target out of range", fn.Name, ip)
			}
			if program.Functions[inst.A].Arity != inst.B {
				return fmt.Errorf("%s ip=%d: arity mismatch for %s", fn.Name, ip, program.Functions[inst.A].Name)
			}
		case OpCallExtern:
			if inst.A < 0 || inst.A >= len(program.Externs) {
				return fmt.Errorf("%s ip=%d: extern target out of range", fn.Name, ip)
			}
		case OpGetField, OpSetField:
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
	case OpConstVoid, OpConstInt, OpConstFloat, OpConstBool, OpConstStr, OpLoadLocal, OpIntAddConstLocal,
		OpListSizeLocal, OpListAtLocal, OpListAtModLocal, OpListIndexLtLocal,
		OpMapSizeLocal, OpMapIndexLtLocal, OpMapKeyAtLocal, OpMapValueAtLocal, OpMapGetLocal, OpMapGetLocalTryMaybe, OpMapGetOrConstIntLocal, OpMapGetOrConstIntLocalKey,
		OpUnionTagLocal, OpUnionValueLocal,
		OpResultExpectLocal, OpResultErrValueLocal, OpResultIsOkLocal:
		return 0, 1, nil
	case OpStoreLocal, OpPop:
		return 1, 0, nil
	case OpJump:
		return 0, 0, nil
	case OpJumpIfFalse:
		return 1, 0, nil
	case OpJumpIfListIndexGeLocal, OpJumpIfMapIndexGeLocal, OpJumpIfIntGtLocal, OpJumpIfIntModConstNotEqConstLocal, OpStoreIntAddConstLocalJump:
		return 0, 0, nil
	case OpCall, OpCallExtern:
		return inst.B, 1, nil
	case OpCallTrait:
		return inst.Imm + 1, 1, nil
	case OpCallClosure:
		return inst.B + 1, 1, nil
	case OpCallClosureLocal:
		return inst.C, 1, nil
	case OpReturn:
		return 1, 0, nil
	case OpIntAdd, OpIntSub, OpIntMul, OpIntDiv, OpIntMod,
		OpFloatAdd, OpFloatSub, OpFloatMul, OpFloatDiv,
		OpStrConcat, OpEq, OpNotEq, OpLt, OpLte, OpGt, OpGte, OpAnd, OpOr:
		return 2, 1, nil
	case OpNot, OpNeg, OpToStr, OpCopy, OpGetField, OpTraitUpcast, OpUnionWrap, OpUnionTag, OpUnionValue,
		OpListSize, OpMapKeys, OpMapSize, OpStrSize, OpStrIsEmpty, OpStrTrim,
		OpMakeMaybeSome, OpMaybeExpect, OpMaybeIsNone, OpMaybeIsSome,
		OpMakeResultOk, OpMakeResultErr, OpResultExpect, OpResultErrValue, OpResultIsOk, OpResultIsErr,
		OpTryResult, OpTryMaybe, OpToDynamic, OpPanic, OpFiberGet, OpFiberJoin:
		return 1, 1, nil
	case OpGetFieldLocal:
		return 0, 1, nil
	case OpSetField:
		return 2, 0, nil
	case OpListPushLocal:
		return 1, 1, nil
	case OpListPushLocalDrop:
		return 1, 0, nil
	case OpMapSetLocal:
		return 1, 1, nil
	case OpMapSetLocalDrop:
		return 1, 0, nil
	case OpMapSetLocalStackKeyDrop:
		return 2, 0, nil
	case OpMapIncrementIntLocal:
		return 0, 1, nil
	case OpMapIncrementIntLocalDrop:
		return 0, 0, nil
	case OpBlock:
		return 0, 0, nil
	case OpMakeList, OpMakeStruct, OpMakeClosure:
		return inst.B, 1, nil
	case OpMakeMap:
		return inst.B * 2, 1, nil
	case OpListAt, OpListPrepend, OpListPush, OpMapGet, OpMapDrop, OpMapHas, OpMapKeyAt, OpMapValueAt,
		OpStrAt, OpStrContains, OpStrSplit, OpStrStartsWith, OpMaybeOr, OpMaybeMap,
		OpMaybeAndThen, OpResultOr, OpResultMap, OpResultMapErr, OpResultAndThen:
		return 2, 1, nil
	case OpSpawnFiber:
		return inst.B, 1, nil
	case OpListSet, OpListSwap, OpMapSet, OpStrReplace, OpStrReplaceAll:
		return 3, 1, nil
	case OpListSort:
		return 2, 1, nil
	case OpEnumVariant, OpMakeMaybeNone:
		return 0, 1, nil
	default:
		return 0, 0, fmt.Errorf("unsupported opcode %d", inst.Op)
	}
}
