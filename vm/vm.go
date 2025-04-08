package vm

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/akonwi/ard/checker"
)

type VM struct {
	program *checker.Program
	scope   *scope
	result  object
}

func New(program *checker.Program) *VM {
	return &VM{program: program, scope: newScope(nil)}
}

func (vm *VM) pushScope() {
	vm.scope = newScope(vm.scope)
}

func (vm *VM) popScope() {
	vm.scope = vm.scope.parent
}

func (vm *VM) Run() (any, error) {
	for _, statement := range vm.program.Statements {
		vm.evalStatement(statement)
	}
	return vm.result.raw, nil
}

func (vm *VM) addVariable(name string, value *object) {
	vm.scope.bindings[name] = value
}

func (vm *VM) evalStatement(stmt checker.Statement) *object {
	switch s := stmt.(type) {
	case checker.Break:
		vm.scope._break()
		return nil
	case checker.VariableBinding:
		vm.evalVariableBinding(s)
	case checker.VariableAssignment:
		vm.evalVariableAssignment(s)
	case checker.FunctionDeclaration:
		vm.evalFunctionDefinition(s)
	case checker.Enum:
		vm.scope.addEnum(s)
	case *checker.Struct:
		vm.scope.addStruct(s)
	case checker.IfStatement:
		var condition bool = true
		if s.Condition != nil {
			condition = vm.evalExpression(s.Condition).raw.(bool)
		}
		if condition {
			vm.pushScope()
			for _, statement := range s.Body {
				vm.evalStatement(statement)
			}
			vm.popScope()
		} else if s.Else != nil {
			vm.evalStatement(s.Else)
		}
	case checker.ForRange:
		for i := vm.evalExpression(s.Start).raw.(int); i <= vm.evalExpression(s.End).raw.(int); i++ {
			cursor := &object{i, checker.Int{}}
			variables := map[string]*object{s.Cursor.Name: cursor}
			if _, breaks := vm.evalBlock(s.Body, variables, true); breaks {
				break
			}
		}
	case checker.ForIn:
		iterable := vm.evalExpression(s.Iterable)
		switch iterable._type.(type) {
		case checker.Str:
			for _, item := range iterable.raw.(string) {
				cursor := &object{string(item), checker.Str{}}
				variables := map[string]*object{s.Cursor.Name: cursor}
				if _, breaks := vm.evalBlock(s.Body, variables, true); breaks {
					break
				}
			}
		case checker.List:
			for _, item := range iterable.raw.([]*object) {
				variables := map[string]*object{s.Cursor.Name: item}
				if _, breaks := vm.evalBlock(s.Body, variables, true); breaks {
					break
				}
			}
		default:
			panic(fmt.Errorf("Unexpected iterable type: %v", iterable._type))
		}

	case checker.ForLoop:
		vm.pushScope()
		vm.scope.breakable = true
		defer vm.popScope()

		for vm.evalStatement(s.Init); vm.evalExpression(s.Condition).raw.(bool); vm.evalStatement(s.Step) {
			if _, breaks := vm.evalBlock(s.Body.Body, map[string]*object{}, true); breaks {
				break
			}
		}

	case checker.WhileLoop:
		for vm.evalExpression(s.Condition).raw.(bool) && !vm.scope.isBroken() {
			_, broke := vm.evalBlock(s.Body, map[string]*object{}, true)
			if broke {
				break
			}
		}
	default:
		expr, ok := s.(checker.Expression)
		if !ok {
			panic(fmt.Sprintf("Unimplemented statement: %T", s))
		}
		result := vm.evalExpression(expr)
		if result != nil {
			vm.result = *result
		}
		return result
	}

	return &object{}
}

func (vm *VM) evalVariableBinding(_binding checker.VariableBinding) {
	value := vm.evalExpression(_binding.Value)
	vm.scope.bindings[_binding.Name] = value
	vm.result = *value
}

func (vm *VM) evalVariableAssignment(assignment checker.VariableAssignment) {
	switch target := assignment.Target.(type) {
	case checker.Identifier:
		value := vm.evalExpression(assignment.Value)
		vm.scope.set(target.Name, value)
		vm.result = *value
	case checker.InstanceProperty:
		subject := vm.evalExpression(target.Subject)
		raw := subject.raw.(map[string]*object)
		raw[target.Property.Name] = vm.evalExpression(assignment.Value)
	default:
		panic(fmt.Sprintf("Unimplemented assignment target: %T", target))
	}
}

