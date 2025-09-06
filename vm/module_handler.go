package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

// Any built-in module should satisfy this interface
type ModuleHandler interface {
	Handle(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object
	HandleStatic(structName string, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object
	Path() string
	get(name string) *runtime.Object
}

type ModuleRegistry struct {
	handlers map[string]ModuleHandler
}

func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		handlers: make(map[string]ModuleHandler),
	}
}

func (r *ModuleRegistry) Register(handler ModuleHandler) {
	r.handlers[handler.Path()] = handler
}

func (r *ModuleRegistry) HandleStatic(moduleName string, structName string, vm *VM, call *checker.FunctionCall) *runtime.Object {
	handler, ok := r.handlers[moduleName]
	if !ok {
		panic(fmt.Errorf("Unimplemented: %s::%s::%s()", moduleName, structName, call.Name))
	}

	// evaluate arguments in current vm context because the function called will be evaluated in another context
	args := make([]*runtime.Object, len(call.Args))
	for i, arg := range call.Args {
		args[i] = vm.Eval(arg)
	}

	return handler.HandleStatic(structName, call, args)
}

func (r *ModuleRegistry) HasModule(moduleName string) bool {
	if _, ok := r.handlers[moduleName]; ok {
		return true
	}
	return false
}
