package vm

import (
	"fmt"
	"sync"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
	corevm "github.com/akonwi/ard/vm"
)

type Frame struct {
	Fn       bytecode.Function
	IP       int
	Locals   []*runtime.Object
	Stack    []*runtime.Object
	MaxStack int
}

type Closure struct {
	FnIndex  int
	Captures []*runtime.Object
	Program  *bytecode.Program
	Params   []checker.Parameter
}

func (c *Closure) Eval(args ...*runtime.Object) *runtime.Object {
	res, err := c.eval(args...)
	if err != nil {
		panic(err)
	}
	return res
}

func (c *Closure) EvalIsolated(args ...*runtime.Object) *runtime.Object {
	return c.Eval(args...)
}

func (c *Closure) GetParams() []checker.Parameter {
	return c.Params
}

func (c *Closure) eval(args ...*runtime.Object) (*runtime.Object, error) {
	if c.Program == nil {
		return nil, fmt.Errorf("closure missing program")
	}
	vm := New(*c.Program)
	return vm.runClosure(c, args)
}

type VM struct {
	Program   bytecode.Program
	Frames    []*Frame
	typeCache map[bytecode.TypeID]checker.Type
	modules   *ModuleRegistry
	funcIndex map[string]int
	ffi       *corevm.RuntimeFFIRegistry
	lastOp    bytecode.Opcode
	lastIP    int
	lastFn    string
}

func New(program bytecode.Program) *VM {
	ffi := corevm.NewRuntimeFFIRegistry()
	_ = ffi.RegisterBuiltinFFIFunctions()
	_ = ffi.RegisterGeneratedFFIFunctions()
	vm := &VM{Program: program, Frames: []*Frame{}, typeCache: map[bytecode.TypeID]checker.Type{}, modules: NewModuleRegistry(), funcIndex: map[string]int{}, ffi: ffi}
	for i := range program.Functions {
		vm.funcIndex[program.Functions[i].Name] = i
	}
	return vm
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
	return vm.run()
}

