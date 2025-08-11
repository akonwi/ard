package vm

//go:generate go run ffi_generate.go

import (
	"fmt"
	"sync"

	"github.com/akonwi/ard/checker"
)

// FFIFunc represents the uniform signature for all FFI functions
// Now includes VM access for calling instance methods and other VM operations
type FFIFunc func(vm *VM, args []*object) (*object, error)

// RuntimeFFIRegistry manages FFI functions available at runtime
type RuntimeFFIRegistry struct {
	functions map[string]FFIFunc
	mutex     sync.RWMutex
}

// NewRuntimeFFIRegistry creates a new runtime FFI registry
func NewRuntimeFFIRegistry() *RuntimeFFIRegistry {
	return &RuntimeFFIRegistry{
		functions: make(map[string]FFIFunc),
	}
}

// Register registers a Go function for FFI usage
func (r *RuntimeFFIRegistry) Register(binding string, goFunc FFIFunc) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.functions[binding] = goFunc
	return nil
}

// Get retrieves an FFI function by binding name
func (r *RuntimeFFIRegistry) Get(binding string) (FFIFunc, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	fn, exists := r.functions[binding]
	return fn, exists
}

// Call executes an FFI function with the given arguments
func (r *RuntimeFFIRegistry) Call(vm *VM, binding string, args []*object, returnType checker.Type) (*object, error) {
	fn, exists := r.Get(binding)
	if !exists {
		return nil, fmt.Errorf("FFI function not found: %s", binding)
	}

	// Direct call with VM access - no reflection needed!
	return fn(vm, args)
}

// RegisterBuiltinFFIFunctions registers the standard FFI functions
func (r *RuntimeFFIRegistry) RegisterBuiltinFFIFunctions() error {
	return r.RegisterGeneratedFFIFunctions()
}
