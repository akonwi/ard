package ffi

import (
	"encoding/json/v2"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// Encode an Ard value into a JSON string
func JsonEncode(args []*runtime.Object, _ checker.Type) *runtime.Object {
	bytes, err := json.Marshal(args[0])
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.MakeStr(string(bytes)))
}
