package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

func evalInResult(vm *VM, call *checker.FunctionCall) *object {
	switch call.Name {
	case "ok", "err":
		res := vm.eval(call.Args[0])
		return &object{raw: res, _type: call.Type()}
	default:
		panic(fmt.Errorf("unimplemented: Result::%s", call.Name))
	}
}

func (vm *VM) evalResultMethod(subj *object, call *checker.FunctionCall) *object {
	switch call.Name {
	case "or":
		rawObj := subj.raw.(*object)
		if rawObj._type == subj._type.(*checker.Result).Val() {
			return rawObj
		}
		return vm.eval(call.Args[0])
	}

	panic(fmt.Errorf("unimplemented: %s.%s", subj._type, call.Name))
}
