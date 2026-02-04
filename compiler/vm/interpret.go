package vm

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

func (vm *VM) Interpret(program *checker.Program, scope *scope) (val any, err error) {
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
		if res := vm.do(statement, scope); res != nil {
			vm.result = *res
		}
	}

	return vm.result.GoValue(), nil
}

func (vm *VM) callMain(name string, scope *scope) {
	if _, ok := scope.get(name); !ok {
		fmt.Printf("%v\n", scope.data.bindings)
		panic(fmt.Sprintf("main function '%s' not found", name))
	}
	vm.evalFunctionCall(scope, &checker.FunctionCall{
		Name: name,
		Args: []checker.Expression{},
	})
}

func (vm *VM) do(stmt checker.Statement, scp *scope) *runtime.Object {
	if stmt.Break {
		scp._break()
		return runtime.Void()
	}
	if stmt.Expr != nil {
		return vm.eval(scp, stmt.Expr)
	}

	switch s := stmt.Stmt.(type) {
	case *checker.Enum:
		// Process enum methods and create closures with captured scope
		for methodName := range s.Methods {
			// Create a modified function definition with "@" as first parameter
			closure := vm.createEnumMethodClosure(s, methodName, scp)
			// Store using enum.method key format
			vm.hq.addMethod(s, methodName, closure)
		}
		return runtime.Void()
	case *checker.StructDef:
		// Process struct methods and create closures with captured scope
		for methodName, _ := range s.Methods {
			// Create a modified function definition with "@" as first parameter
			closure := vm.createMethodClosure(s, methodName, scp)
			// Store using struct.method key format
			vm.hq.addMethod(s, methodName, closure)
		}
		return runtime.Void()
	case *checker.VariableDef:
		val := vm.eval(scp, s.Value)
		// for debugging:
		// if s.Type().String() != val.Type().String() {
		// fmt.Printf("type mismatch: let %s: %s = %s\n", s.Name, s.Type(), val.Type())
		// }
		// the checker node knows its exact type, because the value might be of a generic type
		val.SetRefinedType(s.Type())
		// can be stopped early by `try` expression
		if scp.isStopped() {
			return val
		}
		scp.add(s.Name, val)
		return runtime.Void()
	case *checker.Reassignment:
		val := vm.eval(scp, s.Value)

		// can be stopped early by `try` expression
		if scp.isStopped() {
			return val
		}

		// For simple variable reassignment, update the scope binding
		// instead of mutating the Object in place. This prevents aliasing issues
		// where two variables pointing to the same Object would both be affected.
		if v, ok := s.Target.(*checker.Variable); ok {
			scp.update(v.Name(), val)
			return runtime.Void()
		}

		// For field/index access, we need to mutate the target in place
		target := vm.eval(scp, s.Target)
		*target = *val
		return runtime.Void()
	case *checker.ForLoop:
		init := func() { vm.do(checker.Statement{Stmt: s.Init}, scp) }
		update := func() { vm.do(checker.Statement{Stmt: s.Update}, scp) }
		for init(); vm.eval(scp, s.Condition).AsBool(); update() {
			r, broke := vm.evalBlock(scp, s.Body, func(s *scope) { s.setBreakable(true) })
			if broke {
				if scp.isStopped() {
					// Early return from function (try expression, result type error, etc.)
					scp.stop() // Propagate stopped status to parent scope
					return r
				}
				// Regular break statement, exit loop normally
				break
			}
		}
		return runtime.Void()
	case *checker.ForIntRange:
		i := vm.eval(scp, s.Start).AsInt()
		end := vm.eval(scp, s.End).AsInt()
		iteration := 0
		for i <= end {
			r, broke := vm.evalBlock(scp, s.Body, func(sc *scope) {
				sc.setBreakable(true)
				sc.add(s.Cursor, runtime.MakeInt(i))
				if s.Index != "" {
					sc.add(s.Index, runtime.MakeInt(iteration))
				}
			})
			if broke {
				if scp.isStopped() {
					// Early return from function
					scp.stop()
					return r
				}
				// Regular break statement
				break
			}
			i++
			iteration++
		}
		return runtime.Void()
	case *checker.ForInStr:
		val := vm.eval(scp, s.Value).AsString()
		for i, c := range val {
			r, broke := vm.evalBlock(scp, s.Body, func(sc *scope) {
				sc.setBreakable(true)
				sc.add(s.Cursor, runtime.MakeStr(string(c)))
				if s.Index != "" {
					sc.add(s.Index, runtime.MakeInt(i))
				}
			})
			if broke {
				if scp.isStopped() {
					// Early return from function
					scp.stop()
					return r
				}
				// Regular break statement
				break
			}
		}
		return runtime.Void()
	case *checker.ForInList:
		list := vm.eval(scp, s.List).AsList()
		for i := range list {
			r, broke := vm.evalBlock(scp, s.Body, func(sc *scope) {
				sc.setBreakable(true)
				sc.add(s.Cursor, list[i])
				if s.Index != "" {
					sc.add(s.Index, runtime.MakeInt(i))
				}
			})
			if broke {
				if scp.isStopped() {
					// Early return from function
					scp.stop()
					return r
				}
				// Regular break statement
				break
			}
		}
		return runtime.Void()
	case *checker.ForInMap:
		{
			mapObj := vm.eval(scp, s.Map)
			_map := mapObj.AsMap()
			for k, v := range _map {
				r, broke := vm.evalBlock(scp, s.Body, func(sc *scope) {
					sc.setBreakable(true)
					key := mapObj.Map_GetKey(k)
					sc.add(s.Key, key)
					sc.add(s.Val, v)
				})
				if broke {
					if scp.isStopped() {
						// Early return from function
						scp.stop()
						return r
					}
					// Regular break statement
					break
				}
			}
			return runtime.Void()
		}
	case *checker.WhileLoop:
		for vm.eval(scp, s.Condition).AsBool() {
			_, broke := vm.evalBlock(scp, s.Body, func(sc *scope) { sc.setBreakable(true) })
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

func (vm *VM) createMethodClosure(strct *checker.StructDef, methodName string, scope *scope) *VMClosure {
	methodDef := strct.Methods[methodName]
	// Create a modified function definition with "@" as first parameter
	copy := *methodDef
	methodDefWithSelf := &copy
	methodDefWithSelf.Parameters = append([]checker.Parameter{
		{Name: "@", Type: strct},
	}, methodDef.Parameters...)

	return &VMClosure{
		vm:            vm,
		expr:          methodDefWithSelf,
		capturedScope: scope,
	}
}

func (vm *VM) eval(scp *scope, expr checker.Expression) *runtime.Object {
	switch e := expr.(type) {
	case *checker.StrLiteral:
		return runtime.MakeStr(e.Value)
	case *checker.BoolLiteral:
		return runtime.MakeBool(e.Value)
	case *checker.VoidLiteral:
		return runtime.Void()
	case *checker.IntLiteral:
		return runtime.MakeInt(e.Value)
	case *checker.FloatLiteral:
		return runtime.MakeFloat(e.Value)
	case *checker.TemplateStr:
		sb := strings.Builder{}
		for i := range e.Chunks {
			// Chunks are already prepared by the checker:
			// - String literals are kept as-is
			// - Non-string expressions have to_str() method calls wrapping them
			chunk := vm.eval(scp, e.Chunks[i]).AsString()
			sb.WriteString(chunk)
		}
		return runtime.MakeStr(sb.String())
	case *checker.Identifier:
		val, ok := scp.get(e.Name)
		if !ok {
			panic(fmt.Errorf("identifier not found: %s", e.Name))
		}
		return val
	case *checker.Variable:
		val, ok := scp.get(e.Name())
		if !ok {
			panic(fmt.Errorf("variable not found: %s", e.Name()))
		}
		return val
	case *checker.Not:
		val := vm.eval(scp, e.Value)
		return runtime.MakeBool(!val.AsBool())

	case *checker.Negation:
		val := vm.eval(scp, e.Value)
		if num, isInt := val.IsInt(); isInt {
			return runtime.MakeInt(-num)
		}
		return runtime.MakeFloat(-val.AsFloat())
	case *checker.StrAddition:
		left, right := vm.eval(scp, e.Left), vm.eval(scp, e.Right)
		return runtime.MakeStr(left.AsString() + right.AsString())
	case *checker.IntAddition:
		left, right := vm.eval(scp, e.Left).AsInt(), vm.eval(scp, e.Right).AsInt()
		return runtime.MakeInt(left + right)
	case *checker.IntSubtraction:
		left, right := vm.eval(scp, e.Left).AsInt(), vm.eval(scp, e.Right).AsInt()
		return runtime.MakeInt(left - right)
	case *checker.IntMultiplication:
		left, right := vm.eval(scp, e.Left).AsInt(), vm.eval(scp, e.Right).AsInt()
		return runtime.MakeInt(left * right)
	case *checker.IntDivision:
		left, right := vm.eval(scp, e.Left).AsInt(), vm.eval(scp, e.Right).AsInt()
		return runtime.MakeInt(left / right)
	case *checker.IntModulo:
		left, right := vm.eval(scp, e.Left).AsInt(), vm.eval(scp, e.Right).AsInt()
		return runtime.MakeInt(left % right)
	case *checker.IntGreater:
		left, right := vm.eval(scp, e.Left).AsInt(), vm.eval(scp, e.Right).AsInt()
		return runtime.MakeBool(left > right)
	case *checker.IntGreaterEqual:
		left, right := vm.eval(scp, e.Left).AsInt(), vm.eval(scp, e.Right).AsInt()
		return runtime.MakeBool(left >= right)
	case *checker.IntLess:
		left, right := vm.eval(scp, e.Left).AsInt(), vm.eval(scp, e.Right).AsInt()
		return runtime.MakeBool(left < right)
	case *checker.IntLessEqual:
		left, right := vm.eval(scp, e.Left).AsInt(), vm.eval(scp, e.Right).AsInt()
		return runtime.MakeBool(left <= right)
	case *checker.FloatGreater:
		left, right := vm.eval(scp, e.Left).AsFloat(), vm.eval(scp, e.Right).AsFloat()
		return runtime.MakeBool(left > right)
	case *checker.FloatGreaterEqual:
		left, right := vm.eval(scp, e.Left).AsFloat(), vm.eval(scp, e.Right).AsFloat()
		return runtime.MakeBool(left >= right)
	case *checker.FloatLess:
		left, right := vm.eval(scp, e.Left).AsFloat(), vm.eval(scp, e.Right).AsFloat()
		return runtime.MakeBool(left < right)
	case *checker.FloatLessEqual:
		left, right := vm.eval(scp, e.Left).AsFloat(), vm.eval(scp, e.Right).AsFloat()
		return runtime.MakeBool(left <= right)
	case *checker.FloatDivision:
		left, right := vm.eval(scp, e.Left).AsFloat(), vm.eval(scp, e.Right).AsFloat()
		return runtime.MakeFloat(left / right)
	case *checker.FloatMultiplication:
		left, right := vm.eval(scp, e.Left).AsFloat(), vm.eval(scp, e.Right).AsFloat()
		return runtime.MakeFloat(left * right)
	case *checker.FloatSubtraction:
		left, right := vm.eval(scp, e.Left).AsFloat(), vm.eval(scp, e.Right).AsFloat()
		return runtime.MakeFloat(left - right)
	case *checker.FloatAddition:
		left, right := vm.eval(scp, e.Left).AsFloat(), vm.eval(scp, e.Right).AsFloat()
		return runtime.MakeFloat(left + right)
	case *checker.Equality:
		left, right := vm.eval(scp, e.Left), vm.eval(scp, e.Right)
		return runtime.MakeBool(left.Equals(*right))
	case *checker.And:
		left, right := vm.eval(scp, e.Left).AsBool(), vm.eval(scp, e.Right).AsBool()
		return runtime.MakeBool(left && right)
	case *checker.Or:
		left, right := vm.eval(scp, e.Left).AsBool(), vm.eval(scp, e.Right).AsBool()
		return runtime.MakeBool(left || right)
	case *checker.If:
		if cond := vm.eval(scp, e.Condition); cond.AsBool() {
			res, _ := vm.evalBlock(scp, e.Body, nil)
			return res
		}
		if e.ElseIf != nil && vm.eval(scp, e.ElseIf.Condition).AsBool() {
			res, _ := vm.evalBlock(scp, e.ElseIf.Body, nil)
			return res
		}
		if e.Else != nil {
			res, _ := vm.evalBlock(scp, e.Else, nil)
			return res
		}
		return runtime.Void()
	case *checker.FunctionDef:
		closure := &VMClosure{vm: vm, expr: e, capturedScope: scp}
		obj := runtime.Make(closure, closure.Type())
		if e.Name != "" {
			scp.add(e.Name, obj)
		}
		return obj
	case *checker.ExternalFunctionDef:
		// Create an external function wrapper
		extFn := &ExternClosure{
			hq:      vm.hq,
			binding: e.ExternalBinding,
			def:     *e,
		}
		obj := runtime.Make(extFn, e)
		if e.Name != "" {
			scp.add(e.Name, obj)
		}
		return obj
	case *checker.Panic:
		msg := vm.eval(scp, e.Message)
		panic(fmt.Sprintf("panic at %s:\n%s", e.GetLocation().Start, msg.AsString()))
	case *checker.FunctionCall:
		return vm.evalFunctionCall(scp, e)
	case *checker.InstanceProperty:
		{
			subj := vm.eval(scp, e.Subject)

			// InstanceProperty only supports struct field access (pre-computed by checker)
			return subj.Struct_Get(e.Property)
		}
	case *checker.InstanceMethod:
		{
			subj := vm.eval(scp, e.Subject)
			return vm.evalInstanceMethod(scp, subj, e)
		}
	case *checker.StrMethod:
		{
			subj := vm.eval(scp, e.Subject)
			return vm.evalStrMethodNode(scp, subj, e)
		}
	case *checker.IntMethod:
		{
			subj := vm.eval(scp, e.Subject)
			return vm.evalIntMethodNode(subj, e)
		}
	case *checker.FloatMethod:
		{
			subj := vm.eval(scp, e.Subject)
			return vm.evalFloatMethodNode(subj, e)
		}
	case *checker.BoolMethod:
		{
			subj := vm.eval(scp, e.Subject)
			return vm.evalBoolMethodNode(subj, e)
		}
	case *checker.ListMethod:
		{
			subj := vm.eval(scp, e.Subject)
			return vm.evalListMethodNode(scp, subj, e)
		}
	case *checker.MapMethod:
		{
			subj := vm.eval(scp, e.Subject)
			return vm.evalMapMethodNode(scp, subj, e)
		}
	case *checker.MaybeMethod:
		{
			subj := vm.eval(scp, e.Subject)
			return vm.evalMaybeMethodNode(scp, subj, e)
		}
	case *checker.ResultMethod:
		{
			subj := vm.eval(scp, e.Subject)
			return vm.evalResultMethodNode(scp, subj, e)
		}
	case *checker.ModuleFunctionCall:
		{
			return vm.hq.callOn(e.Module, e.Call, func() []*runtime.Object {
				// Convert call arguments to objects
				args := make([]*runtime.Object, len(e.Call.Args))
				for i, arg := range e.Call.Args {
					args[i] = vm.eval(scp, arg)
				}
				return args
			})
		}
	case *checker.ListLiteral:
		{
			raw := make([]*runtime.Object, len(e.Elements))
			for i, el := range e.Elements {
				raw[i] = vm.eval(scp, el)
			}
			return runtime.Make(raw, e.ListType)
		}
	case *checker.MapLiteral:
		{
			_map := runtime.MakeMap(e.KeyType, e.ValueType)
			for i := range e.Keys {
				key := vm.eval(scp, e.Keys[i])
				value := vm.eval(scp, e.Values[i])

				_map.Map_Set(key, value)
			}
			return _map
		}
	case *checker.OptionMatch:
		{
			subject := vm.eval(scp, e.Subject)
			if subject.IsNone() {
				// None case - evaluate the None block
				res, _ := vm.evalBlock(scp, e.None, nil)
				return res
			} else {
				// Some case - bind the value and evaluate the Some block
				res, _ := vm.evalBlock(scp, e.Some.Body, func(sc *scope) {
					// Bind the pattern name to the value using pre-computed inner type
					subject := runtime.Make(subject.Raw(), e.InnerType)
					sc.add(e.Some.Pattern.Name, subject)
				})
				return res
			}
		}
	case *checker.EnumMatch:
		{
			subject := vm.eval(scp, e.Subject)
			discriminant := subject.Raw().(int)

			// Map discriminant value to variant index
			variantIndex, ok := e.DiscriminantToIndex[discriminant]
			if !ok {
				variantIndex = -1
			}

			// If there is a catch-all case and we do not have a specific handler for this variant
			if e.CatchAll != nil && (variantIndex < 0 || variantIndex >= int8(len(e.Cases)) || e.Cases[variantIndex] == nil) {
				res, _ := vm.evalBlock(scp, e.CatchAll, nil)
				return res
			}

			// Execute the matching case block for this variant
			if variantIndex >= 0 && variantIndex < int8(len(e.Cases)) && e.Cases[variantIndex] != nil {
				res, _ := vm.evalBlock(scp, e.Cases[variantIndex], nil)
				return res
			}

			// This should never happen if the type checker is working correctly
			// because it ensures the match is exhaustive
			panic(fmt.Errorf("No matching case for enum variant with discriminant %d", discriminant))
		}
	case *checker.EnumVariant:
		// Get the enum type and find the discriminant value for this variant
		return runtime.Make(e.Discriminant, e.EnumType)
	case *checker.BoolMatch:
		{
			subject := vm.eval(scp, e.Subject)
			value := subject.AsBool()

			// Execute the appropriate case based on the boolean value
			if value {
				res, _ := vm.evalBlock(scp, e.True, nil)
				return res
			} else {
				res, _ := vm.evalBlock(scp, e.False, nil)
				return res
			}
		}
	case *checker.UnionMatch:
		{
			subject := vm.eval(scp, e.Subject)
			if arm, ok := e.TypeCases[subject.TypeName()]; ok {
				res, _ := vm.evalBlock(scp, arm.Body, func(sc *scope) {
					sc.add(arm.Pattern.Name, subject)
				})
				return res
			}

			// If we have a catch-all case
			if e.CatchAll != nil {
				res, _ := vm.evalBlock(scp, e.CatchAll, nil)
				return res
			}

			// This should never happen if the type checker is working correctly
			// because it ensures the match is exhaustive
			panic(fmt.Errorf("No matching case for union type %s", subject.Kind()))
		}
	case *checker.StructInstance:
		{
			raw := map[string]*runtime.Object{}
			for name, ftype := range e.FieldTypes {
				val, ok := e.Fields[name]
				if ok {
					val := vm.eval(scp, val)
					val.SetRefinedType(ftype)
					raw[name] = val
				} else {
					// assume it's a $T? if the checker allowed it
					raw[name] = runtime.MakeNone(ftype)
				}
			}
			return runtime.Make(raw, e.StructType)
		}
	case *checker.ModuleStructInstance:
		{
			raw := map[string]*runtime.Object{}
			for name, ftype := range e.FieldTypes {
				val, ok := e.Property.Fields[name]
				if ok {
					val := vm.eval(scp, val)
					val.SetRefinedType(ftype)
					raw[name] = val
				} else {
					// assume it's a $T? if the checker allowed it
					raw[name] = runtime.MakeNone(ftype)
				}
			}
			return runtime.MakeStruct(e.StructType, raw)
		}
	case *checker.ResultMatch:
		{
			subj := vm.eval(scp, e.Subject)
			if subj.IsOk() {
				res, _ := vm.evalBlock(scp, e.Ok.Body, func(sc *scope) {
					sc.add(e.Ok.Pattern.Name, subj.UnwrapResult())
				})
				return res
			}
			res, _ := vm.evalBlock(scp, e.Err.Body, func(sc *scope) {
				sc.add(e.Err.Pattern.Name, subj.UnwrapResult())
			})
			return res
		}
	case *checker.IntMatch:
		{
			subject := vm.eval(scp, e.Subject)
			intValue := subject.AsInt()

			// Check if we have a specific case for this integer
			if caseBlock, exists := e.IntCases[intValue]; exists {
				res, _ := vm.evalBlock(scp, caseBlock, nil)
				return res
			}

			// Check if the value falls within any range
			for rangePattern, caseBlock := range e.RangeCases {
				if intValue >= rangePattern.Start && intValue <= rangePattern.End {
					res, _ := vm.evalBlock(scp, caseBlock, nil)
					return res
				}
			}

			// If no specific case or range found, use catch-all if available
			if e.CatchAll != nil {
				res, _ := vm.evalBlock(scp, e.CatchAll, nil)
				return res
			}

			// This should never happen if the type checker is working correctly
			panic(fmt.Errorf("No matching case for int value %d", intValue))
		}
	case *checker.ConditionalMatch:
		{
			// Check conditions in order, execute first matching case
			for _, conditionalCase := range e.Cases {
				condition := vm.eval(scp, conditionalCase.Condition)
				if condition.AsBool() {
					res, _ := vm.evalBlock(scp, conditionalCase.Body, nil)
					return res
				}
			}

			// If no conditions matched, use catch-all (guaranteed to exist by type checker)
			res, _ := vm.evalBlock(scp, e.CatchAll, nil)
			return res
		}
	case *checker.TryOp:
		{
			subj := vm.eval(scp, e.Expr())

			// Dispatch based on pre-computed kind (determined by checker)
			switch e.Kind {
			case checker.TryResult:
				unwrapped := subj.UnwrapResult()
				if subj.IsErr() {
					// Error case: early return from function
					if e.CatchBlock != nil {
						// Execute catch block and early return its result
						result, broken := vm.evalBlock(scp, e.CatchBlock, func(sc *scope) {
							sc.add(e.CatchVar, unwrapped)
						})
						scp.stop()
						if broken {
							return result
						}
						return result
					} else {
						// No catch block: propagate error by early returning
						scp.stop()
						return subj
					}
				}
				// Success case: continue execution with unwrapped value
				return unwrapped

			case checker.TryMaybe:
				if subj.IsNone() {
					// None case: early return from function
					if e.CatchBlock != nil {
						// Execute catch block and early return its result
						result, broken := vm.evalBlock(scp, e.CatchBlock, nil)
						scp.stop()
						if broken {
							return result
						}
						return result
					} else {
						// No catch block: propagate none by early returning
						scp.stop()
						return runtime.MakeNone(e.OkType)
					}
				}
				// Some case: unwrap and continue execution
				return runtime.Make(subj.Raw(), e.OkType)

			default:
				panic(fmt.Errorf("Unknown try kind: %d", e.Kind))
			}
		}
	case *checker.ModuleSymbol:
		return vm.hq.lookup(e.Module, e.Symbol)
	case *checker.CopyExpression:
		// Evaluate the expression and return a deep copy
		original := vm.eval(scp, e.Expr)
		return original.Copy()
	case *checker.FiberExecution:
		// at some point, it may be a good idea to have globally unique names for fiber modules in case the same code is launched in multiple fibers
		name := e.GetModule().Path()
		f, fscope := vm.hq.loadModule(name, e.GetModule().Program(), false)
		fscope.parent = scp
		wg := &sync.WaitGroup{}
		wg.Go(func() {
			defer func() {
				fscope.parent = nil
				// Don't unload the module here - it may be accessed by other fibers or join()
				// The module cache manages its own lifecycle
			}()
			defer func() {
				if r := recover(); r != nil {
					if msg, ok := r.(string); ok {
						fmt.Println(fmt.Errorf("Panic in fiber: %s", msg))
					} else {
						fmt.Printf("Panic in fiber: %v\n", r)
					}
				}
			}()
			f.callMain(e.GetMainName(), fscope)
		})
		return runtime.MakeStruct(e.FiberType, map[string]*runtime.Object{
			"wg": runtime.MakeDynamic(wg),
		})
	case *checker.FiberEval:
		// Execute the closure concurrently and return a Fiber handle with the result
		fn := vm.eval(scp, e.GetFn())
		closure, ok := fn.Raw().(runtime.Closure)
		if !ok {
			panic(fmt.Errorf("async::eval expects a function, got %T", fn.Raw()))
		}

		// Get the async module's program and load it
		// Create a minimal program with the Fiber struct and async functions
		asyncProg := &checker.Program{
			Imports:    make(map[string]checker.Module),
			Statements: []checker.Statement{},
		}
		// Try to get the program from the embedded module
		if embeddedMod, ok := checker.FindEmbeddedModule("ard/async"); ok {
			if prog := embeddedMod.Program(); prog != nil {
				asyncProg = prog
			}
		}
		vm.hq.loadModule("ard/async", asyncProg, false)

		wg := &sync.WaitGroup{}
		// resultValue is wrapped in a container so it can be updated concurrently
		resultContainer := &runtime.Object{}
		wg.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					if msg, ok := r.(string); ok {
						fmt.Println(fmt.Errorf("Panic in eval fiber: %s", msg))
					} else {
						fmt.Printf("Panic in eval fiber: %v\n", r)
					}
				}
			}()
			result := closure.Eval()
			*resultContainer = *result
		})

		return runtime.MakeStruct(e.FiberType, map[string]*runtime.Object{
			"wg":     runtime.MakeDynamic(wg),
			"result": resultContainer,
		})
	case *checker.Block:
		// Evaluate block and return the result of the last statement
		result, _ := vm.evalBlock(scp, e, nil)
		return result
	default:
		panic(fmt.Errorf("Unimplemented expression: %T", e))
	}
}

