package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

// MaybeModule handles ard/maybe module functions
type MaybeModule struct{}

func (m *MaybeModule) Path() string {
	return "ard/maybe"
}

func (m *MaybeModule) Program() *checker.Program {
	return nil
}

func (m *MaybeModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "none":
		return &object{nil, call.Type()}
	case "some":
		arg := args[0]
		arg._type = call.Type()
		return arg
	default:
		panic(fmt.Errorf("Unimplemented: maybe::%s()", call.Name))
	}
}

func (m *MaybeModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: maybe::%s::%s()", structName, call.Name))
}
