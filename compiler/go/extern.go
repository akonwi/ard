package ardgo

import (
	"fmt"
	"sync"
)

type ExternFunc func(args ...any) (any, error)

type ExternRegistry struct {
	mu        sync.RWMutex
	functions map[string]ExternFunc
}

func NewExternRegistry() *ExternRegistry {
	return &ExternRegistry{functions: make(map[string]ExternFunc)}
}

func (r *ExternRegistry) Register(name string, fn ExternFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.functions[name] = fn
}

func (r *ExternRegistry) Call(name string, args ...any) (any, error) {
	fn, ok := r.functions[name]
	if !ok {
		return nil, fmt.Errorf("extern function not found: %s", name)
	}
	return fn(args...)
}

var defaultExternRegistry = NewExternRegistry()

func RegisterExtern(name string, fn ExternFunc) {
	defaultExternRegistry.Register(name, fn)
}

func CallExtern(name string, args ...any) (any, error) {
	return defaultExternRegistry.Call(name, args...)
}