// _args can be provided by caller from different module scopes
func (vm *VM) evalFunctionCall(scope *scope, call *checker.FunctionCall, _args ...*runtime.Object) *runtime.Object {
	sig, ok := scope.get(call.Name)
	if !ok {
		panic(fmt.Errorf("Undefined: %s", call.Name))
	}

	if closure, ok := sig.Raw().(runtime.Closure); ok {
		args := _args
		// if no args are provided but the function has parameters, use the call.Args
		if len(args) == 0 {
			args = make([]*runtime.Object, len(call.Args))

			for i := range call.Args {
				args[i] = vm.eval(scope, call.Args[i])
			}
		}
		return closure.Eval(args...)
	}

	panic(fmt.Errorf("Not a function: %s: %s", call.Name, sig.TypeName()))
}

func (vm *VM) evalBlock(scope *scope, block *checker.Block, init func(s *scope)) (*runtime.Object, bool) {
	blockScope := newScope(scope)

	if init != nil {
		init(blockScope)
	}

	res := runtime.Void()
	for i := range block.Stmts {
		stmt := block.Stmts[i]
		r := vm.do(stmt, blockScope)
		if blockScope.isBroke() || blockScope.isStopped() {
			// Propagate stopped flag to parent scope if this block is breakable (loop context)
			if blockScope.isStopped() && blockScope.data.breakable {
				scope.stop()
			}
			return r, true
		}
		res = r
	}

	return res, false
}

