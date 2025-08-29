package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

// ResultModule handles Result and ard/result module functions
type ResultModule struct{}

func (m *ResultModule) Path() string {
	return "ard/result"
}

func (m *ResultModule) Program() *checker.Program {
	return nil
}

func (m *ResultModule) Handle(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "ok", "err":
		data := args[0]
		if call.Name == "ok" {
			return runtime.MakeOk(data)
		}
		return runtime.MakeErr(data)
	default:
		panic(fmt.Errorf("unimplemented: Result::%s", call.Name))
	}
}

func (m *ResultModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	panic(fmt.Errorf("Unimplemented: result::%s::%s()", structName, call.Name))
}
