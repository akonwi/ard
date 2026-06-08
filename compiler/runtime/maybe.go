package runtime

import (
	"bytes"
	"encoding/json"
)

type Maybe[T any] struct {
	value *T
}

func Some[T any](value T) Maybe[T] {
	return Maybe[T]{value: &value}
}

func None[T any]() Maybe[T] {
	return Maybe[T]{}
}

func FromPtr[T any](value *T) Maybe[T] {
	if value == nil {
		return None[T]()
	}
	return Some(*value)
}

func (m Maybe[T]) IsSome() bool {
	return m.value != nil
}

func (m Maybe[T]) IsNone() bool {
	return m.value == nil
}

func (m Maybe[T]) Value() T {
	if m.value == nil {
		var zero T
		return zero
	}
	return *m.value
}

func (m Maybe[T]) Ptr() *T {
	if m.value == nil {
		return nil
	}
	value := *m.value
	return &value
}

func MaybeEqual[T any](left, right Maybe[T]) bool {
	if left.IsNone() || right.IsNone() {
		return left.IsNone() && right.IsNone()
	}
	return Equal(left.Value(), right.Value())
}

func (m Maybe[T]) MarshalJSON() ([]byte, error) {
	if m.value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*m.value)
}

func (m *Maybe[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*m = None[T]()
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*m = Some(value)
	return nil
}