// this method is for evaluating a checker.InstanceMethod.
func (vm *VM) evalInstanceMethod(scope *scope, subj *runtime.Object, e *checker.InstanceMethod) *runtime.Object {
	switch e.ReceiverKind {
	case checker.ReceiverStruct:
		if e.StructType == nil {
			break
		}
		return vm.EvalStructMethod(scope, subj, e.Method, e.StructType)
	case checker.ReceiverEnum:
		if e.EnumType == nil {
			break
		}
		return vm.EvalEnumMethod(scope, subj, e.Method, e.EnumType)
	case checker.ReceiverTrait:
		// Fall through to dynamic dispatch based on runtime type.
	default:
		// Continue with dynamic dispatch.
	}

	if e.ReceiverKind == checker.ReceiverTrait && e.TraitType != nil {
		dispatch := vm.traitDispatchFor(e.TraitType)
		if dispatch != nil {
			if _, ok := dispatch[subj.Kind()]; !ok {
				panic(fmt.Errorf("Trait dispatch missing for %s.%s()", subj.Kind(), e.Method.Name))
			}
		}
	}

	if subj.IsResult() {
		return vm.evalResultMethod(scope, subj, e.Method)
	}
	switch subj.Kind() {
	case runtime.KindStr:
		return vm.evalStrMethod(scope, subj, e.Method)
	case runtime.KindInt:
		return vm.evalIntMethod(subj, e)
	case runtime.KindFloat:
		return vm.evalFloatMethod(subj, e.Method)
	case runtime.KindBool:
		return vm.evalBoolMethod(subj, e)
	case runtime.KindList:
		return vm.evalListMethod(scope, subj, e)
	case runtime.KindMap:
		return vm.evalMapMethod(scope, subj, e)
	case runtime.KindMaybe:
		return vm.evalMaybeMethod(scope, subj, e)
	case runtime.KindStruct:
		if structType := subj.StructType(); structType != nil {
			return vm.EvalStructMethod(scope, subj, e.Method, structType)
		}
	case runtime.KindEnum:
		if enum := subj.EnumType(); enum != nil {
			return vm.EvalEnumMethod(scope, subj, e.Method, enum)
		}
	}

	panic(fmt.Errorf("Unimplemented method: %s.%s()", subj.Kind(), e.Method.Name))
}

