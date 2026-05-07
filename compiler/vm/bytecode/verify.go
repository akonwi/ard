package bytecode

import "fmt"

const unseenStackHeight = -1

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
	for ip, inst := range fn.Code {
		if err := validateInstruction(program, fn, ip, inst); err != nil {
			return err
		}
	}
	if len(fn.Code) == 0 {
		return fmt.Errorf("%s: function has no code", fn.Name)
	}
	return verifyControlFlow(fn)
}

func validateInstruction(program *Program, fn *Function, ip int, inst Instruction) error {
	localIndexError := func(message string) error {
		return fmt.Errorf("%s ip=%d: %s", fn.Name, ip, message)
	}
	validateLocal := func(index int, message string) error {
		if index < 0 || index >= fn.Locals {
			return localIndexError(message)
		}
		return nil
	}
	validateCodeTarget := func(target int, message string) error {
		if target < 0 || target >= len(fn.Code) {
			return localIndexError(message)
		}
		return nil
	}

	switch inst.Op {
	case OpConstFloat, OpConstStr:
		if inst.B < 0 || inst.B >= len(program.Constants) {
			return localIndexError("constant index out of range")
		}
	case OpLoadLocal, OpStoreLocal:
		if err := validateLocal(inst.A, "local index out of range"); err != nil {
			return err
		}
	case OpIntAddConstLocal:
		if err := validateLocal(inst.B, "int local index out of range"); err != nil {
			return err
		}
	case OpCallClosureLocal:
		if err := validateLocal(inst.B, "closure local index out of range"); err != nil {
			return err
		}
	case OpResultExpectLocal, OpResultErrValueLocal, OpResultIsOkLocal:
		if err := validateLocal(inst.B, "result local index out of range"); err != nil {
			return err
		}
	case OpUnionTagLocal, OpUnionValueLocal:
		if err := validateLocal(inst.B, "union local index out of range"); err != nil {
			return err
		}
	case OpGetFieldLocal:
		if err := validateLocal(inst.B, "local index out of range"); err != nil {
			return err
		}
	case OpListSizeLocal:
		if err := validateLocal(inst.B, "list local index out of range"); err != nil {
			return err
		}
	case OpListAtLocal, OpListAtModLocal, OpListIndexLtLocal:
		if err := validateLocal(inst.B, "list/index local out of range"); err != nil {
			return err
		}
		if err := validateLocal(inst.C, "list/index local out of range"); err != nil {
			return err
		}
	case OpListPushLocal, OpListPushLocalDrop:
		if err := validateLocal(inst.B, "list local index out of range"); err != nil {
			return err
		}
	case OpMapSizeLocal:
		if err := validateLocal(inst.B, "map local index out of range"); err != nil {
			return err
		}
	case OpMapIndexLtLocal, OpMapKeyAtLocal, OpMapValueAtLocal, OpMapGetLocal, OpMapGetLocalTryMaybe, OpMapGetOrConstIntLocal, OpMapSetLocal, OpMapSetLocalDrop, OpMapIncrementIntLocal, OpMapIncrementIntLocalDrop:
		if err := validateLocal(inst.B, "map/index local out of range"); err != nil {
			return err
		}
		if err := validateLocal(inst.C, "map/index local out of range"); err != nil {
			return err
		}
		if inst.Op == OpMapGetLocalTryMaybe {
			if err := validateOptionalCodeTarget(inst.Imm, len(fn.Code), localIndexError, "try maybe jump target out of range"); err != nil {
				return err
			}
		}
	case OpMapGetOrConstIntLocalKey, OpMapSetLocalStackKeyDrop:
		if err := validateLocal(inst.B, "map local index out of range"); err != nil {
			return err
		}
	case OpJump, OpJumpIfFalse:
		if err := validateCodeTarget(inst.A, "jump target out of range"); err != nil {
			return err
		}
	case OpStoreIntAddConstLocalJump:
		if err := validateCodeTarget(inst.A, "jump target out of range"); err != nil {
			return err
		}
		if err := validateLocal(inst.B, "int local index out of range"); err != nil {
			return err
		}
		if err := validateLocal(inst.C, "int local index out of range"); err != nil {
			return err
		}
	case OpJumpIfListIndexGeLocal, OpJumpIfMapIndexGeLocal:
		if err := validateCodeTarget(inst.A, "jump target out of range"); err != nil {
			return err
		}
		if err := validateLocal(inst.B, "jump index/collection local out of range"); err != nil {
			return err
		}
		if err := validateLocal(inst.C, "jump index/collection local out of range"); err != nil {
			return err
		}
	case OpJumpIfIntGtLocal:
		if err := validateCodeTarget(inst.A, "jump target out of range"); err != nil {
			return err
		}
		if err := validateLocal(inst.B, "int local index out of range"); err != nil {
			return err
		}
		if err := validateLocal(inst.C, "int local index out of range"); err != nil {
			return err
		}
	case OpJumpIfIntModConstNotEqConstLocal:
		if err := validateCodeTarget(inst.A, "jump target out of range"); err != nil {
			return err
		}
		if err := validateLocal(inst.B, "int local index out of range"); err != nil {
			return err
		}
	case OpTryResult, OpTryMaybe:
		if err := validateOptionalCodeTarget(inst.B, len(fn.Code), localIndexError, "try jump target out of range"); err != nil {
			return err
		}
		if inst.C >= 0 {
			if err := validateLocal(inst.C, "try catch local index out of range"); err != nil {
				return err
			}
		}
	case OpCall:
		if inst.A < 0 || inst.A >= len(program.Functions) {
			return localIndexError("call target out of range")
		}
		if program.Functions[inst.A].Arity != inst.B {
			return fmt.Errorf("%s ip=%d: arity mismatch for %s", fn.Name, ip, program.Functions[inst.A].Name)
		}
	case OpCallExtern:
		if inst.A < 0 || inst.A >= len(program.Externs) {
			return localIndexError("extern target out of range")
		}
	case OpGetField, OpSetField:
		if inst.B < 0 {
			return localIndexError("invalid field index")
		}
	}
	if _, _, err := stackEffect(inst); err != nil {
		return fmt.Errorf("%s ip=%d: %w", fn.Name, ip, err)
	}
	return nil
}

