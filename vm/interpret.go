package vm

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/akonwi/ard/checker"
)

var void = &object{nil, checker.Void}

// deepCopy creates a deep copy of an object
func deepCopy(obj *object) *object {
	switch obj._type.(type) {
	case *checker.StructDef:
		// Deep copy struct
		originalMap := obj.raw.(map[string]*object)
		copiedMap := make(map[string]*object)
		for key, value := range originalMap {
			copiedMap[key] = deepCopy(value)
		}
		return &object{copiedMap, obj._type}
	case *checker.List:
		// Deep copy list
		originalSlice := obj.raw.([]*object)
		copiedSlice := make([]*object, len(originalSlice))
		for i, value := range originalSlice {
			copiedSlice[i] = deepCopy(value)
		}
		return &object{copiedSlice, obj._type}
	case *checker.Map:
		// Deep copy map
		originalMap := obj.raw.(map[string]*object)
		copiedMap := make(map[string]*object)
		for key, value := range originalMap {
			copiedMap[key] = deepCopy(value)
		}
		return &object{copiedMap, obj._type}
	case *checker.Maybe:
		// Deep copy Maybe - if value is nil (None), copy as-is, otherwise deep copy the value
		if obj.raw == nil {
			return &object{nil, obj._type}
		} else {
			return &object{deepCopy(obj.raw.(*object)).raw, obj._type}
		}
	case *checker.Result:
		// Deep copy Result - the value is an object containing either the success or error value
		return &object{deepCopy(obj.raw.(*object)).raw, obj._type}
	case *checker.Enum:
		// Enums are typically represented as integers or simple values, safe to copy
		return &object{obj.raw, obj._type}
	case *checker.FunctionDef:
		// Functions cannot be copied - return the same function object
		// Functions are immutable so sharing them is safe
		return obj
	default:
		// For primitives (Str, Int, Float, Bool), return a new object with same value
		// These are immutable in Ard, so we can just create a new object
		return &object{obj.raw, obj._type}
	}
}

