package vm

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// deepCopy creates a deep copy of an object
func deepCopy(obj *runtime.Object) *runtime.Object {
	copy := (*obj).Copy()
	return &copy
	// todo: cheating to see if👆🏿 is cheeky enough
	// switch obj._type.(type) {
	// case *checker.StructDef:
	// 	// Deep copy struct
	// 	originalMap := obj.raw.(map[string]*runtime.Object)
	// 	copiedMap := make(map[string]*runtime.Object)
	// 	for key, value := range originalMap {
	// 		copiedMap[key] = deepCopy(value)
	// 	}
	// 	return &object{copiedMap, obj._type}
	// case *checker.List:
	// 	// Deep copy list
	// 	originalSlice := obj.raw.([]*runtime.Object)
	// 	copiedSlice := make([]*runtime.Object, len(originalSlice))
	// 	for i, value := range originalSlice {
	// 		copiedSlice[i] = deepCopy(value)
	// 	}
	// 	return &object{copiedSlice, obj._type}
	// case *checker.Map:
	// 	// Deep copy map
	// 	originalMap := obj.raw.(map[string]*runtime.Object)
	// 	copiedMap := make(map[string]*runtime.Object)
	// 	for key, value := range originalMap {
	// 		copiedMap[key] = deepCopy(value)
	// 	}
	// 	return &object{copiedMap, obj._type}
	// case *checker.Maybe:
	// 	// Deep copy Maybe - if value is nil (None), copy as-is, otherwise deep copy the value
	// 	if obj.raw == nil {
	// 		return &object{nil, obj._type}
	// 	} else {
	// 		return &object{deepCopy(obj.raw.(*runtime.Object)).raw, obj._type}
	// 	}
	// case *checker.Result:
	// 	// Deep copy Result - the value is an object containing either the success or error value
	// 	return &object{deepCopy(obj.raw.(*runtime.Object)).raw, obj._type}
	// case *checker.Enum:
	// 	// Enums are typically represented as integers or simple values, safe to copy
	// 	return &object{obj.raw, obj._type}
	// case *checker.FunctionDef:
	// 	// Functions cannot be copied - return the same function object
	// 	// Functions are immutable so sharing them is safe
	// 	return obj
	// default:
	// 	// For primitives (Str, Int, Float, Bool), return a new object with same value
	// 	// These are immutable in Ard, so we can just create a new object
	// 	return &object{obj.raw, obj._type}
	// }
}

// compareKey is a wrapper around an object to use for map keys
// enabling proper equality comparison
type compareKey struct {
	obj *runtime.Object
	// Store a string representation for hashability
	strKey string
}

func (vm *VM) Interpret(program *checker.Program) (val any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if msg, ok := r.(string); ok {
				err = fmt.Errorf("Panic: %s", msg)
			} else {
				panic(r)
			}
		}
	}()

	for _, statement := range program.Statements {
		vm.result = *vm.do(statement)
	}
	return vm.result.GoValue(), nil
}

func (vm *VM) callMain() error {
	_, err := vm.Interpret(&checker.Program{
		Statements: []checker.Statement{
			{
				Expr: &checker.FunctionCall{
					Name: "main",
					Args: []checker.Expression{},
				},
			},
		},
	})

	return err
}

// evalUserModuleFunction evaluates a function call from a user-defined module
func (vm *VM) evalUserModuleFunction(module checker.Module, call *checker.FunctionCall) *runtime.Object {
	// Look up the function in the module
	symbol := module.Get(call.Name)
	if symbol.IsZero() {
		panic(fmt.Errorf("Function %s not found in module %s", call.Name, module.Path()))
	}

	// Verify it's a function
	_, ok := symbol.Type.(*checker.FunctionDef)
	if !ok {
		panic(fmt.Errorf("%s is not a function in module %s", call.Name, module.Path()))
	}

	// Evaluate arguments
	args := make([]*runtime.Object, len(call.Args))
	for i := range call.Args {
		args[i] = vm.eval(call.Args[i])
	}

	// create new vm for module
	mvm := New(module.Program().Imports)
	// build up the module's environment
	mvm.Interpret(module.Program())
	// call the function
	return mvm.evalFunctionCall(call, args...)
}

