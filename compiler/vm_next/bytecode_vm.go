package vm_next

import (
	"fmt"
	"strconv"

	"github.com/akonwi/ard/air"
	vmcode "github.com/akonwi/ard/vm_next/bytecode"
)

func NewWithBytecode(program *air.Program, externs HostFunctionRegistry) (*VM, error) {
	vm, err := NewWithExterns(program, externs)
	if err != nil {
		return nil, err
	}
	code, err := vmcode.Lower(program)
	if err != nil {
		return nil, err
	}
	if err := vmcode.Verify(code); err != nil {
		return nil, err
	}
	vm.bytecode = code
	return vm, nil
}

func (vm *VM) runBytecode(id air.FunctionID, args []Value) (Value, error) {
	if vm.bytecode == nil {
		return vm.call(id, args)
	}
	fn, ok := vm.bytecode.Function(id)
	if !ok {
		return Value{}, fmt.Errorf("invalid bytecode function id %d", id)
	}
	if len(args) != fn.Arity {
		return Value{}, fmt.Errorf("%s expects %d args, got %d", fn.Name, fn.Arity, len(args))
	}
	locals := make([]Value, fn.Locals)
	copy(locals, args)
	return vm.runBytecodeWithLocals(id, locals)
}

func (vm *VM) runBytecodeWithLocals(id air.FunctionID, locals []Value) (Value, error) {
	fn, ok := vm.bytecode.Function(id)
	if !ok {
		return Value{}, fmt.Errorf("invalid bytecode function id %d", id)
	}
	stack := make([]Value, 0, len(fn.Code))
	pop := func() (Value, error) {
		if len(stack) == 0 {
			return Value{}, fmt.Errorf("%s: stack underflow", fn.Name)
		}
		value := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return value, nil
	}
	push := func(value Value) {
		stack = append(stack, value)
	}

	for ip := 0; ip < len(fn.Code); {
		inst := fn.Code[ip]
		ip++
		switch inst.Op {
		case vmcode.OpNoop, vmcode.OpBlock:
			continue
		case vmcode.OpConstVoid:
			push(Void(air.TypeID(inst.A)))
		case vmcode.OpConstInt:
			push(Int(air.TypeID(inst.A), inst.Imm))
		case vmcode.OpConstFloat:
			constant, err := vm.bytecodeConstant(inst.B, vmcode.ConstFloat)
			if err != nil {
				return Value{}, err
			}
			push(Float(air.TypeID(inst.A), constant.Float))
		case vmcode.OpConstBool:
			push(Bool(air.TypeID(inst.A), inst.Imm != 0))
		case vmcode.OpConstStr:
			constant, err := vm.bytecodeConstant(inst.B, vmcode.ConstStr)
			if err != nil {
				return Value{}, err
			}
			push(Str(air.TypeID(inst.A), constant.Str))
		case vmcode.OpLoadLocal:
			if inst.A < 0 || inst.A >= len(locals) {
				return Value{}, fmt.Errorf("%s: local %d out of range", fn.Name, inst.A)
			}
			push(locals[inst.A])
		case vmcode.OpStoreLocal:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			if inst.A < 0 || inst.A >= len(locals) {
				return Value{}, fmt.Errorf("%s: local %d out of range", fn.Name, inst.A)
			}
			locals[inst.A] = value
		case vmcode.OpPop:
			if _, err := pop(); err != nil {
				return Value{}, err
			}
		case vmcode.OpJump:
			ip = inst.A
		case vmcode.OpJumpIfFalse:
			condition, err := pop()
			if err != nil {
				return Value{}, err
			}
			if condition.Kind != ValueBool {
				return Value{}, fmt.Errorf("jump condition must be Bool, got kind %d", condition.Kind)
			}
			if !condition.Bool {
				ip = inst.A
			}
		case vmcode.OpCall:
			callArgs, err := popArgs(pop, inst.B)
			if err != nil {
				return Value{}, err
			}
			result, err := vm.runBytecode(air.FunctionID(inst.A), callArgs)
			if err != nil {
				return Value{}, err
			}
			push(result)
		case vmcode.OpCallExtern:
			callArgs, err := popArgs(pop, inst.B)
			if err != nil {
				return Value{}, err
			}
			result, err := vm.callExtern(air.ExternID(inst.A), callArgs)
			if err != nil {
				return Value{}, err
			}
			push(result)
		case vmcode.OpMakeClosure:
			captures, err := popArgs(pop, inst.B)
			if err != nil {
				return Value{}, err
			}
			push(Closure(air.TypeID(inst.A), air.FunctionID(inst.C), captures))
		case vmcode.OpCallClosure:
			callArgs, err := popArgs(pop, inst.B)
			if err != nil {
				return Value{}, err
			}
			target, err := pop()
			if err != nil {
				return Value{}, err
			}
			result, err := vm.callClosure(target, callArgs)
			if err != nil {
				return Value{}, err
			}
			push(result)
		case vmcode.OpSpawnFiber:
			fiber := &FiberValue{Type: air.TypeID(inst.A), Done: make(chan struct{})}
			if inst.B > 0 {
				target, err := pop()
				if err != nil {
					return Value{}, err
				}
				go func() {
					defer close(fiber.Done)
					fiber.Result, fiber.Err = vm.callClosure(target, nil)
				}()
			} else {
				go func() {
					defer close(fiber.Done)
					fiber.Result, fiber.Err = vm.runBytecode(air.FunctionID(inst.C), nil)
				}()
			}
			push(Fiber(air.TypeID(inst.A), fiber))
		case vmcode.OpFiberGet:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			fiber, err := value.fiberValue()
			if err != nil {
				return Value{}, err
			}
			<-fiber.Done
			if fiber.Err != nil {
				return Value{}, fiber.Err
			}
			push(fiber.Result)
		case vmcode.OpFiberJoin:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			fiber, err := value.fiberValue()
			if err != nil {
				return Value{}, err
			}
			<-fiber.Done
			if fiber.Err != nil {
				return Value{}, fiber.Err
			}
			push(vm.zeroValue(air.TypeID(inst.A)))
		case vmcode.OpReturn:
			return pop()
		case vmcode.OpCopy:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			push(copyValue(value))
		case vmcode.OpTraitUpcast:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			push(TraitObject(air.TypeID(inst.A), air.TraitID(inst.B), air.ImplID(inst.C), value))
		case vmcode.OpCallTrait:
			callArgs, err := popArgs(pop, inst.Imm)
			if err != nil {
				return Value{}, err
			}
			subject, err := pop()
			if err != nil {
				return Value{}, err
			}
			traitObject, err := subject.traitObjectValue()
			if err != nil {
				return Value{}, err
			}
			if traitObject.Trait != air.TraitID(inst.B) {
				return Value{}, fmt.Errorf("trait object has trait %d, call expects %d", traitObject.Trait, inst.B)
			}
			if traitObject.Impl < 0 || int(traitObject.Impl) >= len(vm.program.Impls) {
				return Value{}, fmt.Errorf("invalid trait object impl %d", traitObject.Impl)
			}
			impl := vm.program.Impls[traitObject.Impl]
			if inst.C < 0 || inst.C >= len(impl.Methods) {
				return Value{}, fmt.Errorf("invalid trait method index %d", inst.C)
			}
			argsWithReceiver := make([]Value, 0, len(callArgs)+1)
			argsWithReceiver = append(argsWithReceiver, traitObject.Value)
			argsWithReceiver = append(argsWithReceiver, callArgs...)
			result, err := vm.runBytecode(impl.Methods[inst.C], argsWithReceiver)
			if err != nil {
				return Value{}, err
			}
			push(result)
		case vmcode.OpUnionWrap:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			push(Union(air.TypeID(inst.A), uint32(inst.Imm), value))
		case vmcode.OpUnionTag:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			unionValue, err := value.unionValue()
			if err != nil {
				return Value{}, err
			}
			push(Int(air.TypeID(inst.A), int(unionValue.Tag)))
		case vmcode.OpUnionValue:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			unionValue, err := value.unionValue()
			if err != nil {
				return Value{}, err
			}
			push(unionValue.Value)
		case vmcode.OpIntAdd, vmcode.OpIntSub, vmcode.OpIntMul, vmcode.OpIntDiv, vmcode.OpIntMod:
			left, right, err := popBinary(pop)
			if err != nil {
				return Value{}, err
			}
			push(evalBytecodeIntBinary(inst, left, right))
		case vmcode.OpFloatAdd, vmcode.OpFloatSub, vmcode.OpFloatMul, vmcode.OpFloatDiv:
			left, right, err := popBinary(pop)
			if err != nil {
				return Value{}, err
			}
			push(evalBytecodeFloatBinary(inst, left, right))
		case vmcode.OpStrConcat:
			left, right, err := popBinary(pop)
			if err != nil {
				return Value{}, err
			}
			push(Str(air.TypeID(inst.A), left.Str+right.Str))
		case vmcode.OpEq, vmcode.OpNotEq:
			left, right, err := popBinary(pop)
			if err != nil {
				return Value{}, err
			}
			equal := valuesEqual(left, right)
			if inst.Op == vmcode.OpNotEq {
				equal = !equal
			}
			push(Bool(air.TypeID(inst.A), equal))
		case vmcode.OpLt, vmcode.OpLte, vmcode.OpGt, vmcode.OpGte:
			left, right, err := popBinary(pop)
			if err != nil {
				return Value{}, err
			}
			result, err := evalBytecodeComparison(inst.Op, left, right)
			if err != nil {
				return Value{}, err
			}
			push(Bool(air.TypeID(inst.A), result))
		case vmcode.OpAnd, vmcode.OpOr:
			left, right, err := popBinary(pop)
			if err != nil {
				return Value{}, err
			}
			if inst.Op == vmcode.OpAnd {
				push(Bool(air.TypeID(inst.A), left.Bool && right.Bool))
			} else {
				push(Bool(air.TypeID(inst.A), left.Bool || right.Bool))
			}
		case vmcode.OpNot:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			push(Bool(air.TypeID(inst.A), !value.Bool))
		case vmcode.OpNeg:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			switch value.Kind {
			case ValueInt:
				push(Int(air.TypeID(inst.A), -value.Int))
			case ValueFloat:
				push(Float(air.TypeID(inst.A), -value.Float))
			default:
				return Value{}, fmt.Errorf("cannot negate value kind %d", value.Kind)
			}
		case vmcode.OpToStr:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			out, err := bytecodeValueToStr(air.TypeID(inst.A), value)
			if err != nil {
				return Value{}, err
			}
			push(out)
		case vmcode.OpEnumVariant:
			push(Enum(air.TypeID(inst.A), inst.Imm))
		case vmcode.OpStrAt, vmcode.OpStrSize, vmcode.OpStrIsEmpty, vmcode.OpStrContains, vmcode.OpStrReplace, vmcode.OpStrReplaceAll, vmcode.OpStrSplit, vmcode.OpStrStartsWith, vmcode.OpStrTrim:
			value, err := vm.execBytecodeStrOp(inst, pop)
			if err != nil {
				return Value{}, err
			}
			push(value)
		case vmcode.OpMakeList:
			items, err := popArgs(pop, inst.B)
			if err != nil {
				return Value{}, err
			}
			push(List(air.TypeID(inst.A), items))
		case vmcode.OpListAt, vmcode.OpListPrepend, vmcode.OpListPush, vmcode.OpListSet, vmcode.OpListSize, vmcode.OpListSort, vmcode.OpListSwap:
			value, err := vm.execBytecodeListOp(inst, pop)
			if err != nil {
				return Value{}, err
			}
			push(value)
		case vmcode.OpMakeMap:
			entries := make([]MapEntryValue, inst.B)
			for i := inst.B - 1; i >= 0; i-- {
				value, err := pop()
				if err != nil {
					return Value{}, err
				}
				key, err := pop()
				if err != nil {
					return Value{}, err
				}
				entries[i] = MapEntryValue{Key: key, Value: value}
			}
			push(Map(air.TypeID(inst.A), entries))
		case vmcode.OpMapKeys, vmcode.OpMapSize, vmcode.OpMapGet, vmcode.OpMapSet, vmcode.OpMapDrop, vmcode.OpMapHas, vmcode.OpMapKeyAt, vmcode.OpMapValueAt:
			value, err := vm.execBytecodeMapOp(inst, pop)
			if err != nil {
				return Value{}, err
			}
			push(value)
		case vmcode.OpMakeStruct:
			fields, err := popArgs(pop, inst.B)
			if err != nil {
				return Value{}, err
			}
			push(Struct(air.TypeID(inst.A), fields))
		case vmcode.OpGetField:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			structValue, err := value.structValue()
			if err != nil {
				return Value{}, err
			}
			if inst.B < 0 || inst.B >= len(structValue.Fields) {
				return Value{}, fmt.Errorf("field index %d out of range", inst.B)
			}
			push(structValue.Fields[inst.B])
		case vmcode.OpSetField:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			target, err := pop()
			if err != nil {
				return Value{}, err
			}
			structValue, err := target.structValue()
			if err != nil {
				return Value{}, err
			}
			if inst.B < 0 || inst.B >= len(structValue.Fields) {
				return Value{}, fmt.Errorf("field index %d out of range", inst.B)
			}
			structValue.Fields[inst.B] = value
		case vmcode.OpMakeMaybeSome:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			push(Maybe(air.TypeID(inst.A), true, value))
		case vmcode.OpMakeMaybeNone:
			push(Maybe(air.TypeID(inst.A), false, vm.zeroValue(air.TypeID(inst.B))))
		case vmcode.OpMaybeExpect, vmcode.OpMaybeIsNone, vmcode.OpMaybeIsSome, vmcode.OpMaybeOr, vmcode.OpMaybeMap, vmcode.OpMaybeAndThen:
			value, err := vm.execBytecodeMaybeOp(inst, pop)
			if err != nil {
				return Value{}, err
			}
			push(value)
		case vmcode.OpMakeResultOk, vmcode.OpMakeResultErr:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			push(Result(air.TypeID(inst.A), inst.Op == vmcode.OpMakeResultOk, value))
		case vmcode.OpResultExpect, vmcode.OpResultErrValue, vmcode.OpResultOr, vmcode.OpResultIsOk, vmcode.OpResultIsErr, vmcode.OpResultMap, vmcode.OpResultMapErr, vmcode.OpResultAndThen:
			value, err := vm.execBytecodeResultOp(inst, pop)
			if err != nil {
				return Value{}, err
			}
			push(value)
		case vmcode.OpTryResult:
			value, jump, returned, err := vm.execBytecodeTryResult(inst, pop, locals)
			if err != nil {
				return Value{}, err
			}
			if returned {
				return value, nil
			}
			if jump >= 0 {
				ip = jump
			} else {
				push(value)
			}
		case vmcode.OpTryMaybe:
			value, jump, returned, err := vm.execBytecodeTryMaybe(inst, pop, locals)
			if err != nil {
				return Value{}, err
			}
			if returned {
				return value, nil
			}
			if jump >= 0 {
				ip = jump
			} else {
				push(value)
			}
		default:
			return Value{}, fmt.Errorf("unsupported bytecode opcode %s", inst.Op)
		}
	}
	return Value{}, fmt.Errorf("%s: missing return", fn.Name)
}

