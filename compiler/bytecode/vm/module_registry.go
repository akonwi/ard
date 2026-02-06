package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
	corevm "github.com/akonwi/ard/vm"
)

type ModuleHandler interface {
	Handle(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object
	Path() string
}

type ModuleRegistry struct {
	handlers map[string]ModuleHandler
}

func NewModuleRegistry() *ModuleRegistry {
	r := &ModuleRegistry{handlers: map[string]ModuleHandler{}}
	r.Register(&corevm.MaybeModule{})
	r.Register(&corevm.ResultModule{})
	return r
}

func (r *ModuleRegistry) Register(handler ModuleHandler) {
	r.handlers[handler.Path()] = handler
}

func (r *ModuleRegistry) Call(module string, call *checker.FunctionCall, args []*runtime.Object) (*runtime.Object, error) {
	h, ok := r.handlers[module]
	if !ok {
		return nil, fmt.Errorf("unknown module: %s", module)
	}
	return h.Handle(call, args), nil
}
