package ardgo

type Result[T, E any] struct {
	value T
	err   E
	ok    bool
}

func Ok[T, E any](value T) Result[T, E] {
	return Result[T, E]{value: value, ok: true}
}

func Err[T, E any](err E) Result[T, E] {
	return Result[T, E]{err: err}
}

func (r Result[T, E]) IsOk() bool {
	return r.ok
}

func (r Result[T, E]) IsErr() bool {
	return !r.ok
}

func (r Result[T, E]) Or(fallback T) T {
	if r.ok {
		return r.value
	}
	return fallback
}

func (r Result[T, E]) Expect(message string) T {
	if !r.ok {
		panic(message)
	}
	return r.value
}

func (r Result[T, E]) UnwrapOk() T {
	return r.value
}

func (r Result[T, E]) UnwrapErr() E {
	return r.err
}
