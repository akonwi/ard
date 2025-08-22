package vm

import (
	"encoding/json/v2"

	"github.com/akonwi/ard/vm/runtime"
)

// encode an Ard value into a JSON string
func encode(vm *VM, args []*runtime.Object) (*runtime.Object, any) {
	bytes, err := json.Marshal(args[0])
	if err != nil {
		return nil, err.Error()
	}
	return runtime.MakeStr(string(bytes)), nil
}
