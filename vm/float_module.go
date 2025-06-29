package vm

import (
	"fmt"
	"strconv"

	"github.com/akonwi/ard/checker"
)

// FloatModule handles Float and ard/float module functions
type FloatModule struct{}

func (m *FloatModule) Path() string {
	return "ard/float"
}

func (m *FloatModule) Program() *checker.Program {
	return nil
}

func (m *FloatModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "from_int":
		input := args[0].raw.(int)
		return &object{float64(input), call.Type()}
	case "from_str":
		input := args[0].raw.(string)
		res := &object{nil, call.Type()}
		if num, err := strconv.ParseFloat(input, 64); err == nil {
			res.raw = num
		}
		return res
	default:
		panic(fmt.Errorf("Unimplemented: Float::%s()", call.Name))
	}
}

func (m *FloatModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: float::%s::%s()", structName, call.Name))
}
