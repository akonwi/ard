package vm

import (
	"fmt"
	"sync"
	"time"

	"github.com/akonwi/ard/checker"
)

// Fiber represents a running async operation
type Fiber struct {
	wg     *sync.WaitGroup
	result *object
}

// AsyncModule handles ard/async module functions
type AsyncModule struct{}

func (m *AsyncModule) Path() string {
	return "ard/async"
}

func (m *AsyncModule) Handle(_ *VM, call *checker.FunctionCall, args []*object) *object {
	// create new vm for module
	vm := New(map[string]checker.Module{})
	switch call.Name {
	case "start":
		return m.handleStart(vm, args)
	case "sleep":
		return m.handleSleep(vm, call, args)
	default:
		panic(fmt.Errorf("Unimplemented: async::%s()", call.Name))
	}
}

func (m *AsyncModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: async::%s::%s()", structName, call.Name))
}

func (m *AsyncModule) handleStart(_ *VM, args []*object) *object {
	workerFn := args[0]

	// Create a new WaitGroup for this fiber
	wg := &sync.WaitGroup{}
	wg.Add(1)

	fiber := &Fiber{
		wg:     wg,
		result: void,
	}

	// Execute the worker function in the current VM context first
	// This will handle the parsing and setup
	if fn, ok := workerFn.raw.(func(args ...*object) *object); ok {
		// fn was defined in a different vm and we need to change its internals to eval with this module's vm
		// Start the goroutine with the evaluated function
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					// Handle panics in the goroutine
					fmt.Printf("Panic in fiber: %v\n", r)
				}
			}()

			// Call the function - this should work since it's already evaluated
			result := fn()
			if result != nil {
				fiber.result = result
			}
		}()
	}

	// Return the fiber object
	return &object{
		raw:   fiber,
		_type: checker.Fiber,
	}
}

func (m *AsyncModule) handleSleep(vm *VM, call *checker.FunctionCall, args []*object) *object {
	duration := args[0].raw.(int)
	time.Sleep(time.Duration(duration) * time.Millisecond)
	return void
}

// EvalFiberMethod handles method calls on Fiber objects
func (m *AsyncModule) EvalFiberMethod(subj *object, call *checker.FunctionCall, args []*object) *object {
	fiber, ok := subj.raw.(*Fiber)
	if !ok {
		panic(fmt.Errorf("Expected Fiber object, got %T", subj.raw))
	}

	switch call.Name {
	case "wait":
		fiber.wg.Wait()
		return void
	default:
		panic(fmt.Errorf("Unimplemented: Fiber.%s()", call.Name))
	}
}