// Handlers for specialized method nodes

func (vm *VM) evalStrMethodNode(scope *scope, subj *runtime.Object, e *checker.StrMethod) *runtime.Object {
	raw := subj.AsString()
	switch e.Kind {
	case checker.StrSize:
		return runtime.MakeInt(len(raw))
	case checker.StrIsEmpty:
		return runtime.MakeBool(len(raw) == 0)
	case checker.StrContains:
		return runtime.MakeBool(strings.Contains(raw, vm.eval(scope, e.Args[0]).AsString()))
	case checker.StrReplace:
		old := vm.eval(scope, e.Args[0]).AsString()
		new := vm.eval(scope, e.Args[1]).AsString()
		return runtime.MakeStr(strings.Replace(raw, old, new, 1))
	case checker.StrReplaceAll:
		old := vm.eval(scope, e.Args[0]).AsString()
		new := vm.eval(scope, e.Args[1]).AsString()
		return runtime.MakeStr(strings.ReplaceAll(raw, old, new))
	case checker.StrSplit:
		sep := vm.eval(scope, e.Args[0]).AsString()
		split := strings.Split(raw, sep)
		values := make([]*runtime.Object, len(split))
		for i, str := range split {
			values[i] = runtime.MakeStr(str)
		}
		return runtime.MakeList(checker.Str, values...)
	case checker.StrStartsWith:
		prefix := vm.eval(scope, e.Args[0]).AsString()
		return runtime.MakeBool(strings.HasPrefix(raw, prefix))
	case checker.StrToStr:
		return subj
	case checker.StrToDyn:
		return runtime.MakeDynamic(raw)
	case checker.StrTrim:
		return runtime.MakeStr(strings.Trim(raw, " "))
	default:
		panic(fmt.Errorf("Unknown StrMethodKind: %d", e.Kind))
	}
}

