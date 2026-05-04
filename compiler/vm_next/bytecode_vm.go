package vm_next

import (
	"fmt"
	"strconv"

	"github.com/akonwi/ard/air"
	vmcode "github.com/akonwi/ard/vm_next/bytecode"
)

type bytecodeFrame struct {
	fn          *vmcode.Function
	locals      []Value
	ip          int
	stackBase   int
	localsBase  int
	arenaLocals bool
}

func NewWithBytecode(program *air.Program, externs HostFunctionRegistry) (*VM, error) {
	return NewWithExterns(program, externs)
}

func (vm *VM) getValueSlice(length int) []Value {
	if length <= 0 {
		return nil
	}
	if raw := vm.valueSlices.Get(); raw != nil {
		slice := raw.([]Value)
		if cap(slice) >= length {
			return slice[:length]
		}
	}
	return make([]Value, length)
}

func (vm *VM) getValueStack(capacity int) []Value {
	if capacity <= 0 {
		return nil
	}
	if raw := vm.valueSlices.Get(); raw != nil {
		slice := raw.([]Value)
		if cap(slice) >= capacity {
			return slice[:0]
		}
	}
	return make([]Value, 0, capacity)
}

func (vm *VM) putValueSlice(slice []Value) {
	if cap(slice) == 0 {
		return
	}
	all := slice[:cap(slice)]
	clear(all)
	vm.valueSlices.Put(all[:0])
}

func (vm *VM) runBytecode(id air.FunctionID, args []Value) (Value, error) {
	if vm.bytecode == nil {
		return Value{}, fmt.Errorf("vm_next bytecode program is not initialized")
	}
	frame, err := vm.newDirectBytecodeFrame(id, args, 0)
	if err != nil {
		return Value{}, err
	}
	return vm.runBytecodeFrameLoop(frame)
}

func (vm *VM) runBytecodeWithLocals(id air.FunctionID, locals []Value) (Value, error) {
	fn, ok := vm.bytecode.Function(id)
	if !ok {
		vm.putValueSlice(locals)
		return Value{}, fmt.Errorf("invalid bytecode function id %d", id)
	}
	return vm.runBytecodeFrameLoop(bytecodeFrame{fn: fn, locals: locals})
}

func (vm *VM) newDirectBytecodeFrame(id air.FunctionID, args []Value, stackBase int) (bytecodeFrame, error) {
	fn, ok := vm.bytecode.Function(id)
	if !ok {
		return bytecodeFrame{}, fmt.Errorf("invalid bytecode function id %d", id)
	}
	if len(args) != fn.Arity {
		return bytecodeFrame{}, fmt.Errorf("%s expects %d args, got %d", fn.Name, fn.Arity, len(args))
	}
	if vm.profile != nil {
		vm.profile.RecordDirectCall(len(args), fn.Locals)
		vm.profile.RecordLocalsAlloc(fn.Locals)
	}
	locals := vm.getValueSlice(fn.Locals)
	if err := initBytecodeLocals(fn.Name, locals, args); err != nil {
		vm.putValueSlice(locals)
		return bytecodeFrame{}, err
	}
	return bytecodeFrame{fn: fn, locals: locals, stackBase: stackBase}, nil
}

func initBytecodeLocals(fnName string, locals []Value, args []Value) error {
	if len(locals) < len(args) {
		return fmt.Errorf("%s has %d locals for %d args", fnName, len(locals), len(args))
	}
	switch len(args) {
	case 0:
	case 1:
		locals[0] = args[0]
	case 2:
		locals[0] = args[0]
		locals[1] = args[1]
	case 3:
		locals[0] = args[0]
		locals[1] = args[1]
		locals[2] = args[2]
	default:
		copy(locals, args)
	}
	return nil
}

func initTraitBytecodeLocals(fnName string, locals []Value, receiver Value, args []Value, arity int) error {
	if len(locals) < arity {
		return fmt.Errorf("%s has %d locals for %d args", fnName, len(locals), arity)
	}
	locals[0] = receiver
	switch len(args) {
	case 0:
	case 1:
		locals[1] = args[0]
	case 2:
		locals[1] = args[0]
		locals[2] = args[1]
	case 3:
		locals[1] = args[0]
		locals[2] = args[1]
		locals[3] = args[2]
	default:
		copy(locals[1:arity], args)
	}
	return nil
}