func (vm *VM) runBytecodeClosure(closure *ClosureValue, args []Value) (Value, error) {
	fn, ok := vm.bytecode.Function(closure.Function)
	if !ok {
		return Value{}, fmt.Errorf("invalid closure function id %d", closure.Function)
	}
	if len(args) != fn.Arity {
		return Value{}, fmt.Errorf("%s expects %d args, got %d", fn.Name, fn.Arity, len(args))
	}
	if len(closure.Captures) != len(fn.Captures) {
		return Value{}, fmt.Errorf("%s expects %d captures, got %d", fn.Name, len(fn.Captures), len(closure.Captures))
	}
	locals := make([]Value, fn.Locals)
	copy(locals, args)
	for i, capture := range fn.Captures {
		if int(capture.Local) < 0 || int(capture.Local) >= len(locals) {
			return Value{}, fmt.Errorf("%s capture %s has invalid local %d", fn.Name, capture.Name, capture.Local)
		}
		locals[capture.Local] = closure.Captures[i]
	}
	return vm.runBytecodeWithLocals(closure.Function, locals)
}

func (vm *VM) bytecodeConstant(index int, kind vmcode.ConstantKind) (vmcode.Constant, error) {
	if index < 0 || index >= len(vm.bytecode.Constants) {
		return vmcode.Constant{}, fmt.Errorf("constant index %d out of range", index)
	}
	constant := vm.bytecode.Constants[index]
	if constant.Kind != kind {
		return vmcode.Constant{}, fmt.Errorf("constant %d has kind %d, want %d", index, constant.Kind, kind)
	}
	return constant, nil
}

