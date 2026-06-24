package runtime

type fiberState[T any] struct {
	ch    chan T
	value T
	done  bool
}

type Fiber[T any] struct {
	state *fiberState[T]
}

func SpawnFiber[T any](do func() T) Fiber[T] {
	state := &fiberState[T]{ch: make(chan T, 1)}
	go func() {
		state.ch <- do()
	}()
	return Fiber[T]{state: state}
}

func JoinFiber[T any](fiber Fiber[T]) {
	if !fiber.state.done {
		fiber.state.value = <-fiber.state.ch
		fiber.state.done = true
	}
}

func GetFiber[T any](fiber Fiber[T]) T {
	JoinFiber(fiber)
	return fiber.state.value
}
