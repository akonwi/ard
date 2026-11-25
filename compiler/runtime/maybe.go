package runtime

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
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

func MaybeEqual[T comparable](left, right Maybe[T]) bool {
	if left.IsNone() || right.IsNone() {
		return left.IsNone() && right.IsNone()
	}
	return left.Value() == right.Value()
}

// Maybe implements both JSON method sets. Go's JSON packages (v1 is built on
// v2 since Go 1.27) prefer MarshalJSONTo and UnmarshalJSONFrom, which encode
// or decode the payload with the caller's encoder or decoder, so options such
// as DisallowUnknownFields, UseNumber, SetEscapeHTML, or RejectUnknownMembers
// also apply inside a Maybe. MarshalJSON and UnmarshalJSON remain for direct
// callers and interface checks and behave the same under default options.

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

func (m Maybe[T]) MarshalJSONTo(enc *jsontext.Encoder) error {
	if m.value == nil {
		return enc.WriteToken(jsontext.Null)
	}
	return jsonv2.MarshalEncode(enc, *m.value)
}

func (m *Maybe[T]) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if dec.PeekKind() == jsontext.KindNull {
		if _, err := dec.ReadToken(); err != nil {
			return err
		}
		*m = None[T]()
		return nil
	}
	var value T
	if err := jsonv2.UnmarshalDecode(dec, &value); err != nil {
		return err
	}
	*m = Some(value)
	return nil
}
