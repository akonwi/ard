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
			result := &object{nil, e.GetType()}
			jsonString := vm.evalExpression(e.Args[0]).raw.(string)

			inner := e.GetType().(*checker.Maybe).GetInnerType()
			if generic, ok := inner.(*checker.Any); ok {
				switch subj := generic.GetInner().(type) {
				case *checker.Struct:
					{
						_map := make(map[string]any)
						err := json.Unmarshal([]byte(jsonString), &_map)
						if err != nil {
							// todo: build error handling
							fmt.Printf("Error unmarshalling: %s\n", err)
							return result
						}

						fields := make(map[string]*object)
						for name, fType := range subj.Fields {
							val := _map[name]
							if f64, ok := val.(float64); ok && fType == (checker.Int{}) {
								val = int(f64)
							}
							fields[name] = &object{val, fType}
						}

						result.raw = fields
						return result
					}
				default:
					panic(fmt.Errorf("trying to decode into %s", subj))
				}
			}

			panic(fmt.Errorf("TODO: decode into %s", inner))
		}
	default:
		panic(fmt.Sprintf("Unimplemented json property: %s", e))
	}
	panic("unreachable")
}
