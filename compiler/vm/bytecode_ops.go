package vm

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/air"
	vmcode "github.com/akonwi/ard/vm/bytecode"
)

func (vm *VM) execBytecodeStrOp(inst vmcode.Instruction, stack *[]Value) (Value, error) {
	args, target, targetIndex, err := methodArgsFromStack(stack, inst.B)
	if err != nil {
		return Value{}, err
	}
	if target.Kind != ValueStr {
		return Value{}, fmt.Errorf("string method target must be Str, got kind %d", target.Kind)
	}
	targetStr := target.Str
	var out Value
	switch inst.Op {
	case vmcode.OpStrAt:
		if len(args) != 1 || args[0].Kind != ValueInt {
			return Value{}, fmt.Errorf("string index must be Int")
		}
		runes := []rune(targetStr)
		if args[0].Int < 0 || args[0].Int >= len(runes) {
			return Value{}, fmt.Errorf("string index out of range")
		}
		out = Str(air.TypeID(inst.A), string(runes[args[0].Int]))
	case vmcode.OpStrSize:
		out = Int(air.TypeID(inst.A), len(targetStr))
	case vmcode.OpStrIsEmpty:
		out = Bool(air.TypeID(inst.A), targetStr == "")
	case vmcode.OpStrContains:
		arg0, err := stringArg(args, 0)
		if err != nil {
			return Value{}, err
		}
		out = Bool(air.TypeID(inst.A), strings.Contains(targetStr, arg0))
	case vmcode.OpStrReplace:
		arg0, arg1, err := stringArgs2(args)
		if err != nil {
			return Value{}, err
		}
		out = Str(air.TypeID(inst.A), strings.Replace(targetStr, arg0, arg1, 1))
	case vmcode.OpStrReplaceAll:
		arg0, arg1, err := stringArgs2(args)
		if err != nil {
			return Value{}, err
		}
		out = Str(air.TypeID(inst.A), strings.ReplaceAll(targetStr, arg0, arg1))
	case vmcode.OpStrSplit:
		arg0, err := stringArg(args, 0)
		if err != nil {
			return Value{}, err
		}
		parts := strings.Split(targetStr, arg0)
		items := make([]Value, len(parts))
		strType := vm.mustTypeID(air.TypeStr)
		for i, part := range parts {
			items[i] = Str(strType, part)
		}
		out = List(air.TypeID(inst.A), items)
	case vmcode.OpStrStartsWith:
		arg0, err := stringArg(args, 0)
		if err != nil {
			return Value{}, err
		}
		out = Bool(air.TypeID(inst.A), strings.HasPrefix(targetStr, arg0))
	case vmcode.OpStrTrim:
		out = Str(air.TypeID(inst.A), strings.Trim(targetStr, " "))
	default:
		return Value{}, fmt.Errorf("unsupported string opcode %s", inst.Op)
	}
	*stack = (*stack)[:targetIndex]
	return out, nil
}

func stringArg(args []Value, index int) (string, error) {
	if index < 0 || index >= len(args) || args[index].Kind != ValueStr {
		return "", fmt.Errorf("string method arg %d must be Str", index)
	}
	return args[index].Str, nil
}

func stringArgs2(args []Value) (string, string, error) {
	left, err := stringArg(args, 0)
	if err != nil {
		return "", "", err
	}
	right, err := stringArg(args, 1)
	if err != nil {
		return "", "", err
	}
	return left, right, nil
}