func (vm *VM) evalFunctionDefinition(fn checker.FunctionDeclaration) {
	raw := func(args ...object) object {
		vm.pushScope()
		for i, arg := range args {
			vm.addVariable(fn.Parameters[i].Name, &arg)
		}
		result := &object{}
		for _, statement := range fn.Body {
			result = vm.evalStatement(statement)
		}
		vm.popScope()
		return *result
	}
	vm.scope.bindings[fn.Name] = &object{
		raw:   raw,
		_type: fn.GetType(),
	}
}

type object struct {
	raw   any
	_type checker.Type
}

func areEqual(a, b *object) bool {
	if a.raw == b.raw {
		if a._type.String() == b._type.String() {
			return true
		}
	}
	return false
}

func (o *object) isCallable() bool {
	_, isFunc := o.raw.(func(args ...object) object)
	return isFunc
}

func (o object) String() string {
	return fmt.Sprintf("%v:%s", o.raw, o._type)
}

func (o *object) premarshal() any {
	switch o._type.(type) {
	case checker.Str, checker.Int, checker.Float, checker.Bool:
		return o.raw
	case checker.List:
		list := o.raw.([]*object)
		rawList := make([]any, len(list))
		for i := range list {
			rawList[i] = list[i].premarshal()
		}
		return rawList
	case checker.Map, *checker.Struct:
		m := o.raw.(map[string]*object)
		rawMap := make(map[string]any)
		for key, value := range m {
			rawMap[key] = value.premarshal()
		}
		return rawMap
	default:
		panic(fmt.Sprintf("Cannot marshall type: %T", o._type))
	}
}

