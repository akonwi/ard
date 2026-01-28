package ffi

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

// Runtime module FFI functions

func OsArgs(_ []*runtime.Object, _ checker.Type) *runtime.Object {
	var out []*runtime.Object = make([]*runtime.Object, len(os.Args))
	for i, a := range os.Args {
		out[i] = runtime.MakeStr(a)
	}
	return runtime.MakeList(checker.Str, out...)
}

// Print prints a value to stdout
func Print(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("print expects 1 argument, got %d", len(args)))
	}

	str := args[0].AsString()

	fmt.Println(str)
	return runtime.Void()
}

// ReadLine reads a line from stdin
func ReadLine(args []*runtime.Object, _ checker.Type) *runtime.Object {
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
func PanicWithMessage(args []*runtime.Object, _ checker.Type) *runtime.Object {
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
func EnvGet(args []*runtime.Object, _ checker.Type) *runtime.Object {
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
		return runtime.MakeNone(checker.Str)
	}

	// Return Some(value)
	return runtime.MakeNone(checker.Str).ToSome(value)
}

func GetTodayString(_ []*runtime.Object, _ checker.Type) *runtime.Object {
	year, month, day := time.Now().Date()
	return runtime.MakeStr(fmt.Sprintf("%d-%02d-%02d", year, month, day))
}

// Chrono module FFI functions

// fn now() Int
func Now(_ []*runtime.Object, _ checker.Type) *runtime.Object {
	seconds := time.Now().Unix()
	return runtime.MakeInt(int(seconds))
}

// fn (ns: Int) Void
func Sleep(args []*runtime.Object, _ checker.Type) *runtime.Object {
	time.Sleep(time.Duration(args[0].AsInt()))
	return runtime.Void()
}

// fn (wg: Dynamic) Void
func WaitFor(args []*runtime.Object, _ checker.Type) *runtime.Object {
	wg := args[0].Raw().(*sync.WaitGroup)
	wg.Wait()
	return runtime.Void()
}

// fn (fibers: [Fiber]) Void
func Join(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("join expects 1 argument, got %d", len(args)))
	}

	fibers := args[0].AsList()
	for _, fiberObj := range fibers {
		fiberFields := fiberObj.Raw().(map[string]*runtime.Object)
		wg := fiberFields["wg"].Raw().(*sync.WaitGroup)
		wg.Wait()
	}
	return runtime.Void()
}
