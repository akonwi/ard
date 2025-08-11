package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

// Math module FFI functions

// go_add adds two integers
func go_add(vm *VM, args []*object) (*object, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("add expects 2 arguments, got %d", len(args))
	}

	a, ok := args[0].raw.(int)
	if !ok {
		return nil, fmt.Errorf("add expects int arguments, got %T for first argument", args[0].raw)
	}

	b, ok := args[1].raw.(int)
	if !ok {
		return nil, fmt.Errorf("add expects int arguments, got %T for second argument", args[1].raw)
	}

	result := a + b
	return &object{raw: result, _type: checker.Int}, nil
}

// go_multiply multiplies two integers
func go_multiply(vm *VM, args []*object) (*object, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("multiply expects 2 arguments, got %d", len(args))
	}

	a, ok := args[0].raw.(int)
	if !ok {
		return nil, fmt.Errorf("multiply expects int arguments, got %T for first argument", args[0].raw)
	}

	b, ok := args[1].raw.(int)
	if !ok {
		return nil, fmt.Errorf("multiply expects int arguments, got %T for second argument", args[1].raw)
	}

	result := a * b
	return &object{raw: result, _type: checker.Int}, nil
}

// go_max returns the maximum of two integers
func go_max(vm *VM, args []*object) (*object, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("max expects 2 arguments, got %d", len(args))
	}

	a, ok := args[0].raw.(int)
	if !ok {
		return nil, fmt.Errorf("max expects int arguments, got %T for first argument", args[0].raw)
	}

	b, ok := args[1].raw.(int)
	if !ok {
		return nil, fmt.Errorf("max expects int arguments, got %T for second argument", args[1].raw)
	}

	var result int
	if a > b {
		result = a
	} else {
		result = b
	}

	return &object{raw: result, _type: checker.Int}, nil
}

// go_subtract subtracts two integers
func go_subtract(vm *VM, args []*object) (*object, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("subtract expects 2 arguments, got %d", len(args))
	}

	a, ok := args[0].raw.(int)
	if !ok {
		return nil, fmt.Errorf("subtract expects int arguments, got %T for first argument", args[0].raw)
	}

	b, ok := args[1].raw.(int)
	if !ok {
		return nil, fmt.Errorf("subtract expects int arguments, got %T for second argument", args[1].raw)
	}

	result := a - b
	return &object{raw: result, _type: checker.Int}, nil
}
