//go:build goexperiment.jsonv2

package vm

import (
	"encoding/json/v2"
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
			bytes, err := json.Marshal(args[0])
			if err != nil {
				return makeErr(&object{err.Error(), checker.Str}, resultType)
			}
			return makeOk(&object{string(bytes), checker.Str}, resultType)
		}
	default:
		panic(fmt.Errorf("Unimplemented: json::%s()", call.Name))
	}
}

func (m *JSONModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: json::%s::%s()", structName, call.Name))
}
