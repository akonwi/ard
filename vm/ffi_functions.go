package vm

import (
	"bufio"
	"fmt"
	"os"

	"github.com/akonwi/ard/checker"
)

// FFI functions with uniform signature: func(vm *VM, args []*object) (*object, error)

// Runtime functions
func go_print(vm *VM, args []*object) (*object, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("print expects 1 argument, got %d", len(args))
	}

	arg := args[0]
	switch arg := arg.raw.(type) {
	case string:
		fmt.Println(arg)
		return void, nil
	case bool, int, float64:
		fmt.Printf("%v\n", arg)
		return void, nil
	}

	var str string

	call := &checker.FunctionCall{Name: "to_str", Args: []checker.Expression{}}
	// could be `arg.cast[T](as: T) (T, bool)`
	if _, ok := arg._type.(*checker.StructDef); ok {
		str = vm.evalStructMethod(arg, call).raw.(string)
	} else if enum, ok := arg._type.(*checker.Enum); ok {
		str = vm.evalEnumMethod(arg, call, enum).raw.(string)
	} else {
		panic(fmt.Errorf("Encountered an unprintable: %s", arg))
	}

	fmt.Println(str)
	return void, nil
}

func go_read_line(vm *VM, args []*object) (*object, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("read_line expects 0 arguments, got %d", len(args))
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	
	// Return Str!Str Result type like IOModule does
	resultType := checker.MakeResult(checker.Str, checker.Str)
	
	if err := scanner.Err(); err != nil {
		return makeErr(&object{err.Error(), checker.Str}, resultType), nil
	}
	return makeOk(&object{scanner.Text(), checker.Str}, resultType), nil
}

func go_panic(vm *VM, args []*object) (*object, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("panic expects 1 argument, got %d", len(args))
	}

	message, ok := args[0].raw.(string)
	if !ok {
		return nil, fmt.Errorf("panic expects string argument, got %T", args[0].raw)
	}

	panic(message)
}

// Math functions
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
