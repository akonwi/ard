package vm

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/akonwi/ard/checker_v2"
)

var void = &object{nil, checker_v2.Void}

// compareKey is a wrapper around an object to use for map keys
// enabling proper equality comparison
type compareKey struct {
	obj *object
	// Store a string representation for hashability
	strKey string
}

func Run2(program *checker_v2.Program) (any, error) {
	vm := New()
	for _, statement := range program.Statements {
		vm.result = *vm.do(statement)
	}
	return vm.result.raw, nil
}

func (vm *VM) do(stmt checker_v2.Statement) *object {
	if stmt.Break {
		vm.scope._break()
		return void
	}
	if stmt.Expr != nil {
		return vm.eval(stmt.Expr)
	}

	switch s := stmt.Stmt.(type) {
	case *checker_v2.Enum:
		return void
	case *checker_v2.VariableDef:
		val := vm.eval(s.Value)
		if !s.Mutable {
			original := val.raw
			var copy any = new(any)
			copy = original
			val.raw = copy
		}
		vm.scope.add(s.Name, val)
		return void
	case *checker_v2.Reassignment:
		target := vm.eval(s.Target)
		val := vm.eval(s.Value)
		target.raw = val.raw
		return void
	case *checker_v2.ForLoop:
		init := func() { vm.do(checker_v2.Statement{Stmt: s.Init}) }
		update := func() { vm.do(checker_v2.Statement{Stmt: s.Update}) }
		for init(); vm.eval(s.Condition).raw.(bool); update() {
			_, broke := vm.evalBlock2(s.Body, func() { vm.scope.breakable = true })
			if broke {
				break
			}
		}
		return void
	case *checker_v2.ForIntRange:
		i := vm.eval(s.Start).raw.(int)
		end := vm.eval(s.End).raw.(int)
		for i <= end {
			_, broke := vm.evalBlock2(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, &object{i, checker_v2.Int})
			})
			if broke {
				break
			}
			i++
		}
		return void
	case *checker_v2.ForInStr:
		val := vm.eval(s.Value).raw.(string)
		for _, c := range val {
			_, broke := vm.evalBlock2(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, &object{string(c), checker_v2.Str})
			})
			if broke {
				break
			}
		}
		return void
	case *checker_v2.ForInList:
		val := vm.eval(s.List).raw.([]*object)
		for i := range val {
			_, broke := vm.evalBlock2(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, val[i])
			})
			if broke {
				break
			}
		}
		return void
	case *checker_v2.WhileLoop:
		for vm.eval(s.Condition).raw.(bool) {
			_, broke := vm.evalBlock2(s.Body, func() { vm.scope.breakable = true })
			if broke {
				break
			}
		}
		return void
	default:
		panic(fmt.Errorf("Unimplemented statement: %T", s))
	}
}

