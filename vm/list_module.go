package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

// ListModule handles ard/list module functions
type ListModule struct{}

func (m *ListModule) Path() string {
	return "ard/list"
}

func (m *ListModule) Program() *checker.Program {
	return nil
}

func (m *ListModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "new":
		raw := []*object{}
		return &object{raw, call.Type()}
	default:
		panic(fmt.Errorf("Unimplemented: list::%s()", call.Name))
	}
}

func (m *ListModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: list::%s::%s()", structName, call.Name))
}
