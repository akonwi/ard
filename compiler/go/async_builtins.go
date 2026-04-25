package ardgo

import (
	"fmt"
	"reflect"
	"sync"
)

type asyncFiberState struct {
	wg        sync.WaitGroup
	mu        sync.Mutex
	result    any
	panicVal  any
	hasResult bool
	hasPanic  bool
}

type asyncFiberValue struct {
	Wg     any
	Result any
}

func (s *asyncFiberState) Wait() {
	s.wg.Wait()
}

func (s *asyncFiberState) setResult(value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result = value
	s.hasResult = true
}

func (s *asyncFiberState) setPanic(value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.panicVal = value
	s.hasPanic = true
}

func (s *asyncFiberState) getResult(fallback any) any {
	s.wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hasPanic {
		panic(fmt.Errorf("panic in fiber: %v", s.panicVal))
	}
	if s.hasResult {
		return s.result
	}
	return fallback
}

func runAsyncClosure(fn any, captureResult bool) *asyncFiberState {
	fnValue := reflect.ValueOf(fn)
	if !fnValue.IsValid() || fnValue.Kind() != reflect.Func {
		panic(fmt.Errorf("async function must be callable, got %T", fn))
	}
	if fnValue.Type().NumIn() != 0 {
		panic(fmt.Errorf("async function must not accept parameters, got %d", fnValue.Type().NumIn()))
	}

	state := &asyncFiberState{}
	state.wg.Add(1)
	go func() {
		defer state.wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				state.setPanic(recovered)
			}
		}()
		results := fnValue.Call(nil)
		if captureResult && len(results) > 0 {
			state.setResult(results[0].Interface())
		}
	}()
	return state
}

func asyncStartFiber(fn any) any {
	state := runAsyncClosure(fn, false)
	return asyncFiberValue{Wg: state, Result: nil}
}

func asyncEvalFiber(fn any) any {
	state := runAsyncClosure(fn, true)
	return asyncFiberValue{Wg: state, Result: nil}
}

func asyncGetResult(handle any, fallback any) any {
	if state, ok := handle.(*asyncFiberState); ok {
		return state.getResult(fallback)
	}
	asyncWaitFor(handle)
	return fallback
}

func asyncWaitFor(handle any) {
	if waiter, ok := handle.(interface{ Wait() }); ok {
		waiter.Wait()
		return
	}
	panic(fmt.Errorf("wait handle does not implement Wait(): %T", handle))
}
