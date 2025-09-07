package vm

import (
	"fmt"
	"sync"
	"time"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

// AsyncModule handles ard/async module functions
type AsyncModule struct {
	hq *GlobalVM
}

func (m *AsyncModule) Path() string {
	return "ard/async"
}

func (m *AsyncModule) get(name string) *runtime.Object {
	return nil
}

func (m *AsyncModule) Handle(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "start":
		return m.handleStart(args)
	case "sleep":
		return m.handleSleep(args)
	default:
		panic(fmt.Errorf("Unimplemented: async::%s()", call.Name))
	}
}

func (m *AsyncModule) HandleStatic(structName string, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	panic(fmt.Errorf("Unimplemented: async::%s::%s()", structName, call.Name))
}

func (m *AsyncModule) handleStart(args []*runtime.Object) *runtime.Object {
	workerFn := args[0]

	// Create a new WaitGroup for this fiber
	wg := &sync.WaitGroup{}
	wg.Add(1)

	// Execute the worker function in the current VM context first
	// This will handle the parsing and setup
	if fn, ok := workerFn.Raw().(*VMClosure); ok {
		// Copy the closure and give it a new VM
		isolatedFn := fn.copy()
		isolatedFn.vm = NewVM()
		isolatedFn.vm.hq = m.hq
		// Start the goroutine with the evaluated function
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					// Handle panics in the goroutine
					fmt.Printf("Panic in fiber: %v\n", r)
				}
			}()

			isolatedFn.Eval()
		}()
	}

	// Return the fiber instance
	fields := map[string]*runtime.Object{
		"__wg": runtime.MakeDynamic(wg),
	}
	return runtime.MakeStruct(checker.Fiber, fields)
}

func (m *AsyncModule) handleSleep(args []*runtime.Object) *runtime.Object {
	duration := args[0].AsInt()
	time.Sleep(time.Duration(duration) * time.Millisecond)
	return runtime.Void()
}

// EvalFiberMethod handles method calls on Fiber objects
func (m *AsyncModule) EvalFiberMethod(subj *runtime.Object, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	wg, ok := subj.Struct_Get("__wg").Raw().(*sync.WaitGroup)
	if !ok {
		panic(fmt.Errorf("Expected Fiber object, got %T", subj.Raw()))
	}

	switch call.Name {
	case "join":
		wg.Wait()
		return runtime.Void()
	default:
		panic(fmt.Errorf("Unimplemented: Fiber.%s()", call.Name))
	}
}