func (vm *VM) do(stmt checker.Statement) *runtime.Object {
	if stmt.Break {
		vm.scope._break()
		return runtime.Void()
	}
	if stmt.Expr != nil {
		return vm.eval(stmt.Expr)
	}

	switch s := stmt.Stmt.(type) {
	case *checker.Enum:
		return runtime.Void()
	case *checker.VariableDef:
		val := vm.eval(s.Value)
		// can be broken by `try`
		if vm.scope.broken {
			return val
		}
		// for read-only bindings, copy the object to be safe
		if !s.Mutable {
			copy := (*val).Copy()
			val = &copy
		}
		vm.scope.add(s.Name, val)
		return runtime.Void()
	case *checker.Reassignment:
		target := vm.eval(s.Target)
		val := vm.eval(s.Value)
		target.Reassign(val)
		return runtime.Void()
	case *checker.ForLoop:
		init := func() { vm.do(checker.Statement{Stmt: s.Init}) }
		update := func() { vm.do(checker.Statement{Stmt: s.Update}) }
		for init(); vm.eval(s.Condition).AsBool(); update() {
			_, broke := vm.evalBlock(s.Body, func() { vm.scope.breakable = true })
			if broke {
				break
			}
		}
		return runtime.Void()
	case *checker.ForIntRange:
		i := vm.eval(s.Start).AsInt()
		end := vm.eval(s.End).AsInt()
		iteration := 0
		for i <= end {
			_, broke := vm.evalBlock(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, runtime.MakeInt(i))
				if s.Index != "" {
					vm.scope.add(s.Index, runtime.MakeInt(iteration))
				}
			})
			if broke {
				break
			}
			i++
			iteration++
		}
		return runtime.Void()
	case *checker.ForInStr:
		val := vm.eval(s.Value).AsString()
		for i, c := range val {
			_, broke := vm.evalBlock(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, runtime.MakeStr(string(c)))
				if s.Index != "" {
					vm.scope.add(s.Index, runtime.MakeInt(i))
				}
			})
			if broke {
				break
			}
		}
		return runtime.Void()
	case *checker.ForInList:
		list := vm.eval(s.List).AsList()
		for i := range list {
			_, broke := vm.evalBlock(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, list[i])
				if s.Index != "" {
					vm.scope.add(s.Index, runtime.MakeInt(i))
				}
			})
			if broke {
				break
			}
		}
		return runtime.Void()
	case *checker.ForInMap:
		{
			mapObj := vm.eval(s.Map)
			_map := mapObj.AsMap()
			for k, v := range _map {
				_, broke := vm.evalBlock(s.Body, func() {
					vm.scope.breakable = true
					key := mapObj.Map_GetKey(k)
					vm.scope.add(s.Key, key)
					vm.scope.add(s.Val, v)
				})
				if broke {
					break
				}
			}
			return runtime.Void()
		}
	case *checker.WhileLoop:
		for vm.eval(s.Condition).AsBool() {
			_, broke := vm.evalBlock(s.Body, func() { vm.scope.breakable = true })
			if broke {
				break
			}
		}
		return runtime.Void()
	case nil:
		return runtime.Void()
	default:
		panic(fmt.Errorf("Unimplemented statement: %T", s))
	}
}

