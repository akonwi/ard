package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// ListModule handles ard/list module functions
type ListModule struct{}

func (m *ListModule) Path() string {
	return "ard/list"
}

func (m *ListModule) Program() *checker.Program {
	return nil
}

func (m *ListModule) Handle(vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "new":
		raw := []*runtime.Object{}
		return runtime.Make(raw, call.Type())
	default:
		panic(fmt.Errorf("Unimplemented: list::%s()", call.Name))
	}
}

func (m *ListModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	panic(fmt.Errorf("Unimplemented: list::%s::%s()", structName, call.Name))
}