func (vm *VM) run() (*runtime.Object, error) {

	for len(vm.Frames) > 0 {
		curr := vm.Frames[len(vm.Frames)-1]
		if curr.IP >= len(curr.Fn.Code) {
			return nil, fmt.Errorf("instruction pointer out of range")
		}
		inst := curr.Fn.Code[curr.IP]
		curr.IP++
		vm.lastOp = inst.Op
		vm.lastIP = curr.IP - 1
		vm.lastFn = curr.Fn.Name

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
		case bytecode.OpCopy:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			vm.push(curr, val.Copy())
		case bytecode.OpPanic:
			msgObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("panic: %s", msgObj.AsString())
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
		case bytecode.OpAnd, bytecode.OpOr:
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
			if len(curr.Stack) == 0 {
				val := runtime.Void()
				vm.Frames = vm.Frames[:len(vm.Frames)-1]
				if len(vm.Frames) == 0 {
					return val, nil
				}
				vm.push(vm.Frames[len(vm.Frames)-1], val)
				continue
			}
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			vm.Frames = vm.Frames[:len(vm.Frames)-1]
			if len(vm.Frames) == 0 {
				return val, nil
			}
			vm.push(vm.Frames[len(vm.Frames)-1], val)
		case bytecode.OpCall:
			fnIndex := inst.A
			if fnIndex < 0 || fnIndex >= len(vm.Program.Functions) {
				return nil, fmt.Errorf("function index out of range")
			}
			fnDef := vm.Program.Functions[fnIndex]
			argc := inst.B
			args := make([]*runtime.Object, argc)
			for i := argc - 1; i >= 0; i-- {
				arg, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				args[i] = arg
			}
			frame, err := vm.newFrame(fnDef, args, nil)
			if err != nil {
				return nil, err
			}
			vm.Frames = append(vm.Frames, frame)
		case bytecode.OpMakeClosure:
			fnIndex := inst.A
			if fnIndex < 0 || fnIndex >= len(vm.Program.Functions) {
				return nil, fmt.Errorf("function index out of range")
			}
			captureCount := inst.B
			if captureCount < 0 {
				return nil, fmt.Errorf("invalid capture count")
			}
			captures := make([]*runtime.Object, captureCount)
			for i := captureCount - 1; i >= 0; i-- {
				val, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				captures[i] = val
			}
			fnType, err := vm.typeFor(bytecode.TypeID(inst.C))
			if err != nil {
				return nil, err
			}
			params := []checker.Parameter{}
			if def, ok := fnType.(*checker.FunctionDef); ok {
				params = def.Parameters
			}
			closure := &Closure{FnIndex: fnIndex, Captures: captures, Program: &vm.Program, Params: params}
			vm.push(curr, runtime.Make(closure, fnType))
		case bytecode.OpCallClosure:
			argc := inst.B
			args := make([]*runtime.Object, argc)
			for i := argc - 1; i >= 0; i-- {
				arg, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				args[i] = arg
			}
			closureObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			closure, ok := closureObj.Raw().(*Closure)
			if !ok {
				return nil, fmt.Errorf("expected closure, got %T", closureObj.Raw())
			}
			if closure.FnIndex < 0 || closure.FnIndex >= len(vm.Program.Functions) {
				return nil, fmt.Errorf("function index out of range")
			}
			fnDef := vm.Program.Functions[closure.FnIndex]
			frame, err := vm.newFrame(fnDef, args, closure.Captures)
			if err != nil {
				return nil, err
			}
			vm.Frames = append(vm.Frames, frame)
		case bytecode.OpAsyncStart:
			closureObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			closure, ok := closureObj.Raw().(*Closure)
			if !ok {
				return nil, fmt.Errorf("expected closure, got %T", closureObj.Raw())
			}
			fiberType, err := vm.structTypeFor(bytecode.TypeID(inst.C))
			if err != nil {
				return nil, err
			}
			wg := &sync.WaitGroup{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						fmt.Println(fmt.Errorf("panic in fiber: %v", r))
					}
				}()
				child := vm.spawn()
				_, _ = child.runClosure(closure, nil)
			}()
			fields := map[string]*runtime.Object{
				"wg": runtime.MakeDynamic(wg),
			}
			vm.push(curr, runtime.MakeStruct(fiberType, fields))
		case bytecode.OpAsyncEval:
			closureObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			closure, ok := closureObj.Raw().(*Closure)
			if !ok {
				return nil, fmt.Errorf("expected closure, got %T", closureObj.Raw())
			}
			fiberType, err := vm.structTypeFor(bytecode.TypeID(inst.C))
			if err != nil {
				return nil, err
			}
			wg := &sync.WaitGroup{}
			wg.Add(1)
			resultContainer := &runtime.Object{}
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						fmt.Println(fmt.Errorf("panic in eval fiber: %v", r))
					}
				}()
				child := vm.spawn()
				if res, err := child.runClosure(closure, nil); err == nil && res != nil {
					*resultContainer = *res
				}
			}()
			fields := map[string]*runtime.Object{
				"wg":     runtime.MakeDynamic(wg),
				"result": resultContainer,
			}
			vm.push(curr, runtime.MakeStruct(fiberType, fields))
		case bytecode.OpMakeList:
			typeID := bytecode.TypeID(inst.A)
			listType, err := vm.typeFor(typeID)
			if err != nil {
				return nil, err
			}
			count := inst.B
			items := make([]*runtime.Object, count)
			for i := count - 1; i >= 0; i-- {
				item, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				items[i] = item
			}
			vm.push(curr, runtime.Make(items, listType))
		case bytecode.OpMakeMap:
			typeID := bytecode.TypeID(inst.A)
			mapType, err := vm.typeFor(typeID)
			if err != nil {
				return nil, err
			}
			mapDef, ok := mapType.(*checker.Map)
			if !ok {
				return nil, fmt.Errorf("expected map type for id %d", typeID)
			}
			count := inst.B
			m := runtime.MakeMap(mapDef.Key(), mapDef.Value())
			for i := 0; i < count; i++ {
				val, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				key, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				m.Map_Set(key, val)
			}
			vm.push(curr, m)
		case bytecode.OpListLen:
			listObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			items := listObj.AsList()
			vm.push(curr, runtime.MakeInt(len(items)))
		case bytecode.OpListGet:
			idxObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			listObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			idx := idxObj.AsInt()
			items := listObj.AsList()
			if idx < 0 || idx >= len(items) {
				return nil, fmt.Errorf("list index out of range")
			}
			vm.push(curr, items[idx])
		case bytecode.OpListSet:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			idxObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			listObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			idx := idxObj.AsInt()
			items := listObj.AsList()
			result := runtime.MakeBool(false)
			if idx >= 0 && idx < len(items) {
				items[idx] = val
				listObj.Set(items)
				result = runtime.MakeBool(true)
			}
			vm.push(curr, result)
		case bytecode.OpListPush:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			listObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			items := listObj.AsList()
			items = append(items, val)
			listObj.Set(items)
			vm.push(curr, listObj)
		case bytecode.OpListPrepend:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			listObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			items := listObj.AsList()
			items = append([]*runtime.Object{val}, items...)
			listObj.Set(items)
			vm.push(curr, listObj)
		case bytecode.OpListMethod:
			args := make([]*runtime.Object, inst.B)
			for i := inst.B - 1; i >= 0; i-- {
				arg, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				args[i] = arg
			}
			subj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			res, err := vm.evalListMethod(bytecodeToListKind(inst.A), subj, args)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpMapKeys:
			mapObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			mapType := mapObj.MapType()
			if mapType == nil {
				return nil, fmt.Errorf("map keys on non-map")
			}
			keys := make([]*runtime.Object, 0, len(mapObj.AsMap()))
			for key := range mapObj.AsMap() {
				keys = append(keys, mapObj.Map_GetKey(key))
			}
			vm.push(curr, runtime.MakeList(mapType.Key(), keys...))
		case bytecode.OpMapGet:
			keyObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			mapObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			mapType, err := vm.typeFor(bytecode.TypeID(inst.A))
			if err != nil {
				return nil, err
			}
			mapDef, ok := mapType.(*checker.Map)
			if !ok {
				return nil, fmt.Errorf("expected map type for id %d", inst.A)
			}
			m := mapObj.AsMap()
			keyStr := runtime.ToMapKey(keyObj)
			out := runtime.MakeNone(mapDef.Value())
			if val, ok := m[keyStr]; ok {
				out = out.ToSome(val.Raw())
			}
			vm.push(curr, out)
		case bytecode.OpMapGetValue:
			keyObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			mapObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			m := mapObj.AsMap()
			keyStr := runtime.ToMapKey(keyObj)
			val, ok := m[keyStr]
			if !ok {
				return nil, fmt.Errorf("map key not found")
			}
			vm.push(curr, val)
		case bytecode.OpMapSet:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			keyObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			mapObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			m := mapObj.AsMap()
			keyStr := runtime.ToMapKey(keyObj)
			m[keyStr] = val
			vm.push(curr, runtime.MakeBool(true))
		case bytecode.OpMapDrop:
			keyObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			mapObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			m := mapObj.AsMap()
			keyStr := runtime.ToMapKey(keyObj)
			delete(m, keyStr)
			vm.push(curr, runtime.Void())
		case bytecode.OpMapHas:
			keyObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			mapObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			m := mapObj.AsMap()
			keyStr := runtime.ToMapKey(keyObj)
			_, ok := m[keyStr]
			vm.push(curr, runtime.MakeBool(ok))
		case bytecode.OpMapSize:
			mapObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			vm.push(curr, runtime.MakeInt(len(mapObj.AsMap())))
		case bytecode.OpMakeNone:
			resolved, err := vm.typeFor(bytecode.TypeID(inst.A))
			if err != nil {
				return nil, err
			}
			vm.push(curr, runtime.MakeNone(resolved))
		case bytecode.OpStrMethod:
			args := make([]*runtime.Object, inst.B)
			for i := inst.B - 1; i >= 0; i-- {
				arg, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				args[i] = arg
			}
			obj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			res, err := vm.evalStrMethod(bytecodeToStrKind(inst.A), obj, args)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpIntMethod:
			obj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			res, err := vm.evalIntMethod(bytecodeToIntKind(inst.A), obj)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpFloatMethod:
			obj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			res, err := vm.evalFloatMethod(bytecodeToFloatKind(inst.A), obj)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpBoolMethod:
			obj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			res, err := vm.evalBoolMethod(bytecodeToBoolKind(inst.A), obj)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpMaybeMethod:
			args := make([]*runtime.Object, inst.B)
			for i := inst.B - 1; i >= 0; i-- {
				arg, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				args[i] = arg
			}
			subj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			res, err := vm.evalMaybeMethod(bytecodeToMaybeKind(inst.A), subj, args, bytecode.TypeID(inst.Imm))
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpResultMethod:
			args := make([]*runtime.Object, inst.B)
			for i := inst.B - 1; i >= 0; i-- {
				arg, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				args[i] = arg
			}
			subj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			res, err := vm.evalResultMethod(bytecodeToResultKind(inst.A), subj, args)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpMaybeUnwrap:
			subj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			if subj.IsNone() {
				return nil, fmt.Errorf("cannot unwrap none")
			}
			obj, err := vm.makeValueWithType(subj.Raw(), bytecode.TypeID(inst.A))
			if err != nil {
				return nil, err
			}
			vm.push(curr, obj)
		case bytecode.OpResultUnwrap:
			subj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			unwrapped := subj.UnwrapResult()
			resolved, err := vm.typeFor(bytecode.TypeID(inst.A))
			if err != nil {
				return nil, err
			}
			unwrapped.SetRefinedType(resolved)
			vm.push(curr, unwrapped)
		case bytecode.OpTypeName:
			subj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			vm.push(curr, runtime.MakeStr(subj.TypeName()))
		case bytecode.OpStrChars:
			subj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			runes := []rune(subj.AsString())
			chars := make([]*runtime.Object, len(runes))
			for i, r := range runes {
				chars[i] = runtime.MakeStr(string(r))
			}
			vm.push(curr, runtime.MakeList(checker.Str, chars...))
		case bytecode.OpTryResult:
			subj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			if subj.IsErr() {
				if inst.A >= 0 {
					if inst.B >= 0 {
						unwrapped := subj.UnwrapResult()
						errType, err := vm.typeFor(bytecode.TypeID(inst.C))
						if err != nil {
							return nil, err
						}
						unwrapped.SetRefinedType(errType)
						if inst.B < len(curr.Locals) {
							curr.Locals[inst.B] = unwrapped
						}
					}
					curr.Stack = curr.Stack[:0]
					curr.IP = inst.A
					continue
				}
				vm.Frames = vm.Frames[:len(vm.Frames)-1]
				if len(vm.Frames) == 0 {
					return subj, nil
				}
				vm.push(vm.Frames[len(vm.Frames)-1], subj)
				continue
			}
			unwrapped := subj.UnwrapResult()
			okType, err := vm.typeFor(bytecode.TypeID(inst.Imm))
			if err != nil {
				return nil, err
			}
			unwrapped.SetRefinedType(okType)
			vm.push(curr, unwrapped)
		case bytecode.OpTryMaybe:
			subj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			if subj.IsNone() {
				if inst.A >= 0 {
					curr.Stack = curr.Stack[:0]
					curr.IP = inst.A
					continue
				}
				vm.Frames = vm.Frames[:len(vm.Frames)-1]
				if len(vm.Frames) == 0 {
					return subj, nil
				}
				vm.push(vm.Frames[len(vm.Frames)-1], subj)
				continue
			}
			obj, err := vm.makeValueWithType(subj.Raw(), bytecode.TypeID(inst.Imm))
			if err != nil {
				return nil, err
			}
			vm.push(curr, obj)
		case bytecode.OpMakeStruct:
			structType, err := vm.structTypeFor(bytecode.TypeID(inst.A))
			if err != nil {
				return nil, err
			}
			count := inst.B
			fields := map[string]*runtime.Object{}
			for i := 0; i < count; i++ {
				val, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				keyObj, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				key := keyObj.AsString()
				fields[key] = val
			}
			vm.push(curr, runtime.MakeStruct(structType, fields))
		case bytecode.OpMakeEnum:
			discObj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			enumType, err := vm.enumTypeFor(bytecode.TypeID(inst.A))
			if err != nil {
				return nil, err
			}
			vm.push(curr, runtime.Make(discObj.AsInt(), enumType))
		case bytecode.OpGetField:
			obj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			nameConst, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			val := obj.Struct_Get(nameConst.Str)
			if val == nil {
				return nil, fmt.Errorf("missing struct field: %s", nameConst.Str)
			}
			vm.push(curr, val)
		case bytecode.OpSetField:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			obj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			nameConst, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			fields, ok := obj.Raw().(map[string]*runtime.Object)
			if !ok {
				return nil, fmt.Errorf("set field on non-struct")
			}
			fields[nameConst.Str] = val
		case bytecode.OpCallMethod:
			methodConst, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			argc := inst.B
			args := make([]*runtime.Object, argc)
			for i := argc - 1; i >= 0; i-- {
				arg, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				args[i] = arg
			}
			subj, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			fnName := fmt.Sprintf("%s.%s", subj.TypeName(), methodConst.Str)
			fnIndex, ok := vm.funcIndex[fnName]
			if !ok {
				res, err := vm.evalTraitMethodByName(subj, methodConst.Str, args)
				if err != nil {
					return nil, fmt.Errorf("unknown method: %s", fnName)
				}
				vm.push(curr, res)
				continue
			}
			fnDef := vm.Program.Functions[fnIndex]
			if fnDef.Arity != argc+1 {
				return nil, fmt.Errorf("arity mismatch: expected %d, got %d", fnDef.Arity, argc+1)
			}
			frame := &Frame{
				Fn:       fnDef,
				IP:       0,
				Locals:   make([]*runtime.Object, fnDef.Locals),
				Stack:    []*runtime.Object{},
				MaxStack: fnDef.MaxStack,
			}
			frame.Locals[0] = subj
			for i := range args {
				frame.Locals[i+1] = args[i]
			}
			vm.Frames = append(vm.Frames, frame)
		case bytecode.OpCallModule:
			modConst, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			fnConst, err := vm.constAt(inst.B)
			if err != nil {
				return nil, err
			}
			if modConst.Kind != bytecode.ConstStr || fnConst.Kind != bytecode.ConstStr {
				return nil, fmt.Errorf("module call expects string constants")
			}
			argc := inst.Imm
			args := make([]*runtime.Object, argc)
			for i := argc - 1; i >= 0; i-- {
				arg, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				args[i] = arg
			}
			retType, err := vm.typeFor(bytecode.TypeID(inst.C))
			if err != nil {
				return nil, err
			}
			call := &checker.FunctionCall{Name: fnConst.Str, ReturnType: retType}
			res, err := vm.modules.Call(modConst.Str, call, args)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpCallExtern:
			bindingConst, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			if bindingConst.Kind != bytecode.ConstStr {
				return nil, fmt.Errorf("extern call expects string binding")
			}
			argc := inst.Imm
			args := make([]*runtime.Object, argc)
			for i := argc - 1; i >= 0; i-- {
				arg, err := vm.pop(curr)
				if err != nil {
					return nil, err
				}
				args[i] = arg
			}
			retType, err := vm.typeFor(bytecode.TypeID(inst.C))
			if err != nil {
				return nil, err
			}
			res, err := vm.ffi.Call(bindingConst.Str, args, retType)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpMatchBool, bytecode.OpMatchInt, bytecode.OpMatchEnum, bytecode.OpMatchUnion,
			bytecode.OpMatchMaybe, bytecode.OpMatchResult:
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