func (vm *VM) eval(expr checker.Expression) *runtime.Object {
	switch e := expr.(type) {
	case *checker.StrLiteral:
		return runtime.MakeStr(e.Value)
	case *checker.BoolLiteral:
		return runtime.MakeBool(e.Value)
	case *checker.IntLiteral:
		return runtime.MakeInt(e.Value)
	case *checker.FloatLiteral:
		return runtime.MakeFloat(e.Value)
	case *checker.TemplateStr:
		sb := strings.Builder{}
		for i := range e.Chunks {
			// chunks implement Str::ToString
			chunk := vm.eval(&checker.InstanceMethod{
				Subject: e.Chunks[i],
				Method: &checker.FunctionCall{
					Name: "to_str",
					Args: []checker.Expression{},
				},
			}).AsString()
			sb.WriteString(chunk)
		}
		return runtime.MakeStr(sb.String())
	case *checker.Variable:
		val, ok := vm.scope.get(e.Name())
		if !ok {
			panic(fmt.Errorf("variable not found: %s", e.Name()))
		}
		return val
	case *checker.Not:
		val := vm.eval(e.Value)
		return runtime.MakeBool(!val.AsBool())

	case *checker.Negation:
		val := vm.eval(e.Value)
		if num, isInt := val.IsInt(); isInt {
			return runtime.MakeInt(-num)
		}
		return runtime.MakeFloat(-val.AsFloat())
	case *checker.StrAddition:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return runtime.MakeStr(left.AsString() + right.AsString())
	case *checker.IntAddition:
		left, right := vm.eval(e.Left).AsInt(), vm.eval(e.Right).AsInt()
		return runtime.MakeInt(left + right)
	case *checker.IntSubtraction:
		left, right := vm.eval(e.Left).AsInt(), vm.eval(e.Right).AsInt()
		return runtime.MakeInt(left - right)
	case *checker.IntMultiplication:
		left, right := vm.eval(e.Left).AsInt(), vm.eval(e.Right).AsInt()
		return runtime.MakeInt(left * right)
	case *checker.IntDivision:
		left, right := vm.eval(e.Left).AsInt(), vm.eval(e.Right).AsInt()
		return runtime.MakeInt(left / right)
	case *checker.IntModulo:
		left, right := vm.eval(e.Left).AsInt(), vm.eval(e.Right).AsInt()
		return runtime.MakeInt(left % right)
	case *checker.IntGreater:
		left, right := vm.eval(e.Left).AsInt(), vm.eval(e.Right).AsInt()
		return runtime.MakeBool(left > right)
	case *checker.IntGreaterEqual:
		left, right := vm.eval(e.Left).AsInt(), vm.eval(e.Right).AsInt()
		return runtime.MakeBool(left >= right)
	case *checker.IntLess:
		left, right := vm.eval(e.Left).AsInt(), vm.eval(e.Right).AsInt()
		return runtime.MakeBool(left < right)
	case *checker.IntLessEqual:
		left, right := vm.eval(e.Left).AsInt(), vm.eval(e.Right).AsInt()
		return runtime.MakeBool(left <= right)
	case *checker.FloatGreater:
		left, right := vm.eval(e.Left).AsFloat(), vm.eval(e.Right).AsFloat()
		return runtime.MakeBool(left > right)
	case *checker.FloatGreaterEqual:
		left, right := vm.eval(e.Left).AsFloat(), vm.eval(e.Right).AsFloat()
		return runtime.MakeBool(left >= right)
	case *checker.FloatLess:
		left, right := vm.eval(e.Left).AsFloat(), vm.eval(e.Right).AsFloat()
		return runtime.MakeBool(left < right)
	case *checker.FloatLessEqual:
		left, right := vm.eval(e.Left).AsFloat(), vm.eval(e.Right).AsFloat()
		return runtime.MakeBool(left <= right)
	case *checker.FloatDivision:
		left, right := vm.eval(e.Left).AsFloat(), vm.eval(e.Right).AsFloat()
		return runtime.MakeFloat(left / right)
	case *checker.FloatMultiplication:
		left, right := vm.eval(e.Left).AsFloat(), vm.eval(e.Right).AsFloat()
		return runtime.MakeFloat(left * right)
	case *checker.FloatSubtraction:
		left, right := vm.eval(e.Left).AsFloat(), vm.eval(e.Right).AsFloat()
		return runtime.MakeFloat(left - right)
	case *checker.FloatAddition:
		left, right := vm.eval(e.Left).AsFloat(), vm.eval(e.Right).AsFloat()
		return runtime.MakeFloat(left + right)
	case *checker.Equality:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return runtime.MakeBool(left == right)
	case *checker.And:
		left, right := vm.eval(e.Left).AsBool(), vm.eval(e.Right).AsBool()
		return runtime.MakeBool(left && right)
	case *checker.Or:
		left, right := vm.eval(e.Left).AsBool(), vm.eval(e.Right).AsBool()
		return runtime.MakeBool(left || right)
	case *checker.If:
		if cond := vm.eval(e.Condition); cond.AsBool() {
			res, _ := vm.evalBlock(e.Body, nil)
			return res
		}
		if e.ElseIf != nil && vm.eval(e.ElseIf.Condition).AsBool() {
			res, _ := vm.evalBlock(e.ElseIf.Body, nil)
			return res
		}
		if e.Else != nil {
			res, _ := vm.evalBlock(e.Else, nil)
			return res
		}
		return runtime.Void()
	case *checker.FunctionDef:
		closure := &Closure{vm: vm, expr: *e, capturedScope: vm.scope}
		obj := runtime.Make(closure, closure.Type())
		if e.Name != "" {
			vm.scope.add(e.Name, obj)
		}
		return obj
	case *checker.ExternalFunctionDef:
		// Create an external function wrapper
		extFn := &ExternalFunctionWrapper{
			vm:      vm,
			binding: e.ExternalBinding,
			def:     *e,
		}
		obj := runtime.Make(extFn, e)
		if e.Name != "" {
			vm.scope.add(e.Name, obj)
		}
		return obj
	case *checker.Panic:
		msg := vm.eval(e.Message)
		panic(fmt.Sprintf("panic at %s:\n%s", e.GetLocation().Start, msg))
	case *checker.FunctionCall:
		return vm.evalFunctionCall(e)
	case *checker.InstanceProperty:
		{
			subj := vm.eval(e.Subject)
			if subj.IsStruct() {
				return subj.Struct_Get(e.Property)
			}

			switch subj.Type() {
			case checker.Str:
				return vm.evalStrProperty(subj, e.Property)
			default:
				panic(fmt.Errorf("Unimplemented instance property: %s.%s", subj.Type(), e.Property))
			}
		}
	case *checker.InstanceMethod:
		{
			subj := vm.eval(e.Subject)
			return vm.evalInstanceMethod(subj, e)
		}
	case *checker.ModuleFunctionCall:
		{
			// first check in std lib
			if vm.moduleRegistry.HasModule(e.Module) {
				return vm.moduleRegistry.Handle(e.Module, vm, e.Call)
			}

			// Check for embedded modules with external functions
			if module, ok := vm.imports[e.Module]; ok {
				if symbol := module.Get(e.Call.Name); !symbol.IsZero() {
					// Check if this is an external function
					if extFuncDef, ok := symbol.Type.(*checker.ExternalFunctionDef); ok {
						// Create an ExternalFunctionWrapper and call it
						wrapper := ExternalFunctionWrapper{
							vm:      vm,
							binding: extFuncDef.ExternalBinding,
							def:     *extFuncDef,
						}

						// Convert call arguments to objects
						args := make([]*runtime.Object, len(e.Call.Args))
						for i, arg := range e.Call.Args {
							args[i] = vm.eval(arg)
						}

						return wrapper.eval(args...)
					}
				}
			}

			// Check for user modules (modules with function bodies)
			if module, ok := vm.imports[e.Module]; ok {
				// Check if this is a user module by seeing if the function has a body
				if symbol := module.Get(e.Call.Name); !symbol.IsZero() {
					if functionDef, ok := symbol.Type.(*checker.FunctionDef); ok && functionDef.Body != nil {
						return vm.evalUserModuleFunction(module, e.Call)
					}
				}
			}

			panic(fmt.Errorf("Unimplemented: %s::%s()", e.Module, e.Call.Name))
		}
	case *checker.ModuleStaticFunctionCall:
		{
			// Handle module static function calls like http::Response::new()
			if vm.moduleRegistry.HasModule(e.Module) {
				// Pass the struct context to the module handler
				return vm.moduleRegistry.HandleStatic(e.Module, e.Struct, vm, e.Call)
			}

			panic(fmt.Errorf("Unimplemented: %s::%s::%s()", e.Module, e.Struct, e.Call.Name))
		}
	case *checker.StaticFunctionCall:
		{
			// retrieve static definition
			def, ok := e.Scope.Statics[e.Call.Name]
			if !ok {
				panic(fmt.Errorf("Undefined function: %s()", e.Call.Name))
			}

			path := e.Scope.Name + "::" + e.Call.Name

			// if it's not yet in scope, add it
			obj, ok := vm.scope.get(path)
			if !ok {
				obj = vm.eval(def)
				vm.scope.add(path, obj)
			}

			// cast to a func
			fn := obj.Raw().(*Closure)

			args := make([]*runtime.Object, len(e.Call.Args))
			for i := range e.Call.Args {
				args[i] = vm.eval(e.Call.Args[i])
			}
			return fn.eval(args...)
		}
	case *checker.ListLiteral:
		{
			raw := make([]*runtime.Object, len(e.Elements))
			for i, el := range e.Elements {
				raw[i] = vm.eval(el)
			}
			return runtime.Make(raw, e.Type())
		}
	case *checker.MapLiteral:
		{
			mapType := e.Type().(*checker.Map)
			_map := runtime.MakeMap(mapType.Key(), mapType.Value())
			for i := range e.Keys {
				key := vm.eval(e.Keys[i])
				value := vm.eval(e.Values[i])

				_map.Map_Set(key, value)
			}
			return _map
		}
	case *checker.OptionMatch:
		{
			subject := vm.eval(e.Subject)
			if subject.Raw() == nil {
				// None case - evaluate the None block
				res, _ := vm.evalBlock(e.None, nil)
				return res
			} else {
				// Some case - bind the value and evaluate the Some block
				res, _ := vm.evalBlock(e.Some.Body, func() {
					// Bind the pattern name to the value
					subject := runtime.Make(subject.Raw(), subject.Type().(*checker.Maybe).Of())
					vm.scope.add(e.Some.Pattern.Name, subject)
				})
				return res
			}
		}
	case *checker.EnumMatch:
		{
			subject := vm.eval(e.Subject)
			variantIndex := subject.Raw().(int8)

			// If there is a catch-all case and we do not have a specific handler for this variant
			if e.CatchAll != nil && (variantIndex >= int8(len(e.Cases)) || e.Cases[variantIndex] == nil) {
				res, _ := vm.evalBlock(e.CatchAll, nil)
				return res
			}

			// Execute the matching case block for this variant
			if variantIndex < int8(len(e.Cases)) && e.Cases[variantIndex] != nil {
				res, _ := vm.evalBlock(e.Cases[variantIndex], nil)
				return res
			}

			// This should never happen if the type checker is working correctly
			// because it ensures the match is exhaustive
			panic(fmt.Errorf("No matching case for enum variant %d", variantIndex))
		}
	case *checker.EnumVariant:
		return runtime.Make(e.Variant, e.Type())
	case *checker.BoolMatch:
		{
			subject := vm.eval(e.Subject)
			value := subject.AsBool()

			// Execute the appropriate case based on the boolean value
			if value {
				res, _ := vm.evalBlock(e.True, nil)
				return res
			} else {
				res, _ := vm.evalBlock(e.False, nil)
				return res
			}
		}
	case *checker.UnionMatch:
		{
			subject := vm.eval(e.Subject)

			// Get the concrete type name as a string
			typeName := subject.AsString()

			// If we have a case for this specific type
			if block, ok := e.TypeCases[typeName]; ok {
				res, _ := vm.evalBlock(block, func() {
					// Bind the pattern variable 'it' to the value
					vm.scope.add("it", subject)
				})
				return res
			}

			// If we have a catch-all case
			if e.CatchAll != nil {
				res, _ := vm.evalBlock(e.CatchAll, nil)
				return res
			}

			// This should never happen if the type checker is working correctly
			// because it ensures the match is exhaustive
			panic(fmt.Errorf("No matching case for union type %s", typeName))
		}
	case *checker.StructInstance:
		{
			strct := e.Type().(*checker.StructDef)
			raw := map[string]*runtime.Object{}
			for name, ftype := range strct.Fields {
				val, ok := e.Fields[name]
				if ok {
					raw[name] = vm.eval(val)
				} else {
					// assume it's a $T? if the checker allowed it
					raw[name] = runtime.MakeMaybe(nil, ftype)
				}
			}
			return runtime.Make(raw, e.Type())
		}
	case *checker.ModuleStructInstance:
		{
			if e.Module == (&HTTPModule{}).Path() {
				return vm.eval(e.Property)
			}
			panic(fmt.Errorf("Unimplemented in module: %s", e.Module))
		}
	case *checker.ResultMatch:
		{
			subj := vm.eval(e.Subject)
			if subj.IsOk() {
				res, _ := vm.evalBlock(e.Ok.Body, func() {
					vm.scope.add(e.Ok.Pattern.Name, subj)
				})
				return res
			}
			res, _ := vm.evalBlock(e.Err.Body, func() {
				vm.scope.add(e.Err.Pattern.Name, subj)
			})
			return res
		}
	case *checker.IntMatch:
		{
			subject := vm.eval(e.Subject)
			intValue := subject.AsInt()

			// Check if we have a specific case for this integer
			if caseBlock, exists := e.IntCases[intValue]; exists {
				res, _ := vm.evalBlock(caseBlock, nil)
				return res
			}

			// Check if the value falls within any range
			for rangePattern, caseBlock := range e.RangeCases {
				if intValue >= rangePattern.Start && intValue <= rangePattern.End {
					res, _ := vm.evalBlock(caseBlock, nil)
					return res
				}
			}

			// If no specific case or range found, use catch-all if available
			if e.CatchAll != nil {
				res, _ := vm.evalBlock(e.CatchAll, nil)
				return res
			}

			// This should never happen if the type checker is working correctly
			panic(fmt.Errorf("No matching case for int value %d", intValue))
		}
	case *checker.TryOp:
		{
			subj := vm.eval(e.Expr())
			switch _type := subj.Type().(type) {
			case *checker.Result:
				unwrapped := runtime.Make(subj.Raw(), subj.Type())
				if subj.IsErr() {
					// Error case: early return from function
					if e.CatchBlock != nil {
						// Execute catch block and early return its result
						result, broken := vm.evalBlock(e.CatchBlock, func() {
							vm.scope.add(e.CatchVar, unwrapped)
						})

						// Early return: the catch block's result becomes the function's return value
						vm.scope.broken = true
						if broken {
							return result
						}
						return result
					} else {
						// No catch block: propagate error by early returning
						// Create a new Result with the same error for the function's return type
						vm.scope.broken = true
						return subj
					}
				}

				// Success case: always continue execution with unwrapped value
				return unwrapped
			default:
				panic(fmt.Errorf("Cannot match on %s", _type))
			}
		}
	case *checker.ModuleSymbol:
		// Handle module symbol references (like decode::string as a function value)
		if _, ok := e.Symbol.Type.(*checker.FunctionDef); ok {
			// For function symbols, we need to get the actual function object from the module
			if vm.moduleRegistry.HasModule(e.Module) {
				// Create a function call to get the function object
				call := checker.CreateCall(e.Symbol.Name, []checker.Expression{}, *e.Symbol.Type.(*checker.FunctionDef))
				return vm.moduleRegistry.Handle(e.Module, vm, call)
			}
			panic(fmt.Errorf("Module not found: %s", e.Module))
		}
		// For other symbol types (like enums), we would handle them here
		// For now, just return the symbol as-is
		return runtime.Make(e.Symbol, e.Symbol.Type)
	case *checker.CopyExpression:
		// Evaluate the expression and return a deep copy
		original := vm.eval(e.Expr)
		return deepCopy(original)
	default:
		panic(fmt.Errorf("Unimplemented expression: %T", e))
	}
}