func (vm *VM) evalIntMethodNode(subj *runtime.Object, e *checker.IntMethod) *runtime.Object {
	switch e.Kind {
	case checker.IntToStr:
		return runtime.MakeStr(strconv.Itoa(subj.AsInt()))
	case checker.IntToDyn:
		return runtime.MakeDynamic(subj.AsInt())
	default:
		panic(fmt.Errorf("Unknown IntMethodKind: %d", e.Kind))
	}
}

func (vm *VM) evalFloatMethodNode(subj *runtime.Object, e *checker.FloatMethod) *runtime.Object {
	switch e.Kind {
	case checker.FloatToStr:
		return runtime.MakeStr(strconv.FormatFloat(subj.AsFloat(), 'f', 2, 64))
	case checker.FloatToInt:
		floatVal := subj.AsFloat()
		intVal := int(floatVal)
		return runtime.MakeInt(intVal)
	case checker.FloatToDyn:
		return runtime.MakeDynamic(subj.AsFloat())
	default:
		panic(fmt.Errorf("Unknown FloatMethodKind: %d", e.Kind))
	}
}

func (vm *VM) evalBoolMethodNode(subj *runtime.Object, e *checker.BoolMethod) *runtime.Object {
	switch e.Kind {
	case checker.BoolToStr:
		return runtime.MakeStr(strconv.FormatBool(subj.AsBool()))
	case checker.BoolToDyn:
		return runtime.MakeDynamic(subj.AsBool())
	default:
		panic(fmt.Errorf("Unknown BoolMethodKind: %d", e.Kind))
	}
}