func (vm *VM) execBytecodeMaybeOp(inst vmcode.Instruction, stack *[]Value) (Value, error) {
	args, target, targetIndex, err := methodArgsFromStack(stack, inst.B)
	if err != nil {
		return Value{}, err
	}
	vm.recordRefAccess(refAccessMaybe)
	maybeValue, err := target.maybeValue()
	if err != nil {
		return Value{}, err
	}
	vm.recordMaybeAccess(maybeValue)
	var out Value
	switch inst.Op {
	case vmcode.OpMaybeExpect:
		if maybeValue.Some {
			out = maybeValue.Value
			break
		}
		message := "expected Maybe to contain a value"
		if len(args) > 0 {
			message = args[0].GoValueString()
		}
		return Value{}, fmt.Errorf("%s", message)
	case vmcode.OpMaybeIsNone:
		out = Bool(air.TypeID(inst.A), !maybeValue.Some)
	case vmcode.OpMaybeIsSome:
		out = Bool(air.TypeID(inst.A), maybeValue.Some)
	case vmcode.OpMaybeOr:
		if maybeValue.Some {
			out = maybeValue.Value
			break
		}
		if len(args) != 1 {
			return Value{}, fmt.Errorf("Maybe.or expects one fallback")
		}
		out = args[0]
	case vmcode.OpMaybeMap, vmcode.OpMaybeAndThen:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("Maybe closure method expects one function")
		}
		if !maybeValue.Some {
			vm.recordMaybeAlloc(false)
			out = Maybe(air.TypeID(inst.A), false, vm.zeroValue(vm.bytecodeMaybeElem(air.TypeID(inst.A))))
			break
		}
		mapped, err := vm.callClosure1(args[0], maybeValue.Value)
		if err != nil {
			return Value{}, err
		}
		if inst.Op == vmcode.OpMaybeAndThen {
			out = mapped
		} else {
			vm.recordMaybeAlloc(true)
			out = Maybe(air.TypeID(inst.A), true, mapped)
		}
	default:
		return Value{}, fmt.Errorf("unsupported maybe opcode %s", inst.Op)
	}
	*stack = (*stack)[:targetIndex]
	return out, nil
}

func (vm *VM) execBytecodeResultLocalOp(inst vmcode.Instruction, locals []Value) (Value, error) {
	if inst.B < 0 || inst.B >= len(locals) {
		return Value{}, fmt.Errorf("result local %d out of range", inst.B)
	}
	target := locals[inst.B]
	if target.Kind == ValueResultInt || target.Kind == ValueResultStr || target.Kind == ValueResultBool || target.Kind == ValueResultFloat {
		if inst.Op == vmcode.OpResultIsOkLocal {
			return Bool(air.TypeID(inst.A), true), nil
		}
		if inst.Op == vmcode.OpResultExpectLocal {
			if target.Kind == ValueResultInt {
				return Int(air.NoType, target.Int), nil
			}
			if target.Kind == ValueResultStr {
				return Str(air.NoType, target.Str), nil
			}
			if target.Kind == ValueResultBool {
				return Bool(air.NoType, target.Bool), nil
			}
			return Float(air.NoType, target.Float), nil
		}
		return Value{}, fmt.Errorf("expected Result error value")
	}
	if target.Kind != ValueResult {
		return Value{}, fmt.Errorf("expected result value, got kind %d", target.Kind)
	}
	vm.recordRefAccess(refAccessResult)
	var resultPayload Value
	if payload, ok := target.Ref.(*Value); ok && payload != nil {
		resultPayload = *payload
	} else {
		return Value{}, fmt.Errorf("result value has invalid payload %T", target.Ref)
	}
	if inst.Op == vmcode.OpResultIsOkLocal {
		return Bool(air.TypeID(inst.A), target.Bool), nil
	}
	if inst.Op == vmcode.OpResultExpectLocal {
		if !target.Bool {
			return Value{}, fmt.Errorf("expected Result to be ok")
		}
		return resultPayload, nil
	}
	if target.Bool {
		return Value{}, fmt.Errorf("expected Result error value")
	}
	return resultPayload, nil
}