func (vm *VM) evalExpression(expr checker.Expression) *object {
	switch e := expr.(type) {
	case checker.StrLiteral:
		return &object{e.Value, expr.GetType()}
	case checker.InterpolatedStr:
		builder := strings.Builder{}
		for _, part := range e.Parts {
			obj := vm.evalExpression(part)
			if str, ok := obj.raw.(string); ok {
				builder.WriteString(str)
			}
		}
		return &object{builder.String(), checker.Str{}}
	case checker.IntLiteral:
		return &object{e.Value, e.GetType()}
	case checker.FloatLiteral:
		return &object{e.Value, e.GetType()}
	case checker.BoolLiteral:
		return &object{e.Value, e.GetType()}
	case checker.ListLiteral:
		list := make([]*object, len(e.Elements))
		for i, elem := range e.Elements {
			list[i] = vm.evalExpression(elem)
		}
		return &object{list, e.GetType()}
	case checker.Identifier:
		if e.Name == "_" {
			return &object{nil, checker.Void{}}
		}
		if v, ok := vm.scope.get(e.Name); ok {
			return v
		}
		panic(fmt.Sprintf("Variable not found: %s", e.Name))
	case checker.Not:
		val := vm.evalExpression(e.Value)
		val.raw = !val.raw.(bool)
		return val
	case checker.Negation:
		val := vm.evalExpression(e.Value)
		switch raw := val.raw.(type) {
		case int:
			val.raw = -raw
		case float64:
			val.raw = -raw
		}
		return val
	case checker.InstanceProperty:
		return vm.evalProperty(vm.evalExpression(e.Subject), e.Property)
	case checker.InstanceMethod:
		return vm.evalInstanceMethod(vm.evalExpression(e.Subject), e.Method)
	case checker.BinaryExpr:
		switch e.Op {
		case checker.Add:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			if _, isInt := left._type.(checker.Int); isInt {
				return &object{left.raw.(int) + right.raw.(int), left._type}
			}
			return &object{left.raw.(float64) + right.raw.(float64), left._type}
		case checker.Sub:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			if _, isInt := left._type.(checker.Int); isInt {
				return &object{left.raw.(int) - right.raw.(int), left._type}
			}
			return &object{left.raw.(float64) - right.raw.(float64), left._type}
		case checker.Mul:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			if _, isInt := left._type.(checker.Int); isInt {
				return &object{left.raw.(int) * right.raw.(int), left._type}
			}
			return &object{left.raw.(float64) * right.raw.(float64), left._type}
		case checker.Div:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			if _, isInt := left._type.(checker.Int); isInt {
				return &object{left.raw.(int) / right.raw.(int), left._type}
			}
			return &object{left.raw.(float64) / right.raw.(float64), left._type}
		case checker.Mod:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(int) % right.raw.(int), left._type}
		case checker.GreaterThan:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			if _, isInt := left._type.(checker.Int); isInt {
				return &object{left.raw.(int) > right.raw.(int), left._type}
			}
			return &object{left.raw.(float64) > right.raw.(float64), left._type}
		case checker.GreaterThanOrEqual:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			if _, isInt := left._type.(checker.Int); isInt {
				return &object{left.raw.(int) >= right.raw.(int), left._type}
			}
			return &object{left.raw.(float64) >= right.raw.(float64), left._type}
		case checker.LessThan:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			if _, isInt := left._type.(checker.Int); isInt {
				return &object{left.raw.(int) < right.raw.(int), left._type}
			}
			return &object{left.raw.(float64) < right.raw.(float64), left._type}
		case checker.LessThanOrEqual:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			if _, isInt := left._type.(checker.Int); isInt {
				return &object{left.raw.(int) <= right.raw.(int), left._type}
			}
			return &object{left.raw.(float64) <= right.raw.(float64), left._type}

		// for equality, compare the entire objects, so that types are considered
		case checker.Equal:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{areEqual(left, right), e.GetType()}
		case checker.NotEqual:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{*(left) != *(right), e.GetType()}
		case checker.And:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(bool) && right.raw.(bool), e.GetType()}
		case checker.Or:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(bool) || right.raw.(bool), e.GetType()}
		default:
			panic(fmt.Sprintf("Unimplemented binary op: %v", e.Op))
		}
	case checker.FunctionCall:
		if fn, ok := vm.scope.getFunction(e.Name); ok {
			args := make([]object, len(e.Args))
			for i, arg := range e.Args {
				args[i] = *vm.evalExpression(arg)
			}
			result := fn(args...)
			return &result
		}
		panic(fmt.Sprintf("Function not found: %s", e.Name))
	case checker.StaticFunctionCall:
		args := make([]object, len(e.Function.Args))
		for i, arg := range e.Function.Args {
			args[i] = *vm.evalExpression(arg)
		}
		switch subj := e.Subject.(type) {
		case checker.Int:
			switch e.Function.Name {
			case "from_str":
				res := &object{nil, checker.MakeMaybe(checker.Int{})}
				if num, err := strconv.Atoi(args[0].raw.(string)); err == nil {
					res.raw = num
				}
				return res
			}

		case checker.Float:
			switch e.Function.Name {
			case "from_str":
				res := &object{nil, checker.Float{}}
				if num, err := strconv.ParseFloat(args[0].raw.(string), 64); err == nil {
					res.raw = num
				}
				return res

			case "from_int":
				return &object{float64(args[0].raw.(int)), checker.Float{}}
			}
		case *checker.Struct:
			switch e.Function.Name {
			case "from_json":
				result := &object{nil, e.Function.GetType()}
				_map := make(map[string]any)
				jsonString := args[0].raw.(string)
				err := json.Unmarshal([]byte(jsonString), &_map)
				if err != nil {
					fmt.Printf("Error unmarshalling: %s\n", err)
					return result
				}

				fields := make(map[string]*object)
				for name, fType := range subj.Fields {
					val := _map[name]
					if f64, ok := val.(float64); ok && fType == (checker.Int{}) {
						val = int(f64)
					}
					fields[name] = &object{val, fType}
				}

				result.raw = fields
				return result
			}
		}
		panic(fmt.Sprintf("Function not found: %s", e.Function.Name))
	case checker.FunctionLiteral:
		return &object{
			raw: func(args ...object) object {
				vm.pushScope()
				for i, arg := range args {
					vm.addVariable(e.Parameters[i].Name, &arg)
				}
				var result object
				for _, statement := range e.Body {
					result = *vm.evalStatement(statement)
				}
				vm.popScope()
				return result
			},
			_type: e.GetType(),
		}
	case checker.EnumVariant:
		return &object{e.Value, e.GetType()}
	case checker.BoolMatch:
		return vm.matchBool(e)
	case checker.EnumMatch:
		return vm.matchEnum(e)
	case checker.OptionMatch:
		return vm.matchOption(e)
	case checker.UnionMatch:
		return vm.matchUnion(e)
	case checker.StructInstance:
		sym, ok := vm.scope.getStruct(e.Name)
		if !ok {
			panic(fmt.Sprintf("Struct not found: %s", e.Name))
		}
		fields := make(map[string]*object)
		for name, value := range e.Fields {
			fields[name] = vm.evalExpression(value)
		}
		return &object{fields, sym}

	case checker.MapLiteral:
		entries := make(map[object]*object)
		for key, value := range e.Entries {
			entries[*vm.evalExpression(key)] = vm.evalExpression(value)
		}
		return &object{entries, e.GetType()}
	case checker.PackageAccess:
		// todo: eval e.Property, then call pkg methods
		// so pkgs do not need to know the VM
		switch e.Package.GetPath() {
		case "ard/fs":
			return vm.invokeFS(e.Property)
		case "ard/io":
			return vm.invokeIO(e.Property)
		case "ard/maybe":
			return vm.invokeMaybe(e.Property)
		case "ard/json":
			return vm.invokeJSON(e.Property)
		default:
			panic(fmt.Sprintf("Unimplemented package: %s", e.Package.GetPath()))
		}
	default:
		panic(fmt.Sprintf("Unimplemented expression: %T", e))
	}
}