func popArgs(pop func() (Value, error), count int) ([]Value, error) {
	args := make([]Value, count)
	for i := count - 1; i >= 0; i-- {
		value, err := pop()
		if err != nil {
			return nil, err
		}
		args[i] = value
	}
	return args, nil
}

func popBinary(pop func() (Value, error)) (Value, Value, error) {
	right, err := pop()
	if err != nil {
		return Value{}, Value{}, err
	}
	left, err := pop()
	if err != nil {
		return Value{}, Value{}, err
	}
	return left, right, nil
}

func evalBytecodeIntBinary(inst vmcode.Instruction, left Value, right Value) Value {
	switch inst.Op {
	case vmcode.OpIntAdd:
		return Int(air.TypeID(inst.A), left.Int+right.Int)
	case vmcode.OpIntSub:
		return Int(air.TypeID(inst.A), left.Int-right.Int)
	case vmcode.OpIntMul:
		return Int(air.TypeID(inst.A), left.Int*right.Int)
	case vmcode.OpIntDiv:
		return Int(air.TypeID(inst.A), left.Int/right.Int)
	case vmcode.OpIntMod:
		return Int(air.TypeID(inst.A), left.Int%right.Int)
	default:
		return Value{}
	}
}

func evalBytecodeFloatBinary(inst vmcode.Instruction, left Value, right Value) Value {
	switch inst.Op {
	case vmcode.OpFloatAdd:
		return Float(air.TypeID(inst.A), left.Float+right.Float)
	case vmcode.OpFloatSub:
		return Float(air.TypeID(inst.A), left.Float-right.Float)
	case vmcode.OpFloatMul:
		return Float(air.TypeID(inst.A), left.Float*right.Float)
	case vmcode.OpFloatDiv:
		return Float(air.TypeID(inst.A), left.Float/right.Float)
	default:
		return Value{}
	}
}

