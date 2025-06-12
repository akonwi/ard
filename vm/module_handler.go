package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

// Any built-in module should satisfy this interface
type ModuleHandler interface {
	Handle(vm *VM, call *checker.FunctionCall, args []*object) *object
	Path() string
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

func (r *ModuleRegistry) Handle(moduleName string, vm *VM, call *checker.FunctionCall) *object {
	handler, ok := r.handlers[moduleName]
	if !ok {
		panic(fmt.Errorf("Unimplemented: %s::%s()", moduleName, call.Name))
	}

	// evaluate arguments in current vm context because the function called will be evaluated in another context
	args := make([]*object, len(call.Args))
	for i, arg := range call.Args {
		args[i] = vm.Eval(arg)
	}

	return handler.Handle(vm, call, args)
}

func (r *ModuleRegistry) HasModule(moduleName string) bool {
	if _, ok := r.handlers[moduleName]; ok {
		return true
	}
	return false
}
