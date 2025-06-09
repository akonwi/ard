package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

// Any built-in module should satisfy this interface
type ModuleHandler interface {
	Handle(vm *VM, call *checker.FunctionCall) *object
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

	return handler.Handle(vm, call)
}

func (r *ModuleRegistry) HasModule(moduleName string) bool {
	if _, ok := r.handlers[moduleName]; ok {
		return true
	}
	return false
}
