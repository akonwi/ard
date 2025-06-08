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

func (m *FloatModule) Functions() []string {
	return []string{"from_int", "from_str"}
}

func (m *FloatModule) Handle(vm VMEvaluator, call *checker.FunctionCall) *object {
	switch call.Name {
	case "from_int":
		input := vm.Eval(call.Args[0]).raw.(int)
		return &object{float64(input), call.Type()}
	case "from_str":
		input := vm.Eval(call.Args[0]).raw.(string)
		res := &object{nil, call.Type()}
		if num, err := strconv.ParseFloat(input, 64); err == nil {
			res.raw = num
		}
		return res
	default:
		panic(fmt.Errorf("Unimplemented: Float::%s()", call.Name))
	}
}