// _args can be provided by caller from different module scopes
func (vm *VM) evalFunctionCall(call *checker.FunctionCall, _args ...*runtime.Object) *runtime.Object {
	sig, ok := vm.scope.get(call.Name)
	if !ok {
		panic(fmt.Errorf("Undefined: %s", call.Name))
	}

	// Check if it's a regular closure
	if closure, ok := sig.Raw().(*Closure); ok {
		args := _args
		// if no args are provided but the function has parameters, use the call.Args
		if len(args) == 0 && len(sig.Type().(*checker.FunctionDef).Parameters) > 0 {
			args = make([]*runtime.Object, len(call.Args))

			for i := range call.Args {
				args[i] = vm.eval(call.Args[i])
			}
		}

		return closure.eval(args...)
	}

	// Check if it's an external function
	if extFn, ok := sig.Raw().(*ExternalFunctionWrapper); ok {
		args := _args
		// if no args are provided but the function has parameters, use the call.Args
		if len(args) == 0 && len(extFn.def.Parameters) > 0 {
			args = make([]*runtime.Object, len(call.Args))

			for i := range call.Args {
				args[i] = vm.eval(call.Args[i])
			}
		}

		return extFn.eval(args...)
	}

	panic(fmt.Errorf("Not a function: %s: %s", call.Name, sig.Type()))
}

