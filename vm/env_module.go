package vm

import (
	"fmt"
	"os"

	"github.com/akonwi/ard/checker"
)

type EnvModule struct{}

func (m *EnvModule) Path() string {
	return "ard/env"
}

func (m *EnvModule) Handle(_ *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "get":
		return m.handleGet(args, call.Type())
	default:
		panic(fmt.Errorf("Unimplemented: ard/env::%s()", call.Name))
	}
}

func (m EnvModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: ard/env::%s::%s()", structName, call.Name))
}

func (m EnvModule) handleGet(args []*object, as checker.Type) *object {
	ret := &object{nil, as}
	_key := args[0].raw.(string)
	if _val, ok := os.LookupEnv(_key); ok {
		ret.raw = _val
	}

	return ret
}