// compareKey is a wrapper around an object to use for map keys
// enabling proper equality comparison
type compareKey struct {
	obj *object
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
	if r, isResult := vm.result.raw.(_result); isResult {
		return r.raw.premarshal(), nil
	}
	return vm.result.premarshal(), nil
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
func (vm *VM) evalUserModuleFunction(module checker.Module, call *checker.FunctionCall) *object {
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
	args := make([]*object, len(call.Args))
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

func (vm *VM) do(stmt checker.Statement) *object {
	if stmt.Break {
		vm.scope._break()
		return void
	}
	if stmt.Expr != nil {
		return vm.eval(stmt.Expr)
	}

	switch s := stmt.Stmt.(type) {
	case *checker.Enum:
		return void
	case *checker.VariableDef:
		val := vm.eval(s.Value)
		// can be broken by `try`
		if vm.scope.broken {
			return val
		}
		if !s.Mutable {
			original := val.raw
			var copy any = new(any)
			copy = original
			val.raw = copy
		}
		vm.scope.add(s.Name, val)
		return void
	case *checker.Reassignment:
		target := vm.eval(s.Target)
		val := vm.eval(s.Value)
		target.raw = val.raw

		// Update target type to match value type
		target._type = val._type
		return void
	case *checker.ForLoop:
		init := func() { vm.do(checker.Statement{Stmt: s.Init}) }
		update := func() { vm.do(checker.Statement{Stmt: s.Update}) }
		for init(); vm.eval(s.Condition).raw.(bool); update() {
			_, broke := vm.evalBlock(s.Body, func() { vm.scope.breakable = true })
			if broke {
				break
			}
		}
		return void
	case *checker.ForIntRange:
		i := vm.eval(s.Start).raw.(int)
		end := vm.eval(s.End).raw.(int)
		iteration := 0
		for i <= end {
			_, broke := vm.evalBlock(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, &object{i, checker.Int})
				if s.Index != "" {
					vm.scope.add(s.Index, &object{iteration, checker.Int})
				}
			})
			if broke {
				break
			}
			i++
			iteration++
		}
		return void
	case *checker.ForInStr:
		val := vm.eval(s.Value).raw.(string)
		for i, c := range val {
			_, broke := vm.evalBlock(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, &object{string(c), checker.Str})
				if s.Index != "" {
					vm.scope.add(s.Index, &object{i, checker.Int})
				}
			})
			if broke {
				break
			}
		}
		return void
	case *checker.ForInList:
		val := vm.eval(s.List).raw.([]*object)
		for i := range val {
			_, broke := vm.evalBlock(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, val[i])
				if s.Index != "" {
					vm.scope.add(s.Index, &object{i, checker.Int})
				}
			})
			if broke {
				break
			}
		}
		return void
	case *checker.ForInMap:
		{
			mapObj := vm.eval(s.Map)
			_map := mapObj.raw.(map[string]*object)
			for k, v := range _map {
				_, broke := vm.evalBlock(s.Body, func() {
					vm.scope.breakable = true

					// parse raw key string into Ard type
					keyType := mapObj._type.(*checker.Map).Key()
					key := &object{nil, keyType}

					switch keyType.String() {
					case checker.Str.String():
						key.raw = k
					case checker.Int.String():
						if _num, err := strconv.Atoi(k); err != nil {
							panic(fmt.Errorf("Couldn't turn map key %s into int", k))
						} else {
							key.raw = _num
						}
					case checker.Bool.String():
						if _bool, err := strconv.ParseBool(k); err != nil {
							panic(fmt.Errorf("Couldn't turn map key %s into bool", k))
						} else {
							key.raw = _bool
						}
					case checker.Float.String():
						if _float, err := strconv.ParseFloat(k, 64); err != nil {
							panic(fmt.Errorf("Couldn't turn map key %s into float", k))
						} else {
							key.raw = _float
						}
					default:
						panic(fmt.Errorf("Unsupported map key: %s", keyType))
					}
					vm.scope.add(s.Key, key)
					vm.scope.add(s.Val, v)
				})
				if broke {
					break
				}
			}
			return void
		}
	case *checker.WhileLoop:
		for vm.eval(s.Condition).raw.(bool) {
			_, broke := vm.evalBlock(s.Body, func() { vm.scope.breakable = true })
			if broke {
				break
			}
		}
		return void
	case nil:
		return void
	default:
		panic(fmt.Errorf("Unimplemented statement: %T", s))
	}
}

