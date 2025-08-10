package vm

import (
	"bufio"
	"fmt"
	"os"

	"github.com/akonwi/ard/checker"
)

// FFI functions with uniform signature: func(args []*object) (*object, error)

// Runtime functions
func go_print(args []*object) (*object, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("print expects 1 argument, got %d", len(args))
	}
	
	// Print the raw value
	fmt.Println(args[0].raw)
	return void, nil
}

func go_read_line(args []*object) (*object, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("read_line expects 0 arguments, got %d", len(args))
	}
	
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return &object{raw: scanner.Text(), _type: checker.Str}, nil
	}
	return &object{raw: "", _type: checker.Str}, nil
}

func go_panic(args []*object) (*object, error) {
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
func go_add(args []*object) (*object, error) {
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

func go_multiply(args []*object) (*object, error) {
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

func go_max(args []*object) (*object, error) {
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
