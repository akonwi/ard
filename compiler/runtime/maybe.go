package runtime

type Maybe[T any] struct {
	Value T
	Some  bool
}

func Some[T any](value T) Maybe[T] {
	return Maybe[T]{Value: value, Some: true}
}

func None[T any]() Maybe[T] {
	return Maybe[T]{}
}
