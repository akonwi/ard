package vm_next

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/air"
	vmcode "github.com/akonwi/ard/vm_next/bytecode"
)

func (vm *VM) execBytecodeStrOp(inst vmcode.Instruction, pop func() (Value, error)) (Value, error) {
	args, target, err := popMethodArgs(pop, inst.B)
	if err != nil {
		return Value{}, err
	}
	if target.Kind != ValueStr {
		return Value{}, fmt.Errorf("string method target must be Str, got kind %d", target.Kind)
	}
	targetStr := target.Str
	strArgs := make([]string, len(args))
	for i, arg := range args {
		if inst.Op == vmcode.OpStrAt && i == 0 {
			continue
		}
		if arg.Kind != ValueStr {
			return Value{}, fmt.Errorf("string method arg %d must be Str, got kind %d", i, arg.Kind)
		}
		strArgs[i] = arg.Str
	}
	switch inst.Op {
	case vmcode.OpStrAt:
		if len(args) != 1 || args[0].Kind != ValueInt {
			return Value{}, fmt.Errorf("string index must be Int")
		}
		runes := []rune(targetStr)
		if args[0].Int < 0 || args[0].Int >= len(runes) {
			return Value{}, fmt.Errorf("string index out of range")
		}
		return Str(air.TypeID(inst.A), string(runes[args[0].Int])), nil
	case vmcode.OpStrSize:
		return Int(air.TypeID(inst.A), len(targetStr)), nil
	case vmcode.OpStrIsEmpty:
		return Bool(air.TypeID(inst.A), targetStr == ""), nil
	case vmcode.OpStrContains:
		return Bool(air.TypeID(inst.A), strings.Contains(targetStr, strArgs[0])), nil
	case vmcode.OpStrReplace:
		return Str(air.TypeID(inst.A), strings.Replace(targetStr, strArgs[0], strArgs[1], 1)), nil
	case vmcode.OpStrReplaceAll:
		return Str(air.TypeID(inst.A), strings.ReplaceAll(targetStr, strArgs[0], strArgs[1])), nil
	case vmcode.OpStrSplit:
		parts := strings.Split(targetStr, strArgs[0])
		items := make([]Value, len(parts))
		strType := vm.mustTypeID(air.TypeStr)
		for i, part := range parts {
			items[i] = Str(strType, part)
		}
		return List(air.TypeID(inst.A), items), nil
	case vmcode.OpStrStartsWith:
		return Bool(air.TypeID(inst.A), strings.HasPrefix(targetStr, strArgs[0])), nil
	case vmcode.OpStrTrim:
		return Str(air.TypeID(inst.A), strings.Trim(targetStr, " ")), nil
	default:
		return Value{}, fmt.Errorf("unsupported string opcode %s", inst.Op)
	}
}

func (vm *VM) execBytecodeMaybeOp(inst vmcode.Instruction, pop func() (Value, error)) (Value, error) {
	args, target, err := popMethodArgs(pop, inst.B)
	if err != nil {
		return Value{}, err
	}
	maybeValue, err := target.maybeValue()
	if err != nil {
		return Value{}, err
	}
	switch inst.Op {
	case vmcode.OpMaybeExpect:
		if maybeValue.Some {
			return maybeValue.Value, nil
		}
		message := "expected Maybe to contain a value"
		if len(args) > 0 {
			message = args[0].GoValueString()
		}
		return Value{}, fmt.Errorf("%s", message)
	case vmcode.OpMaybeIsNone:
		return Bool(air.TypeID(inst.A), !maybeValue.Some), nil
	case vmcode.OpMaybeIsSome:
		return Bool(air.TypeID(inst.A), maybeValue.Some), nil
	case vmcode.OpMaybeOr:
		if maybeValue.Some {
			return maybeValue.Value, nil
		}
		if len(args) != 1 {
			return Value{}, fmt.Errorf("Maybe.or expects one fallback")
		}
		return args[0], nil
	case vmcode.OpMaybeMap, vmcode.OpMaybeAndThen:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("Maybe closure method expects one function")
		}
		if !maybeValue.Some {
			return Maybe(air.TypeID(inst.A), false, vm.zeroValue(vm.bytecodeMaybeElem(air.TypeID(inst.A)))), nil
		}
		mapped, err := vm.callClosure(args[0], []Value{maybeValue.Value})
		if err != nil {
			return Value{}, err
		}
		if inst.Op == vmcode.OpMaybeAndThen {
			return mapped, nil
		}
		return Maybe(air.TypeID(inst.A), true, mapped), nil
	default:
		return Value{}, fmt.Errorf("unsupported maybe opcode %s", inst.Op)
	}
}

