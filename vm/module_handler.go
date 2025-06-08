package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

// VMEvaluator interface for module handlers to evaluate expressions
type VMEvaluator interface {
	Eval(expr checker.Expression) *object
}

// ModuleHandler defines the interface for built-in module implementations
type ModuleHandler interface {
	Handle(vm VMEvaluator, call *checker.FunctionCall) *object
	Path() string
}

// ModuleRegistry manages built-in module handlers
type ModuleRegistry struct {
	handlers map[string]ModuleHandler
	aliases  map[string]string // Map prelude names to full paths
}

// NewModuleRegistry creates a new module registry
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		handlers: make(map[string]ModuleHandler),
		aliases: map[string]string{
			"Int":    "ard/ints",
			"Float":  "ard/float",
			"Result": "ard/result",
		},
	}
}

// Register adds a module handler to the registry
func (r *ModuleRegistry) Register(handler ModuleHandler) {
	r.handlers[handler.Path()] = handler
}

// Handle dispatches a function call to the appropriate module handler
func (r *ModuleRegistry) Handle(moduleName string, vm VMEvaluator, call *checker.FunctionCall) *object {
	// Check if it's an alias (prelude module)
	if fullPath, isAlias := r.aliases[moduleName]; isAlias {
		moduleName = fullPath
	}

	// Look up the handler
	handler, ok := r.handlers[moduleName]
	if !ok {
		panic(fmt.Errorf("Unimplemented: %s::%s()", moduleName, call.Name))
	}

	return handler.Handle(vm, call)
}

// HasModule checks if a module is registered
func (r *ModuleRegistry) HasModule(moduleName string) bool {
	// Check direct registration
	if _, ok := r.handlers[moduleName]; ok {
		return true
	}
	// Check aliases
	if fullPath, isAlias := r.aliases[moduleName]; isAlias {
		_, ok := r.handlers[fullPath]
		return ok
	}
	return false
}