func (vm *VM) eval(expr checker_v2.Expression) *object {
	switch e := expr.(type) {
	case *checker_v2.StrLiteral:
		return &object{e.Value, e.Type()}
	case *checker_v2.BoolLiteral:
		return &object{e.Value, e.Type()}
	case *checker_v2.IntLiteral:
		return &object{e.Value, e.Type()}
	case *checker_v2.FloatLiteral:
		return &object{e.Value, e.Type()}
	case *checker_v2.TemplateStr:
		sb := strings.Builder{}
		for i := range e.Chunks {
			chunk := vm.eval(e.Chunks[i])
			sb.WriteString(chunk.raw.(string))
		}
		return &object{sb.String(), checker_v2.Str}
	case *checker_v2.Variable:
		val, ok := vm.scope.get(e.Name())
		if !ok {
			panic(fmt.Errorf("variable not found: %s", e.Name()))
		}
		return val
	case *checker_v2.Not:
		val := vm.eval(e.Value)
		return &object{!val.raw.(bool), val._type}

	case *checker_v2.Negation:
		val := vm.eval(e.Value)
		if num, isInt := val.raw.(int); isInt {
			return &object{-num, val._type}
		}
		return &object{-val.raw.(float64), val._type}
	case *checker_v2.IntAddition:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) + right.raw.(int),
			left._type,
		}
	case *checker_v2.IntSubtraction:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) - right.raw.(int),
			left._type,
		}
	case *checker_v2.IntMultiplication:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) * right.raw.(int),
			left._type,
		}
	case *checker_v2.IntDivision:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) / right.raw.(int),
			left._type,
		}
	case *checker_v2.IntModulo:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) % right.raw.(int),
			left._type,
		}
	case *checker_v2.IntGreater:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) > right.raw.(int),
			checker_v2.Bool,
		}
	case *checker_v2.IntGreaterEqual:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) >= right.raw.(int),
			checker_v2.Bool,
		}
	case *checker_v2.IntLess:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) < right.raw.(int),
			checker_v2.Bool,
		}
	case *checker_v2.IntLessEqual:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) <= right.raw.(int),
			checker_v2.Bool,
		}
	case *checker_v2.FloatDivision:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) / right.raw.(float64),
			left._type,
		}
	case *checker_v2.FloatMultiplication:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) * right.raw.(float64),
			left._type,
		}
	case *checker_v2.FloatSubtraction:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) - right.raw.(float64),
			left._type,
		}
	case *checker_v2.FloatAddition:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) + right.raw.(float64),
			left._type,
		}
	case *checker_v2.Equality:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw == right.raw, checker_v2.Bool}
	case *checker_v2.And:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw.(bool) && right.raw.(bool), checker_v2.Bool}
	case *checker_v2.Or:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw.(bool) || right.raw.(bool), checker_v2.Bool}
	case *checker_v2.If:
		if cond := vm.eval(e.Condition); cond.raw.(bool) {
			res, _ := vm.evalBlock2(e.Body, nil)
			return res
		} else if e.ElseIf != nil {
			if cond := vm.eval(e.ElseIf.Condition); cond.raw.(bool) {
				res, _ := vm.evalBlock2(e.ElseIf.Body, nil)
				return res
			}
		} else if e.Else != nil {
			res, _ := vm.evalBlock2(e.Else, nil)
			return res
		}
		return void
	case *checker_v2.FunctionDef:
		raw := func(args ...*object) *object {
			res, _ := vm.evalBlock2(e.Body, func() {
				for i := range args {
					vm.scope.add(e.Parameters[i].Name, args[i])
				}
			})
			return res
		}
		obj := &object{raw, e.Type()}
		vm.scope.add(e.Name, obj)
		return obj
	case *checker_v2.FunctionCall:
		sig, ok := vm.scope.get(e.Name)
		if !ok {
			panic(fmt.Errorf("Undefined: %s", e.Name))
		}
		fn, ok := sig.raw.(func(args ...*object) *object)
		if !ok {
			panic(fmt.Errorf("Not a function: %s: %s", e.Name, sig._type))
		}

		args := make([]*object, len(e.Args))
		for i := range e.Args {
			args[i] = vm.eval(e.Args[i])
		}

		return fn(args...)
	case *checker_v2.InstanceProperty:
		{
			subj := vm.eval(e.Subject)
			switch subj._type {
			case checker_v2.Str:
				return vm.evalStrProperty(subj, e.Property)
			default:
				return void
			}
		}
	case *checker_v2.InstanceMethod:
		{
			subj := vm.eval(e.Subject)
			if subj._type == checker_v2.Str {
				return vm.evalStrMethod(subj, e.Method)
			}
			if subj._type == checker_v2.Int {
				return vm.evalIntMethod(subj, e)
			}
			if subj._type == checker_v2.Float {
				return vm.evalFloatMethod(subj, e.Method)
			}
			if subj._type == checker_v2.Bool {
				return vm.evalBoolMethod(subj, e)
			}
			if _, ok := subj._type.(*checker_v2.List); ok {
				return vm.evalListMethod(subj, e)
			}
			if _, ok := subj._type.(*checker_v2.Map); ok {
				return vm.evalMapMethod(subj, e)
			}
			if _, ok := subj._type.(*checker_v2.Maybe); ok {
				return vm.evalMaybeMethod(subj, e)
			}

			panic(fmt.Errorf("Unimplemented: %s.%s() on %T", e.Subject.Type(), e.Method.Name, e.Subject.Type()))
		}
	case *checker_v2.PackageFunctionCall:
		{
			if e.Package == "ard/ints" {
				switch e.Call.Name {
				case "from_str":
					input := vm.eval(e.Call.Args[0]).raw.(string)

					res := &object{nil, e.Call.Type()}
					if num, err := strconv.Atoi(input); err == nil {
						res.raw = num
					}
					return res
				default:
					panic(fmt.Errorf("Unimplemented: Int::%s()", e.Call.Name))
				}
			}

			if e.Package == "ard/float" {
				switch e.Call.Name {
				case "from_int":
					input := vm.eval(e.Call.Args[0]).raw.(int)
					return &object{float64(input), e.Call.Type()}
				case "from_str":
					input := vm.eval(e.Call.Args[0]).raw.(string)

					res := &object{nil, e.Call.Type()}
					if num, err := strconv.ParseFloat(input, 64); err == nil {
						res.raw = num
					}
					return res
				default:
					panic(fmt.Errorf("Unimplemented: Float::%s()", e.Call.Name))
				}
			}

			if e.Package == "ard/fs" {
				switch e.Call.Name {
				case "append":
					path := vm.eval(e.Call.Args[0]).raw.(string)
					content := vm.eval(e.Call.Args[1]).raw.(string)
					res := &object{false, e.Call.Type()}
					if file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644); err == nil {
						if _, err := file.WriteString(content); err == nil {
							res.raw = true
						}
						file.Close()
					}
					return res
				case "create_file":
					path := vm.eval(e.Call.Args[0]).raw.(string)
					res := &object{false, e.Call.Type()}
					if file, err := os.Create(path); err == nil {
						file.Close()
						res.raw = true
					}
					return res
				case "delete":
					path := vm.eval(e.Call.Args[0]).raw.(string)
					res := &object{false, e.Call.Type()}
					if err := os.Remove(path); err == nil {
						res.raw = true
					}
					return res
				case "exists":
					path := vm.eval(e.Call.Args[0]).raw.(string)
					res := &object{false, e.Call.Type()}
					if _, err := os.Stat(path); err == nil {
						res.raw = true
					}
					return res
				case "read":
					path := vm.eval(e.Call.Args[0]).raw.(string)

					res := &object{nil, e.Call.Type()}
					if content, err := os.ReadFile(path); err == nil {
						res.raw = string(content)
					}
					return res
				case "write":
					path := vm.eval(e.Call.Args[0]).raw.(string)
					content := vm.eval(e.Call.Args[1]).raw.(string)

					res := &object{false, e.Call.Type()}

					/* file permissions:
					- `6` (owner): read (4) + write (2) = 6
					- `4` (group): read only
					- `4` (others): read only
					*/
					if err := os.WriteFile(path, []byte(content), 0644); err == nil {
						res.raw = true
					}
					return res
				default:
					panic(fmt.Errorf("Unimplemented: fs::%s()", e.Call.Name))
				}
			}

			if e.Package == "ard/io" {
				switch e.Call.Name {
				case "print":
					arg := vm.eval(e.Call.Args[0])

					string, ok := arg.raw.(string)
					if !ok {
						panic(fmt.Errorf("Unprintable arg to print: %s", arg))
					}
					fmt.Println(string)
					return void
				default:
					panic(fmt.Errorf("Unimplemented: io::%s()", e.Call.Name))
				}
			}

			if e.Package == "ard/maybe" {
				switch e.Call.Name {
				case "none":
					return &object{nil, e.Call.Type()}
				case "some":
					arg := vm.eval(e.Call.Args[0])
					arg._type = e.Call.Type()
					return arg
				default:
					panic(fmt.Errorf("Unimplemented: maybe::%s()", e.Call.Name))
				}
			}
			panic(fmt.Errorf("Unimplemented: %s::%s()", e.Package, e.Call.Name))
		}
	case *checker_v2.ListLiteral:
		{
			raw := make([]*object, len(e.Elements))
			for i, el := range e.Elements {
				raw[i] = vm.eval(el)
			}
			return &object{raw, e.Type()}
		}
	case *checker_v2.MapLiteral:
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
	case *checker_v2.OptionMatch:
		{
			subject := vm.eval(e.Subject)
			if subject.raw == nil {
				// None case - evaluate the None block
				res, _ := vm.evalBlock2(e.None, nil)
				return res
			} else {
				// Some case - bind the value and evaluate the Some block
				res, _ := vm.evalBlock2(e.Some.Body, func() {
					// Bind the pattern name to the value
					subject := &object{subject.raw, subject._type.(*checker_v2.Maybe).Of()}
					vm.scope.add(e.Some.Pattern.Name, subject)
				})
				return res
			}
		}
	case *checker_v2.EnumMatch:
		{
			subject := vm.eval(e.Subject)
			variantIndex := subject.raw.(int8)

			// If there is a catch-all case and we do not have a specific handler for this variant
			if e.CatchAll != nil && (variantIndex >= int8(len(e.Cases)) || e.Cases[variantIndex] == nil) {
				res, _ := vm.evalBlock2(e.CatchAll, nil)
				return res
			}

			// Execute the matching case block for this variant
			if variantIndex < int8(len(e.Cases)) && e.Cases[variantIndex] != nil {
				res, _ := vm.evalBlock2(e.Cases[variantIndex], nil)
				return res
			}

			// This should never happen if the type checker is working correctly
			// because it ensures the match is exhaustive
			panic(fmt.Errorf("No matching case for enum variant %d", variantIndex))
		}
	case *checker_v2.EnumVariant:
		return &object{e.Variant, e.Type()}
	case *checker_v2.BoolMatch:
		{
			subject := vm.eval(e.Subject)
			value := subject.raw.(bool)

			// Execute the appropriate case based on the boolean value
			if value {
				res, _ := vm.evalBlock2(e.True, nil)
				return res
			} else {
				res, _ := vm.evalBlock2(e.False, nil)
				return res
			}
		}
	case *checker_v2.UnionMatch:
		{
			subject := vm.eval(e.Subject)

			// Get the concrete type name as a string
			typeName := subject._type.(checker_v2.Type).String()

			// If we have a case for this specific type
			if block, ok := e.TypeCases[typeName]; ok {
				res, _ := vm.evalBlock2(block, func() {
					// Bind the pattern variable 'it' to the value
					vm.scope.add("it", subject)
				})
				return res
			}

			// If we have a catch-all case
			if e.CatchAll != nil {
				res, _ := vm.evalBlock2(e.CatchAll, nil)
				return res
			}

			// This should never happen if the type checker is working correctly
			// because it ensures the match is exhaustive
			panic(fmt.Errorf("No matching case for union type %s", typeName))
		}
	default:
		panic(fmt.Errorf("Unimplemented expression: %T", e))
	}
}

