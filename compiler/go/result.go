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

func ResultMap[T, U, E any](r Result[T, E], with func(T) U) Result[U, E] {
	if r.ok {
		return Ok[U, E](with(r.value))
	}
	return Err[U, E](r.err)
}

func ResultMapErr[T, E, F any](r Result[T, E], with func(E) F) Result[T, F] {
	if r.ok {
		return Ok[T, F](r.value)
	}
	return Err[T, F](with(r.err))
}

func ResultAndThen[T, U, E any](r Result[T, E], with func(T) Result[U, E]) Result[U, E] {
	if r.ok {
		return with(r.value)
	}
	return Err[U, E](r.err)
}
