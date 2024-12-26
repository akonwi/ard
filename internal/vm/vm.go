package vm

import (
	"fmt"
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
	default:
		panic(fmt.Sprintf("Unimplemented expression: %T", e))
	}
}
