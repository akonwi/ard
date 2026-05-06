package runtime

type Result[T any, E any] struct {
	Value T
	Err   E
	Ok    bool
}

func Ok[T any, E any](value T) Result[T, E] {
	return Result[T, E]{Value: value, Ok: true}
}

func Err[T any, E any](err E) Result[T, E] {
	return Result[T, E]{Err: err}
}