func validateOptionalCodeTarget(target int, codeLen int, errf func(string) error, message string) error {
	if target >= 0 && target >= codeLen {
		return errf(message)
	}
	return nil
}

type verifyState struct {
	ip    int
	stack int
}

type verifyEdge struct {
	next  int
	stack int
}

func verifyControlFlow(fn *Function) error {
	entryStackHeights := make([]int, len(fn.Code))
	for i := range entryStackHeights {
		entryStackHeights[i] = unseenStackHeight
	}
	worklist := []verifyState{{ip: 0, stack: 0}}
	for len(worklist) > 0 {
		state := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		seenStack := entryStackHeights[state.ip]
		if seenStack != unseenStackHeight {
			if seenStack != state.stack {
				return fmt.Errorf("%s ip=%d: stack height mismatch, got %d and %d", fn.Name, state.ip, seenStack, state.stack)
			}
			continue
		}
		entryStackHeights[state.ip] = state.stack
		successors, err := verifyInstructionFlow(fn, state.ip, state.stack)
		if err != nil {
			return err
		}
		for _, successor := range successors {
			if successor.next < 0 || successor.next >= len(fn.Code) {
				return fmt.Errorf("%s ip=%d: reachable path can fall off end without terminating", fn.Name, state.ip)
			}
			worklist = append(worklist, verifyState{ip: successor.next, stack: successor.stack})
		}
	}
	return nil
}

func verifyInstructionFlow(fn *Function, ip int, stack int) ([]verifyEdge, error) {
	inst := fn.Code[ip]
	nextIP := ip + 1
	requireStack := func(pop int) error {
		if stack < pop {
			return fmt.Errorf("%s ip=%d (%s): stack underflow", fn.Name, ip, inst.Op)
		}
		return nil
	}
	advance := func(nextStack int) ([]verifyEdge, error) {
		return []verifyEdge{{next: nextIP, stack: nextStack}}, nil
	}
	branch := func(target int, targetStack int) verifyEdge {
		return verifyEdge{next: target, stack: targetStack}
	}

	switch inst.Op {
	case OpJump:
		return []verifyEdge{branch(inst.A, stack)}, nil
	case OpJumpIfFalse:
		if err := requireStack(1); err != nil {
			return nil, err
		}
		nextStack := stack - 1
		return []verifyEdge{branch(inst.A, nextStack), branch(nextIP, nextStack)}, nil
	case OpJumpIfListIndexGeLocal, OpJumpIfMapIndexGeLocal, OpJumpIfIntGtLocal, OpJumpIfIntModConstNotEqConstLocal:
		return []verifyEdge{branch(inst.A, stack), branch(nextIP, stack)}, nil
	case OpStoreIntAddConstLocalJump:
		return []verifyEdge{branch(inst.A, stack)}, nil
	case OpTryResult, OpTryMaybe:
		if err := requireStack(1); err != nil {
			return nil, err
		}
		successors := []verifyEdge{branch(nextIP, stack)}
		if inst.B >= 0 {
			successors = append(successors, branch(inst.B, stack-1))
		}
		return successors, nil
	case OpMapGetLocalTryMaybe:
		successors := []verifyEdge{branch(nextIP, stack+1)}
		if inst.Imm >= 0 {
			successors = append(successors, branch(inst.Imm, stack))
		}
		return successors, nil
	case OpReturn, OpPanic:
		if err := requireStack(1); err != nil {
			return nil, err
		}
		return nil, nil
	default:
		pop, push, err := stackEffect(inst)
		if err != nil {
			return nil, fmt.Errorf("%s ip=%d: %w", fn.Name, ip, err)
		}
		if err := requireStack(pop); err != nil {
			return nil, err
		}
		return advance(stack - pop + push)
	}
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
		OpMakeMaybeSome,
		OpMakeResultOk, OpMakeResultErr,
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
	case OpStrAt, OpStrSize, OpStrIsEmpty, OpStrContains, OpStrSplit, OpStrStartsWith, OpStrTrim,
		OpMaybeExpect, OpMaybeIsNone, OpMaybeIsSome, OpMaybeOr, OpMaybeMap, OpMaybeAndThen,
		OpResultExpect, OpResultErrValue, OpResultOr, OpResultIsOk, OpResultIsErr, OpResultMap, OpResultMapErr, OpResultAndThen,
		OpListAt, OpListPrepend, OpListPush, OpListSize, OpListSort,
		OpMapKeys, OpMapSize, OpMapGet, OpMapDrop, OpMapHas, OpMapKeyAt, OpMapValueAt:
		return inst.B + 1, 1, nil
	case OpSpawnFiber:
		return inst.B, 1, nil
	case OpListSet, OpListSwap, OpMapSet, OpStrReplace, OpStrReplaceAll:
		return 3, 1, nil
	case OpEnumVariant, OpMakeMaybeNone:
		return 0, 1, nil
	default:
		return 0, 0, fmt.Errorf("unsupported opcode %d", inst.Op)
	}
}
