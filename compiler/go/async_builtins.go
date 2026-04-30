package ardgo

import (
	"fmt"
	"reflect"

	"github.com/akonwi/ard/ffi"
)

func runAsyncClosure(fn any, captureResult bool) *ffi.AsyncHandle {
	fnValue := reflect.ValueOf(fn)
	if !fnValue.IsValid() || fnValue.Kind() != reflect.Func {
		panic(fmt.Errorf("async function must be callable, got %T", fn))
	}
	if fnValue.Type().NumIn() != 0 {
		panic(fmt.Errorf("async function must not accept parameters, got %d", fnValue.Type().NumIn()))
	}

	handle := ffi.NewAsyncHandle()
	go func() {
		defer handle.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				handle.SetPanic(recovered)
			}
		}()
		results := fnValue.Call(nil)
		if captureResult && len(results) > 0 {
			handle.SetResult(results[0].Interface())
		}
	}()
	return handle
}

func asyncStartFiber(fn any) any {
	return runAsyncClosure(fn, false)
}

func asyncEvalFiber(fn any) any {
	return runAsyncClosure(fn, true)
}

func asyncGetResult(handle any) any {
	if state, ok := handle.(*ffi.AsyncHandle); ok {
		return state.GetResult()
	}
	asyncWaitFor(handle)
	return nil
}

func asyncWaitFor(handle any) {
	if waiter, ok := handle.(interface{ Wait() }); ok {
		waiter.Wait()
		return
	}
	panic(fmt.Errorf("wait handle does not implement Wait(): %T", handle))
}
