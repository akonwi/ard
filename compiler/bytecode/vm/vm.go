package vm

import (
	"fmt"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/runtime"
)

type Frame struct {
	Fn       bytecode.Function
	IP       int
	Locals   []*runtime.Object
	Stack    []*runtime.Object
	MaxStack int
}

type VM struct {
	Program bytecode.Program
	Frames  []*Frame
}

func New(program bytecode.Program) *VM {
	return &VM{Program: program, Frames: []*Frame{}}
}

func (vm *VM) Run(functionName string) (*runtime.Object, error) {
	fn, ok := vm.lookupFunction(functionName)
	if !ok {
		return nil, fmt.Errorf("function not found: %s", functionName)
	}

	frame := &Frame{
		Fn:       fn,
		IP:       0,
		Locals:   make([]*runtime.Object, fn.Locals),
		Stack:    []*runtime.Object{},
		MaxStack: fn.MaxStack,
	}
	vm.Frames = append(vm.Frames, frame)

	for len(vm.Frames) > 0 {
		curr := vm.Frames[len(vm.Frames)-1]
		if curr.IP >= len(curr.Fn.Code) {
			return nil, fmt.Errorf("instruction pointer out of range")
		}
		inst := curr.Fn.Code[curr.IP]
		curr.IP++

		switch inst.Op {
		case bytecode.OpNoop:
			continue
		case bytecode.OpConstInt:
			vm.push(curr, runtime.MakeInt(inst.Imm))
		case bytecode.OpConstFloat:
			c, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			if c.Kind != bytecode.ConstFloat {
				return nil, fmt.Errorf("expected float constant, got %d", c.Kind)
			}
			vm.push(curr, runtime.MakeFloat(c.Float))
		case bytecode.OpConstStr:
			c, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			if c.Kind != bytecode.ConstStr {
				return nil, fmt.Errorf("expected string constant, got %d", c.Kind)
			}
			vm.push(curr, runtime.MakeStr(c.Str))
		case bytecode.OpConstBool:
			vm.push(curr, runtime.MakeBool(inst.Imm != 0))
		case bytecode.OpConstVoid:
			vm.push(curr, runtime.Void())
		case bytecode.OpConst:
			c, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			obj, err := vm.objectFromConst(c)
			if err != nil {
				return nil, err
			}
			vm.push(curr, obj)
		case bytecode.OpLoadLocal:
			if inst.A < 0 || inst.A >= len(curr.Locals) {
				return nil, fmt.Errorf("local index out of range")
			}
			vm.push(curr, curr.Locals[inst.A])
		case bytecode.OpStoreLocal:
			if inst.A < 0 || inst.A >= len(curr.Locals) {
				return nil, fmt.Errorf("local index out of range")
			}
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			curr.Locals[inst.A] = val
		case bytecode.OpPop:
			if _, err := vm.pop(curr); err != nil {
				return nil, err
			}
		case bytecode.OpDup:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			vm.push(curr, val)
			vm.push(curr, val)
		case bytecode.OpSwap:
			b, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			a, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			vm.push(curr, b)
			vm.push(curr, a)
		case bytecode.OpAdd, bytecode.OpSub, bytecode.OpMul, bytecode.OpDiv, bytecode.OpMod:
			b, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			a, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			res, err := vm.evalBinary(inst.Op, a, b)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpNeg:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			res, err := vm.evalUnary(inst.Op, val)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpNot:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			vm.push(curr, runtime.MakeBool(!val.AsBool()))
		case bytecode.OpEq, bytecode.OpNeq, bytecode.OpLt, bytecode.OpLte, bytecode.OpGt, bytecode.OpGte:
			b, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			a, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			res, err := vm.evalCompare(inst.Op, a, b)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpJump:
			curr.IP = inst.A
		case bytecode.OpJumpIfFalse:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			if !val.AsBool() {
				curr.IP = inst.A
			}
		case bytecode.OpJumpIfTrue:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			if val.AsBool() {
				curr.IP = inst.A
			}
		case bytecode.OpReturn:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			vm.Frames = vm.Frames[:len(vm.Frames)-1]
			if len(vm.Frames) == 0 {
				return val, nil
			}
			vm.push(vm.Frames[len(vm.Frames)-1], val)
		case bytecode.OpCall, bytecode.OpCallExtern, bytecode.OpCallModule,
			bytecode.OpMakeList, bytecode.OpMakeMap, bytecode.OpMakeStruct, bytecode.OpMakeEnum,
			bytecode.OpGetField, bytecode.OpSetField, bytecode.OpCallMethod,
			bytecode.OpMatchBool, bytecode.OpMatchInt, bytecode.OpMatchEnum, bytecode.OpMatchUnion,
			bytecode.OpMatchMaybe, bytecode.OpMatchResult,
			bytecode.OpTryResult, bytecode.OpTryMaybe,
			bytecode.OpAsyncStart, bytecode.OpAsyncEval:
			return nil, fmt.Errorf("opcode not implemented: %s", inst.Op)
		default:
			return nil, fmt.Errorf("unknown opcode: %d", inst.Op)
		}
	}

	return nil, fmt.Errorf("no frames left")
}

func (vm *VM) lookupFunction(name string) (bytecode.Function, bool) {
	for _, fn := range vm.Program.Functions {
		if fn.Name == name {
			return fn, true
		}
	}
	return bytecode.Function{}, false
}