func (vm *VM) evalBlock2(block *checker_v2.Block, init func()) (*object, bool) {
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
		return &object{len(subj.raw.(string)), checker_v2.Int}
	default:
		return void
	}
}

func (vm *VM) evalStrMethod(subj *object, m *checker_v2.FunctionCall) *object {
	switch m.Name {
	case "size":
		return &object{len(subj.raw.(string)), checker_v2.Int}
	case "is_empty":
		return &object{len(subj.raw.(string)) == 0, checker_v2.Bool}
	case "contains":
		return &object{strings.Contains(subj.raw.(string), vm.eval(m.Args[0]).raw.(string)), checker_v2.Bool}
	default:
		return void
	}
}

func (vm *VM) evalIntMethod(subj *object, m *checker_v2.InstanceMethod) *object {
	switch m.Method.Name {
	case "to_str":
		return &object{strconv.Itoa(subj.raw.(int)), checker_v2.Str}
	default:
		return void
	}
}

func (vm *VM) evalFloatMethod(subj *object, m *checker_v2.FunctionCall) *object {
	switch m.Name {
	case "to_str":
		return &object{strconv.FormatFloat(subj.raw.(float64), 'f', 2, 64), checker_v2.Str}
	default:
		return void
	}
}

func (vm *VM) evalBoolMethod(subj *object, m *checker_v2.InstanceMethod) *object {
	switch m.Method.Name {
	case "to_str":
		return &object{strconv.FormatBool(subj.raw.(bool)), checker_v2.Str}
	default:
		return void
	}
}