func (vm *VM) execBytecodeResultOp(inst vmcode.Instruction, pop func() (Value, error)) (Value, error) {
	args, target, err := popMethodArgs(pop, inst.B)
	if err != nil {
		return Value{}, err
	}
	resultValue, err := target.resultValue()
	if err != nil {
		return Value{}, err
	}
	switch inst.Op {
	case vmcode.OpResultExpect:
		if resultValue.Ok {
			return resultValue.Value, nil
		}
		message := "expected Result to be ok"
		if len(args) > 0 {
			message = args[0].GoValueString()
		}
		return Value{}, fmt.Errorf("%s", message)
	case vmcode.OpResultErrValue:
		if resultValue.Ok {
			return Value{}, fmt.Errorf("expected Result error value")
		}
		return resultValue.Value, nil
	case vmcode.OpResultOr:
		if resultValue.Ok {
			return resultValue.Value, nil
		}
		if len(args) != 1 {
			return Value{}, fmt.Errorf("Result.or expects one fallback")
		}
		return args[0], nil
	case vmcode.OpResultIsOk:
		return Bool(air.TypeID(inst.A), resultValue.Ok), nil
	case vmcode.OpResultIsErr:
		return Bool(air.TypeID(inst.A), !resultValue.Ok), nil
	case vmcode.OpResultMap:
		if !resultValue.Ok {
			return Result(air.TypeID(inst.A), false, resultValue.Value), nil
		}
		mapped, err := vm.callClosure(args[0], []Value{resultValue.Value})
		if err != nil {
			return Value{}, err
		}
		return Result(air.TypeID(inst.A), true, mapped), nil
	case vmcode.OpResultMapErr:
		if resultValue.Ok {
			return Result(air.TypeID(inst.A), true, resultValue.Value), nil
		}
		mapped, err := vm.callClosure(args[0], []Value{resultValue.Value})
		if err != nil {
			return Value{}, err
		}
		return Result(air.TypeID(inst.A), false, mapped), nil
	case vmcode.OpResultAndThen:
		if !resultValue.Ok {
			return Result(air.TypeID(inst.A), false, resultValue.Value), nil
		}
		return vm.callClosure(args[0], []Value{resultValue.Value})
	default:
		return Value{}, fmt.Errorf("unsupported result opcode %s", inst.Op)
	}
}

func (vm *VM) execBytecodeTryResult(inst vmcode.Instruction, pop func() (Value, error), locals []Value) (value Value, jump int, returned bool, err error) {
	target, err := pop()
	if err != nil {
		return Value{}, -1, false, err
	}
	resultValue, err := target.resultValue()
	if err != nil {
		return Value{}, -1, false, err
	}
	if resultValue.Ok {
		return resultValue.Value, -1, false, nil
	}
	if inst.B >= 0 {
		if inst.C >= 0 && inst.C < len(locals) {
			locals[inst.C] = resultValue.Value
		}
		return Value{}, inst.B, false, nil
	}
	return Result(air.TypeID(inst.A), false, resultValue.Value), -1, true, nil
}

func (vm *VM) execBytecodeTryMaybe(inst vmcode.Instruction, pop func() (Value, error), locals []Value) (value Value, jump int, returned bool, err error) {
	target, err := pop()
	if err != nil {
		return Value{}, -1, false, err
	}
	maybeValue, err := target.maybeValue()
	if err != nil {
		return Value{}, -1, false, err
	}
	if maybeValue.Some {
		return maybeValue.Value, -1, false, nil
	}
	if inst.B >= 0 {
		return Value{}, inst.B, false, nil
	}
	return Maybe(air.TypeID(inst.A), false, vm.bytecodeZeroMaybeForReturn(air.TypeID(inst.A))), -1, true, nil
}

func (vm *VM) bytecodeZeroMaybeForReturn(returnType air.TypeID) Value {
	typeInfo, err := vm.typeInfo(returnType)
	if err != nil || typeInfo.Kind != air.TypeMaybe {
		return vm.zeroValue(vm.mustTypeID(air.TypeVoid))
	}
	return vm.zeroValue(typeInfo.Elem)
}
