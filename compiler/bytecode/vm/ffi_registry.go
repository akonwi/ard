package vm

//go:generate go run ffi_generate.go

import (
	"fmt"
	"sync"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

type FFIFunc func(args []*runtime.Object, returnType checker.Type) *runtime.Object

type RuntimeFFIRegistry struct {
	functions map[string]FFIFunc
	mutex     sync.RWMutex
}

func NewRuntimeFFIRegistry() *RuntimeFFIRegistry {
	return &RuntimeFFIRegistry{functions: make(map[string]FFIFunc)}
}

func (r *RuntimeFFIRegistry) Register(binding string, goFunc FFIFunc) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.functions[binding] = goFunc
	return nil
}

func (r *RuntimeFFIRegistry) Get(binding string) (FFIFunc, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	fn, exists := r.functions[binding]
	return fn, exists
}

func (r *RuntimeFFIRegistry) Call(binding string, args []*runtime.Object, returnType checker.Type) (result *runtime.Object, err error) {
	fn, exists := r.Get(binding)
	if !exists {
		return nil, fmt.Errorf("External function not found: %s", binding)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			panicMsg := fmt.Sprintf("panic in FFI function '%s': %v", binding, recovered)
			if _, ok := returnType.(*checker.Result); ok {
				result = runtime.MakeErr(runtime.MakeStr(panicMsg))
				err = nil
				return
			}
			panic(panicMsg)
		}
	}()

	result = fn(args, returnType)
	return result, nil
}

func (r *RuntimeFFIRegistry) RegisterBuiltinFFIFunctions() error {
	return r.RegisterGeneratedFFIFunctions()
}
