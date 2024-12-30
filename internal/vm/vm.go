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

func (vm *VM) addVariable(mut bool, name string, value object) {
	vm.scope.bindings[name] = &binding{mut, value, false}
}

func (vm *VM) evalStatement(stmt checker.Statement) object {
	switch s := stmt.(type) {
	case checker.VariableBinding:
		vm.evalVariableBinding(s)
	case checker.VariableAssignment:
		vm.evalVariableAssignment(s)
	case checker.FunctionDeclaration:
		vm.evalFunctionDefinition(s)
	case checker.PackageAccess:
		switch s.Package.Path {
		case "std/io":
			vm.doIO(s.Property)
		default:
			panic(fmt.Sprintf("Unimplemented package: %s", s.Package.Path))
		}
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
		cursor := &binding{false, object{}, false}
		vm.scope.bindings[s.Cursor.Name] = cursor
		for i := vm.evalExpression(s.Start).raw.(int); i <= vm.evalExpression(s.End).raw.(int); i++ {
			cursor.value = object{i, checker.Num{}}
			for _, statement := range s.Body {
				vm.evalStatement(statement)
			}
		}
		vm.popScope()
	case checker.ForIn:
		vm.pushScope()
		cursor := &binding{false, object{}, false}
		vm.scope.bindings[s.Cursor.Name] = cursor
		iterable := vm.evalExpression(s.Iterable).raw
		switch iter := iterable.(type) {
		case string:
			for _, item := range iter {
				cursor.value = object{string(item), checker.Str{}}
				for _, statement := range s.Body {
					vm.evalStatement(statement)
				}
			}
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
		vm.result = vm.evalExpression(expr)
		return vm.result
	}

	return object{}
}

func (vm *VM) evalVariableBinding(_binding checker.VariableBinding) {
	value := vm.evalExpression(_binding.Value)
	// todo: callable could be determined by casting value._type to checker.function
	_, callable := value.raw.(func(args ...object) object)
	vm.scope.bindings[_binding.Name] = &binding{false, value, callable}
	vm.result = value
}

func (vm *VM) evalVariableAssignment(assignment checker.VariableAssignment) {
	value := vm.evalExpression(assignment.Value)
	if variable, ok := vm.scope.get(assignment.Name); ok {
		(*variable).value = value
		vm.result = value
	}
}

func (vm *VM) evalFunctionDefinition(fn checker.FunctionDeclaration) {
	vm.scope.bindings[fn.Name] = &binding{
		mut:      false,
		callable: true,
		value: object{
			raw: func(args ...object) object {
				vm.pushScope()
				for i, arg := range args {
					vm.addVariable(false, fn.Parameters[i].Name, arg)
				}
				result := object{}
				for _, statement := range fn.Body {
					result = vm.evalStatement(statement)
				}
				vm.popScope()
				return result
			},
			_type: fn.GetType(),
		},
	}
}

func (vm VM) doIO(expr checker.Expression) any {
	// TODO: use this for 3rd party packages
	// iiio := reflect.TypeFor[IO]()
	// if print, ok := iiio.MethodByName("print"); ok {
	// 	print.Func.Call([]reflect.Value{reflect.ValueOf("Hello, World)")})
	// }

	io := IO{}
	switch e := expr.(type) {
	case checker.FunctionCall:
		switch e.Name {
		case "print":
			arg := vm.evalExpression(e.Args[0])
			// todo: check if arg has as_str method
			string, ok := arg.raw.(string)
			if !ok {
				panic(fmt.Sprintf("Expected string, got %T", arg))
			}
			io.print(string)
		default:
			return nil
		}
	default:
		panic(fmt.Sprintf("Unimplemented io property: %T", e))
	}
	return nil
}

type object struct {
	raw   any
	_type checker.Type
}

func (vm VM) evalExpression(expr checker.Expression) object {
	switch e := expr.(type) {
	case checker.StrLiteral:
		return object{e.Value, expr.GetType()}
	case checker.InterpolatedStr:
		builder := strings.Builder{}
		for _, part := range e.Parts {
			obj := vm.evalExpression(part)
			if str, ok := obj.raw.(string); ok {
				builder.WriteString(str)
			} else {
				panic(fmt.Sprintf("Expected string, got %s", expr.GetType()))
			}
		}
		return object{builder.String(), checker.Str{}}
	case checker.NumLiteral:
		return object{e.Value, e.GetType()}
	case checker.BoolLiteral:
		return object{e.Value, e.GetType()}
	case checker.Identifier:
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
			return object{left.raw.(int) + right.raw.(int), left._type}
		case checker.Sub:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left.raw.(int) - right.raw.(int), left._type}
		case checker.Mul:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left.raw.(int) * right.raw.(int), left._type}
		case checker.Div:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left.raw.(int) / right.raw.(int), left._type}
		case checker.Mod:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left.raw.(int) % right.raw.(int), left._type}
		case checker.GreaterThan:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left.raw.(int) > right.raw.(int), left._type}
		case checker.GreaterThanOrEqual:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left.raw.(int) >= right.raw.(int), left._type}
		case checker.LessThan:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left.raw.(int) < right.raw.(int), left._type}
		case checker.LessThanOrEqual:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left.raw.(int) <= right.raw.(int), left._type}
		case checker.Equal:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left == right, left._type}
		case checker.NotEqual:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left != right, left._type}
		case checker.And:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left.raw.(bool) && right.raw.(bool), left._type}
		case checker.Or:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return object{left.raw.(bool) || right.raw.(bool), left._type}
		default:
			panic(fmt.Sprintf("Unimplemented binary op: %v", e.Op))
		}
	case checker.FunctionCall:
		if fn, ok := vm.scope.getFunction(e.Name); ok {
			args := make([]object, len(e.Args))
			for i, arg := range e.Args {
				args[i] = vm.evalExpression(arg)
			}
			return fn(args...)
		}
		panic(fmt.Sprintf("Function not found: %s", e.Name))
	case checker.FunctionLiteral:
		return object{
			raw: func(args ...object) object {
				vm.pushScope()
				for i, arg := range args {
					vm.addVariable(false, e.Parameters[i].Name, arg)
				}
				var result object
				for _, statement := range e.Body {
					result = vm.evalStatement(statement)
				}
				vm.popScope()
				return result
			},
			_type: e.GetType(),
		}
	default:
		panic(fmt.Sprintf("Unimplemented expression: %T", e))
	}
}

func (vm VM) evalProperty(i object, prop checker.Expression) object {
	// TODO: InstanceProperty.Property should only be an Identifier
	propName := prop.(checker.Identifier).Name

	switch i._type.(type) {
	case checker.Str:
		switch propName {
		case "size":
			return object{len(i.raw.(string)), checker.Num{}}
		default:
			panic(fmt.Errorf("Unimplemented property: Str.%v", propName))
		}
	case checker.Num:
		switch propName {
		case "as_str":
			return object{strconv.Itoa(i.raw.(int)), checker.Str{}}
		default:
			panic(fmt.Errorf("Unimplemented property: Num.%v", propName))
		}
	case checker.Bool:
		switch propName {
		case "as_str":
			return object{strconv.FormatBool(i.raw.(bool)), checker.Str{}}
		default:
			panic(fmt.Errorf("Unimplemented property: Bool.%v", propName))
		}
	default:
		return object{nil, checker.Void{}}
	}
}