func (vm *VM) evalBlock(block *checker.Block, init func()) (*runtime.Object, bool) {
	vm.pushScope()
	defer vm.popScope()

	if init != nil {
		init()
	}

	res := runtime.Void()
	for i := range block.Stmts {
		stmt := block.Stmts[i]
		r := vm.do(stmt)
		if vm.scope.broken {
			return r, true
		}
		res = r
	}

	return res, false
}

func (vm *VM) evalStrProperty(subj *runtime.Object, name string) *runtime.Object {
	self := subj.AsString()
	switch name {
	// todo: delete this because .size() is a method
	case "size":
		return runtime.MakeInt(len(self))
	default:
		return runtime.Void()
	}
}

func (vm *VM) evalInstanceMethod(subj *runtime.Object, e *checker.InstanceMethod) *runtime.Object {
	if subj.Type() == checker.Str {
		return vm.evalStrMethod(subj, e.Method)
	}
	if _, isInt := subj.IsInt(); isInt {
		return vm.evalIntMethod(subj, e)
	}
	if subj.IsFloat() {
		return vm.evalFloatMethod(subj, e.Method)
	}
	if subj.Type() == checker.Bool {
		return vm.evalBoolMethod(subj, e)
	}
	if _, ok := subj.Type().(*checker.List); ok {
		return vm.evalListMethod(subj, e)
	}
	if _, ok := subj.Type().(*checker.Map); ok {
		return vm.evalMapMethod(subj, e)
	}
	if _, ok := subj.Type().(*checker.Maybe); ok {
		return vm.evalMaybeMethod(subj, e)
	}
	if subj.IsStruct() {
		return vm.evalStructMethod(subj, e.Method)
	}
	if subj.IsResult() {
		return vm.evalResultMethod(subj, e.Method)
	}
	if enum, ok := subj.Type().(*checker.Enum); ok {
		return vm.evalEnumMethod(subj, e.Method, enum)
	}

	panic(fmt.Errorf("Unimplemented method: %s.%s()", subj.Type(), e.Method.Name))
}

