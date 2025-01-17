package vm

import (
	"fmt"

	"github.com/akonwi/ard/internal/checker"
)

func (vm *VM) callInOptionPackage(expr checker.Expression) *object {
	option := expr.GetType().(checker.Option)
	switch e := expr.(type) {
	case checker.FunctionCall:
		switch e.Name {
		case "make":
			return &object{nil, option}
		default:
			panic(fmt.Sprintf("Unknown function option.%s", e.Name))
		}
	default:
		panic(fmt.Sprintf("Unknown option export: %s", e))
	}
}
