package runtime

import (
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
)

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

// MarshalJSONTo is the encoding/json/v2 form of MarshalJSON, which Go's JSON
// packages prefer. It encodes the unwrapped value with the caller's encoder so
// encoder options (for example SetEscapeHTML) also apply inside a Result.
// Results have no unmarshalers: choosing ok or err from JSON is ambiguous.
func (r Result[T, E]) MarshalJSONTo(enc *jsontext.Encoder) error {
	if r.Ok {
		return jsonv2.MarshalEncode(enc, r.Value)
	}
	return jsonv2.MarshalEncode(enc, r.Err)
}