func (vm *VM) evalStrMethod(subj *runtime.Object, m *checker.FunctionCall) *runtime.Object {
	raw := subj.AsString()
	switch m.Name {
	case "size":
		return runtime.MakeInt(len(raw))
	case "is_empty":
		return runtime.MakeBool(len(raw) == 0)
	case "contains":
		return runtime.MakeBool(strings.Contains(raw, vm.eval(m.Args[0]).AsString()))
	case "split":
		sep := vm.eval(m.Args[0]).AsString()
		split := strings.Split(raw, sep)
		values := make([]*runtime.Object, len(split))
		for i, str := range split {
			values[i] = runtime.MakeStr(str)
		}
		return runtime.MakeList(checker.Str, values...)
	case "to_str":
		return subj
	case "trim":
		return runtime.MakeStr(strings.Trim(raw, " "))
	default:
		panic(fmt.Errorf(`Undefined method: "%s".%s()`, raw, m.Name))
	}
}

func (vm *VM) evalIntMethod(subj *runtime.Object, m *checker.InstanceMethod) *runtime.Object {
	switch m.Method.Name {
	case "to_str":
		return runtime.MakeStr(strconv.Itoa(subj.AsInt()))
	default:
		return runtime.Void()
	}
}

func (vm *VM) evalFloatMethod(subj *runtime.Object, m *checker.FunctionCall) *runtime.Object {
	switch m.Name {
	case "to_str":
		return runtime.MakeStr(strconv.FormatFloat(subj.AsFloat(), 'f', 2, 64))
	case "to_int":
		floatVal := subj.AsFloat()
		intVal := int(floatVal) // Truncates toward zero
		return runtime.MakeInt(intVal)
	default:
		return runtime.Void()
	}
}

