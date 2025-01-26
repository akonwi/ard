package vm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/akonwi/ard/internal/checker"
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

func (vm *VM) addVariable(mut bool, name string, value *object) {
	vm.scope.bindings[name] = &binding{mut, value, false}
}

func (vm *VM) evalStatement(stmt checker.Statement) *object {
	switch s := stmt.(type) {
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
		vm.pushScope()
		cursor := &binding{false, &object{}, false}
		vm.scope.bindings[s.Cursor.Name] = cursor
		for i := vm.evalExpression(s.Start).raw.(int); i <= vm.evalExpression(s.End).raw.(int); i++ {
			cursor.value = &object{i, checker.Num{}}
			for _, statement := range s.Body {
				vm.evalStatement(statement)
			}
		}
		vm.popScope()
	case checker.ForIn:
		vm.pushScope()
		cursor := &binding{false, &object{}, false}
		vm.scope.bindings[s.Cursor.Name] = cursor
		iterable := vm.evalExpression(s.Iterable)
		switch iterable._type.(type) {
		case checker.Str:
			for _, item := range iterable.raw.(string) {
				cursor.value = &object{string(item), checker.Str{}}
				for _, statement := range s.Body {
					vm.evalStatement(statement)
				}
			}
		case checker.List:
			for _, item := range iterable.raw.([]*object) {
				cursor.value = item
				for _, statement := range s.Body {
					vm.evalStatement(statement)
				}
			}
		default:
			panic(fmt.Errorf("Unimplemented iterable type: %v", iterable._type))
		}
		vm.popScope()
	case checker.WhileLoop:
		for vm.evalExpression(s.Condition).raw.(bool) {
			vm.pushScope()
			for _, statement := range s.Body {
				vm.evalStatement(statement)
			}
			vm.popScope()
		}
	default:
		expr, ok := s.(checker.Expression)
		if !ok {
			panic(fmt.Sprintf("Unimplemented statement: %T", s))
		}
		result := vm.evalExpression(expr)
		vm.result = *result
		return result
	}

	return &object{}
}

func (vm *VM) evalVariableBinding(_binding checker.VariableBinding) {
	value := vm.evalExpression(_binding.Value)
	// todo: callable could be determined by casting value._type to checker.function
	_, callable := value.raw.(func(args ...object) object)
	vm.scope.bindings[_binding.Name] = &binding{false, value, callable}
	vm.result = *value
}

func (vm *VM) evalVariableAssignment(assignment checker.VariableAssignment) {
	value := vm.evalExpression(assignment.Value)
	if variable, ok := vm.scope.get(assignment.Name); ok {
		(*variable).value = value
		vm.result = *value
	}
}

func (vm *VM) evalFunctionDefinition(fn checker.FunctionDeclaration) {
	vm.scope.bindings[fn.Name] = &binding{
		mut:      false,
		callable: true,
		value: &object{
			raw: func(args ...object) object {
				vm.pushScope()
				for i, arg := range args {
					vm.addVariable(false, fn.Parameters[i].Name, &arg)
				}
				result := &object{}
				for _, statement := range fn.Body {
					result = vm.evalStatement(statement)
				}
				vm.popScope()
				return *result
			},
			_type: fn.GetType(),
		},
	}
}

type object struct {
	raw   any
	_type checker.Type
}

func (o object) String() string {
	return fmt.Sprintf("%v:%s", o.raw, o._type)
}

