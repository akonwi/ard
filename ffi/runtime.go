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
		return runtime.MakeMaybe(nil, checker.Str)
	}

	// Return Some(value)
	return runtime.MakeStr(value).ToSome()
}

func GetTodayString(_ []*runtime.Object, _ checker.Type) *runtime.Object {
	year, month, day := time.Now().Date()
	return runtime.MakeStr(fmt.Sprintf("%d-%02d-%02d", year, month, day))
}

// fn (ms: Int) Void
func Sleep(args []*runtime.Object, _ checker.Type) *runtime.Object {
	duration := args[0].AsInt()
	time.Sleep(time.Duration(duration) * time.Millisecond)
	return runtime.Void()
}

// returns waitgroup
// fn (do: fn() Void) Dynamic
func StartGoRoutine(args []*runtime.Object, _ checker.Type) *runtime.Object {
	workerFn := args[0]

	// Create a new WaitGroup for this fiber
	wg := &sync.WaitGroup{}
	wg.Add(1)

	// Execute the worker function in the current VM context first
	// This will handle the parsing and setup
	if fn, ok := workerFn.Raw().(runtime.Closure); ok {
		// Evaluate the closure in a goroutine
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					// Handle panics in the goroutine
					fmt.Printf("Panic in fiber: %v\n", r)
				}
			}()

			fn.IsolateEval()
		}()
	}

	return runtime.MakeDynamic(wg)
}

// fn (wg: Dynamic) Void
func JoinRoutine(args []*runtime.Object, _ checker.Type) *runtime.Object {
	wg := args[0].Raw().(*sync.WaitGroup)
	wg.Wait()
	return runtime.Void()
}
