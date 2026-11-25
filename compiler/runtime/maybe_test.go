package runtime

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"strings"
	"testing"
)

func TestMaybeCanHoldRecursiveType(t *testing.T) {
	type Node struct {
		Value  int
		Parent Maybe[Node]
	}

	root := Node{Value: 1, Parent: None[Node]()}
	child := Node{Value: 2, Parent: Some(root)}

	if child.Parent.IsNone() {
		t.Fatal("parent = none, want some")
	}
	if got := child.Parent.Value().Value; got != 1 {
		t.Fatalf("parent value = %d, want 1", got)
	}
}
func TestMaybeJSON(t *testing.T) {
	encodedNone, err := json.Marshal(None[int]())
	if err != nil {
		t.Fatalf("marshal none: %v", err)
	}
	if string(encodedNone) != "null" {
		t.Fatalf("marshal none = %s, want null", encodedNone)
	}

	encodedSome, err := json.Marshal(Some(42))
	if err != nil {
		t.Fatalf("marshal some: %v", err)
	}
	if string(encodedSome) != "42" {
		t.Fatalf("marshal some = %s, want 42", encodedSome)
	}

	var decoded Maybe[int]
	if err := json.Unmarshal([]byte("42"), &decoded); err != nil {
		t.Fatalf("unmarshal some: %v", err)
	}
	if decoded.IsNone() || decoded.Value() != 42 {
		t.Fatalf("unmarshal some = %#v, want some(42)", decoded)
	}
	if err := json.Unmarshal([]byte("null"), &decoded); err != nil {
		t.Fatalf("unmarshal none: %v", err)
	}
	if decoded.IsSome() {
		t.Fatalf("unmarshal null = some, want none")
	}
}

func TestMaybeSomeNilPointerIsDistinctFromNone(t *testing.T) {
	type Item struct{ Value int }

	var ptr *Item
	someNil := Some(ptr)
	if someNil.IsNone() {
		t.Fatal("some(nil pointer) reported none")
	}
	if got := someNil.Value(); got != nil {
		t.Fatalf("some(nil pointer).Value() = %#v, want nil", got)
	}

	none := None[*Item]()
	if none.IsSome() {
		t.Fatal("none reported some")
	}
}

type maybeJSONInner struct {
	Known int `json:"known"`
	Extra any `json:"extra"`
}

type maybeJSONOuter struct {
	Inner Maybe[maybeJSONInner] `json:"inner"`
}

// TestMaybeJSONDecodeHonorsCallerDecoderOptions pins issue #634: decoding a
// Maybe payload must reuse the caller's decoder and its options rather than
// a fresh json.Unmarshal, so strictness applies inside a `T?`.
func TestMaybeJSONDecodeHonorsCallerDecoderOptions(t *testing.T) {
	const input = `{"inner": {"known": 1, "extra": 12345678901234567890, "bogus": 2}}`

	t.Run("v1 DisallowUnknownFields", func(t *testing.T) {
		dec := json.NewDecoder(strings.NewReader(input))
		dec.DisallowUnknownFields()
		var out maybeJSONOuter
		err := dec.Decode(&out)
		if err == nil || !strings.Contains(err.Error(), "bogus") {
			t.Fatalf("Decode error = %v, want unknown field \"bogus\"", err)
		}
	})

	t.Run("v1 UseNumber", func(t *testing.T) {
		dec := json.NewDecoder(strings.NewReader(input))
		dec.UseNumber()
		var out maybeJSONOuter
		if err := dec.Decode(&out); err != nil {
			t.Fatalf("Decode error = %v", err)
		}
		got, ok := out.Inner.Value().Extra.(json.Number)
		if !ok || got.String() != "12345678901234567890" {
			t.Fatalf("Extra = %#v, want json.Number(12345678901234567890)", out.Inner.Value().Extra)
		}
	})

	t.Run("v2 RejectUnknownMembers", func(t *testing.T) {
		var out maybeJSONOuter
		err := jsonv2.Unmarshal([]byte(input), &out, jsonv2.RejectUnknownMembers(true))
		if err == nil || !strings.Contains(err.Error(), "bogus") {
			t.Fatalf("Unmarshal error = %v, want unknown member \"bogus\"", err)
		}
	})

	t.Run("lenient decode still accepts unknown fields", func(t *testing.T) {
		var out maybeJSONOuter
		if err := json.Unmarshal([]byte(input), &out); err != nil {
			t.Fatalf("Unmarshal error = %v", err)
		}
		if out.Inner.IsNone() || out.Inner.Value().Known != 1 {
			t.Fatalf("Inner = %#v, want some({known: 1})", out.Inner)
		}
	})
}