// Handlers for collection method nodes

func (vm *VM) evalListMethodNode(scope *scope, self *runtime.Object, e *checker.ListMethod) *runtime.Object {
	raw := self.AsList()
	switch e.Kind {
	case checker.ListAt:
		index := vm.eval(scope, e.Args[0]).AsInt()
		if index >= len(raw) {
			panic(fmt.Errorf("Index out of range (%d) on list of length %d", index, len(raw)))
		}
		return raw[index]
	case checker.ListPrepend:
		newItem := vm.eval(scope, e.Args[0])
		raw = append([]*runtime.Object{newItem}, raw...)
		self.Set(raw)
		return self
	case checker.ListPush:
		raw = append(raw, vm.eval(scope, e.Args[0]))
		self.Set(raw)
		return self
	case checker.ListSet:
		index := vm.eval(scope, e.Args[0]).AsInt()
		value := vm.eval(scope, e.Args[1])
		result := runtime.MakeBool(false)
		if index < len(raw) {
			raw[index] = value
			result.Set(true)
		}
		return result
	case checker.ListSize:
		return runtime.MakeInt(len(raw))
	case checker.ListSort:
		_isLess := vm.eval(scope, e.Args[0]).Raw().(*VMClosure)
		slices.SortFunc(raw, func(a, b *runtime.Object) int {
			if _isLess.Eval(a, b).AsBool() {
				return -1
			}
			return 0
		})
		return runtime.Void()
	case checker.ListSwap:
		l := vm.eval(scope, e.Args[0]).AsInt()
		r := vm.eval(scope, e.Args[1]).AsInt()
		_l, _r := raw[l], raw[r]
		raw[l] = _r
		raw[r] = _l
		return runtime.Void()
	default:
		panic(fmt.Errorf("Unknown ListMethodKind: %d", e.Kind))
	}
}

