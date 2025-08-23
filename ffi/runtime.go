package ffi

import (
	"bufio"
	"fmt"
	"os"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// Runtime module FFI functions

// Print prints a value to stdout
func Print(vm runtime.VM, args []*runtime.Object) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("print expects 1 argument, got %d", len(args)))
	}

	arg := args[0]
	switch arg := arg.Raw().(type) {
	case string:
		fmt.Println(arg)
		return runtime.Void()
	case bool, int, float64:
		fmt.Printf("%v\n", arg)
		return runtime.Void()
	}

	var str string

	call := &checker.FunctionCall{Name: "to_str", Args: []checker.Expression{}}
	// could be `arg.cast[T](as: T) (T, bool)`
	if _, ok := arg.Type().(*checker.StructDef); ok {
		str = vm.EvalStructMethod(arg, call).Raw().(string)
	} else if enum, ok := arg.Type().(*checker.Enum); ok {
		str = vm.EvalEnumMethod(arg, call, enum).Raw().(string)
	} else {
		panic(fmt.Errorf("Encountered an unprintable: %s", arg))
	}

	fmt.Println(str)
	return runtime.Void()
}

// ReadLine reads a line from stdin
func ReadLine(vm runtime.VM, args []*runtime.Object) *runtime.Object {
	if len(args) != 0 {
		panic(fmt.Errorf("read_line expects 0 arguments, got %d", len(args)))
	}

	scanner := bufio.NewScanner(os.Stdin)

	if !scanner.Scan() {
		// No more input available or EOF
		if err := scanner.Err(); err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}
		// EOF - return empty string as success
		return runtime.MakeOk(runtime.MakeStr(""))
	}

	return runtime.MakeOk(runtime.MakeStr(scanner.Text()))
}

// PanicWithMessage panics with a message
func PanicWithMessage(vm runtime.VM, args []*runtime.Object) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("panic expects 1 argument, got %d", len(args)))
	}

	message, ok := args[0].Raw().(string)
	if !ok {
		panic(fmt.Errorf("panic expects string argument, got %T", args[0].Raw()))
	}

	panic(message)
}

// Environment module FFI functions

// EnvGet retrieves an environment variable
func EnvGet(vm runtime.VM, args []*runtime.Object) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("get expects 1 argument, got %d", len(args)))
	}

	key, ok := args[0].Raw().(string)
	if !ok {
		panic(fmt.Errorf("get expects string argument, got %T", args[0].Raw()))
	}

	value, exists := os.LookupEnv(key)
	if !exists {
		// Return None
		return runtime.MakeMaybe(nil, checker.Str)
	}

	// Return Some(value)
	return runtime.MakeStr(value).ToMaybe()
}
