package vm

import (
	"encoding/json"
	"fmt"
	"reflect"

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
				case checker.List:
					{
						fmt.Printf("subj: %v\n", reflect.TypeOf(subj))
						array := []any{}
						err := json.Unmarshal([]byte(jsonString), &array)
						if err != nil {
							// todo: build error handling
							fmt.Printf("Error unmarshalling: %s\n", err)
							return result
						}

						raw := make([]*object, len(array))
						for i, val := range array {
							raw[i] = makeObject(val, subj.GetElementType())
						}

						result.raw = array
						return result
					}
				case checker.Int, checker.Str:
					panic(fmt.Errorf("Cannot decode into primitive: %s", subj))
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

func makeObject(val any, typ checker.Type) *object {
	switch typ.(type) {
	case checker.Int:
		switch v := val.(type) {
		case int:
			return &object{v, typ}
		case float64:
			return &object{int(v), typ}
		default:
			panic(fmt.Errorf("unexpected type for int: %T", val))
		}
	case checker.Str:
		if str, ok := val.(string); ok {
			return &object{str, typ}
		}
		return &object{fmt.Sprintf("%v", val), typ}
	default:
		panic(fmt.Errorf("trying to decode into %s", typ))
	}
}
