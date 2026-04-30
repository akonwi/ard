package vm

//go:generate go run ffi_generate.go

import (
	"fmt"
	"sync"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

type FFIFunc func(args []*runtime.Object) *runtime.Object

type resolvedExtern struct {
	Binding string
	Func    FFIFunc
}

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

func callFFI(binding string, fn FFIFunc, args []*runtime.Object, returnType checker.Type) (result *runtime.Object, err error) {
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

	result = fn(args)
	return result, nil
}

func (r *RuntimeFFIRegistry) Resolve(binding string) (resolvedExtern, error) {
	fn, exists := r.Get(binding)
	if !exists {
		return resolvedExtern{}, fmt.Errorf("External function not found: %s", binding)
	}
	return resolvedExtern{Binding: binding, Func: fn}, nil
}

func (r *RuntimeFFIRegistry) Call(binding string, args []*runtime.Object, returnType checker.Type) (*runtime.Object, error) {
	resolved, err := r.Resolve(binding)
	if err != nil {
		return nil, err
	}
	return callFFI(resolved.Binding, resolved.Func, args, returnType)
}

func (r *RuntimeFFIRegistry) RegisterBuiltinFFIFunctions() error {
	return r.RegisterGeneratedFFIFunctions()
}
