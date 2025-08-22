package vm

//go:generate go run ffi_generate.go

import (
	"fmt"
	"sync"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// FFIFunc represents the uniform signature for all FFI functions
// Now includes VM access for calling instance methods and other VM operations
// Returns (*runtime.Object, any) where any can be the appropriate Ard error type for Results
type FFIFunc func(vm *VM, args []*runtime.Object) (*runtime.Object, any)

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
func (r *RuntimeFFIRegistry) Call(vm *VM, binding string, args []*runtime.Object, returnType checker.Type) (result *runtime.Object, err error) {
	fn, exists := r.Get(binding)
	if !exists {
		return nil, fmt.Errorf("External function not found: %s", binding)
	}

	// Recover from panics in FFI functions and add context
	defer func() {
		if r := recover(); r != nil {
			panicMsg := fmt.Sprintf("panic in FFI function '%s': %v", binding, r)

			// If the expected return type is a Result, convert panic to Ard Error result
			if _, ok := returnType.(*checker.Result); ok {
				result = runtime.MakeErr(runtime.MakeStr(panicMsg))
				err = nil
				return
			}

			// For non-Result return types, re-panic with enhanced context
			panic(panicMsg)
		}
	}()

	// Direct call with VM access - no reflection needed!
	result, errValue := fn(vm, args)

	// todo: let external functions do this themselves

	// If the expected return type is a Result, translate error value to Ard Result
	if resultType, ok := returnType.(*checker.Result); ok {
		if errValue != nil {
			// Trust FFI author to return correct error type - create error object directly
			return runtime.MakeErr(runtime.Make(errValue, resultType.Err())), nil
		}
		// Convert successful result to Ard Ok result
		return runtime.MakeOk(result), nil
	}

	// If the expected return type is a Maybe, automatically wrap result
	if maybeType, ok := returnType.(*checker.Maybe); ok {
		// Init maybe::none()
		ret := runtime.MakeMaybe(nil, maybeType.Of())
		if result != nil && result.Raw() != nil {
			// Return Some(value) - the result is already the inner value
			ret.Set(result.Raw())
		}
		return ret, nil
	}

	// For non-Result return types, convert any error to Go error
	if errValue != nil {
		if goErr, ok := errValue.(error); ok {
			return result, goErr
		}
		return result, fmt.Errorf("FFI function returned error: %v", errValue)
	}
	return result, nil
}

// RegisterBuiltinFFIFunctions registers the standard FFI functions
func (r *RuntimeFFIRegistry) RegisterBuiltinFFIFunctions() error {
	return r.RegisterGeneratedFFIFunctions()
}
