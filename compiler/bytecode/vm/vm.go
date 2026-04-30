package vm

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

var (
	sharedModulesOnce sync.Once
	sharedModules     *ModuleRegistry
	sharedFFIOnce     sync.Once
	sharedFFI         *RuntimeFFIRegistry
)

func defaultModuleRegistry() *ModuleRegistry {
	sharedModulesOnce.Do(func() {
		sharedModules = NewModuleRegistry()
	})
	return sharedModules
}

func defaultFFIRegistry() *RuntimeFFIRegistry {
	sharedFFIOnce.Do(func() {
		ffi := NewRuntimeFFIRegistry()
		_ = ffi.RegisterBuiltinFFIFunctions()
		sharedFFI = ffi
	})
	return sharedFFI
}

type Frame struct {
	Fn         *bytecode.Function
	IP         int
	Locals     []any
	Stack      []any
	StackTop   int
	MaxStack   int
	ReturnType checker.Type
}

type Closure struct {
	FnIndex    int
	Captures   []any
	Program    *bytecode.Program
	Params     []checker.Parameter
	ReturnType checker.Type
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
	values := make([]any, len(args))
	for i := range args {
		values[i] = args[i]
	}
	return vm.runClosure(c, values)
}

type VM struct {
	Program           bytecode.Program
	Frames            []*Frame
	freeFrames        []*Frame
	typeCache         map[bytecode.TypeID]checker.Type
	modules           *ModuleRegistry
	methodIndex       map[string]map[string]int
	functionLookup    map[string]int
	moduleCallScratch checker.FunctionCall
	moduleArgsScratch []*runtime.Object
	externArgsScratch []*runtime.Object
	ffi               *RuntimeFFIRegistry
	resolvedExterns   []resolvedExtern
	initErr           error
	profile           *executionProfile
}

func New(program bytecode.Program) *VM {
	vm := &VM{Program: program, Frames: make([]*Frame, 0, 8), freeFrames: make([]*Frame, 0, 8), modules: defaultModuleRegistry(), ffi: defaultFFIRegistry(), profile: newExecutionProfile()}
	vm.resolveExterns()
	return vm
}

func (vm *VM) ProfileReport() string {
	if vm == nil || vm.profile == nil {
		return ""
	}
	return vm.profile.Report()
}

func (vm *VM) resolveExterns() {
	if vm.initErr != nil {
		return
	}
	if len(vm.Program.Externs) == 0 {
		vm.resolvedExterns = nil
		return
	}
	vm.resolvedExterns = make([]resolvedExtern, len(vm.Program.Externs))
	for i := range vm.Program.Externs {
		entry := vm.Program.Externs[i]
		resolved, err := vm.ffi.Resolve(entry.Binding)
		if err != nil {
			vm.initErr = err
			vm.resolvedExterns = nil
			return
		}
		vm.resolvedExterns[i] = resolved
	}
}

