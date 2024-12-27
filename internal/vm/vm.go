package vm

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/akonwi/ard/internal/checker"
)

type VM struct {
	program *checker.Program
	scope   *scope
	result  any
}

func New(program *checker.Program) *VM {
	return &VM{program: program, scope: newScope()}
}

func (vm *VM) Run() (any, error) {
	for _, statement := range vm.program.Statements {
		vm.evalStatement(statement)
	}
	return vm.result, nil
}

func (vm *VM) addVariable(mut bool, name string, value any) {
	vm.scope.variables[name] = variable{mut, value}
}

func (vm *VM) evalStatement(stmt checker.Statement) {
	switch s := stmt.(type) {
	case checker.VariableBinding:
		vm.evalVariableBinding(s)
	case checker.VariableAssignment:
		vm.evalVariableAssignment(s)
	case checker.PackageAccess:
		switch s.Package.Path {
		case "std/io":
			vm.doIO(s.Property)
		default:
			panic(fmt.Sprintf("Unimplemented package: %s", s.Package.Path))
		}
	default:
		expr, ok := s.(checker.Expression)
		if !ok {
			panic(fmt.Sprintf("Unimplemented statement: %T", s))
		}
		vm.result = vm.evalExpression(expr)
	}
}

func (vm *VM) evalVariableBinding(binding checker.VariableBinding) {
	value := vm.evalExpression(binding.Value)
	vm.addVariable(binding.Mut, binding.Name, value)
	vm.result = value
}

func (vm *VM) evalVariableAssignment(assignment checker.VariableAssignment) {
	value := vm.evalExpression(assignment.Value)
	vm.scope.variables[assignment.Name] = variable{true, value}
	vm.result = value
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
		return vm.scope.variables[e.Name].value
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
		default:
			panic(fmt.Sprintf("Unimplemented binary op: %v", e.Op))
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
	default:
		return nil
	}
}
