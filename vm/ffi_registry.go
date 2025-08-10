package vm

import (
	"fmt"
	"sync"

	"github.com/akonwi/ard/checker"
)

// FFIFunc represents the uniform signature for all FFI functions
type FFIFunc func(args []*object) (*object, error)

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

	// Direct call - no reflection needed!
	return fn(args)
}

// RegisterBuiltinFFIFunctions registers the standard FFI functions
func (r *RuntimeFFIRegistry) RegisterBuiltinFFIFunctions() error {
	// Register runtime functions
	if err := r.Register("runtime.go_print", go_print); err != nil {
		return fmt.Errorf("failed to register runtime.go_print: %w", err)
	}
	if err := r.Register("runtime.go_read_line", go_read_line); err != nil {
		return fmt.Errorf("failed to register runtime.go_read_line: %w", err)
	}
	if err := r.Register("runtime.go_panic", go_panic); err != nil {
		return fmt.Errorf("failed to register runtime.go_panic: %w", err)
	}

	// Register math functions
	if err := r.Register("math.go_add", go_add); err != nil {
		return fmt.Errorf("failed to register math.go_add: %w", err)
	}
	if err := r.Register("math.go_multiply", go_multiply); err != nil {
		return fmt.Errorf("failed to register math.go_multiply: %w", err)
	}
	if err := r.Register("math.go_max", go_max); err != nil {
		return fmt.Errorf("failed to register math.go_max: %w", err)
	}

	return nil
}