func (vm VM) evalProperty(i *object, prop checker.Identifier) *object {
	switch i._type.(type) {
	case *checker.Struct:
		if field, ok := i.raw.(map[string]*object)[prop.Name]; ok {
			return field
		}
	}
	panic(fmt.Errorf("Undefined property: %s.%s", i._type, prop))
}

func (vm VM) evalInstanceMethod(o *object, fn checker.FunctionCall) *object {
	switch t := o._type.(type) {
	case checker.Str:
		str := o.raw.(string)
		switch fn.Name {
		case "size":
			return &object{len(str), checker.Int{}}
		case "is_empty":
			return &object{len(str) == 0, checker.Bool{}}
		case "contains":
			needle := vm.evalExpression(fn.Args[0]).raw.(string)
			return &object{strings.Contains(str, needle), checker.Bool{}}
		}
		panic(fmt.Sprintf("Undefined method: %s.%s", o._type, fn.Name))
	case checker.Int:
		switch fn.Name {
		case "to_str":
			return &object{strconv.Itoa(o.raw.(int)), checker.Str{}}
		}

	case checker.Float:
		switch fn.Name {
		case "to_str":
			return &object{strconv.FormatFloat(o.raw.(float64), 'f', 2, 64), checker.Str{}}
		}
	case checker.Bool:
		switch fn.Name {
		case "to_str":
			return &object{strconv.FormatBool(o.raw.(bool)), checker.Str{}}
		}
	case checker.List:
		list, ok := o.raw.([]*object)
		if !ok {
			panic(fmt.Sprintf("Expected list, got %T", o.raw))
		}
		switch fn.Name {
		case "size":
			return &object{len(list), checker.Int{}}
		case "push":
			item := vm.evalExpression(fn.Args[0])
			o.raw = append(list, item)
			return &object{len(list), checker.Int{}}
		case "at":
			index := vm.evalExpression(fn.Args[0]).raw.(int)
			val := list[index]
			if val == nil {
				return &object{val, checker.MakeMaybe(t.GetElementType())}
			}
			return &object{val.raw, checker.MakeMaybe(t.GetElementType())}
		case "set":
			result := &object{false, checker.Bool{}}
			index := vm.evalExpression(fn.Args[0]).raw.(int)
			if index >= len(list) {
				return result
			}
			list[index] = vm.evalExpression(fn.Args[1])
			result.raw = true
			return result
		default:
			panic(fmt.Sprintf("Unimplemented method: %s.%s", o._type, fn.Name))
		}

	case checker.Map:
		m := o.raw.(map[object]*object)
		switch fn.Name {
		case "size":
			return &object{len(m), checker.Int{}}
		case "set":
			key := vm.evalExpression(fn.Args[0])
			val := vm.evalExpression(fn.Args[1])
			m[*key] = val
			return &object{nil, checker.Void{}}
		case "get":
			key := vm.evalExpression(fn.Args[0])
			if val, ok := m[*key]; ok {
				return &object{val.raw, fn.GetType()}
			}
			return &object{nil, fn.GetType()}
		case "drop":
			key := vm.evalExpression(fn.Args[0])
			delete(m, *key)
			return &object{nil, checker.Void{}}
		case "has":
			key := vm.evalExpression(fn.Args[0])
			return &object{m[*key] != nil, checker.Bool{}}
		default:
			panic(fmt.Sprintf("Unknown method: %s.%s", o._type, fn.Name))
		}

	case checker.Maybe:
		switch fn.Name {
		case "or":
			if o.raw != nil {
				return &object{o.raw, t.GetInnerType()}
			}
			return vm.evalExpression(fn.Args[0])
		default:
			panic(fmt.Sprintf("Unknown method: %s.%s", o._type, fn.Name))
		}
	case *checker.Struct:
		method, ok := t.GetMethod(fn.Name)
		if !ok {
			panic(fmt.Sprintf("Undefined method: %s.%s", o._type, fn.Name))
		}
		args := map[string]*object{
			t.GetInstanceId(): o,
		}
		for i, param := range method.Parameters {
			args[param.Name] = vm.evalExpression(fn.Args[i])
		}
		res, _ := vm.evalBlock(method.Body, args, false)
		return res
	}
	panic(fmt.Sprintf("Unknown method: %s.%s", o._type, fn.Name))
}

