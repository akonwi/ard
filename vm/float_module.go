package vm

import (
	"fmt"
	"math"
	"strconv"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// FloatModule handles Float and ard/float module functions
type FloatModule struct{}

func (m *FloatModule) Path() string {
	return "ard/float"
}

func (m *FloatModule) Program() *checker.Program {
	return nil
}

func (m *FloatModule) Handle(vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "floor":
		input := args[0].Raw().(float64)
		return runtime.MakeFloat(math.Floor(input))
	case "from_int":
		input := args[0].Raw().(int)
		return runtime.MakeFloat(float64(input))
	case "from_str":
		input := args[0].Raw().(string)
		res := runtime.Make(nil, call.Type())
		if num, err := strconv.ParseFloat(input, 64); err == nil {
			res.Set(num)
		}
		return res
	default:
		panic(fmt.Errorf("Unimplemented: Float::%s()", call.Name))
	}
}

func (m *FloatModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	panic(fmt.Errorf("Unimplemented: float::%s::%s()", structName, call.Name))
}