func (vm *VM) evalBoolMethod(subj *runtime.Object, m *checker.InstanceMethod) *runtime.Object {
	switch m.Method.Name {
	case "to_str":
		return runtime.MakeStr(strconv.FormatBool(subj.AsBool()))
	default:
		return runtime.Void()
	}
}

func (vm *VM) evalListMethod(self *runtime.Object, m *checker.InstanceMethod) *runtime.Object {
	raw := self.AsList()
	switch m.Method.Name {
	case "at":
		index := vm.eval(m.Method.Args[0]).AsInt()
		if index >= len(raw) {
			panic(fmt.Errorf("Index out of range (%d) on list of length %d", index, len(raw)))
		}
		return raw[index]
	case "push":
		raw = append(raw, vm.eval(m.Method.Args[0]))
		self.Set(raw)
		return self
	case "set":
		index := vm.eval(m.Method.Args[0]).AsInt()
		value := vm.eval(m.Method.Args[1])
		result := runtime.MakeBool(false)
		if index <= len(raw) {
			raw[index] = value
			result.Set(true)
		}
		return result
	case "size":
		return runtime.MakeInt(len(raw))
	case "sort":
		{
			_isLess := vm.eval(m.Method.Args[0]).Raw().(*Closure)

			slices.SortFunc(raw, func(a, b *runtime.Object) int {
				if _isLess.eval(a, b).AsBool() {
					return -1
				}
				return 0
			})

			return runtime.Void()
		}
	case "swap":
		l := vm.eval(m.Method.Args[0]).AsInt()
		r := vm.eval(m.Method.Args[1]).AsInt()
		_l, _r := raw[l], raw[r]
		raw[l] = _r
		raw[r] = _l
		return runtime.Void()
	default:
		panic(fmt.Errorf("Unimplemented: List.%s()", m.Method.Name))
	}
}

func (vm *VM) evalMapMethod(subj *runtime.Object, m *checker.InstanceMethod) *runtime.Object {
	raw := subj.AsMap()
	switch m.Method.Name {
	case "keys":
		keys := make([]*runtime.Object, len(raw))
		i := 0
		for k := range raw {
			keys[i] = subj.Map_GetKey(k)
			i += 1
		}
		return runtime.MakeList(checker.Str, keys...)
	case "size":
		return runtime.MakeInt(len(raw))
	case "get":
		keyArg := vm.eval(m.Method.Args[0])
		_key := runtime.ToMapKey(keyArg)

		mapType := subj.Type().(*checker.Map)
		// Try to find the key
		value, found := raw[_key]
		if !found {
			// Return nil for the maybe type
			return runtime.MakeMaybe(nil, mapType.Value())
		}
		return runtime.MakeMaybe(value.Raw(), mapType.Value())
	case "set":
		keyArg := vm.eval(m.Method.Args[0])
		valueArg := vm.eval(m.Method.Args[1])

		keyStr := runtime.ToMapKey(keyArg)
		raw[keyStr] = valueArg
		return runtime.MakeBool(true)
	case "drop":
		keyArg := vm.eval(m.Method.Args[0])
		keyStr := runtime.ToMapKey(keyArg)

		delete(raw, keyStr)
		return runtime.Void()
	case "has":
		keyArg := vm.eval(m.Method.Args[0])

		// Convert key to string
		keyStr := runtime.ToMapKey(keyArg)
		_, found := raw[keyStr]
		return runtime.MakeBool(found)
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj.Type(), m.Method.Name))
	}
}

func (vm *VM) evalMaybeMethod(subj *runtime.Object, m *checker.InstanceMethod) *runtime.Object {
	switch m.Method.Name {
	case "is_none":
		return runtime.MakeBool(subj.Raw() == nil)
	case "is_some":
		return runtime.MakeBool(subj.Raw() != nil)
	case "or":
		if subj.Raw() == nil {
			return vm.eval(m.Method.Args[0])
		}
		return runtime.Make(subj.Raw(), m.Type())
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj.Type(), m.Method.Name))
	}
}

