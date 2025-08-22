package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

func (vm *VM) evalResultMethod(self *runtime.Object, call *checker.FunctionCall) *runtime.Object {
	switch call.Name {
	case "expect":
		if self.IsErr() {
			actual := ""
			if str, ok := self.IsStr(); ok {
				actual = str
			}
			_msg := vm.eval(call.Args[0]).AsString()
			panic(_msg + ": " + actual)
		}
		return self.Unwrap()
	case "or":
		if self.IsOk() {
			return self.Unwrap()
		}
		return vm.eval(call.Args[0])
	case "is_ok":
		return runtime.MakeBool(self.IsOk())
	case "is_err":
		return runtime.MakeBool(self.IsErr())
	}

	panic(fmt.Errorf("unimplemented: Result.%s", call.Name))
}
