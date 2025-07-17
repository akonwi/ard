package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

// JSONModule handles ard/json module functions
type JSONModule struct{}

func (m *JSONModule) Path() string {
	return "ard/json"
}

func (m *JSONModule) Program() *checker.Program {
	return nil
}

func (m *JSONModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "encode":
		{
			resultType := call.Type().(*checker.Result)
			o := args[0]
			bytes, err := json_encode(o.raw, o._type)
			if err != nil {
				return makeErr(&object{err.Error(), checker.Str}, resultType)
			}
			return makeOk(&object{string(bytes), checker.Str}, resultType)
		}
	case "decode":
		{
			resultType := call.Type().(*checker.Result)
			toErr := func(msg error) *object {
				return makeErr(&object{msg.Error(), checker.Str}, resultType)
			}
			jsonString := vm.eval(call.Args[0]).raw.(string)
			jsonBytes := []byte(jsonString)

			inner := resultType.Val()
			val, err := json_decode(inner, jsonBytes)
			if err != nil {
				return toErr(err)
			}

			return makeOk(&val, resultType)
		}
	default:
		panic(fmt.Errorf("Unimplemented: json::%s()", call.Name))
	}
}

func (m *JSONModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: json::%s::%s()", structName, call.Name))
}
