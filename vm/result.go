package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

func (vm *VM) evalResultMethod(self *runtime.Object, call *checker.FunctionCall) *runtime.Object {
	switch call.Name {
	case "expect":
		if self.IsErr() {
			actual := ""
			if str, ok := self.IsStr(); ok {
				actual = str
			} else {
				actual = fmt.Sprintf("%v", self.GoValue())
			}
			_msg := vm.eval(call.Args[0]).AsString()
			panic(_msg + ": " + actual)
		}
		return self.UnwrapResult()
	case "or":
		if self.IsOk() {
			return self.UnwrapResult()
		}
		return vm.eval(call.Args[0])
	case "is_ok":
		return runtime.MakeBool(self.IsOk())
	case "is_err":
		return runtime.MakeBool(self.IsErr())
	}

	panic(fmt.Errorf("unimplemented: Result.%s", call.Name))
}
