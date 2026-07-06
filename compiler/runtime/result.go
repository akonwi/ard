package runtime

import "encoding/json"

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

// MarshalJSON encodes a Result as its unwrapped value: ok(x) marshals to x and
// err(e) marshals to e (ADR 0031). The ok/err distinction is not preserved on
// the wire; the value's own type carries that meaning.
func (r Result[T, E]) MarshalJSON() ([]byte, error) {
	if r.Ok {
		return json.Marshal(r.Value)
	}
	return json.Marshal(r.Err)
}
