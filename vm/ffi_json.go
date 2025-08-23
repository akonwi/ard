package vm

import (
	"encoding/json/v2"

	"github.com/akonwi/ard/vm/runtime"
)

// encode an Ard value into a JSON string
func encode(vm *VM, args []*runtime.Object) *runtime.Object {
	bytes, err := json.Marshal(args[0])
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.MakeStr(string(bytes)))
}
