package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker_v2"
)

var void = &object{nil, checker_v2.Void}

func Run2(program *checker_v2.Program) (any, error) {
	vm := New()
	for _, statement := range program.Statements {
		vm.result = *vm.do(statement)
	}
	return vm.result.raw, nil
}

func (vm *VM) do(stmt checker_v2.Statement) *object {
	if stmt.Expr != nil {
		return vm.eval(stmt.Expr)
	}
	return void
}

func (vm *VM) eval(expr checker_v2.Expression) *object {
	switch e := expr.(type) {
	case *checker_v2.StrLiteral:
		return &object{e.Value, e.Type()}
	case *checker_v2.PackageFunctionCall:
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
		panic(fmt.Errorf("Unimplemented: %s::%s()", e.Package, e.Call.Name))
	default:
		panic(fmt.Errorf("Unimplemented expression: %T", e))
	}
}