func (o object) equals(other object) bool {
	return o.raw == other.raw && o._type.Matches(other._type)
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
			} else {
				panic(fmt.Sprintf("Expected Str, got %s", obj))
			}
		}
		return &object{builder.String(), checker.Str{}}
	case checker.NumLiteral:
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
			return v.value
		}
		panic(fmt.Sprintf("Variable not found: %s", e.Name))
	case checker.Not:
		val := vm.evalExpression(e.Value)
		val.raw = !val.raw.(bool)
		return val
	case checker.Negation:
		val := vm.evalExpression(e.Value)
		val.raw = -val.raw.(int)
		return val
	case checker.InstanceProperty:
		i := vm.evalExpression(e.Subject)
		return vm.evalProperty(i, e.Property)
	case checker.BinaryExpr:
		switch e.Op {
		case checker.Add:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(int) + right.raw.(int), left._type}
		case checker.Sub:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(int) - right.raw.(int), left._type}
		case checker.Mul:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(int) * right.raw.(int), left._type}
		case checker.Div:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(int) / right.raw.(int), left._type}
		case checker.Mod:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(int) % right.raw.(int), left._type}
		case checker.GreaterThan:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(int) > right.raw.(int), e.GetType()}
		case checker.GreaterThanOrEqual:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(int) >= right.raw.(int), e.GetType()}
		case checker.LessThan:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(int) < right.raw.(int), e.GetType()}
		case checker.LessThanOrEqual:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{left.raw.(int) <= right.raw.(int), e.GetType()}

		// for equality, compare the entire objects, so that types are considered
		case checker.Equal:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return &object{*(left) == *(right), e.GetType()}
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
	case checker.FunctionLiteral:
		return &object{
			raw: func(args ...object) object {
				vm.pushScope()
				for i, arg := range args {
					vm.addVariable(false, e.Parameters[i].Name, &arg)
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
		switch e.Package.Path {
		case "ard/io":
			return vm.invokeIO(e.Property)
		case "ard/option":
			return vm.invokeOption(e.Property)
		default:
			panic(fmt.Sprintf("Unimplemented package: %s", e.Package.Path))
		}
	default:
		panic(fmt.Sprintf("Unimplemented expression: %T", e))
	}
}

func (vm VM) evalProperty(i *object, prop checker.Expression) *object {
	// TODO: InstanceProperty.Property should only be an Identifier
	if fn, ok := prop.(checker.FunctionCall); ok {
		return vm.evalInstanceMethod(i, fn)
	}
	propName := prop.(checker.Identifier).Name

	switch i._type.(type) {
	case checker.Str:
		raw := i.raw.(string)
		switch propName {
		case "size":
			return &object{len(raw), checker.Num{}}
		case "is_empty":
			return &object{len(raw) == 0, checker.Bool{}}
		default:
			panic(fmt.Errorf("Undefined property: Str.%v", propName))
		}
	case checker.Num:
		switch propName {
		case "as_str":
			return &object{strconv.Itoa(i.raw.(int)), checker.Str{}}
		default:
			panic(fmt.Errorf("Undefined property: Num.%v", propName))
		}
	case checker.Bool:
		switch propName {
		case "as_str":
			return &object{strconv.FormatBool(i.raw.(bool)), checker.Str{}}
		default:
			panic(fmt.Errorf("Undefined property: Bool.%v", propName))
		}
	case checker.List:
		switch propName {
		case "size":
			return &object{len(i.raw.([]*object)), checker.Num{}}
		default:
			panic(fmt.Errorf("Unimplemented property: %s.%v", i._type, propName))
		}
	case checker.Map:
		switch propName {
		case "size":
			return &object{len(i.raw.(map[object]*object)), checker.Num{}}
		default:
			panic(fmt.Errorf("Unimplemented property: %s.%v", i._type, propName))
		}
	case *checker.Struct:
		if field, ok := i.raw.(map[string]*object)[propName]; ok {
			return field
		}
		panic(fmt.Sprintf("Field not found: %s", propName))
	default:
		return &object{nil, checker.Void{}}
	}
}

func (vm VM) evalInstanceMethod(o *object, fn checker.FunctionCall) *object {
	switch t := o._type.(type) {
	case checker.List:
		list, ok := o.raw.([]*object)
		if !ok {
			panic(fmt.Sprintf("Expected list, got %T", o.raw))
		}
		switch fn.Name {
		case "push":
			item := vm.evalExpression(fn.Args[0])
			o.raw = append(list, item)
			return &object{len(list), checker.Num{}}
		case "at":
			index := vm.evalExpression(fn.Args[0]).raw.(int)
			return list[index]
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
		case "set":
			key := vm.evalExpression(fn.Args[0])
			val := vm.evalExpression(fn.Args[1])
			m[*key] = val
			return &object{nil, checker.Void{}}
		case "get":
			key := vm.evalExpression(fn.Args[0])
			if val, ok := m[*key]; ok {
				return &object{val.raw, val._type}
			}
			return &object{nil, fn.GetType().(checker.Option).GetInnerType()}
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

	case checker.Option:
		switch fn.Name {
		case "some":
			o.raw = vm.evalExpression(fn.Args[0]).raw
			return &object{nil, checker.Void{}}
		case "none":
			o.raw = nil
			return &object{nil, checker.Void{}}
		default:
			panic(fmt.Sprintf("Unknown method: %s.%s", o._type, fn.Name))
		}
	case *checker.Struct:
		method, ok := t.GetMethod(fn.Name)
		if !ok {
			panic(fmt.Sprintf("Undefined method: %s.%s", o._type, fn.Name))
		}
		args := map[string]binding{
			t.GetInstanceId(): {false, o, false},
		}
		for i, param := range method.Parameters {
			args[param.Name] = binding{false, vm.evalExpression(fn.Args[i]), false}
		}
		return vm.evalBlock(method.Body, args)
	default:
		panic(fmt.Sprintf("Unknown method: %s.%s", o._type, fn.Name))
		// return &object{nil, checker.Void{}}
	}
}

func (vm VM) matchBool(match checker.BoolMatch) *object {
	subj := vm.evalExpression(match.Subject)

	if subj.raw == true {
		return vm.evalBlock(match.True.Body, nil)
	}
	return vm.evalBlock(match.False.Body, nil)
}

func (vm VM) matchEnum(match checker.EnumMatch) *object {
	subj := vm.evalExpression(match.Subject)
	for value, arm := range match.Cases {
		if subj.raw == value {
			return vm.evalBlock(arm.Body, nil)
		}
	}

	if match.CatchAll.Body != nil {
		variables := map[string]binding{}
		if id, ok := match.CatchAll.Pattern.(checker.Identifier); ok {
			variables[id.Name] = binding{false, subj, false}
		}
		return vm.evalBlock(match.CatchAll.Body, variables)
	}
	panic(fmt.Sprintf("No match found for %s", subj))
}

func (vm VM) matchOption(match checker.OptionMatch) *object {
	subj := vm.evalExpression(match.Subject)
	if subj.raw == nil {
		return vm.evalBlock(match.None.Body, nil)
	}

	bindingName := match.Some.Pattern.(checker.Identifier).Name
	it := binding{false, subj, false}
	return vm.evalBlock(
		match.Some.Body,
		map[string]binding{bindingName: it},
	)
}

func (vm VM) matchUnion(match checker.UnionMatch) *object {
	subj := vm.evalExpression(match.Subject)
	for expected_type, arm := range match.Cases {
		if subj._type.Matches(expected_type) {
			return vm.evalBlock(arm.Body, map[string]binding{
				"it": {false, subj, false},
			})
		}
	}
	if match.CatchAll.Body != nil {
		return vm.evalBlock(match.CatchAll.Body, map[string]binding{})
	}
	panic(fmt.Sprintf("No match found for %s", subj))
}

func (vm VM) evalBlock(block []checker.Statement, variables map[string]binding) *object {
	vm.pushScope()
	for name, variable := range variables {
		vm.scope.bindings[name] = &variable
	}

	var result *object
	for _, stmt := range block {
		result = vm.evalStatement(stmt)
	}
	vm.popScope()
	return result
}