func (vm *VM) evalListMethod(subj *object, m *checker_v2.InstanceMethod) *object {
	raw := subj.raw.([]*object)
	switch m.Method.Name {
	case "at":
		index := vm.eval(m.Method.Args[0]).raw.(int)
		return &object{raw[index].raw, m.Type()}
	case "set":
		index := vm.eval(m.Method.Args[0]).raw.(int)
		value := vm.eval(m.Method.Args[1])
		result := &object{false, checker_v2.Bool}
		if index <= len(raw) {
			raw[index] = value
			result.raw = true
		}
		return result
	case "size":
		return &object{len(raw), checker_v2.Int}
	case "push":
		subj.raw = append(raw, vm.eval(m.Method.Args[0]))
		return subj
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj._type, m.Method.Name))
	}
}

func (vm *VM) evalMapMethod(subj *object, m *checker_v2.InstanceMethod) *object {
	raw := subj.raw.(map[string]*object)
	switch m.Method.Name {
	case "size":
		return &object{len(raw), checker_v2.Int}
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
		return &object{true, checker_v2.Bool}
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
		return &object{found, checker_v2.Bool}
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj._type, m.Method.Name))
	}
}

func (vm *VM) evalMaybeMethod(subj *object, m *checker_v2.InstanceMethod) *object {
	switch m.Method.Name {
	case "or":
		if subj.raw == nil {
			return vm.eval(m.Method.Args[0])
		}
		return &object{subj.raw, m.Type()}
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj._type, m.Method.Name))
	}
}
