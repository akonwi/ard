package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

// ResultModule handles Result and ard/result module functions
type ResultModule struct{}

func (m *ResultModule) Path() string {
	return "ard/result"
}

func (m *ResultModule) Program() *checker.Program {
	return nil
}

func (m *ResultModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "ok", "err":
		resultType := call.Type().(*checker.Result)
		data := args[0]
		if call.Name == "ok" {
			return makeOk(data, resultType)
		}
		return makeErr(data, resultType)
	default:
		panic(fmt.Errorf("unimplemented: Result::%s", call.Name))
	}
}

func (m *ResultModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: result::%s::%s()", structName, call.Name))
}