func (vm VM) matchBool(match checker.BoolMatch) *object {
	subj := vm.evalExpression(match.Subject)

	if subj.raw == true {
		res, _ := vm.evalBlock(match.True.Body, nil, false)
		return res
	}
	res, _ := vm.evalBlock(match.False.Body, nil, false)
	return res
}

func (vm VM) matchEnum(match checker.EnumMatch) *object {
	subj := vm.evalExpression(match.Subject)
	for value, arm := range match.Cases {
		if subj.raw == value {
			res, _ := vm.evalBlock(arm.Body, nil, false)
			return res
		}
	}

	if match.CatchAll.Body != nil {
		variables := map[string]*object{}
		if id, ok := match.CatchAll.Pattern.(checker.Identifier); ok {
			variables[id.Name] = subj
		}
		res, _ := vm.evalBlock(match.CatchAll.Body, variables, false)
		return res
	}
	panic(fmt.Sprintf("No match found for %s", subj))
}

func (vm VM) matchOption(match checker.OptionMatch) *object {
	subj := vm.evalExpression(match.Subject)
	if subj.raw == nil {
		res, _ := vm.evalBlock(match.None.Body, nil, false)
		return res
	}

	bindingName := match.Some.Pattern.(checker.Identifier).Name
	it := &object{subj.raw, subj._type.(checker.Maybe).GetInnerType()}
	res, _ := vm.evalBlock(
		match.Some.Body,
		map[string]*object{bindingName: it},
		false,
	)
	return res
}

func (vm VM) matchUnion(match checker.UnionMatch) *object {
	subj := vm.evalExpression(match.Subject)
	for expected_type, arm := range match.Cases {
		if checker.AreCoherent(subj._type, expected_type) {
			res, _ := vm.evalBlock(
				arm.Body,
				map[string]*object{
					"it": subj,
				},
				false,
			)
			return res
		}
	}
	if match.CatchAll.Body != nil {
		res, _ := vm.evalBlock(match.CatchAll.Body, map[string]*object{}, false)
		return res
	}
	panic(fmt.Sprintf("No match found for %s", subj))
}

func (vm VM) evalBlock(block []checker.Statement, variables map[string]*object, breakable bool) (*object, bool) {
	vm.pushScope()
	defer vm.popScope()

	if breakable {
		vm.scope.breakable = true
	}
	for name, variable := range variables {
		vm.scope.bindings[name] = variable
	}

	var result *object
	for _, stmt := range block {
		result = vm.evalStatement(stmt)
		if vm.scope.isBroken() {
			return result, true
		}
	}
	return result, false
}
