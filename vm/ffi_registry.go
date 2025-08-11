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
func (r *RuntimeFFIRegistry) Call(vm *VM, binding string, args []*object, returnType checker.Type) (result *object, err error) {
	fn, exists := r.Get(binding)
	if !exists {
		return nil, fmt.Errorf("External function not found: %s", binding)
	}

	// Recover from panics in FFI functions and add context
	defer func() {
		if r := recover(); r != nil {
			panicMsg := fmt.Sprintf("panic in FFI function '%s': %v", binding, r)

			// If the expected return type is a Result, convert panic to Ard Error result
			if resultType, ok := returnType.(*checker.Result); ok {
				errorObj := &object{raw: panicMsg, _type: checker.Str}
				result = makeErr(errorObj, resultType)
				err = nil
				return
			}

			// For non-Result return types, re-panic with enhanced context
			panic(panicMsg)
		}
	}()

	// Direct call with VM access - no reflection needed!
	result, err = fn(vm, args)

	// If the expected return type is a Result, translate Go error handling to Ard Result
	if resultType, ok := returnType.(*checker.Result); ok {
		if err != nil {
			// Convert Go error to Ard Error result
			errorObj := &object{raw: err.Error(), _type: checker.Str}
			return makeErr(errorObj, resultType), nil
		}
		// Convert successful result to Ard Ok result
		return makeOk(result, resultType), nil
	}

	// For non-Result return types, pass through Go error as-is
	return result, err
}

// RegisterBuiltinFFIFunctions registers the standard FFI functions
func (r *RuntimeFFIRegistry) RegisterBuiltinFFIFunctions() error {
	return r.RegisterGeneratedFFIFunctions()
}
