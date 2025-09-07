package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

// HTTPModule handles ard/http module functions
type HTTPModule struct {
	vm *VM
	hq *GlobalVM
}

func (m *HTTPModule) Path() string {
	return "ard/http"
}

func (m *HTTPModule) Program() *checker.Program {
	return nil
}

func (m *HTTPModule) get(name string) *runtime.Object {
	sym, _ := m.vm.scope.get(name)
	return sym
}

func (m *HTTPModule) Handle(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	panic(fmt.Errorf("Unimplemented: http::%s()", call.Name))
}

func (m *HTTPModule) HandleStatic(structName string, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch structName {
	case "Response":
		return m.handleResponseStatic(call, args)
	default:
		panic(fmt.Errorf("Unimplemented: http::%s::%s()", structName, call.Name))
	}
}

func (m *HTTPModule) handleResponseStatic(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	panic(fmt.Errorf("Unimplemented: Response::%s()", call.Name))
}