func TestMaybeJSONDecodeNullReplacesSome(t *testing.T) {
	out := maybeJSONOuter{Inner: Some(maybeJSONInner{Known: 9})}
	dec := json.NewDecoder(strings.NewReader(`{"inner": null}`))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("Decode error = %v", err)
	}
	if out.Inner.IsSome() {
		t.Fatalf("Inner = %#v, want none", out.Inner)
	}
}

func TestMaybeJSONEncodeHonorsCallerEncoderOptions(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(Some("<a>")); err != nil {
		t.Fatalf("Encode error = %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != `"<a>"` {
		t.Fatalf("Encode = %s, want \"<a>\" (SetEscapeHTML(false) ignored)", got)
	}
}

// TestMaybeJSONMethodSetsAgree keeps the v1 (MarshalJSON/UnmarshalJSON) and
// v2 (MarshalJSONTo/UnmarshalJSONFrom) methods equivalent under default
// options, as encoding/json/v2 requires of types implementing both.
func TestMaybeJSONMethodSetsAgree(t *testing.T) {
	for _, m := range []Maybe[maybeJSONInner]{None[maybeJSONInner](), Some(maybeJSONInner{Known: 3, Extra: "x"})} {
		v1, err := m.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON error = %v", err)
		}
		v2, err := jsonv2.Marshal(m, json.DefaultOptionsV1())
		if err != nil {
			t.Fatalf("v2 Marshal error = %v", err)
		}
		if string(v1) != string(v2) {
			t.Fatalf("MarshalJSON = %s, MarshalJSONTo = %s", v1, v2)
		}
	}

	for _, input := range []string{`null`, `{"known": 3, "extra": "x"}`} {
		var viaV1, viaV2 Maybe[maybeJSONInner]
		if err := viaV1.UnmarshalJSON([]byte(input)); err != nil {
			t.Fatalf("UnmarshalJSON(%s) error = %v", input, err)
		}
		if err := jsonv2.Unmarshal([]byte(input), &viaV2); err != nil {
			t.Fatalf("UnmarshalJSONFrom(%s) error = %v", input, err)
		}
		if viaV1.IsSome() != viaV2.IsSome() || viaV1.Value() != viaV2.Value() {
			t.Fatalf("decode %s: UnmarshalJSON = %#v, UnmarshalJSONFrom = %#v", input, viaV1, viaV2)
		}
	}
}

func TestMaybeJSONDecodeEdgeCases(t *testing.T) {
	t.Run("absent field stays none", func(t *testing.T) {
		dec := json.NewDecoder(strings.NewReader(`{}`))
		dec.DisallowUnknownFields()
		var out maybeJSONOuter
		if err := dec.Decode(&out); err != nil {
			t.Fatalf("Decode error = %v", err)
		}
		if out.Inner.IsSome() {
			t.Fatalf("Inner = %#v, want none", out.Inner)
		}
	})

	for _, input := range []string{`{"inner": }`, `{"inner": {"known": 1}`, `{"inner": nul}`} {
		t.Run("malformed "+input, func(t *testing.T) {
			var out maybeJSONOuter
			if err := json.Unmarshal([]byte(input), &out); err == nil {
				t.Fatalf("Unmarshal(%s) error = nil, want syntax error", input)
			}
		})
	}

	t.Run("type error inside payload is reported", func(t *testing.T) {
		var out maybeJSONOuter
		err := json.Unmarshal([]byte(`{"inner": {"known": "x"}}`), &out)
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			t.Fatalf("Unmarshal error = %v, want *json.UnmarshalTypeError", err)
		}
	})
}

// TestMaybeJSONEncodeNilSliceFollowsCallerSemantics pins that a some(nil
// slice) payload is encoded with the caller's semantics: v1 emits null (as
// encoding/json does for nil slices) and v2 emits [].
func TestMaybeJSONEncodeNilSliceFollowsCallerSemantics(t *testing.T) {
	m := Some[[]int](nil)
	v1, err := json.Marshal(m)
	if err != nil || string(v1) != "null" {
		t.Fatalf("v1 Marshal = %s, %v; want null", v1, err)
	}
	v2, err := jsonv2.Marshal(m)
	if err != nil || string(v2) != "[]" {
		t.Fatalf("v2 Marshal = %s, %v; want []", v2, err)
	}
}
