package vm

import (
	"bufio"
	"fmt"
	"os"

	"github.com/akonwi/ard/checker"
)

// Runtime module FFI functions

// print prints a value to stdout
func print(vm *VM, args []*object) (*object, any) {
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

// read_line reads a line from stdin
func read_line(vm *VM, args []*object) (*object, any) {
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
		return &object{raw: "", _type: checker.Str}, nil
	}

	return &object{raw: scanner.Text(), _type: checker.Str}, nil
}

// panic_with_message panics with a message
func panic_with_message(vm *VM, args []*object) (*object, any) {
	if len(args) != 1 {
		return nil, fmt.Errorf("panic expects 1 argument, got %d", len(args))
	}

	message, ok := args[0].raw.(string)
	if !ok {
		return nil, fmt.Errorf("panic expects string argument, got %T", args[0].raw)
	}

	panic(message)
}