func (vm *VM) spawn() *VM {
	child := New(vm.Program)
	child.modules = vm.modules
	child.funcIndex = vm.funcIndex
	return child
}

func (vm *VM) newFrame(fnDef bytecode.Function, args []*runtime.Object, captures []*runtime.Object) (*Frame, error) {
	if len(args) != fnDef.Arity {
		return nil, fmt.Errorf("arity mismatch: expected %d, got %d", fnDef.Arity, len(args))
	}
	if captures == nil {
		captures = []*runtime.Object{}
	}
	if len(captures) != len(fnDef.Captures) {
		return nil, fmt.Errorf("capture mismatch: expected %d, got %d", len(fnDef.Captures), len(captures))
	}
	frame := &Frame{
		Fn:       fnDef,
		IP:       0,
		Locals:   make([]*runtime.Object, fnDef.Locals),
		Stack:    []*runtime.Object{},
		MaxStack: fnDef.MaxStack,
	}
	for i, localIdx := range fnDef.Captures {
		if localIdx < 0 || localIdx >= len(frame.Locals) {
			return nil, fmt.Errorf("capture local index out of range")
		}
		frame.Locals[localIdx] = captures[i]
	}
	for i := range args {
		frame.Locals[i] = args[i]
	}
	return frame, nil
}

