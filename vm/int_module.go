package vm

import (
	"fmt"
	"strconv"

	"github.com/akonwi/ard/checker"
)

// IntModule handles Int and ard/ints module functions
type IntModule struct{}

func (m *IntModule) Path() string {
	return "ard/int"
}

func (m *IntModule) Program() *checker.Program {
	return nil
}

func (m *IntModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "from_str":
		input := args[0].raw.(string)
		res := &object{nil, call.Type()}
		if num, err := strconv.Atoi(input); err == nil {
			res.raw = num
		}
		return res
	default:
		panic(fmt.Errorf("Unimplemented: Int::%s()", call.Name))
	}
}

func (m *IntModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: int::%s::%s()", structName, call.Name))
}