func (vm *VM) evalMapMethodNode(scope *scope, subj *runtime.Object, e *checker.MapMethod) *runtime.Object {
	raw := subj.AsMap()
	switch e.Kind {
	case checker.MapKeys:
		keys := make([]*runtime.Object, len(raw))
		i := 0
		for k := range raw {
			keys[i] = subj.Map_GetKey(k)
			i++
		}
		return runtime.MakeList(e.KeyType, keys...)
	case checker.MapSize:
		return runtime.MakeInt(len(raw))
	case checker.MapGet:
		keyArg := vm.eval(scope, e.Args[0])
		_key := runtime.ToMapKey(keyArg)
		out := runtime.MakeNone(e.ValueType)
		if value, found := raw[_key]; found {
			out = out.ToSome(value.Raw())
		}
		return out
	case checker.MapSet:
		keyArg := vm.eval(scope, e.Args[0])
		valueArg := vm.eval(scope, e.Args[1])
		keyStr := runtime.ToMapKey(keyArg)
		raw[keyStr] = valueArg
		return runtime.MakeBool(true)
	case checker.MapDrop:
		keyArg := vm.eval(scope, e.Args[0])
		keyStr := runtime.ToMapKey(keyArg)
		delete(raw, keyStr)
		return runtime.Void()
	case checker.MapHas:
		keyArg := vm.eval(scope, e.Args[0])
		keyStr := runtime.ToMapKey(keyArg)
		_, found := raw[keyStr]
		return runtime.MakeBool(found)
	default:
		panic(fmt.Errorf("Unknown MapMethodKind: %d", e.Kind))
	}
}

func (vm *VM) evalMaybeMethodNode(scope *scope, subj *runtime.Object, e *checker.MaybeMethod) *runtime.Object {
	switch e.Kind {
	case checker.MaybeExpect:
		if subj.Raw() == nil {
			_msg := vm.eval(scope, e.Args[0]).AsString()
			panic(_msg)
		}
		return runtime.Make(subj.Raw(), e.InnerType)
	case checker.MaybeIsNone:
		return runtime.MakeBool(subj.Raw() == nil)
	case checker.MaybeIsSome:
		return runtime.MakeBool(subj.Raw() != nil)
	case checker.MaybeOr:
		if subj.Raw() == nil {
			return vm.eval(scope, e.Args[0])
		}
		return runtime.Make(subj.Raw(), e.InnerType)
	default:
		panic(fmt.Errorf("Unknown MaybeMethodKind: %d", e.Kind))
	}
}

func (vm *VM) evalResultMethodNode(scope *scope, subj *runtime.Object, e *checker.ResultMethod) *runtime.Object {
	switch e.Kind {
	case checker.ResultExpect:
		if subj.IsErr() {
			actual := ""
			if str, ok := subj.IsStr(); ok {
				actual = str
			} else {
				actual = fmt.Sprintf("%v", subj.GoValue())
			}
			_msg := vm.eval(scope, e.Args[0]).AsString()
			panic(_msg + ": " + actual)
		}
		return subj.UnwrapResult()
	case checker.ResultOr:
		if subj.IsErr() {
			return vm.eval(scope, e.Args[0])
		}
		return subj.UnwrapResult()
	case checker.ResultIsOk:
		return runtime.MakeBool(!subj.IsErr())
	case checker.ResultIsErr:
		return runtime.MakeBool(subj.IsErr())
	default:
		panic(fmt.Errorf("Unknown ResultMethodKind: %d", e.Kind))
	}
}

func (vm *VM) evalStrMethod(scope *scope, subj *runtime.Object, m *checker.FunctionCall) *runtime.Object {
	raw := subj.AsString()
	switch m.Name {
	case "size":
		return runtime.MakeInt(len(raw))
	case "is_empty":
		return runtime.MakeBool(len(raw) == 0)
	case "contains":
		return runtime.MakeBool(strings.Contains(raw, vm.eval(scope, m.Args[0]).AsString()))
	case "replace":
		old := vm.eval(scope, m.Args[0]).AsString()
		new := vm.eval(scope, m.Args[1]).AsString()
		return runtime.MakeStr(strings.Replace(raw, old, new, 1))
	case "replace_all":
		old := vm.eval(scope, m.Args[0]).AsString()
		new := vm.eval(scope, m.Args[1]).AsString()
		return runtime.MakeStr(strings.ReplaceAll(raw, old, new))
	case "split":
		sep := vm.eval(scope, m.Args[0]).AsString()
		split := strings.Split(raw, sep)
		values := make([]*runtime.Object, len(split))
		for i, str := range split {
			values[i] = runtime.MakeStr(str)
		}
		return runtime.MakeList(checker.Str, values...)
	case "starts_with":
		prefix := vm.eval(scope, m.Args[0]).AsString()
		return runtime.MakeBool(strings.HasPrefix(raw, prefix))
	case "to_str":
		return subj
	case "to_dyn":
		return runtime.MakeDynamic(raw)
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
	case "to_dyn":
		return runtime.MakeDynamic(subj.AsInt())
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
	case "to_dyn":
		return runtime.MakeDynamic(subj.AsFloat())
	default:
		return runtime.Void()
	}
}

func (vm *VM) evalBoolMethod(subj *runtime.Object, m *checker.InstanceMethod) *runtime.Object {
	switch m.Method.Name {
	case "to_str":
		return runtime.MakeStr(strconv.FormatBool(subj.AsBool()))
	case "to_dyn":
		return runtime.MakeDynamic(subj.AsBool())
	default:
		return runtime.Void()
	}
}