func (vm *VM) runClosure(closure *Closure, args []*runtime.Object) (*runtime.Object, error) {
	if closure.FnIndex < 0 || closure.FnIndex >= len(vm.Program.Functions) {
		return nil, fmt.Errorf("function index out of range")
	}
	child := vm.spawn()
	fnDef := child.Program.Functions[closure.FnIndex]
	frame, err := child.newFrame(fnDef, args, closure.Captures)
	if err != nil {
		return nil, err
	}
	child.Frames = append(child.Frames, frame)
	return child.run()
}

func (vm *VM) push(frame *Frame, obj *runtime.Object) {
	frame.Stack = append(frame.Stack, obj)
}

func (vm *VM) pop(frame *Frame) (*runtime.Object, error) {
	if len(frame.Stack) == 0 {
		return nil, fmt.Errorf("stack underflow at %s ip=%d fn=%s", vm.lastOp, vm.lastIP, vm.lastFn)
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
	if left.Kind() == runtime.KindBool && right.Kind() == runtime.KindBool {
		a := left.AsBool()
		b := right.AsBool()
		switch op {
		case bytecode.OpAnd:
			return runtime.MakeBool(a && b), nil
		case bytecode.OpOr:
			return runtime.MakeBool(a || b), nil
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
		if left.Kind() == runtime.KindEnum && right.Kind() == runtime.KindInt {
			eq := left.Raw().(int) == right.AsInt()
			if op == bytecode.OpEq {
				return runtime.MakeBool(eq), nil
			}
			return runtime.MakeBool(!eq), nil
		}
		if left.Kind() == runtime.KindInt && right.Kind() == runtime.KindEnum {
			eq := left.AsInt() == right.Raw().(int)
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