func (vm *VM) evalStructMethod(subj *runtime.Object, call *checker.FunctionCall) *runtime.Object {
	// Special handling for HTTP Response methods
	if subj.Type() == checker.HttpResponseDef {
		http := vm.moduleRegistry.handlers[checker.HttpPkg{}.Path()].(*HTTPModule)
		args := make([]*runtime.Object, len(call.Args))
		for i := range call.Args {
			args[i] = vm.eval(call.Args[i])
		}
		return http.evalHttpResponseMethod(subj, call, args)
	}
	if subj.Type() == checker.HttpRequestDef {
		http := vm.moduleRegistry.handlers[checker.HttpPkg{}.Path()].(*HTTPModule)
		args := make([]*runtime.Object, len(call.Args))
		for i := range call.Args {
			args[i] = vm.eval(call.Args[i])
		}
		return http.evalHttpRequestMethod(subj, call, args)
	}
	// Special handling for SQLite Database methods
	if subj.Type() == checker.DatabaseDef {
		sqlite := vm.moduleRegistry.handlers[checker.SQLitePkg{}.Path()].(*SQLiteModule)
		args := make([]*runtime.Object, len(call.Args))
		for i := range call.Args {
			args[i] = vm.eval(call.Args[i])
		}
		return sqlite.evalDatabaseMethod(subj, call, args)
	}
	// Special handling for Fiber methods
	if subj.Type() == checker.Fiber {
		async := vm.moduleRegistry.handlers[checker.AsyncPkg{}.Path()].(*AsyncModule)
		args := make([]*runtime.Object, len(call.Args))
		for i := range call.Args {
			args[i] = vm.eval(call.Args[i])
		}
		return async.EvalFiberMethod(subj, call, args)
	}
	// Special handling for decode::Error methods
	if subj.Type() == checker.DecodeErrorDef {
		switch call.Name {
		case "to_str":
			expected := subj.Struct_Get("expected").AsString()
			found := subj.Struct_Get("found").AsString()
			pathList := subj.Struct_Get("path").AsList()

			pathStr := ""
			if len(pathList) > 0 {
				var pathBuilder strings.Builder
				for i, part := range pathList {
					partStr := part.AsString()
					if i == 0 {
						// First element: no leading dot
						pathBuilder.WriteString(partStr)
					} else {
						// Subsequent elements: add dot only if not starting with bracket
						if strings.HasPrefix(partStr, "[") {
							pathBuilder.WriteString(partStr)
						} else {
							pathBuilder.WriteString(".")
							pathBuilder.WriteString(partStr)
						}
					}
				}
				pathStr = " at " + pathBuilder.String()
			}

			errorMsg := "Decode error: expected " + expected + ", found " + found + pathStr
			return runtime.MakeStr(errorMsg)
		}
	}

	var sig checker.Type
	var ok bool

	istruct := subj.Type().(*checker.StructDef)
	// Check for methods first
	if method, exists := istruct.Methods[call.Name]; exists {
		sig = method
		ok = true
	} else {
		// Fall back to fields (for backward compatibility)
		sig, ok = istruct.Fields[call.Name]
	}

	if !ok {
		panic(fmt.Errorf("Undefined: %s.%s", istruct.Name, call.Name))
	}

	fnDef, ok := sig.(*checker.FunctionDef)
	if !ok {
		panic(fmt.Errorf("Not a function: %s.%s", istruct.Name, call.Name))
	}

	fn := func(args ...*runtime.Object) *runtime.Object {
		res, _ := vm.evalBlock(fnDef.Body, func() {
			vm.scope.add("@", subj)
			for i := range args {
				vm.scope.add(fnDef.Parameters[i].Name, args[i])
			}
		})
		return res
	}

	args := make([]*runtime.Object, len(call.Args))
	for i := range call.Args {
		args[i] = vm.eval(call.Args[i])
	}

	return fn(args...)
}

func (vm *VM) evalEnumMethod(self *runtime.Object, method *checker.FunctionCall, enum *checker.Enum) *runtime.Object {
	switch method.Name {
	case "to_str":
		// Special handling for http::Method enum
		if enum.Name == "Method" {
			variantIndex := self.Raw().(int8)
			if variantIndex >= 0 && int(variantIndex) < len(enum.Variants) {
				// Map enum variants to HTTP method strings
				methodStrings := map[string]string{
					"Get":     "GET",
					"Post":    "POST",
					"Put":     "PUT",
					"Del":     "DELETE",
					"Patch":   "PATCH",
					"Options": "OPTIONS",
				}
				variantName := enum.Variants[variantIndex]
				if methodStr, ok := methodStrings[variantName]; ok {
					return runtime.MakeStr(methodStr)
				}
			}
		}
		// Default behavior: return the variant name as string
		variantIndex := self.Raw().(int8)
		if variantIndex >= 0 && int(variantIndex) < len(enum.Variants) {
			return runtime.MakeStr(enum.Variants[variantIndex])
		}
		return runtime.MakeStr("")
	default:
		panic(fmt.Errorf("Undefined method: %s.%s()", enum.Name, method.Name))
	}
}