func evalBytecodeComparison(op vmcode.Opcode, left Value, right Value) (bool, error) {
	switch left.Kind {
	case ValueInt, ValueEnum:
		switch op {
		case vmcode.OpLt:
			return left.Int < right.Int, nil
		case vmcode.OpLte:
			return left.Int <= right.Int, nil
		case vmcode.OpGt:
			return left.Int > right.Int, nil
		case vmcode.OpGte:
			return left.Int >= right.Int, nil
		}
	case ValueFloat:
		switch op {
		case vmcode.OpLt:
			return left.Float < right.Float, nil
		case vmcode.OpLte:
			return left.Float <= right.Float, nil
		case vmcode.OpGt:
			return left.Float > right.Float, nil
		case vmcode.OpGte:
			return left.Float >= right.Float, nil
		}
	}
	return false, fmt.Errorf("cannot compare value kind %d", left.Kind)
}

func bytecodeValueToStr(typeID air.TypeID, value Value) (Value, error) {
	switch value.Kind {
	case ValueStr:
		return Str(typeID, value.Str), nil
	case ValueInt:
		return Str(typeID, strconv.Itoa(value.Int)), nil
	case ValueFloat:
		return Str(typeID, strconv.FormatFloat(value.Float, 'f', -1, 64)), nil
	case ValueBool:
		return Str(typeID, strconv.FormatBool(value.Bool)), nil
	default:
		return Value{}, fmt.Errorf("cannot convert value kind %d to Str", value.Kind)
	}
}
