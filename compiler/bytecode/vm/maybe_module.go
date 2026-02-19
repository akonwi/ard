package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

type MaybeModule struct{}

func (m *MaybeModule) Path() string {
	return "ard/maybe"
}

func (m *MaybeModule) Handle(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "none":
		maybeType, ok := call.ReturnType.(*checker.Maybe)
		if !ok {
			panic(fmt.Errorf("maybe::none expected Maybe return type, got %s", call.ReturnType))
		}
		o := runtime.MakeNone(maybeType.Of())
		o.SetRefinedType(call.ReturnType)
		return o
	case "some":
		arg := args[0]
		maybeType, ok := call.ReturnType.(*checker.Maybe)
		if !ok {
			panic(fmt.Errorf("maybe::some expected Maybe return type, got %s", call.ReturnType))
		}
		o := runtime.MakeNone(maybeType.Of()).ToSome(arg.Raw())
		o.SetRefinedType(call.ReturnType)
		return o
	default:
		panic(fmt.Errorf("Unimplemented: maybe::%s()", call.Name))
	}
}
