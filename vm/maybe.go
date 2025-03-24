package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

func (vm *VM) invokeMaybe(expr checker.Expression) *object {
	maybe := expr.GetType().(checker.Maybe)
	switch e := expr.(type) {
	case checker.FunctionCall:
		switch e.Name {
		case "none":
			return &object{nil, maybe}
		case "some":
			return &object{vm.evalExpression(e.Args[0]).raw, e.GetType()}
		default:
			panic(fmt.Sprintf("Undefined maybe.%s", e.Name))
		}
	default:
		panic(fmt.Sprintf("Unknown maybe export: %s", e))
	}
}
