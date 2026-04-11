package ardgo

type Maybe[T any] struct {
	value T
	none  bool
}

func Some[T any](value T) Maybe[T] {
	return Maybe[T]{value: value}
}

func None[T any]() Maybe[T] {
	return Maybe[T]{none: true}
}

func (m Maybe[T]) IsNone() bool {
	return m.none
}

func (m Maybe[T]) IsSome() bool {
	return !m.none
}

func (m Maybe[T]) Expect(message string) T {
	if m.none {
		panic(message)
	}
	return m.value
}

func (m Maybe[T]) Or(defaultValue T) T {
	if m.none {
		return defaultValue
	}
	return m.value
}