func (vm *VM) push(frame *Frame, obj *runtime.Object) {
	frame.Stack = append(frame.Stack, obj)
}

func (vm *VM) pop(frame *Frame) (*runtime.Object, error) {
	if len(frame.Stack) == 0 {
		return nil, fmt.Errorf("stack underflow")
	}
	idx := len(frame.Stack) - 1
	val := frame.Stack[idx]
	frame.Stack = frame.Stack[:idx]
	return val, nil
}

func (vm *VM) constAt(index int) (bytecode.Constant, error) {
	if index < 0 || index >= len(vm.Program.Constants) {
		return bytecode.Constant{}, fmt.Errorf("constant index out of range")
	}
	return vm.Program.Constants[index], nil
}

func (vm *VM) objectFromConst(c bytecode.Constant) (*runtime.Object, error) {
	switch c.Kind {
	case bytecode.ConstInt:
		return runtime.MakeInt(c.Int), nil
	case bytecode.ConstFloat:
		return runtime.MakeFloat(c.Float), nil
	case bytecode.ConstStr:
		return runtime.MakeStr(c.Str), nil
	case bytecode.ConstBool:
		return runtime.MakeBool(c.Bool), nil
	default:
		return nil, fmt.Errorf("unknown constant kind: %d", c.Kind)
	}
}

func (vm *VM) evalBinary(op bytecode.Opcode, left, right *runtime.Object) (*runtime.Object, error) {
	if left.Kind() == runtime.KindInt && right.Kind() == runtime.KindInt {
		a := left.AsInt()
		b := right.AsInt()
		switch op {
		case bytecode.OpAdd:
			return runtime.MakeInt(a + b), nil
		case bytecode.OpSub:
			return runtime.MakeInt(a - b), nil
		case bytecode.OpMul:
			return runtime.MakeInt(a * b), nil
		case bytecode.OpDiv:
			return runtime.MakeInt(a / b), nil
		case bytecode.OpMod:
			return runtime.MakeInt(a % b), nil
		}
	}
	if left.Kind() == runtime.KindFloat && right.Kind() == runtime.KindFloat {
		a := left.AsFloat()
		b := right.AsFloat()
		switch op {
		case bytecode.OpAdd:
			return runtime.MakeFloat(a + b), nil
		case bytecode.OpSub:
			return runtime.MakeFloat(a - b), nil
		case bytecode.OpMul:
			return runtime.MakeFloat(a * b), nil
		case bytecode.OpDiv:
			return runtime.MakeFloat(a / b), nil
		}
	}
	if left.Kind() == runtime.KindStr && right.Kind() == runtime.KindStr {
		if op == bytecode.OpAdd {
			return runtime.MakeStr(left.AsString() + right.AsString()), nil
		}
	}

	return nil, fmt.Errorf("unsupported binary op %s for %s and %s", op, left.Kind(), right.Kind())
}

func (vm *VM) evalUnary(op bytecode.Opcode, val *runtime.Object) (*runtime.Object, error) {
	if val.Kind() == runtime.KindInt {
		switch op {
		case bytecode.OpNeg:
			return runtime.MakeInt(-val.AsInt()), nil
		}
	}
	if val.Kind() == runtime.KindFloat {
		switch op {
		case bytecode.OpNeg:
			return runtime.MakeFloat(-val.AsFloat()), nil
		}
	}
	return nil, fmt.Errorf("unsupported unary op %s for %s", op, val.Kind())
}

func (vm *VM) evalCompare(op bytecode.Opcode, left, right *runtime.Object) (*runtime.Object, error) {
	if op == bytecode.OpEq || op == bytecode.OpNeq {
		if left.Kind() == right.Kind() {
			eq := left.Equals(*right)
			if op == bytecode.OpEq {
				return runtime.MakeBool(eq), nil
			}
			return runtime.MakeBool(!eq), nil
		}
		return runtime.MakeBool(false), nil
	}
	if left.Kind() == runtime.KindInt && right.Kind() == runtime.KindInt {
		a := left.AsInt()
		b := right.AsInt()
		switch op {
		case bytecode.OpEq:
			return runtime.MakeBool(a == b), nil
		case bytecode.OpNeq:
			return runtime.MakeBool(a != b), nil
		case bytecode.OpLt:
			return runtime.MakeBool(a < b), nil
		case bytecode.OpLte:
			return runtime.MakeBool(a <= b), nil
		case bytecode.OpGt:
			return runtime.MakeBool(a > b), nil
		case bytecode.OpGte:
			return runtime.MakeBool(a >= b), nil
		}
	}
	if left.Kind() == runtime.KindFloat && right.Kind() == runtime.KindFloat {
		a := left.AsFloat()
		b := right.AsFloat()
		switch op {
		case bytecode.OpEq:
			return runtime.MakeBool(a == b), nil
		case bytecode.OpNeq:
			return runtime.MakeBool(a != b), nil
		case bytecode.OpLt:
			return runtime.MakeBool(a < b), nil
		case bytecode.OpLte:
			return runtime.MakeBool(a <= b), nil
		case bytecode.OpGt:
			return runtime.MakeBool(a > b), nil
		case bytecode.OpGte:
			return runtime.MakeBool(a >= b), nil
		}
	}
	return nil, fmt.Errorf("unsupported comparison %s for %s and %s", op, left.Kind(), right.Kind())
}
