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

func (m *ResultModule) Functions() []string {
	return []string{"ok", "err"}
}

func (m *ResultModule) Handle(vm VMEvaluator, call *checker.FunctionCall) *object {
	switch call.Name {
	case "ok", "err":
		resultType := call.Type().(*checker.Result)
		res := vm.Eval(call.Args[0])
		if call.Name == "ok" {
			return makeOk(res, resultType)
		}
		return makeErr(res, resultType)
	default:
		panic(fmt.Errorf("unimplemented: Result::%s", call.Name))
	}
}