func (vm *VM) eval(expr checker.Expression) *object {
	switch e := expr.(type) {
	case *checker.StrLiteral:
		return &object{e.Value, e.Type()}
	case *checker.BoolLiteral:
		return &object{e.Value, e.Type()}
	case *checker.IntLiteral:
		return &object{e.Value, e.Type()}
	case *checker.FloatLiteral:
		return &object{e.Value, e.Type()}
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
			}).raw.(string)
			sb.WriteString(chunk)
		}
		return &object{sb.String(), checker.Str}
	case *checker.Variable:
		val, ok := vm.scope.get(e.Name())
		if !ok {
			panic(fmt.Errorf("variable not found: %s", e.Name()))
		}
		return val
	case *checker.Not:
		val := vm.eval(e.Value)
		return &object{!val.raw.(bool), val._type}

	case *checker.Negation:
		val := vm.eval(e.Value)
		if num, isInt := val.raw.(int); isInt {
			return &object{-num, val._type}
		}
		return &object{-val.raw.(float64), val._type}
	case *checker.StrAddition:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(string) + right.raw.(string),
			left._type,
		}
	case *checker.IntAddition:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) + right.raw.(int),
			left._type,
		}
	case *checker.IntSubtraction:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) - right.raw.(int),
			left._type,
		}
	case *checker.IntMultiplication:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) * right.raw.(int),
			left._type,
		}
	case *checker.IntDivision:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) / right.raw.(int),
			left._type,
		}
	case *checker.IntModulo:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) % right.raw.(int),
			left._type,
		}
	case *checker.IntGreater:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) > right.raw.(int),
			checker.Bool,
		}
	case *checker.IntGreaterEqual:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) >= right.raw.(int),
			checker.Bool,
		}
	case *checker.IntLess:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) < right.raw.(int),
			checker.Bool,
		}
	case *checker.IntLessEqual:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) <= right.raw.(int),
			checker.Bool,
		}
	case *checker.FloatDivision:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) / right.raw.(float64),
			left._type,
		}
	case *checker.FloatMultiplication:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) * right.raw.(float64),
			left._type,
		}
	case *checker.FloatSubtraction:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) - right.raw.(float64),
			left._type,
		}
	case *checker.FloatAddition:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) + right.raw.(float64),
			left._type,
		}
	case *checker.Equality:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw == right.raw, checker.Bool}
	case *checker.And:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw.(bool) && right.raw.(bool), checker.Bool}
	case *checker.Or:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw.(bool) || right.raw.(bool), checker.Bool}
	case *checker.If:
		if cond := vm.eval(e.Condition); cond.raw.(bool) {
			res, _ := vm.evalBlock(e.Body, nil)
			return res
		}
		if e.ElseIf != nil && vm.eval(e.ElseIf.Condition).raw.(bool) {
			res, _ := vm.evalBlock(e.ElseIf.Body, nil)
			return res
		}
		if e.Else != nil {
			res, _ := vm.evalBlock(e.Else, nil)
			return res
		}
		return void
	case *checker.FunctionDef:
		closure := &Closure{vm: vm, expr: *e}
		obj := &object{closure, closure.Type()}
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
		obj := &object{extFn, e}
		if e.Name != "" {
			vm.scope.add(e.Name, obj)
		}
		return obj
	case *checker.Panic:
		msg := vm.eval(e.Message)
		panic(fmt.Sprintf("panic at %s:\n%s", e.GetLocation().Start, msg.raw))
	case *checker.FunctionCall:
		return vm.evalFunctionCall(e)
	case *checker.InstanceProperty:
		{
			subj := vm.eval(e.Subject)
			_type := subj._type

			if _, ok := _type.(*checker.StructDef); ok {
				raw := subj.raw.(map[string]*object)
				return raw[e.Property]
			}

			switch _type {
			case checker.Str:
				return vm.evalStrProperty(subj, e.Property)
			default:
				panic(fmt.Errorf("Unimplemented instance property: %s.%s", _type, e.Property))
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
			fn := obj.raw.(*Closure)

			args := make([]*object, len(e.Call.Args))
			for i := range e.Call.Args {
				args[i] = vm.eval(e.Call.Args[i])
			}
			return fn.eval(args...)
		}
	case *checker.ListLiteral:
		{
			raw := make([]*object, len(e.Elements))
			for i, el := range e.Elements {
				raw[i] = vm.eval(el)
			}
			return &object{raw, e.Type()}
		}
	case *checker.MapLiteral:
		{
			raw := make(map[string]*object)
			for i := range e.Keys {
				key := vm.eval(e.Keys[i])
				value := vm.eval(e.Values[i])

				// Create a string representation for the key
				var keyStr string
				switch v := key.raw.(type) {
				case string:
					keyStr = v
				case int:
					keyStr = strconv.Itoa(v)
				case bool:
					keyStr = strconv.FormatBool(v)
				case float64:
					keyStr = strconv.FormatFloat(v, 'g', -1, 64)
				default:
					// For complex types use the pointer address
					keyStr = fmt.Sprintf("%p", key.raw)
				}

				raw[keyStr] = value
			}
			return &object{raw, e.Type()}
		}
	case *checker.OptionMatch:
		{
			subject := vm.eval(e.Subject)
			if subject.raw == nil {
				// None case - evaluate the None block
				res, _ := vm.evalBlock(e.None, nil)
				return res
			} else {
				// Some case - bind the value and evaluate the Some block
				res, _ := vm.evalBlock(e.Some.Body, func() {
					// Bind the pattern name to the value
					subject := &object{subject.raw, subject._type.(*checker.Maybe).Of()}
					vm.scope.add(e.Some.Pattern.Name, subject)
				})
				return res
			}
		}
	case *checker.EnumMatch:
		{
			subject := vm.eval(e.Subject)
			variantIndex := subject.raw.(int8)

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
		return &object{e.Variant, e.Type()}
	case *checker.BoolMatch:
		{
			subject := vm.eval(e.Subject)
			value := subject.raw.(bool)

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
			typeName := subject._type.String()

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
			raw := map[string]*object{}
			for name, ftype := range strct.Fields {
				val, ok := e.Fields[name]
				if ok {
					raw[name] = vm.eval(val)
				} else {
					// assume it's a Maybe<T> if the checker allowed it
					raw[name] = &object{nil, ftype}
				}
			}
			return &object{raw, e.Type()}
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
			raw := subj.raw.(_result)

			if raw.ok {
				res, _ := vm.evalBlock(e.Ok.Body, func() {
					vm.scope.add(e.Ok.Pattern.Name, raw.raw)
				})
				return res
			}
			res, _ := vm.evalBlock(e.Err.Body, func() {
				vm.scope.add(e.Err.Pattern.Name, raw.raw)
			})
			return res
		}
	case *checker.IntMatch:
		{
			subject := vm.eval(e.Subject)
			intValue := subject.raw.(int)

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
			switch _type := subj._type.(type) {
			case *checker.Result:
				raw := subj.raw.(_result)
				if !raw.ok {
					// Error case: early return from function
					if e.CatchBlock != nil {
						// Execute catch block and early return its result
						result, broken := vm.evalBlock(e.CatchBlock, func() {
							vm.scope.add(e.CatchVar, raw.raw)
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
				return raw.raw
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
		return &object{e.Symbol, e.Symbol.Type}
	case *checker.CopyExpression:
		// Evaluate the expression and return a deep copy
		original := vm.eval(e.Expr)
		return deepCopy(original)
	default:
		panic(fmt.Errorf("Unimplemented expression: %T", e))
	}
}

// _args can be provided by caller from different module scopes
func (vm *VM) evalFunctionCall(call *checker.FunctionCall, _args ...*object) *object {
	sig, ok := vm.scope.get(call.Name)
	if !ok {
		panic(fmt.Errorf("Undefined: %s", call.Name))
	}

	// Check if it's a regular closure
	if closure, ok := sig.raw.(*Closure); ok {
		args := _args
		// if no args are provided but the function has parameters, use the call.Args
		if len(args) == 0 && len(sig._type.(*checker.FunctionDef).Parameters) > 0 {
			args = make([]*object, len(call.Args))

			for i := range call.Args {
				args[i] = vm.eval(call.Args[i])
			}
		}

		return closure.eval(args...)
	}

	// Check if it's an external function
	if extFn, ok := sig.raw.(*ExternalFunctionWrapper); ok {
		args := _args
		// if no args are provided but the function has parameters, use the call.Args
		if len(args) == 0 && len(extFn.def.Parameters) > 0 {
			args = make([]*object, len(call.Args))

			for i := range call.Args {
				args[i] = vm.eval(call.Args[i])
			}
		}

		return extFn.eval(args...)
	}

	panic(fmt.Errorf("Not a function: %s: %s", call.Name, sig._type))
}

func (vm *VM) evalBlock(block *checker.Block, init func()) (*object, bool) {
	vm.pushScope()
	defer vm.popScope()

	if init != nil {
		init()
	}

	res := void
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

func (vm *VM) evalStrProperty(subj *object, name string) *object {
	switch name {
	case "size":
		return &object{len(subj.raw.(string)), checker.Int}
	default:
		return void
	}
}

func (vm *VM) evalInstanceMethod(self *object, e *checker.InstanceMethod) *object {
	if self._type == checker.Str {
		return vm.evalStrMethod(self, e.Method)
	}
	if self._type == checker.Int {
		return vm.evalIntMethod(self, e)
	}
	if self._type == checker.Float {
		return vm.evalFloatMethod(self, e.Method)
	}
	if self._type == checker.Bool {
		return vm.evalBoolMethod(self, e)
	}
	if _, ok := self._type.(*checker.List); ok {
		return vm.evalListMethod(self, e)
	}
	if _, ok := self._type.(*checker.Map); ok {
		return vm.evalMapMethod(self, e)
	}
	if _, ok := self._type.(*checker.Maybe); ok {
		return vm.evalMaybeMethod(self, e)
	}
	if _, ok := self._type.(*checker.StructDef); ok {
		return vm.evalStructMethod(self, e.Method)
	}
	if _, ok := self._type.(*checker.Result); ok {
		return vm.evalResultMethod(self, e.Method)
	}
	if enum, ok := self._type.(*checker.Enum); ok {
		return vm.evalEnumMethod(self, e.Method, enum)
	}

	panic(fmt.Errorf("Unimplemented: %s.%s() on %T", self._type, e.Method.Name, self._type))
}

func (vm *VM) evalStrMethod(subj *object, m *checker.FunctionCall) *object {
	raw := subj.raw.(string)
	switch m.Name {
	case "size":
		return &object{len(raw), m.Type()}
	case "is_empty":
		return &object{len(raw) == 0, m.Type()}
	case "contains":
		return &object{strings.Contains(raw, vm.eval(m.Args[0]).raw.(string)), m.Type()}
	case "split":
		sep := vm.eval(m.Args[0]).raw.(string)
		_list := strings.Split(raw, sep)
		list := make([]*object, len(_list))

		for i, str := range _list {
			list[i] = &object{str, checker.Str}
		}

		return &object{list, m.Type()}
	case "to_str":
		return subj
	case "trim":
		return &object{strings.Trim(raw, " "), m.Type()}
	default:
		panic(fmt.Errorf(`Undefined method: "%s".%s()`, raw, m.Name))
	}
}

func (vm *VM) evalIntMethod(subj *object, m *checker.InstanceMethod) *object {
	switch m.Method.Name {
	case "to_str":
		return &object{strconv.Itoa(subj.raw.(int)), checker.Str}
	default:
		return void
	}
}

func (vm *VM) evalFloatMethod(subj *object, m *checker.FunctionCall) *object {
	switch m.Name {
	case "to_str":
		return &object{strconv.FormatFloat(subj.raw.(float64), 'f', 2, 64), checker.Str}
	default:
		return void
	}
}

func (vm *VM) evalBoolMethod(subj *object, m *checker.InstanceMethod) *object {
	switch m.Method.Name {
	case "to_str":
		return &object{strconv.FormatBool(subj.raw.(bool)), checker.Str}
	default:
		return void
	}
}

func (vm *VM) evalListMethod(self *object, m *checker.InstanceMethod) *object {
	raw := self.raw.([]*object)
	switch m.Method.Name {
	case "at":
		index := vm.eval(m.Method.Args[0]).raw.(int)
		if index >= len(raw) {
			panic(fmt.Errorf("Index out of range (%d) on list of length %d", index, len(raw)))
		}
		return &object{raw[index].raw, m.Type()}
	case "push":
		self.raw = append(raw, vm.eval(m.Method.Args[0]))
		return self
	case "set":
		index := vm.eval(m.Method.Args[0]).raw.(int)
		value := vm.eval(m.Method.Args[1])
		result := &object{false, checker.Bool}
		if index <= len(raw) {
			raw[index] = value
			result.raw = true
		}
		return result
	case "size":
		return &object{len(raw), checker.Int}
	case "sort":
		{
			_isLess := vm.eval(m.Method.Args[0]).raw.(*Closure)
			slices.SortFunc(raw, func(a, b *object) int {
				if _isLess.eval(a, b).raw.(bool) {
					return -1
				}
				return 0
			})
			return void
		}
	case "swap":
		l := vm.eval(m.Method.Args[0]).raw.(int)
		r := vm.eval(m.Method.Args[1]).raw.(int)
		_l, _r := raw[l], raw[r]
		raw[l] = _r
		raw[r] = _l
		return void
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", self._type, m.Method.Name))
	}
}

func (vm *VM) evalMapMethod(subj *object, m *checker.InstanceMethod) *object {
	raw := subj.raw.(map[string]*object)
	switch m.Method.Name {
	case "keys":
		keys := make([]*object, len(raw))
		i := 0
		for k := range raw {
			keys[i] = &object{k, checker.Str}
			i += 1
		}
		return &object{keys, m.Type()}
	case "size":
		return &object{len(raw), checker.Int}
	case "get":
		keyArg := vm.eval(m.Method.Args[0])

		// Convert key to string
		var keyStr string
		switch v := keyArg.raw.(type) {
		case string:
			keyStr = v
		case int:
			keyStr = strconv.Itoa(v)
		case bool:
			keyStr = strconv.FormatBool(v)
		case float64:
			keyStr = strconv.FormatFloat(v, 'g', -1, 64)
		default:
			keyStr = fmt.Sprintf("%p", keyArg.raw)
		}

		// Try to find the key
		value, found := raw[keyStr]
		if !found {
			// Return nil for the maybe type
			return &object{nil, m.Type()}
		}
		return &object{value.raw, m.Type()}
	case "set":
		keyArg := vm.eval(m.Method.Args[0])
		valueArg := vm.eval(m.Method.Args[1])

		// Convert key to string
		var keyStr string
		switch v := keyArg.raw.(type) {
		case string:
			keyStr = v
		case int:
			keyStr = strconv.Itoa(v)
		case bool:
			keyStr = strconv.FormatBool(v)
		case float64:
			keyStr = strconv.FormatFloat(v, 'g', -1, 64)
		default:
			keyStr = fmt.Sprintf("%p", keyArg.raw)
		}

		// Add or update the entry
		raw[keyStr] = valueArg
		// Return success
		return &object{true, checker.Bool}
	case "drop":
		keyArg := vm.eval(m.Method.Args[0])

		// Convert key to string
		var keyStr string
		switch v := keyArg.raw.(type) {
		case string:
			keyStr = v
		case int:
			keyStr = strconv.Itoa(v)
		case bool:
			keyStr = strconv.FormatBool(v)
		case float64:
			keyStr = strconv.FormatFloat(v, 'g', -1, 64)
		default:
			keyStr = fmt.Sprintf("%p", keyArg.raw)
		}

		// Remove the entry
		delete(raw, keyStr)
		return void
	case "has":
		keyArg := vm.eval(m.Method.Args[0])

		// Convert key to string
		var keyStr string
		switch v := keyArg.raw.(type) {
		case string:
			keyStr = v
		case int:
			keyStr = strconv.Itoa(v)
		case bool:
			keyStr = strconv.FormatBool(v)
		case float64:
			keyStr = strconv.FormatFloat(v, 'g', -1, 64)
		default:
			keyStr = fmt.Sprintf("%p", keyArg.raw)
		}

		// Check if the key exists
		_, found := raw[keyStr]
		return &object{found, checker.Bool}
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj._type, m.Method.Name))
	}
}

func (vm *VM) evalMaybeMethod(subj *object, m *checker.InstanceMethod) *object {
	switch m.Method.Name {
	case "is_none":
		return &object{subj.raw == nil, m.Type()}
	case "is_some":
		return &object{subj.raw != nil, m.Type()}
	case "or":
		if subj.raw == nil {
			return vm.eval(m.Method.Args[0])
		}
		return &object{subj.raw, m.Type()}
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj._type, m.Method.Name))
	}
}

func (vm *VM) evalStructMethod(subj *object, call *checker.FunctionCall) *object {
	istruct := subj._type.(*checker.StructDef)

	// Special handling for HTTP Response methods
	if istruct == checker.HttpResponseDef {
		http := vm.moduleRegistry.handlers[checker.HttpPkg{}.Path()].(*HTTPModule)
		args := make([]*object, len(call.Args))
		for i := range call.Args {
			args[i] = vm.eval(call.Args[i])
		}
		return http.evalHttpResponseMethod(subj, call, args)
	}
	if istruct == checker.HttpRequestDef {
		http := vm.moduleRegistry.handlers[checker.HttpPkg{}.Path()].(*HTTPModule)
		args := make([]*object, len(call.Args))
		for i := range call.Args {
			args[i] = vm.eval(call.Args[i])
		}
		return http.evalHttpRequestMethod(subj, call, args)
	}
	// Special handling for SQLite Database methods
	if istruct == checker.DatabaseDef {
		sqlite := vm.moduleRegistry.handlers[checker.SQLitePkg{}.Path()].(*SQLiteModule)
		args := make([]*object, len(call.Args))
		for i := range call.Args {
			args[i] = vm.eval(call.Args[i])
		}
		return sqlite.evalDatabaseMethod(subj, call, args)
	}
	// Special handling for Fiber methods
	if istruct == checker.Fiber {
		async := vm.moduleRegistry.handlers[checker.AsyncPkg{}.Path()].(*AsyncModule)
		args := make([]*object, len(call.Args))
		for i := range call.Args {
			args[i] = vm.eval(call.Args[i])
		}
		return async.EvalFiberMethod(subj, call, args)
	}
	// Special handling for decode::Error methods
	if istruct == checker.DecodeErrorDef {
		switch call.Name {
		case "to_str":
			structMap := subj.raw.(map[string]*object)
			expected := structMap["expected"].raw.(string)
			found := structMap["found"].raw.(string)
			pathList := structMap["path"].raw.([]*object)

			pathStr := ""
			if len(pathList) > 0 {
				var pathBuilder strings.Builder
				for i, part := range pathList {
					partStr := part.raw.(string)
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
			return &object{errorMsg, checker.Str}
		}
	}

	var sig checker.Type
	var ok bool

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

	fn := func(args ...*object) *object {
		res, _ := vm.evalBlock(fnDef.Body, func() {
			vm.scope.add("@", subj)
			for i := range args {
				vm.scope.add(fnDef.Parameters[i].Name, args[i])
			}
		})
		return res
	}

	args := make([]*object, len(call.Args))
	for i := range call.Args {
		args[i] = vm.eval(call.Args[i])
	}

	return fn(args...)
}

func (vm *VM) evalEnumMethod(self *object, method *checker.FunctionCall, enum *checker.Enum) *object {
	switch method.Name {
	case "to_str":
		// Special handling for http::Method enum
		if enum.Name == "Method" {
			variantIndex := self.raw.(int8)
			if variantIndex >= 0 && int(variantIndex) < len(enum.Variants) {
				// Map enum variants to HTTP method strings
				methodStrings := map[string]string{
					"Get":   "GET",
					"Post":  "POST",
					"Put":   "PUT",
					"Del":   "DELETE",
					"Patch": "PATCH",
				}
				variantName := enum.Variants[variantIndex]
				if methodStr, ok := methodStrings[variantName]; ok {
					return &object{methodStr, checker.Str}
				}
			}
		}
		// Default behavior: return the variant name as string
		variantIndex := self.raw.(int8)
		if variantIndex >= 0 && int(variantIndex) < len(enum.Variants) {
			return &object{enum.Variants[variantIndex], checker.Str}
		}
		return &object{"", checker.Str}
	default:
		panic(fmt.Errorf("Undefined method: %s.%s()", enum.Name, method.Name))
	}
}
