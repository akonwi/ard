package vm

//go:generate go run ffi_generate.go

import (
	"fmt"
	"sync"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

type FFIFunc func(args []*runtime.Object) *runtime.Object

type ValueFFIFunc func(args []any) any

type ExternABI uint8

const (
	ExternABIUnsafeObject ExternABI = iota
	ExternABIValue
)

type resolvedExtern struct {
	Binding   string
	ABI       ExternABI
	Func      FFIFunc
	ValueFunc ValueFFIFunc
}

type RuntimeFFIRegistry struct {
	functions      map[string]FFIFunc
	valueFunctions map[string]ValueFFIFunc
	mutex          sync.RWMutex
}

func NewRuntimeFFIRegistry() *RuntimeFFIRegistry {
	return &RuntimeFFIRegistry{functions: make(map[string]FFIFunc), valueFunctions: make(map[string]ValueFFIFunc)}
}

func (r *RuntimeFFIRegistry) Register(binding string, goFunc FFIFunc) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.functions[binding] = goFunc
	return nil
}

func (r *RuntimeFFIRegistry) RegisterValue(binding string, goFunc ValueFFIFunc) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.valueFunctions[binding] = goFunc
	return nil
}

func (r *RuntimeFFIRegistry) Get(binding string) (FFIFunc, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	fn, exists := r.functions[binding]
	return fn, exists
}

func (r *RuntimeFFIRegistry) GetValue(binding string) (ValueFFIFunc, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	fn, exists := r.valueFunctions[binding]
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

func callValueFFI(binding string, fn ValueFFIFunc, args []any, returnType checker.Type) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			panicMsg := fmt.Sprintf("panic in FFI function '%s': %v", binding, recovered)
			if _, ok := returnType.(*checker.Result); ok {
				result = runtime.ErrValue(panicMsg)
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
	if fn, exists := r.GetValue(binding); exists {
		return resolvedExtern{Binding: binding, ABI: ExternABIValue, ValueFunc: fn}, nil
	}
	fn, exists := r.Get(binding)
	if !exists {
		return resolvedExtern{}, fmt.Errorf("External function not found: %s", binding)
	}
	return resolvedExtern{Binding: binding, ABI: ExternABIUnsafeObject, Func: fn}, nil
}

func (r *RuntimeFFIRegistry) Call(binding string, args []*runtime.Object, returnType checker.Type) (*runtime.Object, error) {
	resolved, err := r.Resolve(binding)
	if err != nil {
		return nil, err
	}
	if resolved.ABI == ExternABIValue {
		rawArgs := make([]any, len(args))
		for i := range args {
			rawArgs[i] = runtime.ObjectToValue(args[i], nil)
		}
		result, err := callValueFFI(resolved.Binding, resolved.ValueFunc, rawArgs, returnType)
		if err != nil {
			return nil, err
		}
		return runtime.ValueToObject(result, returnType), nil
	}
	return callFFI(resolved.Binding, resolved.Func, args, returnType)
}

func (r *RuntimeFFIRegistry) RegisterBuiltinFFIFunctions() error {
	if err := r.RegisterGeneratedFFIFunctions(); err != nil {
		return err
	}
	if err := r.RegisterValue("FloatFromInt", vmFFIFloatFromInt); err != nil {
		return err
	}
	if err := r.RegisterValue("IntFromStr", vmFFIIntFromStr); err != nil {
		return err
	}
	if err := r.RegisterValue("FloatFromStr", vmFFIFloatFromStr); err != nil {
		return err
	}
	if err := r.RegisterValue("FloatFloor", vmFFIFloatFloor); err != nil {
		return err
	}
	if err := r.RegisterValue("EnvGet", vmFFIEnvGet); err != nil {
		return err
	}
	if err := r.RegisterValue("IsNil", vmFFIIsNil); err != nil {
		return err
	}
	if err := r.RegisterValue("JsonToDynamic", vmFFIJsonToDynamic); err != nil {
		return err
	}
	return nil
}
