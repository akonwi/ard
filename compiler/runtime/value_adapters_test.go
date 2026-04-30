package runtime

import (
	"reflect"
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestValueObjectRoundTripScalars(t *testing.T) {
	tests := []struct {
		name  string
		type_ checker.Type
		value any
	}{
		{name: "str", type_: checker.Str, value: "hello"},
		{name: "int", type_: checker.Int, value: 42},
		{name: "float", type_: checker.Float, value: 3.5},
		{name: "bool", type_: checker.Bool, value: true},
		{name: "dynamic", type_: checker.Dynamic, value: map[string]any{"x": 1}},
		{name: "void", type_: checker.Void, value: NativeVoid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := ValueToObject(tt.value, tt.type_)
			got := ObjectToValue(obj, tt.type_)
			if !reflect.DeepEqual(got, tt.value) {
				t.Fatalf("round-trip mismatch: want %#v got %#v", tt.value, got)
			}
		})
	}
}

func TestValueObjectRoundTripMaybeAndResult(t *testing.T) {
	maybeType := checker.MakeMaybe(checker.Int)
	maybeValue := SomeValue(99)
	maybeObj := ValueToObject(maybeValue, maybeType)
	if got := ObjectToValue(maybeObj, maybeType); !reflect.DeepEqual(got, maybeValue) {
		t.Fatalf("maybe round-trip mismatch: want %#v got %#v", maybeValue, got)
	}

	noneValue := NoneValue()
	noneObj := ValueToObject(noneValue, maybeType)
	if got := ObjectToValue(noneObj, maybeType); !reflect.DeepEqual(got, noneValue) {
		t.Fatalf("none round-trip mismatch: want %#v got %#v", noneValue, got)
	}

	resultType := checker.MakeResult(checker.Str, checker.Int)
	okValue := OkValue("done")
	okObj := ValueToObject(okValue, resultType)
	if got := ObjectToValue(okObj, resultType); !reflect.DeepEqual(got, okValue) {
		t.Fatalf("ok result round-trip mismatch: want %#v got %#v", okValue, got)
	}

	errValue := ErrValue(7)
	errObj := ValueToObject(errValue, resultType)
	if got := ObjectToValue(errObj, resultType); !reflect.DeepEqual(got, errValue) {
		t.Fatalf("err result round-trip mismatch: want %#v got %#v", errValue, got)
	}
}

func TestValueObjectRoundTripListValue(t *testing.T) {
	listType := checker.MakeList(checker.Int)
	value := ListValue{1, 2, 3}
	obj := ValueToObject(value, listType)
	got := ObjectToValue(obj, listType)
	if !reflect.DeepEqual(got, value) {
		t.Fatalf("list round-trip mismatch: want %#v got %#v", value, got)
	}
}

func TestValueObjectRoundTripMapValue(t *testing.T) {
	mapType := checker.MakeMap(checker.Int, checker.Str)
	storage := NewMap[int]()
	_ = storage.SetAny(2, "two")
	_ = storage.SetAny(1, "one")
	value := MapValue{Storage: storage}

	obj := ValueToObject(value, mapType)
	got, ok := ObjectToValue(obj, mapType).(MapValue)
	if !ok {
		t.Fatalf("expected MapValue round-trip result, got %T", ObjectToValue(obj, mapType))
	}
	if keys := got.Storage.Keys(); !reflect.DeepEqual(keys, []any{1, 2}) {
		t.Fatalf("unexpected map keys after round-trip: %#v", keys)
	}
	if gotVal, ok := got.Storage.GetAny(1); !ok || gotVal != "one" {
		t.Fatalf("expected key 1 => one, got %v,%v", gotVal, ok)
	}
	if gotVal, ok := got.Storage.GetAny(2); !ok || gotVal != "two" {
		t.Fatalf("expected key 2 => two, got %v,%v", gotVal, ok)
	}
}

func TestValueObjectRoundTripStructValue(t *testing.T) {
	structType := &checker.StructDef{
		Name: "Person",
		Fields: map[string]checker.Type{
			"name": checker.Str,
			"age":  checker.Int,
		},
		Methods: map[string]*checker.FunctionDef{},
	}
	value := StructValue{Fields: []any{30, "Alice"}}

	obj := ValueToObject(value, structType)
	got := ObjectToValue(obj, structType)
	if !reflect.DeepEqual(got, value) {
		t.Fatalf("struct round-trip mismatch: want %#v got %#v", value, got)
	}
}

func TestValueObjectRoundTripEnumValue(t *testing.T) {
	enumType := &checker.Enum{Name: "Light", Values: []checker.EnumValue{{Name: "Red", Value: 1}, {Name: "Green", Value: 2}}}
	value := EnumValue{Tag: 2}
	obj := ValueToObject(value, enumType)
	got := ObjectToValue(obj, enumType)
	if !reflect.DeepEqual(got, value) {
		t.Fatalf("enum round-trip mismatch: want %#v got %#v", value, got)
	}
}
