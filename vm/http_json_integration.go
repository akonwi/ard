//go:build goexperiment.jsonv2

package vm

import (
	"github.com/akonwi/ard/checker"
)

func (mod *HTTPModule) decodeJsonResponse(bodyStr string, returnType checker.Type) *object {
	// Create a synthetic function call to json::decode()
	jsonMod := &JSONModule{}
	vm := New(map[string]checker.Module{})
	return jsonMod.Handle(vm, checker.CreateCall("decode",
		[]checker.Expression{&checker.StrLiteral{Value: bodyStr}},
		checker.FunctionDef{
			ReturnType: returnType,
		},
	), []*object{&object{bodyStr, checker.Str}})
}