func (vm *VM) Run(functionName string) (*runtime.Object, error) {
	if vm.initErr != nil {
		return nil, vm.initErr
	}
	fn, ok := vm.lookupFunction(functionName)
	if !ok {
		return nil, fmt.Errorf("function not found: %s", functionName)
	}

	frame, err := vm.newFrame(fn, nil, nil, nil)
	if err != nil {
		return nil, err
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

		switch inst.Op {
		case bytecode.OpNoop:
			continue
		case bytecode.OpConstInt:
			vm.push(curr, inst.Imm)
		case bytecode.OpConstFloat:
			c, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			if c.Kind != bytecode.ConstFloat {
				return nil, fmt.Errorf("expected float constant, got %d", c.Kind)
			}
			vm.push(curr, c.Float)
		case bytecode.OpConstStr:
			c, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			if c.Kind != bytecode.ConstStr {
				return nil, fmt.Errorf("expected string constant, got %d", c.Kind)
			}
			vm.push(curr, c.Str)
		case bytecode.OpConstBool:
			vm.push(curr, inst.Imm != 0)
		case bytecode.OpConstVoid:
			vm.push(curr, runtime.NativeVoid)
		case bytecode.OpConst:
			c, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			val, err := vm.valueFromConst(c)
			if err != nil {
				return nil, err
			}
			vm.push(curr, val)
		case bytecode.OpLoadLocal:
			if inst.A < 0 || inst.A >= len(curr.Locals) {
				return nil, fmt.Errorf("local index out of range")
			}
			vm.push(curr, curr.Locals[inst.A])
		case bytecode.OpStoreLocal:
			if inst.A < 0 || inst.A >= len(curr.Locals) {
				return nil, fmt.Errorf("local index out of range")
			}
			curr.Locals[inst.A] = vm.popUnsafe(curr)
		case bytecode.OpPop:
			_ = vm.popUnsafe(curr)
		case bytecode.OpDup:
			val := vm.popUnsafe(curr)
			vm.push(curr, val)
			vm.push(curr, val)
		case bytecode.OpSwap:
			b := vm.popUnsafe(curr)
			a := vm.popUnsafe(curr)
			vm.push(curr, b)
			vm.push(curr, a)
		case bytecode.OpCopy:
			vm.push(curr, vm.popUnsafe(curr).Copy())
		case bytecode.OpPanic:
			msgVal := vm.popValueUnsafe(curr)
			msgObj := runtime.ValueToObject(msgVal, checker.Str)
			return nil, fmt.Errorf("panic: %s", msgObj.AsString())
		case bytecode.OpAdd, bytecode.OpSub, bytecode.OpMul, bytecode.OpDiv, bytecode.OpMod:
			right := vm.popValueUnsafe(curr)
			left := vm.popValueUnsafe(curr)
			if res, handled, err := vm.evalBinaryValue(inst.Op, left, right); handled {
				if err != nil {
					return nil, err
				}
				vm.push(curr, res)
				continue
			}
			rightObj := runtime.ValueToObject(right, nil)
			leftObj := runtime.ValueToObject(left, nil)
			res, err := vm.evalBinary(inst.Op, leftObj, rightObj)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpAnd, bytecode.OpOr:
			right := vm.popValueUnsafe(curr)
			left := vm.popValueUnsafe(curr)
			if res, handled, err := vm.evalBinaryValue(inst.Op, left, right); handled {
				if err != nil {
					return nil, err
				}
				vm.push(curr, res)
				continue
			}
			rightObj := runtime.ValueToObject(right, nil)
			leftObj := runtime.ValueToObject(left, nil)
			res, err := vm.evalBinary(inst.Op, leftObj, rightObj)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpNeg:
			val := vm.popValueUnsafe(curr)
			if res, handled, err := vm.evalUnaryValue(inst.Op, val); handled {
				if err != nil {
					return nil, err
				}
				vm.push(curr, res)
				continue
			}
			res, err := vm.evalUnary(inst.Op, runtime.ValueToObject(val, nil))
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpNot:
			val := vm.popValueUnsafe(curr)
			if boolVal, ok := val.(bool); ok {
				vm.push(curr, !boolVal)
				continue
			}
			vm.push(curr, runtime.MakeBool(!runtime.ValueToObject(val, checker.Bool).AsBool()))
		case bytecode.OpEq, bytecode.OpNeq, bytecode.OpLt, bytecode.OpLte, bytecode.OpGt, bytecode.OpGte:
			right := vm.popValueUnsafe(curr)
			left := vm.popValueUnsafe(curr)
			if res, handled, err := vm.evalCompareValue(inst.Op, left, right); handled {
				if err != nil {
					return nil, err
				}
				vm.push(curr, res)
				continue
			}
			rightObj := runtime.ValueToObject(right, nil)
			leftObj := runtime.ValueToObject(left, nil)
			res, err := vm.evalCompare(inst.Op, leftObj, rightObj)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpJump:
			curr.IP = inst.A
		case bytecode.OpJumpIfFalse:
			cond := vm.popValueUnsafe(curr)
			if boolVal, ok := cond.(bool); ok {
				if !boolVal {
					curr.IP = inst.A
				}
				continue
			}
			if !runtime.ValueToObject(cond, checker.Bool).AsBool() {
				curr.IP = inst.A
			}
		case bytecode.OpJumpIfTrue:
			cond := vm.popValueUnsafe(curr)
			if boolVal, ok := cond.(bool); ok {
				if boolVal {
					curr.IP = inst.A
				}
				continue
			}
			if runtime.ValueToObject(cond, checker.Bool).AsBool() {
				curr.IP = inst.A
			}
		case bytecode.OpReturn:
			if curr.StackTop == 0 {
				val := runtime.Void()
				if curr.ReturnType != nil {
					val.SetRefinedType(curr.ReturnType)
				}
				vm.Frames = vm.Frames[:len(vm.Frames)-1]
				vm.recycleFrame(curr)
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
			if curr.ReturnType != nil {
				val.SetRefinedType(curr.ReturnType)
			}
			vm.Frames = vm.Frames[:len(vm.Frames)-1]
			vm.recycleFrame(curr)
			if len(vm.Frames) == 0 {
				return val, nil
			}
			vm.push(vm.Frames[len(vm.Frames)-1], val)
		case bytecode.OpCall:
			vm.profile.RecordDirectCall()
			fnDef := &vm.Program.Functions[inst.A]
			argc := inst.B
			retType, _ := vm.typeFor(bytecode.TypeID(inst.C))
			frame, err := vm.newFrameFromStackUnchecked(curr, fnDef, argc, nil, retType)
			if err != nil {
				return nil, err
			}
			vm.Frames = append(vm.Frames, frame)
		case bytecode.OpMakeClosure:
			vm.profile.RecordClosureCreation(inst.B)
			fnIndex := inst.A
			if fnIndex < 0 || fnIndex >= len(vm.Program.Functions) {
				return nil, fmt.Errorf("function index out of range")
			}
			captureCount := inst.B
			if captureCount < 0 {
				return nil, fmt.Errorf("invalid capture count")
			}
			captures := make([]any, captureCount)
			for i := captureCount - 1; i >= 0; i-- {
				captures[i] = vm.popUnsafe(curr)
			}
			fnType, err := vm.typeFor(bytecode.TypeID(inst.C))
			if err != nil {
				return nil, err
			}
			params := []checker.Parameter{}
			if def, ok := fnType.(*checker.FunctionDef); ok {
				params = def.Parameters
			}
			var returnType checker.Type = checker.Dynamic
			if def, ok := fnType.(*checker.FunctionDef); ok {
				returnType = def.ReturnType
			}
			closure := &Closure{FnIndex: fnIndex, Captures: captures, Program: &vm.Program, Params: params, ReturnType: returnType}
			vm.push(curr, runtime.Make(closure, fnType))
		case bytecode.OpCallClosure:
			argc := inst.B
			vm.profile.RecordClosureCall(argc)
			args := make([]any, argc)
			for i := argc - 1; i >= 0; i-- {
				args[i] = vm.popUnsafe(curr)
			}
			closureObj := vm.popUnsafe(curr)
			closure, ok := closureObj.Raw().(*Closure)
			if !ok {
				return nil, fmt.Errorf("expected closure, got %T", closureObj.Raw())
			}
			if closure.FnIndex < 0 || closure.FnIndex >= len(vm.Program.Functions) {
				return nil, fmt.Errorf("function index out of range")
			}
			fnDef := &vm.Program.Functions[closure.FnIndex]
			retType, err := vm.typeFor(bytecode.TypeID(inst.C))
			if err != nil {
				return nil, err
			}
			frame, err := vm.newFrame(fnDef, args, closure.Captures, retType)
			if err != nil {
				return nil, err
			}
			vm.Frames = append(vm.Frames, frame)
		case bytecode.OpAsyncStart:
			closureObj := vm.popUnsafe(curr)
			closure, ok := closureObj.Raw().(*Closure)
			if !ok {
				return nil, fmt.Errorf("expected closure, got %T", closureObj.Raw())
			}
			fiberType, err := vm.structTypeFor(bytecode.TypeID(inst.C))
			if err != nil {
				return nil, err
			}
			wg := &sync.WaitGroup{}
			wg.Go(func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Println(fmt.Errorf("panic in fiber: %v", r))
					}
				}()
				child := vm.spawn()
				_, _ = child.runClosure(closure, nil)
			})
			fields := map[string]*runtime.Object{
				"wg":     runtime.MakeDynamic(wg),
				"result": runtime.Void(),
			}
			vm.push(curr, runtime.MakeStruct(fiberType, fields))
		case bytecode.OpAsyncEval:
			closureObj := vm.popUnsafe(curr)
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
				items[i] = vm.popUnsafe(curr)
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
			for range count {
				val := vm.popUnsafe(curr)
				key := vm.popUnsafe(curr)
				m.Map_Set(key, val)
			}
			vm.push(curr, m)
		case bytecode.OpListLen:
			listObj := vm.popUnsafe(curr)
			items := listObj.AsList()
			vm.push(curr, runtime.MakeInt(len(items)))
		case bytecode.OpListGet:
			idxObj := vm.popUnsafe(curr)
			listObj := vm.popUnsafe(curr)
			idx := idxObj.AsInt()
			items := listObj.AsList()
			if idx < 0 || idx >= len(items) {
				return nil, fmt.Errorf("list index out of range")
			}
			vm.push(curr, items[idx])
		case bytecode.OpListSet:
			val := vm.popUnsafe(curr)
			idxObj := vm.popUnsafe(curr)
			listObj := vm.popUnsafe(curr)
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
			val := vm.popUnsafe(curr)
			listObj := vm.popUnsafe(curr)
			items := listObj.AsList()
			items = append(items, val)
			listObj.Set(items)
			vm.push(curr, listObj)
		case bytecode.OpListPrepend:
			val := vm.popUnsafe(curr)
			listObj := vm.popUnsafe(curr)
			items := listObj.AsList()
			items = append([]*runtime.Object{val}, items...)
			listObj.Set(items)
			vm.push(curr, listObj)
		case bytecode.OpListMethod:
			args := make([]*runtime.Object, inst.B)
			for i := inst.B - 1; i >= 0; i-- {
				args[i] = vm.popUnsafe(curr)
			}
			subj := vm.popUnsafe(curr)
			res, err := vm.evalListMethod(bytecodeToListKind(inst.A), subj, args)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpMapKeys:
			mapObj := vm.popUnsafe(curr)
			mapType := mapObj.MapType()
			if mapType == nil {
				return nil, fmt.Errorf("map keys on non-map")
			}
			keys := runtime.SortedMapKeys(mapObj)
			vm.push(curr, runtime.MakeList(mapType.Key(), keys...))
		case bytecode.OpMapGet:
			keyObj := vm.popUnsafe(curr)
			mapObj := vm.popUnsafe(curr)
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
			keyObj := vm.popUnsafe(curr)
			mapObj := vm.popUnsafe(curr)
			m := mapObj.AsMap()
			keyStr := runtime.ToMapKey(keyObj)
			val, ok := m[keyStr]
			if !ok {
				return nil, fmt.Errorf("map key not found")
			}
			vm.push(curr, val)
		case bytecode.OpMapSet:
			val := vm.popUnsafe(curr)
			keyObj := vm.popUnsafe(curr)
			mapObj := vm.popUnsafe(curr)
			m := mapObj.AsMap()
			keyStr := runtime.ToMapKey(keyObj)
			m[keyStr] = val
			vm.push(curr, runtime.MakeBool(true))
		case bytecode.OpMapDrop:
			keyObj := vm.popUnsafe(curr)
			mapObj := vm.popUnsafe(curr)
			m := mapObj.AsMap()
			keyStr := runtime.ToMapKey(keyObj)
			delete(m, keyStr)
			vm.push(curr, runtime.Void())
		case bytecode.OpMapHas:
			keyObj := vm.popUnsafe(curr)
			mapObj := vm.popUnsafe(curr)
			m := mapObj.AsMap()
			keyStr := runtime.ToMapKey(keyObj)
			_, ok := m[keyStr]
			vm.push(curr, runtime.MakeBool(ok))
		case bytecode.OpMapSize:
			mapObj := vm.popUnsafe(curr)
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
				args[i] = vm.popUnsafe(curr)
			}
			obj := vm.popUnsafe(curr)
			res, err := vm.evalStrMethod(bytecodeToStrKind(inst.A), obj, args)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpIntMethod:
			obj := vm.popUnsafe(curr)
			res, err := vm.evalIntMethod(bytecodeToIntKind(inst.A), obj)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpFloatMethod:
			obj := vm.popUnsafe(curr)
			res, err := vm.evalFloatMethod(bytecodeToFloatKind(inst.A), obj)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpBoolMethod:
			obj := vm.popUnsafe(curr)
			res, err := vm.evalBoolMethod(bytecodeToBoolKind(inst.A), obj)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpMaybeMethod:
			args := make([]*runtime.Object, inst.B)
			for i := inst.B - 1; i >= 0; i-- {
				args[i] = vm.popUnsafe(curr)
			}
			subj := vm.popUnsafe(curr)
			res, err := vm.evalMaybeMethod(bytecodeToMaybeKind(inst.A), subj, args, bytecode.TypeID(inst.Imm))
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpResultMethod:
			args := make([]*runtime.Object, inst.B)
			for i := inst.B - 1; i >= 0; i-- {
				args[i] = vm.popUnsafe(curr)
			}
			subj := vm.popUnsafe(curr)
			res, err := vm.evalResultMethod(bytecodeToResultKind(inst.A), subj, args)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpMaybeUnwrap:
			subj := vm.popUnsafe(curr)
			if subj.IsNone() {
				return nil, fmt.Errorf("cannot unwrap none")
			}
			resolved, err := vm.typeFor(bytecode.TypeID(inst.A))
			if err != nil {
				return nil, err
			}
			vm.push(curr, subj.UnwrapMaybeInPlace(resolved))
		case bytecode.OpResultUnwrap:
			subj := vm.popUnsafe(curr)
			unwrapped := subj.UnwrapResultInPlace()
			resolved, err := vm.typeFor(bytecode.TypeID(inst.A))
			if err != nil {
				return nil, err
			}
			unwrapped.SetRefinedType(resolved)
			vm.push(curr, unwrapped)
		case bytecode.OpTypeName:
			subj := vm.popUnsafe(curr)
			vm.push(curr, runtime.MakeStr(subj.TypeName()))
		case bytecode.OpStrChars:
			subj := vm.popUnsafe(curr)
			runes := []rune(subj.AsString())
			chars := make([]*runtime.Object, len(runes))
			for i, r := range runes {
				chars[i] = runtime.MakeStr(string(r))
			}
			vm.push(curr, runtime.MakeList(checker.Str, chars...))
		case bytecode.OpTryResult:
			subj := vm.popUnsafe(curr)
			if subj.IsErr() {
				if inst.A >= 0 {
					if inst.B >= 0 {
						unwrapped := subj.UnwrapResultInPlace()
						errType, err := vm.typeFor(bytecode.TypeID(inst.C))
						if err != nil {
							return nil, err
						}
						unwrapped.SetRefinedType(errType)
						if inst.B < len(curr.Locals) {
							curr.Locals[inst.B] = unwrapped
						}
					}
					curr.StackTop = 0
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
			unwrapped := subj.UnwrapResultInPlace()
			okType, err := vm.typeFor(bytecode.TypeID(inst.Imm))
			if err != nil {
				return nil, err
			}
			unwrapped.SetRefinedType(okType)
			vm.push(curr, unwrapped)
		case bytecode.OpTryMaybe:
			subj := vm.popUnsafe(curr)
			if subj.IsNone() {
				if inst.A >= 0 {
					curr.StackTop = 0
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
			okType, err := vm.typeFor(bytecode.TypeID(inst.Imm))
			if err != nil {
				return nil, err
			}
			vm.push(curr, subj.UnwrapMaybeInPlace(okType))
		case bytecode.OpMakeStruct:
			structType, err := vm.structTypeFor(bytecode.TypeID(inst.A))
			if err != nil {
				return nil, err
			}
			count := inst.B
			fields := map[string]*runtime.Object{}
			for range count {
				val := vm.popUnsafe(curr)
				keyObj := vm.popUnsafe(curr)
				key := keyObj.AsString()
				fields[key] = val
			}
			vm.push(curr, runtime.MakeStruct(structType, fields))
		case bytecode.OpMakeEnum:
			discObj := vm.popUnsafe(curr)
			enumType, err := vm.enumTypeFor(bytecode.TypeID(inst.A))
			if err != nil {
				return nil, err
			}
			vm.push(curr, runtime.Make(discObj.AsInt(), enumType))
		case bytecode.OpGetField:
			obj := vm.popUnsafe(curr)
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
			val := vm.popUnsafe(curr)
			obj := vm.popUnsafe(curr)
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
			subjIndex := curr.StackTop - argc - 1
			if subjIndex < 0 {
				return nil, fmt.Errorf("stack underflow")
			}
			subj, err := vm.stackObjectAt(curr, subjIndex)
			if err != nil {
				return nil, err
			}
			receiverType := subj.TypeName()
			fnIndex, ok := vm.methodFunctionIndex(receiverType, methodConst.Str)
			if !ok {
				args := make([]*runtime.Object, argc)
				for i := argc - 1; i >= 0; i-- {
					args[i] = vm.popUnsafe(curr)
				}
				subj = vm.popUnsafe(curr)
				res, err := vm.evalTraitMethodByName(subj, methodConst.Str, args)
				if err != nil {
					return nil, fmt.Errorf("unknown method: %s.%s", receiverType, methodConst.Str)
				}
				vm.push(curr, res)
				continue
			}
			fnDef := &vm.Program.Functions[fnIndex]
			if fnDef.Arity != argc+1 {
				return nil, fmt.Errorf("arity mismatch: expected %d, got %d", fnDef.Arity, argc+1)
			}
			retType, err := vm.typeFor(bytecode.TypeID(inst.C))
			if err != nil {
				return nil, err
			}
			frame, err := vm.newFrameBase(fnDef, nil, retType)
			if err != nil {
				return nil, err
			}
			for i := argc; i >= 1; i-- {
				frame.Locals[i] = vm.popUnsafe(curr)
			}
			frame.Locals[0] = vm.popUnsafe(curr)
			vm.Frames = append(vm.Frames, frame)
		case bytecode.OpCallModule:
			vm.profile.RecordModuleCall()
			modConst, err := vm.constAt(inst.A)
			if err != nil {
				return nil, err
			}
			fnConst, err := vm.constAt(inst.B)
			if err != nil {
				return nil, err
			}
			argc := inst.Imm
			if cap(vm.moduleArgsScratch) < argc {
				vm.moduleArgsScratch = make([]*runtime.Object, argc)
			}
			args := vm.moduleArgsScratch[:argc]
			for i := argc - 1; i >= 0; i-- {
				args[i] = vm.popUnsafe(curr)
			}
			retType, err := vm.typeFor(bytecode.TypeID(inst.C))
			if err != nil {
				return nil, err
			}
			vm.moduleCallScratch.Name = fnConst.Str
			vm.moduleCallScratch.ReturnType = retType
			res, err := vm.modules.Call(modConst.Str, &vm.moduleCallScratch, args)
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpCallExtern:
			if inst.A < 0 || inst.A >= len(vm.resolvedExterns) {
				return nil, fmt.Errorf("extern target out of range")
			}
			resolved := vm.resolvedExterns[inst.A]
			argc := inst.Imm
			if cap(vm.externArgsScratch) < argc {
				vm.externArgsScratch = make([]*runtime.Object, argc)
			}
			args := vm.externArgsScratch[:argc]
			for i := argc - 1; i >= 0; i-- {
				args[i] = vm.popUnsafe(curr)
			}
			retType, err := vm.typeFor(bytecode.TypeID(inst.C))
			if err != nil {
				return nil, err
			}
			start := time.Time{}
			if vm.profile != nil {
				start = time.Now()
			}
			res, err := callFFI(resolved.Binding, resolved.Func, args, retType)
			if vm.profile != nil {
				vm.profile.RecordExternCall(resolved.Binding, argc, time.Since(start))
			}
			if err != nil {
				return nil, err
			}
			vm.push(curr, res)
		case bytecode.OpToDynamic:
			val, err := vm.pop(curr)
			if err != nil {
				return nil, err
			}
			vm.push(curr, runtime.MakeDynamic(val.Raw()))
		case bytecode.OpMatchBool, bytecode.OpMatchInt, bytecode.OpMatchEnum, bytecode.OpMatchUnion,
			bytecode.OpMatchMaybe, bytecode.OpMatchResult:
			return nil, fmt.Errorf("opcode not implemented: %s", inst.Op)
		default:
			return nil, fmt.Errorf("unknown opcode: %d", inst.Op)
		}
	}

	return nil, fmt.Errorf("no frames left")
}

func (vm *VM) lookupFunction(name string) (*bytecode.Function, bool) {
	if vm.functionLookup == nil {
		lookup := map[string]int{}
		for i := range vm.Program.Functions {
			fnName := vm.Program.Functions[i].Name
			if _, exists := lookup[fnName]; !exists {
				lookup[fnName] = i
			}
		}
		vm.functionLookup = lookup
	}
	idx, ok := vm.functionLookup[name]
	if !ok {
		return nil, false
	}
	return &vm.Program.Functions[idx], true
}

func (vm *VM) spawn() *VM {
	child := New(vm.Program)
	child.modules = vm.modules
	child.methodIndex = vm.methodIndex
	child.functionLookup = vm.functionLookup
	child.ffi = vm.ffi
	child.resolvedExterns = vm.resolvedExterns
	child.initErr = vm.initErr
	return child
}

func (vm *VM) methodFunctionIndex(receiverType, methodName string) (int, bool) {
	if vm.methodIndex == nil {
		index := make(map[string]map[string]int)
		for i := range vm.Program.Functions {
			name := vm.Program.Functions[i].Name
			dot := strings.LastIndexByte(name, '.')
			if dot <= 0 || dot >= len(name)-1 {
				continue
			}
			recv := name[:dot]
			byType, ok := index[recv]
			if !ok {
				byType = map[string]int{}
				index[recv] = byType
			}
			byType[name[dot+1:]] = i
		}
		vm.methodIndex = index
	}
	byType, ok := vm.methodIndex[receiverType]
	if !ok {
		return 0, false
	}
	fnIndex, ok := byType[methodName]
	return fnIndex, ok
}

func (vm *VM) newFrameBase(fnDef *bytecode.Function, captures []any, returnType checker.Type) (*Frame, error) {
	captureLen := len(captures)
	if captures == nil {
		captureLen = 0
	}
	if captureLen != len(fnDef.Captures) {
		return nil, fmt.Errorf("capture mismatch: expected %d, got %d", len(fnDef.Captures), captureLen)
	}

	var frame *Frame
	if n := len(vm.freeFrames); n > 0 {
		frame = vm.freeFrames[n-1]
		vm.freeFrames = vm.freeFrames[:n-1]
	} else {
		frame = &Frame{}
	}

	frame.Fn = fnDef
	frame.IP = 0
	frame.MaxStack = fnDef.MaxStack
	frame.ReturnType = returnType
	if cap(frame.Locals) < fnDef.Locals {
		frame.Locals = make([]any, fnDef.Locals)
	} else {
		frame.Locals = frame.Locals[:fnDef.Locals]
		clear(frame.Locals)
	}
	if len(frame.Stack) < fnDef.MaxStack {
		frame.Stack = make([]any, fnDef.MaxStack)
	} else {
		clear(frame.Stack[:frame.StackTop])
	}
	frame.StackTop = 0

	for i, localIdx := range fnDef.Captures {
		if localIdx < 0 || localIdx >= len(frame.Locals) {
			return nil, fmt.Errorf("capture local index out of range")
		}
		frame.Locals[localIdx] = captures[i]
	}
	return frame, nil
}

func (vm *VM) newFrame(fnDef *bytecode.Function, args []any, captures []any, returnType checker.Type) (*Frame, error) {
	if len(args) != fnDef.Arity {
		return nil, fmt.Errorf("arity mismatch: expected %d, got %d", fnDef.Arity, len(args))
	}
	frame, err := vm.newFrameBase(fnDef, captures, returnType)
	if err != nil {
		return nil, err
	}
	for i := range args {
		frame.Locals[i] = args[i]
	}
	return frame, nil
}

func (vm *VM) newFrameFromStack(caller *Frame, fnDef *bytecode.Function, argc int, captures []any, returnType checker.Type) (*Frame, error) {
	if argc != fnDef.Arity {
		return nil, fmt.Errorf("arity mismatch: expected %d, got %d", fnDef.Arity, argc)
	}
	return vm.newFrameFromStackUnchecked(caller, fnDef, argc, captures, returnType)
}

func (vm *VM) newFrameFromStackUnchecked(caller *Frame, fnDef *bytecode.Function, argc int, captures []any, returnType checker.Type) (*Frame, error) {
	frame, err := vm.newFrameBase(fnDef, captures, returnType)
	if err != nil {
		return nil, err
	}
	for i := argc - 1; i >= 0; i-- {
		frame.Locals[i] = vm.popUnsafe(caller)
	}
	return frame, nil
}

func (vm *VM) runClosure(closure *Closure, args []any) (*runtime.Object, error) {
	if closure.FnIndex < 0 || closure.FnIndex >= len(vm.Program.Functions) {
		return nil, fmt.Errorf("function index out of range")
	}
	child := vm.spawn()
	fnDef := &child.Program.Functions[closure.FnIndex]
	frame, err := child.newFrame(fnDef, args, closure.Captures, closure.ReturnType)
	if err != nil {
		return nil, err
	}
	child.Frames = append(child.Frames, frame)
	return child.run()
}

func (vm *VM) runClosure1(closure *Closure, arg *runtime.Object) (*runtime.Object, error) {
	args := [1]any{arg}
	return vm.runClosure(closure, args[:])
}

func (vm *VM) recycleFrame(frame *Frame) {
	clear(frame.Locals)
	clear(frame.Stack[:frame.StackTop])
	frame.Locals = frame.Locals[:0]
	frame.StackTop = 0
	frame.ReturnType = nil
	vm.freeFrames = append(vm.freeFrames, frame)
}

func (vm *VM) push(frame *Frame, val any) {
	if frame.StackTop >= len(frame.Stack) {
		frame.Stack = append(frame.Stack, val)
		frame.StackTop++
		return
	}
	frame.Stack[frame.StackTop] = val
	frame.StackTop++
}

func (vm *VM) pop(frame *Frame) (*runtime.Object, error) {
	if frame.StackTop == 0 {
		return nil, fmt.Errorf("stack underflow")
	}
	return vm.popUnsafe(frame), nil
}

func (vm *VM) popValue(frame *Frame) (any, error) {
	if frame.StackTop == 0 {
		return nil, fmt.Errorf("stack underflow")
	}
	return vm.popValueUnsafe(frame), nil
}

func (vm *VM) popValueUnsafe(frame *Frame) any {
	frame.StackTop--
	val := frame.Stack[frame.StackTop]
	frame.Stack[frame.StackTop] = nil
	return val
}

func (vm *VM) popUnsafe(frame *Frame) *runtime.Object {
	return vm.objectFromValue(vm.popValueUnsafe(frame))
}

func (vm *VM) stackObjectAt(frame *Frame, index int) (*runtime.Object, error) {
	if index < 0 || index >= frame.StackTop {
		return nil, fmt.Errorf("stack index out of range")
	}
	return vm.objectFromValue(frame.Stack[index]), nil
}

func (vm *VM) objectFromValue(val any) *runtime.Object {
	if obj, ok := val.(*runtime.Object); ok {
		return obj
	}
	return runtime.ValueToObject(val, nil)
}

func (vm *VM) constAt(index int) (bytecode.Constant, error) {
	if index < 0 || index >= len(vm.Program.Constants) {
		return bytecode.Constant{}, fmt.Errorf("constant index out of range")
	}
	return vm.Program.Constants[index], nil
}

func (vm *VM) valueFromConst(c bytecode.Constant) (any, error) {
	switch c.Kind {
	case bytecode.ConstInt:
		return c.Int, nil
	case bytecode.ConstFloat:
		return c.Float, nil
	case bytecode.ConstStr:
		return c.Str, nil
	case bytecode.ConstBool:
		return c.Bool, nil
	default:
		return nil, fmt.Errorf("unknown constant kind: %d", c.Kind)
	}
}

func (vm *VM) evalBinaryValue(op bytecode.Opcode, left, right any) (any, bool, error) {
	switch a := left.(type) {
	case int:
		b, ok := right.(int)
		if !ok {
			return nil, false, nil
		}
		switch op {
		case bytecode.OpAdd:
			return a + b, true, nil
		case bytecode.OpSub:
			return a - b, true, nil
		case bytecode.OpMul:
			return a * b, true, nil
		case bytecode.OpDiv:
			return a / b, true, nil
		case bytecode.OpMod:
			return a % b, true, nil
		case bytecode.OpEq:
			return a == b, true, nil
		case bytecode.OpNeq:
			return a != b, true, nil
		case bytecode.OpLt:
			return a < b, true, nil
		case bytecode.OpLte:
			return a <= b, true, nil
		case bytecode.OpGt:
			return a > b, true, nil
		case bytecode.OpGte:
			return a >= b, true, nil
		}
	case float64:
		b, ok := right.(float64)
		if !ok {
			return nil, false, nil
		}
		switch op {
		case bytecode.OpAdd:
			return a + b, true, nil
		case bytecode.OpSub:
			return a - b, true, nil
		case bytecode.OpMul:
			return a * b, true, nil
		case bytecode.OpDiv:
			return a / b, true, nil
		case bytecode.OpEq:
			return a == b, true, nil
		case bytecode.OpNeq:
			return a != b, true, nil
		case bytecode.OpLt:
			return a < b, true, nil
		case bytecode.OpLte:
			return a <= b, true, nil
		case bytecode.OpGt:
			return a > b, true, nil
		case bytecode.OpGte:
			return a >= b, true, nil
		}
	case string:
		b, ok := right.(string)
		if !ok {
			return nil, false, nil
		}
		switch op {
		case bytecode.OpAdd:
			return a + b, true, nil
		case bytecode.OpEq:
			return a == b, true, nil
		case bytecode.OpNeq:
			return a != b, true, nil
		}
	case bool:
		b, ok := right.(bool)
		if !ok {
			return nil, false, nil
		}
		switch op {
		case bytecode.OpAnd:
			return a && b, true, nil
		case bytecode.OpOr:
			return a || b, true, nil
		case bytecode.OpEq:
			return a == b, true, nil
		case bytecode.OpNeq:
			return a != b, true, nil
		}
	}
	return nil, false, nil
}

func (vm *VM) evalUnaryValue(op bytecode.Opcode, val any) (any, bool, error) {
	switch value := val.(type) {
	case int:
		if op == bytecode.OpNeg {
			return -value, true, nil
		}
	case float64:
		if op == bytecode.OpNeg {
			return -value, true, nil
		}
	}
	return nil, false, nil
}

func (vm *VM) evalCompareValue(op bytecode.Opcode, left, right any) (any, bool, error) {
	return vm.evalBinaryValue(op, left, right)
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
	leftKind, rightKind := left.Kind(), right.Kind()
	if op == bytecode.OpEq || op == bytecode.OpNeq {
		if leftKind == runtime.KindStr && rightKind == runtime.KindStr {
			eq := left.AsString() == right.AsString()
			if op == bytecode.OpEq {
				return runtime.MakeBool(eq), nil
			}
			return runtime.MakeBool(!eq), nil
		}
		if leftKind == runtime.KindBool && rightKind == runtime.KindBool {
			eq := left.AsBool() == right.AsBool()
			if op == bytecode.OpEq {
				return runtime.MakeBool(eq), nil
			}
			return runtime.MakeBool(!eq), nil
		}
		if leftKind == rightKind {
			eq := left.Equals(*right)
			if op == bytecode.OpEq {
				return runtime.MakeBool(eq), nil
			}
			return runtime.MakeBool(!eq), nil
		}
		if leftKind == runtime.KindEnum && rightKind == runtime.KindInt {
			eq := left.Raw().(int) == right.AsInt()
			if op == bytecode.OpEq {
				return runtime.MakeBool(eq), nil
			}
			return runtime.MakeBool(!eq), nil
		}
		if leftKind == runtime.KindInt && rightKind == runtime.KindEnum {
			eq := left.AsInt() == right.Raw().(int)
			if op == bytecode.OpEq {
				return runtime.MakeBool(eq), nil
			}
			return runtime.MakeBool(!eq), nil
		}
		return runtime.MakeBool(false), nil
	}
	if leftKind == runtime.KindInt && rightKind == runtime.KindInt {
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
	if leftKind == runtime.KindFloat && rightKind == runtime.KindFloat {
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
	return nil, fmt.Errorf("unsupported comparison %s for %s and %s", op, leftKind, rightKind)
}
