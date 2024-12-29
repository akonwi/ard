package vm

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/akonwi/ard/internal/checker"
)

type VM struct {
	program *checker.Program
	scope   *scope
	result  any
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
	return vm.result, nil
}

func (vm *VM) addVariable(mut bool, name string, value any) {
	vm.scope.bindings[name] = &binding{mut, value, false}
}

func (vm *VM) evalStatement(stmt checker.Statement) any {
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
			condition = vm.evalExpression(s.Condition).(bool)
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
		cursor := &binding{false, nil, false}
		vm.scope.bindings[s.Cursor.Name] = cursor
		for i := vm.evalExpression(s.Start).(int); i <= vm.evalExpression(s.End).(int); i++ {
			cursor.value = i
			for _, statement := range s.Body {
				vm.evalStatement(statement)
			}
		}
		vm.popScope()
	case checker.ForIn:
		vm.pushScope()
		cursor := &binding{false, nil, false}
		vm.scope.bindings[s.Cursor.Name] = cursor
		iterable := vm.evalExpression(s.Iterable)
		switch iter := iterable.(type) {
		case string:
			for _, item := range iter {
				cursor.value = string(item)
				for _, statement := range s.Body {
					vm.evalStatement(statement)
				}
			}
		}
		vm.popScope()
	case checker.WhileLoop:
		for vm.evalExpression(s.Condition).(bool) {
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

	return nil
}

func (vm *VM) evalVariableBinding(_binding checker.VariableBinding) {
	value := vm.evalExpression(_binding.Value)
	_, callable := value.(func(args ...any) any)
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
		value: func(args ...any) any {
			vm.pushScope()
			for i, arg := range args {
				vm.addVariable(false, fn.Parameters[i].Name, arg)
			}
			var result any
			for _, statement := range fn.Body {
				result = vm.evalStatement(statement)
			}
			vm.popScope()
			return result
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
			string, ok := arg.(string)
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

func (vm VM) evalExpression(expr checker.Expression) any {
	switch e := expr.(type) {
	case checker.StrLiteral:
		return e.Value
	case checker.InterpolatedStr:
		builder := strings.Builder{}
		for _, part := range e.Parts {
			expr := vm.evalExpression(part)
			if str, ok := expr.(string); ok {
				builder.WriteString(str)
			} else {
				panic(fmt.Sprintf("Expected string, got %T", expr))
			}
		}
		return builder.String()
	case checker.NumLiteral:
		return e.Value
	case checker.BoolLiteral:
		return e.Value
	case checker.Identifier:
		if v, ok := vm.scope.get(e.Name); ok {
			return v.value
		}
		panic(fmt.Sprintf("Variable not found: %s", e.Name))
	case checker.Not:
		val := vm.evalExpression(e.Value)
		return !val.(bool)
	case checker.Negation:
		val := vm.evalExpression(e.Value)
		return -(val.(int))
	case checker.InstanceProperty:
		i := vm.evalExpression(e.Subject)
		return vm.evalProperty(i, e.Property)
	case checker.BinaryExpr:
		switch e.Op {
		case checker.Add:
			left := vm.evalExpression(e.Left).(int)
			right := vm.evalExpression(e.Right).(int)
			return left + right
		case checker.Sub:
			left := vm.evalExpression(e.Left).(int)
			right := vm.evalExpression(e.Right).(int)
			return left - right
		case checker.Mul:
			left := vm.evalExpression(e.Left).(int)
			right := vm.evalExpression(e.Right).(int)
			return left * right
		case checker.Div:
			left := vm.evalExpression(e.Left).(int)
			right := vm.evalExpression(e.Right).(int)
			return left / right
		case checker.Mod:
			left := vm.evalExpression(e.Left).(int)
			right := vm.evalExpression(e.Right).(int)
			return left % right
		case checker.GreaterThan:
			left := vm.evalExpression(e.Left).(int)
			right := vm.evalExpression(e.Right).(int)
			return left > right
		case checker.GreaterThanOrEqual:
			left := vm.evalExpression(e.Left).(int)
			right := vm.evalExpression(e.Right).(int)
			return left >= right
		case checker.LessThan:
			left := vm.evalExpression(e.Left).(int)
			right := vm.evalExpression(e.Right).(int)
			return left < right
		case checker.LessThanOrEqual:
			left := vm.evalExpression(e.Left).(int)
			right := vm.evalExpression(e.Right).(int)
			return left <= right
		case checker.Equal:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return left == right
		case checker.NotEqual:
			left := vm.evalExpression(e.Left)
			right := vm.evalExpression(e.Right)
			return left != right
		case checker.And:
			left := vm.evalExpression(e.Left).(bool)
			right := vm.evalExpression(e.Right).(bool)
			return left && right
		case checker.Or:
			left := vm.evalExpression(e.Left).(bool)
			right := vm.evalExpression(e.Right).(bool)
			return left || right
		default:
			panic(fmt.Sprintf("Unimplemented binary op: %v", e.Op))
		}
	case checker.FunctionCall:
		if fn, ok := vm.scope.getFunction(e.Name); ok {
			args := make([]any, len(e.Args))
			for i, arg := range e.Args {
				args[i] = vm.evalExpression(arg)
			}
			return fn(args...)
		}
		panic(fmt.Sprintf("Function not found: %s", e.Name))
	case checker.FunctionLiteral:
		return func(args ...any) any {
			vm.pushScope()
			for i, arg := range args {
				vm.addVariable(false, e.Parameters[i].Name, arg)
			}
			var result any
			for _, statement := range e.Body {
				result = vm.evalStatement(statement)
			}
			vm.popScope()
			return result
		}
	default:
		panic(fmt.Sprintf("Unimplemented expression: %T", e))
	}
}

func (vm VM) evalProperty(i any, prop checker.Expression) any {
	if i == nil {
		panic(fmt.Errorf("Cannot access property on nil: nil.%v", prop))
	}

	// TODO: InstanceProperty.Property should only be an Identifier
	propName := prop.(checker.Identifier).Name

	switch i_type := reflect.TypeOf(i); i_type.Kind() {
	case reflect.String:
		switch propName {
		case "size":
			return len(i.(string))
		default:
			panic(fmt.Errorf("Unimplemented property: Str.%v", propName))
		}
	case reflect.Int:
		switch propName {
		case "as_str":
			return strconv.Itoa(i.(int))
		default:
			panic(fmt.Errorf("Unimplemented property: Num.%v", propName))
		}
	case reflect.Bool:
		switch propName {
		case "as_str":
			return strconv.FormatBool(i.(bool))
		default:
			panic(fmt.Errorf("Unimplemented property: Bool.%v", propName))
		}
	default:
		return nil
	}
}