func (vm *VM) newClosureBytecodeFrame(captures []Value, fn *vmcode.Function, args []Value, stackBase int) (bytecodeFrame, error) {
	if len(args) != fn.Arity {
		return bytecodeFrame{}, fmt.Errorf("%s expects %d args, got %d", fn.Name, fn.Arity, len(args))
	}
	if len(captures) != len(fn.Captures) {
		return bytecodeFrame{}, fmt.Errorf("%s expects %d captures, got %d", fn.Name, len(fn.Captures), len(captures))
	}
	if vm.profile != nil {
		vm.profile.RecordClosureCall(len(args), fn.Locals)
		vm.profile.RecordClosureFunctionCall(fn.Name, len(args), len(captures), fn.Locals)
		vm.profile.RecordLocalsAlloc(fn.Locals)
	}
	locals := vm.getValueSlice(fn.Locals)
	if err := initBytecodeLocals(fn.Name, locals, args); err != nil {
		vm.putValueSlice(locals)
		return bytecodeFrame{}, err
	}
	for i, capture := range fn.Captures {
		if int(capture.Local) < 0 || int(capture.Local) >= len(locals) {
			vm.putValueSlice(locals)
			return bytecodeFrame{}, fmt.Errorf("%s capture %s has invalid local %d", fn.Name, capture.Name, capture.Local)
		}
		locals[capture.Local] = captures[i]
	}
	return bytecodeFrame{fn: fn, locals: locals, stackBase: stackBase}, nil
}

