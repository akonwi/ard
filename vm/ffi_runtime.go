package vm

import (
	"bufio"
	"fmt"
	"os"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// Runtime module FFI functions

// print prints a value to stdout
func print(vm *VM, args []*runtime.Object) (*runtime.Object, any) {
	if len(args) != 1 {
		return nil, fmt.Errorf("print expects 1 argument, got %d", len(args))
	}

	arg := args[0]
	switch arg := arg.Raw().(type) {
	case string:
		fmt.Println(arg)
		return runtime.Void(), nil
	case bool, int, float64:
		fmt.Printf("%v\n", arg)
		return runtime.Void(), nil
	}

	var str string

	call := &checker.FunctionCall{Name: "to_str", Args: []checker.Expression{}}
	// could be `arg.cast[T](as: T) (T, bool)`
	if _, ok := arg.Type().(*checker.StructDef); ok {
		str = vm.evalStructMethod(arg, call).Raw().(string)
	} else if enum, ok := arg.Type().(*checker.Enum); ok {
		str = vm.evalEnumMethod(arg, call, enum).Raw().(string)
	} else {
		panic(fmt.Errorf("Encountered an unprintable: %s", arg))
	}

	fmt.Println(str)
	return runtime.Void(), nil
}

// read_line reads a line from stdin
func read_line(vm *VM, args []*runtime.Object) (*runtime.Object, any) {
	if len(args) != 0 {
		return nil, fmt.Errorf("read_line expects 0 arguments, got %d", len(args))
	}

	scanner := bufio.NewScanner(os.Stdin)

	if !scanner.Scan() {
		// No more input available or EOF
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		// EOF - return empty string as success
		return runtime.MakeStr(""), nil
	}

	return runtime.MakeStr(scanner.Text()), nil
}

// panic_with_message panics with a message
func panic_with_message(vm *VM, args []*runtime.Object) (*runtime.Object, any) {
	if len(args) != 1 {
		return nil, fmt.Errorf("panic expects 1 argument, got %d", len(args))
	}

	message, ok := args[0].Raw().(string)
	if !ok {
		return nil, fmt.Errorf("panic expects string argument, got %T", args[0].Raw())
	}

	panic(message)
}

// Environment module FFI functions

// get retrieves an environment variable
func env_get(vm *VM, args []*runtime.Object) (*runtime.Object, any) {
	if len(args) != 1 {
		return nil, fmt.Errorf("get expects 1 argument, got %d", len(args))
	}

	key, ok := args[0].Raw().(string)
	if !ok {
		return nil, fmt.Errorf("get expects string argument, got %T", args[0].Raw())
	}

	value, exists := os.LookupEnv(key)
	if !exists {
		// VM will convert nil to None based on Maybe return type
		return nil, nil
	}

	// VM will convert this to Some(value) based on Maybe return type
	return runtime.MakeStr(value), nil
}