func (vm *VM) execBytecodeResultOp(inst vmcode.Instruction, stack *[]Value) (Value, error) {
	args, target, targetIndex, err := methodArgsFromStack(stack, inst.B)
	if err != nil {
		return Value{}, err
	}
	if target.Kind == ValueResult {
		vm.recordRefAccess(refAccessResult)
	}
	resultOK, resultPayload, err := target.resultParts()
	if err != nil {
		return Value{}, err
	}
	var out Value
	switch inst.Op {
	case vmcode.OpResultExpect:
		if resultOK {
			out = resultPayload
			break
		}
		message := "expected Result to be ok"
		if len(args) > 0 {
			message = args[0].GoValueString()
		}
		return Value{}, fmt.Errorf("%s", message)
	case vmcode.OpResultErrValue:
		if resultOK {
			return Value{}, fmt.Errorf("expected Result error value")
		}
		out = resultPayload
	case vmcode.OpResultOr:
		if resultOK {
			out = resultPayload
			break
		}
		if len(args) != 1 {
			return Value{}, fmt.Errorf("Result.or expects one fallback")
		}
		out = args[0]
	case vmcode.OpResultIsOk:
		out = Bool(air.TypeID(inst.A), resultOK)
	case vmcode.OpResultIsErr:
		out = Bool(air.TypeID(inst.A), !resultOK)
	case vmcode.OpResultMap:
		if !resultOK {
			if vm.profile != nil {
				vm.profile.RecordValueAlloc(valueAllocResult)
			}
			out = Result(air.TypeID(inst.A), false, resultPayload)
			break
		}
		mapped, err := vm.callClosure1(args[0], resultPayload)
		if err != nil {
			return Value{}, err
		}
		if vm.profile != nil {
			vm.profile.RecordValueAlloc(valueAllocResult)
		}
		out = Result(air.TypeID(inst.A), true, mapped)
	case vmcode.OpResultMapErr:
		if resultOK {
			if vm.profile != nil {
				vm.profile.RecordValueAlloc(valueAllocResult)
			}
			out = Result(air.TypeID(inst.A), true, resultPayload)
			break
		}
		mapped, err := vm.callClosure1(args[0], resultPayload)
		if err != nil {
			return Value{}, err
		}
		if vm.profile != nil {
			vm.profile.RecordValueAlloc(valueAllocResult)
		}
		out = Result(air.TypeID(inst.A), false, mapped)
	case vmcode.OpResultAndThen:
		if !resultOK {
			if vm.profile != nil {
				vm.profile.RecordValueAlloc(valueAllocResult)
			}
			out = Result(air.TypeID(inst.A), false, resultPayload)
			break
		}
		out, err = vm.callClosure1(args[0], resultPayload)
		if err != nil {
			return Value{}, err
		}
	default:
		return Value{}, fmt.Errorf("unsupported result opcode %s", inst.Op)
	}
	*stack = (*stack)[:targetIndex]
	return out, nil
}

func (vm *VM) execBytecodeTryResult(inst vmcode.Instruction, pop func() (Value, error), locals []Value) (value Value, jump int, returned bool, err error) {
	target, err := pop()
	if err != nil {
		return Value{}, -1, false, err
	}
	if target.Kind == ValueResultInt {
		return Int(air.NoType, target.Int), -1, false, nil
	}
	if target.Kind == ValueResultStr {
		return Str(air.NoType, target.Str), -1, false, nil
	}
	if target.Kind == ValueResultBool {
		return Bool(air.NoType, target.Bool), -1, false, nil
	}
	if target.Kind == ValueResultFloat {
		return Float(air.NoType, target.Float), -1, false, nil
	}
	if target.Kind != ValueResult {
		return Value{}, -1, false, fmt.Errorf("expected result value, got kind %d", target.Kind)
	}
	vm.recordRefAccess(refAccessResult)
	var resultPayload Value
	if payload, ok := target.Ref.(*Value); ok && payload != nil {
		resultPayload = *payload
	} else {
		return Value{}, -1, false, fmt.Errorf("result value has invalid payload %T", target.Ref)
	}
	if target.Bool {
		return resultPayload, -1, false, nil
	}
	if inst.B >= 0 {
		if inst.C >= 0 && inst.C < len(locals) {
			locals[inst.C] = resultPayload
		}
		return Value{}, inst.B, false, nil
	}
	if vm.profile != nil {
		vm.profile.RecordValueAlloc(valueAllocResult)
	}
	return Result(air.TypeID(inst.A), false, resultPayload), -1, true, nil
}

func (vm *VM) execBytecodeTryMaybe(inst vmcode.Instruction, pop func() (Value, error), locals []Value) (value Value, jump int, returned bool, err error) {
	target, err := pop()
	if err != nil {
		return Value{}, -1, false, err
	}
	vm.recordRefAccess(refAccessMaybe)
	maybeValue, err := target.maybeValue()
	if err != nil {
		return Value{}, -1, false, err
	}
	vm.recordMaybeAccess(maybeValue)
	if maybeValue.Some {
		return maybeValue.Value, -1, false, nil
	}
	if inst.B >= 0 {
		return Value{}, inst.B, false, nil
	}
	vm.recordMaybeAlloc(false)
	return Maybe(air.TypeID(inst.A), false, vm.bytecodeZeroMaybeForReturn(air.TypeID(inst.A))), -1, true, nil
}

func (vm *VM) bytecodeZeroMaybeForReturn(returnType air.TypeID) Value {
	typeInfo, err := vm.typeInfo(returnType)
	if err != nil || typeInfo.Kind != air.TypeMaybe {
		return vm.zeroValue(vm.voidType)
	}
	return vm.zeroValue(typeInfo.Elem)
}