func (vm *VM) runBytecodeFrameLoop(first bytecodeFrame) (Value, error) {
	if vm.profile != nil {
		vm.profile.RecordStackAlloc(len(first.fn.Code))
	}
	stack := vm.getValueStack(len(first.fn.Code))
	frames := []bytecodeFrame{first}
	localsArena := make([]Value, 0, first.fn.Locals)
	takeArenaLocals := func(length int) ([]Value, int) {
		base := len(localsArena)
		if length <= 0 {
			return nil, base
		}
		needed := base + length
		if needed > cap(localsArena) {
			capacity := cap(localsArena) * 2
			if capacity < needed {
				capacity = needed
			}
			if capacity == 0 {
				capacity = length
			}
			next := make([]Value, needed, capacity)
			copy(next, localsArena)
			localsArena = next
		} else {
			localsArena = localsArena[:needed]
			clear(localsArena[base:needed])
		}
		return localsArena[base:needed], base
	}
	releaseFrameLocals := func(frame bytecodeFrame) {
		if frame.arenaLocals {
			clear(frame.locals)
			localsArena = localsArena[:frame.localsBase]
			return
		}
		vm.putValueSlice(frame.locals)
	}
	defer func() {
		for i := len(frames) - 1; i >= 0; i-- {
			releaseFrameLocals(frames[i])
		}
		vm.putValueSlice(stack)
	}()

	var frame *bytecodeFrame
	var fn *vmcode.Function
	pop := func() (Value, error) {
		if len(stack) <= frame.stackBase {
			return Value{}, fmt.Errorf("%s: stack underflow", fn.Name)
		}
		value := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return value, nil
	}
	push := func(value Value) {
		stack = append(stack, value)
	}
	returnFromFrame := func(value Value) (Value, bool) {
		stack = stack[:frame.stackBase]
		releaseFrameLocals(*frame)
		frames = frames[:len(frames)-1]
		if len(frames) == 0 {
			return value, true
		}
		stack = append(stack, value)
		return Value{}, false
	}
	prepareClosureFrame := func(target Value, args []Value) (*vmcode.Function, []Value, int, error) {
		vm.recordRefAccess(refAccessClosure)
		function, captures, ok := closureParts(target)
		if !ok {
			return nil, nil, 0, closureValueError(target)
		}
		callee, ok := vm.bytecode.Function(function)
		if !ok {
			return nil, nil, 0, fmt.Errorf("invalid closure function id %d", function)
		}
		if len(args) != callee.Arity {
			return nil, nil, 0, fmt.Errorf("%s expects %d args, got %d", callee.Name, callee.Arity, len(args))
		}
		if len(captures) != len(callee.Captures) {
			return nil, nil, 0, fmt.Errorf("%s expects %d captures, got %d", callee.Name, len(callee.Captures), len(captures))
		}
		if vm.profile != nil {
			vm.profile.RecordClosureCall(len(args), callee.Locals)
			vm.profile.RecordClosureFunctionCall(callee.Name, len(args), len(captures), callee.Locals)
			vm.profile.RecordLocalsAlloc(callee.Locals)
		}
		nextLocals, localsBase := takeArenaLocals(callee.Locals)
		if err := initBytecodeLocals(callee.Name, nextLocals, args); err != nil {
			localsArena = localsArena[:localsBase]
			return nil, nil, 0, err
		}
		for i, capture := range callee.Captures {
			if int(capture.Local) < 0 || int(capture.Local) >= len(nextLocals) {
				localsArena = localsArena[:localsBase]
				return nil, nil, 0, fmt.Errorf("%s capture %s has invalid local %d", callee.Name, capture.Name, capture.Local)
			}
			nextLocals[capture.Local] = captures[i]
		}
		return callee, nextLocals, localsBase, nil
	}

	for len(frames) > 0 {
		frame = &frames[len(frames)-1]
		fn = frame.fn
		locals := frame.locals

		if frame.ip >= len(fn.Code) {
			return Value{}, fmt.Errorf("%s: missing return", fn.Name)
		}
		inst := fn.Code[frame.ip]
		frame.ip++
		if vm.profile != nil {
			vm.profile.RecordOpcode(inst.Op)
		}
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
			frame.ip = inst.A
		case vmcode.OpJumpIfFalse:
			condition, err := pop()
			if err != nil {
				return Value{}, err
			}
			if condition.Kind != ValueBool {
				return Value{}, fmt.Errorf("jump condition must be Bool, got kind %d", condition.Kind)
			}
			if !condition.Bool {
				frame.ip = inst.A
			}
		case vmcode.OpStoreIntAddConstLocalJump:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: int local out of range", fn.Name)
			}
			value := locals[inst.C]
			if value.Kind != ValueInt {
				return Value{}, fmt.Errorf("int add local must be Int, got kind %d", value.Kind)
			}
			locals[inst.B] = Int(value.Type, value.Int+inst.Imm)
			frame.ip = inst.A
		case vmcode.OpJumpIfIntGtLocal:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: int local out of range", fn.Name)
			}
			left := locals[inst.B]
			right := locals[inst.C]
			if left.Kind != ValueInt || right.Kind != ValueInt {
				return Value{}, fmt.Errorf("int comparison locals must be Int")
			}
			if left.Int > right.Int {
				frame.ip = inst.A
			}
		case vmcode.OpJumpIfIntModConstNotEqConstLocal:
			if inst.B < 0 || inst.B >= len(locals) {
				return Value{}, fmt.Errorf("%s: int local %d out of range", fn.Name, inst.B)
			}
			value := locals[inst.B]
			if value.Kind != ValueInt {
				return Value{}, fmt.Errorf("int modulo local must be Int, got kind %d", value.Kind)
			}
			if inst.C == 0 {
				return Value{}, fmt.Errorf("integer modulo by zero")
			}
			if value.Int%inst.C != inst.Imm {
				frame.ip = inst.A
			}
		case vmcode.OpJumpIfListIndexGeLocal:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: list/index local out of range", fn.Name)
			}
			indexValue := locals[inst.B]
			if indexValue.Kind != ValueInt {
				return Value{}, fmt.Errorf("list index must be Int")
			}
			vm.recordRefAccess(refAccessList)
			listValue, ok := listRef(locals[inst.C])
			if !ok {
				return Value{}, listValueError(locals[inst.C])
			}
			if indexValue.Int >= len(listValue.Items) {
				frame.ip = inst.A
			}
		case vmcode.OpJumpIfMapIndexGeLocal:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: map/index local out of range", fn.Name)
			}
			indexValue := locals[inst.B]
			if indexValue.Kind != ValueInt {
				return Value{}, fmt.Errorf("map entry index must be Int")
			}
			vm.recordRefAccess(refAccessMap)
			mapValue, ok := mapRef(locals[inst.C])
			if !ok {
				return Value{}, mapValueError(locals[inst.C])
			}
			if indexValue.Int >= len(mapValue.Entries) {
				frame.ip = inst.A
			}
		case vmcode.OpCall:
			if inst.B < 0 || inst.B > len(stack)-frame.stackBase {
				return Value{}, fmt.Errorf("%s: stack underflow", fn.Name)
			}
			callee, ok := vm.bytecode.Function(air.FunctionID(inst.A))
			if !ok {
				return Value{}, fmt.Errorf("invalid bytecode function id %d", inst.A)
			}
			if inst.B != callee.Arity {
				return Value{}, fmt.Errorf("%s expects %d args, got %d", callee.Name, callee.Arity, inst.B)
			}
			argsStart := len(stack) - inst.B
			if vm.profile != nil {
				vm.profile.RecordDirectCall(inst.B, callee.Locals)
				vm.profile.RecordLocalsAlloc(callee.Locals)
			}
			locals, localsBase := takeArenaLocals(callee.Locals)
			if err := initBytecodeLocals(callee.Name, locals, stack[argsStart:]); err != nil {
				localsArena = localsArena[:localsBase]
				return Value{}, err
			}
			stack = stack[:argsStart]
			frames = append(frames, bytecodeFrame{fn: callee, locals: locals, stackBase: argsStart, localsBase: localsBase, arenaLocals: true})
			continue
		case vmcode.OpCallExtern:
			callArgs, err := stackTailArgs(&stack, inst.B)
			if err != nil {
				return Value{}, err
			}
			result, err := vm.callExtern(air.ExternID(inst.A), callArgs)
			if err != nil {
				return Value{}, err
			}
			stack = stack[:len(stack)-inst.B]
			push(result)
		case vmcode.OpMakeClosure:
			captures, err := popArgs(vm.profile, pop, inst.B)
			if err != nil {
				return Value{}, err
			}
			if vm.profile != nil {
				closureName := ""
				if closureFn, ok := vm.bytecode.Function(air.FunctionID(inst.C)); ok {
					closureName = closureFn.Name
				}
				vm.profile.RecordClosureFunctionCreation(closureName, len(captures))
				if len(captures) > 0 {
					vm.profile.RecordValueAlloc(valueAllocClosure)
				}
			}
			push(Closure(air.TypeID(inst.A), air.FunctionID(inst.C), captures))
		case vmcode.OpCallClosure:
			if inst.B < 0 || inst.B+1 > len(stack)-frame.stackBase {
				return Value{}, fmt.Errorf("closure call: stack underflow")
			}
			targetIndex := len(stack) - inst.B - 1
			target := stack[targetIndex]
			args := stack[targetIndex+1:]
			callee, locals, localsBase, err := prepareClosureFrame(target, args)
			if err != nil {
				return Value{}, err
			}
			stack = stack[:targetIndex]
			frames = append(frames, bytecodeFrame{fn: callee, locals: locals, stackBase: targetIndex, localsBase: localsBase, arenaLocals: true})
			continue
		case vmcode.OpCallClosureLocal:
			if inst.B < 0 || inst.B >= len(locals) {
				return Value{}, fmt.Errorf("%s: closure local %d out of range", fn.Name, inst.B)
			}
			if inst.C < 0 || inst.C > len(stack)-frame.stackBase {
				return Value{}, fmt.Errorf("closure local call: stack underflow")
			}
			targetIndex := len(stack) - inst.C
			target := locals[inst.B]
			args := stack[targetIndex:]
			callee, nextLocals, localsBase, err := prepareClosureFrame(target, args)
			if err != nil {
				return Value{}, err
			}
			stack = stack[:targetIndex]
			frames = append(frames, bytecodeFrame{fn: callee, locals: nextLocals, stackBase: targetIndex, localsBase: localsBase, arenaLocals: true})
			continue
		case vmcode.OpSpawnFiber:
			fiber := &FiberValue{Type: air.TypeID(inst.A), Done: make(chan struct{})}
			if vm.profile != nil {
				vm.profile.RecordFiberSpawn()
			}
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
			vm.recordRefAccess(refAccessFiber)
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
			vm.recordRefAccess(refAccessFiber)
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
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			if result, done := returnFromFrame(value); done {
				return result, nil
			}
			continue
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
			if inst.Imm < 0 || inst.Imm+1 > len(stack)-frame.stackBase {
				return Value{}, fmt.Errorf("trait call: stack underflow")
			}
			subjectIndex := len(stack) - inst.Imm - 1
			subject := stack[subjectIndex]
			vm.recordRefAccess(refAccessTraitObject)
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
			callee, ok := vm.bytecode.Function(impl.Methods[inst.C])
			if !ok {
				return Value{}, fmt.Errorf("invalid bytecode function id %d", impl.Methods[inst.C])
			}
			if callee.Arity != inst.Imm+1 {
				return Value{}, fmt.Errorf("%s expects %d args, got %d", callee.Name, callee.Arity, inst.Imm+1)
			}
			if vm.profile != nil {
				vm.profile.RecordTraitCall()
				vm.profile.RecordDirectCall(callee.Arity, callee.Locals)
				vm.profile.RecordLocalsAlloc(callee.Locals)
			}
			locals, localsBase := takeArenaLocals(callee.Locals)
			if err := initTraitBytecodeLocals(callee.Name, locals, traitObject.Value, stack[subjectIndex+1:], callee.Arity); err != nil {
				localsArena = localsArena[:localsBase]
				return Value{}, err
			}
			stack = stack[:subjectIndex]
			frames = append(frames, bytecodeFrame{fn: callee, locals: locals, stackBase: subjectIndex, localsBase: localsBase, arenaLocals: true})
			continue
		case vmcode.OpUnionWrap:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			if vm.profile != nil {
				vm.profile.RecordValueAlloc(valueAllocUnion)
			}
			push(Union(air.TypeID(inst.A), uint32(inst.Imm), value))
		case vmcode.OpUnionTag:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			vm.recordRefAccess(refAccessUnion)
			unionValue, ok := unionRef(value)
			if !ok {
				return Value{}, unionValueError(value)
			}
			push(Int(air.TypeID(inst.A), int(unionValue.Tag)))
		case vmcode.OpUnionTagLocal:
			if inst.B < 0 || inst.B >= len(locals) {
				return Value{}, fmt.Errorf("%s: local %d out of range", fn.Name, inst.B)
			}
			vm.recordRefAccess(refAccessUnion)
			unionValue, ok := unionRef(locals[inst.B])
			if !ok {
				return Value{}, unionValueError(locals[inst.B])
			}
			push(Int(air.TypeID(inst.A), int(unionValue.Tag)))
		case vmcode.OpUnionValue:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			vm.recordRefAccess(refAccessUnion)
			unionValue, ok := unionRef(value)
			if !ok {
				return Value{}, unionValueError(value)
			}
			push(unionValue.Value)
		case vmcode.OpUnionValueLocal:
			if inst.B < 0 || inst.B >= len(locals) {
				return Value{}, fmt.Errorf("%s: local %d out of range", fn.Name, inst.B)
			}
			vm.recordRefAccess(refAccessUnion)
			unionValue, ok := unionRef(locals[inst.B])
			if !ok {
				return Value{}, unionValueError(locals[inst.B])
			}
			push(unionValue.Value)
		case vmcode.OpIntAddConstLocal:
			if inst.B < 0 || inst.B >= len(locals) {
				return Value{}, fmt.Errorf("%s: int local %d out of range", fn.Name, inst.B)
			}
			value := locals[inst.B]
			if value.Kind != ValueInt {
				return Value{}, fmt.Errorf("int add local must be Int, got kind %d", value.Kind)
			}
			push(Int(air.TypeID(inst.A), value.Int+inst.Imm))
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
		case vmcode.OpToDynamic:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			if vm.profile != nil {
				vm.profile.RecordValueAlloc(valueAllocDynamic)
			}
			push(Dynamic(air.TypeID(inst.A), value.GoValue()))
		case vmcode.OpPanic:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			return Value{}, fmt.Errorf("%s", value.GoValueString())
		case vmcode.OpEnumVariant:
			push(Enum(air.TypeID(inst.A), inst.Imm))
		case vmcode.OpStrAt, vmcode.OpStrSize, vmcode.OpStrIsEmpty, vmcode.OpStrContains, vmcode.OpStrReplace, vmcode.OpStrReplaceAll, vmcode.OpStrSplit, vmcode.OpStrStartsWith, vmcode.OpStrTrim:
			value, err := vm.execBytecodeStrOp(inst, &stack)
			if err != nil {
				return Value{}, err
			}
			push(value)
		case vmcode.OpMakeList:
			items, err := popArgs(nil, pop, inst.B)
			if err != nil {
				return Value{}, err
			}
			if vm.profile != nil {
				vm.profile.RecordValueAlloc(valueAllocList)
			}
			push(List(air.TypeID(inst.A), items))
		case vmcode.OpListSizeLocal:
			if inst.B < 0 || inst.B >= len(locals) {
				return Value{}, fmt.Errorf("%s: list local %d out of range", fn.Name, inst.B)
			}
			vm.recordRefAccess(refAccessList)
			listValue, ok := listRef(locals[inst.B])
			if !ok {
				return Value{}, listValueError(locals[inst.B])
			}
			push(Int(air.TypeID(inst.A), len(listValue.Items)))
		case vmcode.OpListAtLocal:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: list/index local out of range", fn.Name)
			}
			vm.recordRefAccess(refAccessList)
			listValue, ok := listRef(locals[inst.B])
			if !ok {
				return Value{}, listValueError(locals[inst.B])
			}
			indexValue := locals[inst.C]
			if indexValue.Kind != ValueInt {
				return Value{}, fmt.Errorf("list index must be Int")
			}
			index := indexValue.Int
			if index < 0 || index >= len(listValue.Items) {
				return Value{}, fmt.Errorf("list index out of range")
			}
			push(listValue.Items[index])
		case vmcode.OpListAtModLocal:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: list/index local out of range", fn.Name)
			}
			vm.recordRefAccess(refAccessList)
			listValue, ok := listRef(locals[inst.B])
			if !ok {
				return Value{}, listValueError(locals[inst.B])
			}
			indexValue := locals[inst.C]
			if indexValue.Kind != ValueInt {
				return Value{}, fmt.Errorf("list index must be Int")
			}
			if len(listValue.Items) == 0 {
				return Value{}, fmt.Errorf("integer modulo by zero")
			}
			index := (indexValue.Int + inst.Imm) % len(listValue.Items)
			if index < 0 {
				index += len(listValue.Items)
			}
			push(listValue.Items[index])
		case vmcode.OpListIndexLtLocal:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: list/index local out of range", fn.Name)
			}
			indexValue := locals[inst.B]
			if indexValue.Kind != ValueInt {
				return Value{}, fmt.Errorf("list index must be Int")
			}
			vm.recordRefAccess(refAccessList)
			listValue, ok := listRef(locals[inst.C])
			if !ok {
				return Value{}, listValueError(locals[inst.C])
			}
			push(Bool(air.TypeID(inst.A), indexValue.Int < len(listValue.Items)))
		case vmcode.OpListPushLocal, vmcode.OpListPushLocalDrop:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			if inst.B < 0 || inst.B >= len(locals) {
				return Value{}, fmt.Errorf("%s: list local %d out of range", fn.Name, inst.B)
			}
			vm.recordRefAccess(refAccessList)
			listValue, ok := listRef(locals[inst.B])
			if !ok {
				return Value{}, listValueError(locals[inst.B])
			}
			listValue.Items = append(listValue.Items, value)
			if inst.Op == vmcode.OpListPushLocal {
				push(Int(air.TypeID(inst.A), len(listValue.Items)))
			}
		case vmcode.OpListAt, vmcode.OpListPrepend, vmcode.OpListPush, vmcode.OpListSet, vmcode.OpListSize, vmcode.OpListSort, vmcode.OpListSwap:
			value, err := vm.execBytecodeListOp(inst, &stack)
			if err != nil {
				return Value{}, err
			}
			push(value)
		case vmcode.OpMakeMap:
			if vm.profile != nil {
				vm.profile.RecordValueAlloc(valueAllocMap)
			}
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
		case vmcode.OpMapSizeLocal:
			if inst.B < 0 || inst.B >= len(locals) {
				return Value{}, fmt.Errorf("%s: map local %d out of range", fn.Name, inst.B)
			}
			vm.recordRefAccess(refAccessMap)
			mapValue, ok := mapRef(locals[inst.B])
			if !ok {
				return Value{}, mapValueError(locals[inst.B])
			}
			push(Int(air.TypeID(inst.A), len(mapValue.Entries)))
		case vmcode.OpMapIndexLtLocal:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: map/index local out of range", fn.Name)
			}
			indexValue := locals[inst.B]
			if indexValue.Kind != ValueInt {
				return Value{}, fmt.Errorf("map entry index must be Int")
			}
			vm.recordRefAccess(refAccessMap)
			mapValue, ok := mapRef(locals[inst.C])
			if !ok {
				return Value{}, mapValueError(locals[inst.C])
			}
			push(Bool(air.TypeID(inst.A), indexValue.Int < len(mapValue.Entries)))
		case vmcode.OpMapGetLocal:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: map/key local out of range", fn.Name)
			}
			vm.recordRefAccess(refAccessMap)
			mapValue, ok := mapRef(locals[inst.B])
			if !ok {
				return Value{}, mapValueError(locals[inst.B])
			}
			push(vm.mapGetOutput(air.TypeID(inst.A), mapValue, locals[inst.C]))
		case vmcode.OpMapGetLocalTryMaybe:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: map/key local out of range", fn.Name)
			}
			vm.recordRefAccess(refAccessMap)
			mapValue, ok := mapRef(locals[inst.B])
			if !ok {
				return Value{}, mapValueError(locals[inst.B])
			}
			if index := mapEntryIndex(mapValue, locals[inst.C]); index >= 0 {
				push(mapValue.Entries[index].Value)
			} else if inst.Imm >= 0 {
				frame.ip = inst.Imm
			} else {
				vm.recordMaybeAlloc(false)
				if result, done := returnFromFrame(Maybe(air.TypeID(inst.A), false, vm.bytecodeZeroMaybeForReturn(air.TypeID(inst.A)))); done {
					return result, nil
				}
			}
		case vmcode.OpMapGetOrConstIntLocal:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: map/key local out of range", fn.Name)
			}
			vm.recordRefAccess(refAccessMap)
			mapValue, ok := mapRef(locals[inst.B])
			if !ok {
				return Value{}, mapValueError(locals[inst.B])
			}
			if index := mapEntryIndex(mapValue, locals[inst.C]); index >= 0 {
				push(mapValue.Entries[index].Value)
			} else {
				push(Int(air.TypeID(inst.A), inst.Imm))
			}
		case vmcode.OpMapGetOrConstIntLocalKey:
			if inst.B < 0 || inst.B >= len(locals) {
				return Value{}, fmt.Errorf("%s: map local %d out of range", fn.Name, inst.B)
			}
			if len(stack) == 0 {
				return Value{}, fmt.Errorf("map get key: stack underflow")
			}
			vm.recordRefAccess(refAccessMap)
			mapValue, ok := mapRef(locals[inst.B])
			if !ok {
				return Value{}, mapValueError(locals[inst.B])
			}
			key := stack[len(stack)-1]
			if index := mapEntryIndex(mapValue, key); index >= 0 {
				push(mapValue.Entries[index].Value)
			} else {
				push(Int(air.TypeID(inst.A), inst.Imm))
			}
		case vmcode.OpMapIncrementIntLocal, vmcode.OpMapIncrementIntLocalDrop:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: map/key local out of range", fn.Name)
			}
			vm.recordRefAccess(refAccessMap)
			mapValue, ok := mapRef(locals[inst.B])
			if !ok {
				return Value{}, mapValueError(locals[inst.B])
			}
			out, err := vm.mapIncrementIntOutput(air.TypeID(inst.A), mapValue, locals[inst.C], inst.Imm)
			if err != nil {
				return Value{}, err
			}
			if inst.Op == vmcode.OpMapIncrementIntLocal {
				push(out)
			}
		case vmcode.OpMapSetLocalStackKeyDrop:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			key, err := pop()
			if err != nil {
				return Value{}, err
			}
			if inst.B < 0 || inst.B >= len(locals) {
				return Value{}, fmt.Errorf("%s: map local %d out of range", fn.Name, inst.B)
			}
			vm.recordRefAccess(refAccessMap)
			mapValue, ok := mapRef(locals[inst.B])
			if !ok {
				return Value{}, mapValueError(locals[inst.B])
			}
			mapSetOutput(air.TypeID(inst.A), mapValue, key, value)
		case vmcode.OpMapSetLocal, vmcode.OpMapSetLocalDrop:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: map/key local out of range", fn.Name)
			}
			vm.recordRefAccess(refAccessMap)
			mapValue, ok := mapRef(locals[inst.B])
			if !ok {
				return Value{}, mapValueError(locals[inst.B])
			}
			out := mapSetOutput(air.TypeID(inst.A), mapValue, locals[inst.C], value)
			if inst.Op == vmcode.OpMapSetLocal {
				push(out)
			}
		case vmcode.OpMapKeyAtLocal, vmcode.OpMapValueAtLocal:
			if inst.B < 0 || inst.B >= len(locals) || inst.C < 0 || inst.C >= len(locals) {
				return Value{}, fmt.Errorf("%s: map/index local out of range", fn.Name)
			}
			vm.recordRefAccess(refAccessMap)
			mapValue, ok := mapRef(locals[inst.B])
			if !ok {
				return Value{}, mapValueError(locals[inst.B])
			}
			indexValue := locals[inst.C]
			if indexValue.Kind != ValueInt {
				return Value{}, fmt.Errorf("map entry index must be Int")
			}
			entry, ok := sortedMapEntryAt(mapValue, indexValue.Int)
			if !ok {
				return Value{}, fmt.Errorf("map entry index out of range")
			}
			if inst.Op == vmcode.OpMapKeyAtLocal {
				push(entry.Key)
			} else {
				push(entry.Value)
			}
		case vmcode.OpMapKeys, vmcode.OpMapSize, vmcode.OpMapGet, vmcode.OpMapSet, vmcode.OpMapDrop, vmcode.OpMapHas, vmcode.OpMapKeyAt, vmcode.OpMapValueAt:
			value, err := vm.execBytecodeMapOp(inst, &stack)
			if err != nil {
				return Value{}, err
			}
			push(value)
		case vmcode.OpMakeStruct:
			fields, err := popArgs(nil, pop, inst.B)
			if err != nil {
				return Value{}, err
			}
			if vm.profile != nil {
				vm.profile.RecordValueAlloc(valueAllocStruct)
			}
			push(Struct(air.TypeID(inst.A), fields))
		case vmcode.OpGetField:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			vm.recordRefAccess(refAccessStruct)
			structValue, ok := structRef(value)
			if !ok {
				return Value{}, structValueError(value)
			}
			if inst.B < 0 || inst.B >= len(structValue.Fields) {
				return Value{}, fmt.Errorf("field index %d out of range", inst.B)
			}
			push(structValue.Fields[inst.B])
		case vmcode.OpGetFieldLocal:
			if inst.B < 0 || inst.B >= len(locals) {
				return Value{}, fmt.Errorf("%s: local %d out of range", fn.Name, inst.B)
			}
			vm.recordRefAccess(refAccessStruct)
			structValue, ok := structRef(locals[inst.B])
			if !ok {
				return Value{}, structValueError(locals[inst.B])
			}
			if inst.C < 0 || inst.C >= len(structValue.Fields) {
				return Value{}, fmt.Errorf("field index %d out of range", inst.C)
			}
			push(structValue.Fields[inst.C])
		case vmcode.OpSetField:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			target, err := pop()
			if err != nil {
				return Value{}, err
			}
			vm.recordRefAccess(refAccessStruct)
			structValue, ok := structRef(target)
			if !ok {
				return Value{}, structValueError(target)
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
			vm.recordMaybeAlloc(true)
			push(Maybe(air.TypeID(inst.A), true, value))
		case vmcode.OpMakeMaybeNone:
			vm.recordMaybeAlloc(false)
			push(Maybe(air.TypeID(inst.A), false, vm.zeroValue(air.TypeID(inst.B))))
		case vmcode.OpMaybeExpect, vmcode.OpMaybeIsNone, vmcode.OpMaybeIsSome, vmcode.OpMaybeOr, vmcode.OpMaybeMap, vmcode.OpMaybeAndThen:
			value, err := vm.execBytecodeMaybeOp(inst, &stack)
			if err != nil {
				return Value{}, err
			}
			push(value)
		case vmcode.OpMakeResultOk, vmcode.OpMakeResultErr:
			value, err := pop()
			if err != nil {
				return Value{}, err
			}
			if vm.profile != nil {
				vm.profile.RecordValueAlloc(valueAllocResult)
			}
			push(Result(air.TypeID(inst.A), inst.Op == vmcode.OpMakeResultOk, value))
		case vmcode.OpResultExpect, vmcode.OpResultErrValue, vmcode.OpResultOr, vmcode.OpResultIsOk, vmcode.OpResultIsErr, vmcode.OpResultMap, vmcode.OpResultMapErr, vmcode.OpResultAndThen:
			value, err := vm.execBytecodeResultOp(inst, &stack)
			if err != nil {
				return Value{}, err
			}
			push(value)
		case vmcode.OpResultExpectLocal, vmcode.OpResultErrValueLocal, vmcode.OpResultIsOkLocal:
			value, err := vm.execBytecodeResultLocalOp(inst, locals)
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
				if result, done := returnFromFrame(value); done {
					return result, nil
				}
				continue
			}
			if jump >= 0 {
				frame.ip = jump
			} else {
				push(value)
			}
		case vmcode.OpTryMaybe:
			value, jump, returned, err := vm.execBytecodeTryMaybe(inst, pop, locals)
			if err != nil {
				return Value{}, err
			}
			if returned {
				if result, done := returnFromFrame(value); done {
					return result, nil
				}
				continue
			}
			if jump >= 0 {
				frame.ip = jump
			} else {
				push(value)
			}
		default:
			return Value{}, fmt.Errorf("unsupported bytecode opcode %s", inst.Op)
		}
	}
	return Value{}, fmt.Errorf("bytecode frame loop exited without return")
}

