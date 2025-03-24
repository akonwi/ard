package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

func (vm *VM) invokeOption(expr checker.Expression) *object {
	option := expr.GetType().(checker.Option)
	switch e := expr.(type) {
	case checker.FunctionCall:
		switch e.Name {
		case "none":
			return &object{nil, option}
		case "some":
			return &object{vm.evalExpression(e.Args[0]).raw, e.GetType()}
		default:
			panic(fmt.Sprintf("Undefined option.%s", e.Name))
		}
	default:
		panic(fmt.Sprintf("Unknown option export: %s", e))
	}
}
