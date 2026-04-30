package runtime

import (
	"reflect"
	"testing"

	"github.com/akonwi/ard/bytecode"
)

func TestMaybeValueHelpers(t *testing.T) {
	some := SomeValue("hello")
	if some.None {
		t.Fatal("expected SomeValue to produce non-none maybe")
	}
	if got := some.Value; got != "hello" {
		t.Fatalf("expected maybe value %q, got %v", "hello", got)
	}

	none := NoneValue()
	if !none.None {
		t.Fatal("expected NoneValue to produce none maybe")
	}
	if none.Value != nil {
		t.Fatalf("expected none maybe value to be nil, got %v", none.Value)
	}
}

func TestResultValueHelpers(t *testing.T) {
	ok := OkValue(42)
	if ok.IsErr {
		t.Fatal("expected OkValue to produce non-error result")
	}
	if got := ok.Ok; got != 42 {
		t.Fatalf("expected ok payload 42, got %v", got)
	}
	if ok.Err != nil {
		t.Fatalf("expected inactive err payload to be nil, got %v", ok.Err)
	}

	err := ErrValue("boom")
	if !err.IsErr {
		t.Fatal("expected ErrValue to produce error result")
	}
	if got := err.Err; got != "boom" {
		t.Fatalf("expected err payload %q, got %v", "boom", got)
	}
	if err.Ok != nil {
		t.Fatalf("expected inactive ok payload to be nil, got %v", err.Ok)
	}
}

func TestNativeVoidSentinel(t *testing.T) {
	if NativeVoid != (VoidValue{}) {
		t.Fatalf("expected NativeVoid to equal zero-value VoidValue, got %#v", NativeVoid)
	}
}

func TestNativeMapStringOperations(t *testing.T) {
	m := NewMap[string]()
	if m.Len() != 0 {
		t.Fatalf("expected empty map len 0, got %d", m.Len())
	}
	if ok := m.SetAny("b", 2); !ok {
		t.Fatal("expected SetAny to accept string key")
	}
	if ok := m.SetAny("a", 1); !ok {
		t.Fatal("expected SetAny to accept string key")
	}
	if ok := m.SetAny(1, 99); ok {
		t.Fatal("expected SetAny to reject wrong key type")
	}
	if got, ok := m.GetAny("a"); !ok || got != 1 {
		t.Fatalf("expected GetAny to return 1,true got %v,%v", got, ok)
	}
	if _, ok := m.GetAny(1); ok {
		t.Fatal("expected GetAny to reject wrong key type")
	}
	if !m.HasAny("b") {
		t.Fatal("expected HasAny to find existing key")
	}
	if m.HasAny(2) {
		t.Fatal("expected HasAny to reject wrong key type")
	}
	if keys := m.Keys(); !reflect.DeepEqual(keys, []any{"a", "b"}) {
		t.Fatalf("expected sorted string keys [a b], got %#v", keys)
	}
	if ok := m.DropAny("a"); !ok {
		t.Fatal("expected DropAny to remove existing key")
	}
	if ok := m.DropAny("a"); ok {
		t.Fatal("expected DropAny to report false for missing key")
	}
}

func TestNativeMapBoolKeysAndCopy(t *testing.T) {
	m := NewMap[bool]()
	if ok := m.SetAny(true, "yes"); !ok {
		t.Fatal("expected SetAny to accept bool key")
	}
	if ok := m.SetAny(false, "no"); !ok {
		t.Fatal("expected SetAny to accept bool key")
	}
	if keys := m.Keys(); !reflect.DeepEqual(keys, []any{false, true}) {
		t.Fatalf("expected sorted bool keys [false true], got %#v", keys)
	}

	copied, ok := m.Copy().(*Map[bool])
	if !ok {
		t.Fatalf("expected Copy to return *Map[bool], got %T", m.Copy())
	}
	if got, ok := copied.GetAny(true); !ok || got != "yes" {
		t.Fatalf("expected copied map to contain true => yes, got %v,%v", got, ok)
	}
	if ok := copied.SetAny(true, "changed"); !ok {
		t.Fatal("expected SetAny on copied map to succeed")
	}
	if got, _ := m.GetAny(true); got != "yes" {
		t.Fatalf("expected original map to remain unchanged, got %v", got)
	}
}

func TestMapValueStructValueAndEnumValueShapes(t *testing.T) {
	storage := NewMap[int]()
	_ = storage.SetAny(1, "one")

	mapValue := MapValue{
		KeyType:   bytecode.TypeID(1),
		ValueType: bytecode.TypeID(2),
		Storage:   storage,
	}
	if mapValue.KeyType != 1 || mapValue.ValueType != 2 {
		t.Fatalf("unexpected map type ids: %#v", mapValue)
	}
	if got, ok := mapValue.Storage.GetAny(1); !ok || got != "one" {
		t.Fatalf("expected map storage to contain 1 => one, got %v,%v", got, ok)
	}

	structValue := StructValue{TypeID: 3, Fields: []any{"name", 42}}
	if structValue.TypeID != 3 || len(structValue.Fields) != 2 {
		t.Fatalf("unexpected struct value: %#v", structValue)
	}

	enumValue := EnumValue{TypeID: 4, Tag: 2, Value: "payload"}
	if enumValue.TypeID != 4 || enumValue.Tag != 2 || enumValue.Value != "payload" {
		t.Fatalf("unexpected enum value: %#v", enumValue)
	}
}
