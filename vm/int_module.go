package vm

import (
	"fmt"
	"strconv"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// IntModule handles Int and ard/ints module functions
type IntModule struct{}

func (m *IntModule) Path() string {
	return "ard/int"
}

func (m *IntModule) Program() *checker.Program {
	return nil
}

func (m *IntModule) Handle(vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "from_str":
		input := args[0].Raw().(string)
		res := runtime.Make(nil, call.Type())
		if num, err := strconv.Atoi(input); err == nil {
			res.Set(num)
		}
		return res
	default:
		panic(fmt.Errorf("Unimplemented: Int::%s()", call.Name))
	}
}

func (m *IntModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	panic(fmt.Errorf("Unimplemented: int::%s::%s()", structName, call.Name))
}
