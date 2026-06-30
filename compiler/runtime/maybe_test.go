package runtime

import (
	"encoding/json"
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