func (vm *VM) evalListMethod(scope *scope, self *runtime.Object, m *checker.InstanceMethod) *runtime.Object {
	raw := self.AsList()
	switch m.Method.Name {
	case "at":
		index := vm.eval(scope, m.Method.Args[0]).AsInt()
		if index >= len(raw) {
			panic(fmt.Errorf("Index out of range (%d) on list of length %d", index, len(raw)))
		}
		return raw[index]
	case "prepend":
		newItem := vm.eval(scope, m.Method.Args[0])
		raw = append([]*runtime.Object{newItem}, raw...)
		self.Set(raw)
		return self
	case "push":
		raw = append(raw, vm.eval(scope, m.Method.Args[0]))
		self.Set(raw)
		return self
	case "set":
		index := vm.eval(scope, m.Method.Args[0]).AsInt()
		value := vm.eval(scope, m.Method.Args[1])
		result := runtime.MakeBool(false)
		if index < len(raw) {
			raw[index] = value
			result.Set(true)
		}
		return result
	case "size":
		return runtime.MakeInt(len(raw))
	case "sort":
		{
			_isLess := vm.eval(scope, m.Method.Args[0]).Raw().(*VMClosure)

			slices.SortFunc(raw, func(a, b *runtime.Object) int {
				if _isLess.Eval(a, b).AsBool() {
					return -1
				}
				return 0
			})

			return runtime.Void()
		}
	case "swap":
		l := vm.eval(scope, m.Method.Args[0]).AsInt()
		r := vm.eval(scope, m.Method.Args[1]).AsInt()
		_l, _r := raw[l], raw[r]
		raw[l] = _r
		raw[r] = _l
		return runtime.Void()
	default:
		panic(fmt.Errorf("Unimplemented: List.%s()", m.Method.Name))
	}
}

func (vm *VM) evalMapMethod(scope *scope, subj *runtime.Object, m *checker.InstanceMethod) *runtime.Object {
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
		keyArg := vm.eval(scope, m.Method.Args[0])
		_key := runtime.ToMapKey(keyArg)
		mapType := subj.MapType()
		if mapType == nil {
			panic(fmt.Errorf("Map.get called on %s", subj.Kind()))
		}
		out := runtime.MakeNone(mapType.Value())
		if value, found := raw[_key]; found {
			out = out.ToSome(value.Raw())
		}
		return out
	case "set":
		keyArg := vm.eval(scope, m.Method.Args[0])
		valueArg := vm.eval(scope, m.Method.Args[1])

		keyStr := runtime.ToMapKey(keyArg)
		raw[keyStr] = valueArg
		return runtime.MakeBool(true)
	case "drop":
		keyArg := vm.eval(scope, m.Method.Args[0])
		keyStr := runtime.ToMapKey(keyArg)

		delete(raw, keyStr)
		return runtime.Void()
	case "has":
		keyArg := vm.eval(scope, m.Method.Args[0])

		// Convert key to string
		keyStr := runtime.ToMapKey(keyArg)
		_, found := raw[keyStr]
		return runtime.MakeBool(found)
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj.Kind(), m.Method.Name))
	}
}

func (vm *VM) evalMaybeMethod(scope *scope, subj *runtime.Object, m *checker.InstanceMethod) *runtime.Object {
	switch m.Method.Name {
	case "expect":
		if subj.Raw() == nil { // This is a none
			_msg := vm.eval(scope, m.Method.Args[0]).AsString()
			panic(_msg)
		}
		// Return the unwrapped value for some
		return runtime.Make(subj.Raw(), m.Method.ReturnType)
	case "is_none":
		return runtime.MakeBool(subj.Raw() == nil)
	case "is_some":
		return runtime.MakeBool(subj.Raw() != nil)
	case "or":
		if subj.Raw() == nil {
			return vm.eval(scope, m.Method.Args[0])
		}
		return runtime.Make(subj.Raw(), m.Method.ReturnType)
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj.Kind(), m.Method.Name))
	}
}

func (vm *VM) EvalStructMethod(scope *scope, subj *runtime.Object, call *checker.FunctionCall, structType *checker.StructDef) *runtime.Object {
	closure, ok := vm.hq.getMethod(structType, call.Name)
	if ok {
		// Prepare arguments: struct instance first, then regular args
		args := make([]*runtime.Object, len(call.Args)+1)
		args[0] = subj // "@" - the struct instance
		for i := range call.Args {
			args[i+1] = vm.eval(scope, call.Args[i])
		}

		return closure.Eval(args...)
	}

	panic(fmt.Errorf("Method not found: %s.%s", structType.Name, call.Name))
}

func (vm *VM) createEnumMethodClosure(enum *checker.Enum, methodName string, scope *scope) *VMClosure {
	methodDef := enum.Methods[methodName]
	// Create a modified function definition with "@" as first parameter
	copy := *methodDef // Copy the original
	methodDefWithSelf := &copy
	methodDefWithSelf.Parameters = append([]checker.Parameter{
		{Name: "@", Type: enum},
	}, methodDef.Parameters...)

	return &VMClosure{
		vm:            vm,
		expr:          methodDefWithSelf,
		capturedScope: scope,
	}
}

func (vm *VM) EvalEnumMethod(scope *scope, subj *runtime.Object, call *checker.FunctionCall, enum *checker.Enum) *runtime.Object {
	closure, ok := vm.hq.getMethod(enum, call.Name)
	if ok {
		// Prepare arguments: enum instance first, then regular args
		args := make([]*runtime.Object, len(call.Args)+1)
		args[0] = subj // "@" - the enum instance
		for i := range call.Args {
			args[i+1] = vm.eval(scope, call.Args[i])
		}

		return closure.Eval(args...)
	}

	panic(fmt.Errorf("Method not found: %s.%s", enum.Name, call.Name))
}
