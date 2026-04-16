package list

import (
	ardgo "github.com/akonwi/ard/go"
)

type Partition[T any] struct {
	Others   []T
	Selected []T
}

func New[T any]() []T {
	result, err := ardgo.CallExtern("NewList")
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[[]T](result)
}

func Concat[T any](a []T, b []T) []T {
	res := append([]T(nil), a...)
	for _, i := range b {
		_ = func() []T { res = append(res, i); return res }()
	}
	return res
}

func Drop[T any](from []T, till int) []T {
	out := append([]T(nil), []T{}...)
	for idx, t := range from {
		if idx >= till {
			_ = func() []T { out = append(out, t); return out }()
		}
	}
	return out
}

func Keep[T any](list []T, where func(T) bool) []T {
	out := append([]T(nil), []T{}...)
	for _, t := range list {
		if where(t) {
			_ = func() []T { out = append(out, t); return out }()
		}
	}
	return out
}

func Map[A any, B any](list []A, transform func(A) B) []B {
	out := append([]B(nil), []B{}...)
	for _, t := range list {
		_ = func() []B { out = append(out, transform(t)); return out }()
	}
	return out
}

func Find[T any](list []T, where func(T) bool) ardgo.Maybe[T] {
	var found ardgo.Maybe[T] = ardgo.None[T]()
	for _, t := range list {
		if where(t) {
			found = ardgo.Some[T](t)
			break
		}
	}
	return found
}

func PartitionFn[T any](list []T, where func(T) bool) Partition[T] {
	selected := append([]T(nil), []T{}...)
	others := append([]T(nil), []T{}...)
	for _, t := range list {
		if where(t) {
			_ = func() []T { selected = append(selected, t); return selected }()
		} else {
			_ = func() []T { others = append(others, t); return others }()
		}
	}
	return Partition[T]{Others: others, Selected: selected}
}
