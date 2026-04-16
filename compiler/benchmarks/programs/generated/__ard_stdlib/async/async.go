package async

import (
	ardgo "github.com/akonwi/ard/go"
	"sync"
)

type fiberState[T any] struct {
	wg     sync.WaitGroup
	result T
}

type Fiber[T any] struct {
	Result T
	Wg     any
}

func (self Fiber[T]) fiberHandle() any {
	return self.Wg
}

func (self Fiber[T]) Get() T {
	self.Join()
	return fiberGet[T](self.Wg)
}

func (self Fiber[T]) Join() {
	fiberWait(self.Wg)
}

func Sleep(ms int) {
	_, err := ardgo.CallExtern("Sleep", ms)
	if err != nil {
		panic(err)
	}
}

func Start(do func()) Fiber[struct{}] {
	state := &fiberState[struct{}]{}
	state.wg.Add(1)
	go func() {
		defer state.wg.Done()
		do()
	}()
	return Fiber[struct{}]{Wg: state}
}

func Eval[T any](do func() T) Fiber[T] {
	state := &fiberState[T]{}
	state.wg.Add(1)
	go func() {
		defer state.wg.Done()
		state.result = do()
	}()
	return Fiber[T]{Wg: state}
}

func Join[T any](fibers []Fiber[T]) {
	for _, fiber := range fibers {
		fiberWait(fiber.Wg)
	}
}

func JoinAny(fibers []any) {
	for _, fiber := range fibers {
		handleProvider, ok := fiber.(interface{ fiberHandle() any })
		if !ok {
			panic("unexpected async fiber")
		}
		fiberWait(handleProvider.fiberHandle())
	}
}

type fiberWaiter interface {
	wait()
}

type fiberGetter[T any] interface {
	get() T
}

func (state *fiberState[T]) wait() {
	state.wg.Wait()
}

func (state *fiberState[T]) get() T {
	state.wg.Wait()
	return state.result
}

func fiberWait(handle any) {
	if waiter, ok := handle.(fiberWaiter); ok {
		waiter.wait()
		return
	}
	panic("unexpected async fiber handle")
}

func fiberGet[T any](handle any) T {
	if getter, ok := handle.(fiberGetter[T]); ok {
		return getter.get()
	}
	panic("unexpected async fiber handle")
}