func (vm *VM) runBytecodeClosureValue(value Value, args []Value) (Value, error) {
	function, captures, ok := closureParts(value)
	if !ok {
		return Value{}, closureValueError(value)
	}
	fn, ok := vm.bytecode.Function(function)
	if !ok {
		return Value{}, fmt.Errorf("invalid closure function id %d", function)
	}
	frame, err := vm.newClosureBytecodeFrame(captures, fn, args, 0)
	if err != nil {
		return Value{}, err
	}
	return vm.runBytecodeFrameLoop(frame)
}

func (vm *VM) runBytecodeClosureValue1(value Value, arg Value) (Value, error) {
	function, captures, ok := closureParts(value)
	if !ok {
		return Value{}, closureValueError(value)
	}
	fn, ok := vm.bytecode.Function(function)
	if !ok {
		return Value{}, fmt.Errorf("invalid closure function id %d", function)
	}
	if fn.Arity != 1 {
		return Value{}, fmt.Errorf("%s expects %d args, got 1", fn.Name, fn.Arity)
	}
	var args [1]Value
	args[0] = arg
	frame, err := vm.newClosureBytecodeFrame(captures, fn, args[:], 0)
	if err != nil {
		return Value{}, err
	}
	return vm.runBytecodeFrameLoop(frame)
}

func (vm *VM) runBytecodeClosureValue2(value Value, left Value, right Value) (Value, error) {
	function, captures, ok := closureParts(value)
	if !ok {
		return Value{}, closureValueError(value)
	}
	fn, ok := vm.bytecode.Function(function)
	if !ok {
		return Value{}, fmt.Errorf("invalid closure function id %d", function)
	}
	if fn.Arity != 2 {
		return Value{}, fmt.Errorf("%s expects %d args, got 2", fn.Name, fn.Arity)
	}
	var args [2]Value
	args[0] = left
	args[1] = right
	frame, err := vm.newClosureBytecodeFrame(captures, fn, args[:], 0)
	if err != nil {
		return Value{}, err
	}
	return vm.runBytecodeFrameLoop(frame)
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

func stackTailArgs(stack *[]Value, count int) ([]Value, error) {
	if count < 0 || count > len(*stack) {
		return nil, fmt.Errorf("stack underflow")
	}
	return (*stack)[len(*stack)-count:], nil
}

func popArgs(profile *executionProfile, pop func() (Value, error), count int) ([]Value, error) {
	if profile != nil {
		profile.RecordArgSliceAlloc(count)
	}
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
