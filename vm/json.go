package vm

import (
	"encoding/json"
	"fmt"

	"github.com/akonwi/ard/checker"
)

func (vm *VM) invokeJSON(expr checker.Expression) *object {
	switch e := expr.(type) {
	case checker.FunctionCall:
		switch e.Name {
		case "encode":
			obj := vm.evalExpression(e.Args[0])
			result := &object{nil, checker.MakeMaybe(checker.Str{})}
			bytes, err := json.Marshal(obj.premarshal())
			if err != nil {
				return result
			}
			result.raw = string(bytes)
			return result
		case "decode":
		default:
			panic(fmt.Sprintf("Undefined json.%s", e.Name))
		}
	default:
		panic(fmt.Sprintf("Unimplemented json property: %s", e))
	}
	panic("Unreachable")
}
