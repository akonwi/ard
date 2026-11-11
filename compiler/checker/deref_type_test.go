package checker

import (
	"maps"
	"testing"
)

// TestDerefTypeReturnsOriginalWhenNothingIsBound pins derefType's identity
// guarantee: a compound type whose members contain no bound type variables
// comes back as the same pointer, so callers can rely on pointer equality to
// detect "no change".
func TestDerefTypeReturnsOriginalWhenNothingIsBound(t *testing.T) {
	structType := &StructDef{
		Name:     "Pair",
		Fields:   map[string]Type{"left": Int, "right": &List{of: Str}},
		TypeArgs: []Type{Int},
	}
	fnType := &FunctionDef{
		Name:       "f",
		Parameters: []Parameter{{Name: "a", Type: Int}, {Name: "b", Type: structType}},
		ReturnType: &Maybe{of: Str},
	}
	for name, typ := range map[string]Type{
		"struct":   structType,
		"function": fnType,
		"union":    &Union{Name: "u", Types: []Type{Int, Str}},
		"foreign":  &ForeignType{TypeArgs: []Type{Int}},
	} {
		if got := derefType(typ); got != typ {
			t.Errorf("%s: derefType returned a new value for an unchanged type", name)
		}
	}
}

// TestDerefTypeRebuildsOnlyChangedCompounds checks that a bound type variable
// anywhere inside a struct or function type yields a rebuilt type whose other
// members are preserved, and that unchanged members are not shared storage
// with the original.
func TestDerefTypeRebuildsOnlyChangedCompounds(t *testing.T) {
	bound := &TypeVar{name: "T", actual: Int, bound: true}

	structType := &StructDef{
		Name:     "Box",
		Fields:   map[string]Type{"value": bound, "label": Str},
		TypeArgs: []Type{Str},
	}
	got, ok := derefType(structType).(*StructDef)
	if !ok || got == structType {
		t.Fatalf("struct with a bound field must be rebuilt, got %T (same=%v)", got, got == structType)
	}
	if got.Fields["value"] != Int || got.Fields["label"] != Str {
		t.Fatalf("rebuilt struct fields = %v, want value=Int label=Str", got.Fields)
	}
	if len(got.TypeArgs) != 1 || got.TypeArgs[0] != Str {
		t.Fatalf("rebuilt struct type args = %v, want [Str]", got.TypeArgs)
	}
	if &got.TypeArgs[0] == &structType.TypeArgs[0] {
		t.Fatal("rebuilt struct must not alias the original type args")
	}
	if maps.Equal(got.Fields, structType.Fields) {
		t.Fatal("rebuilt struct must not reuse the original field map")
	}

	fnType := &FunctionDef{
		Name:       "f",
		Parameters: []Parameter{{Name: "a", Type: Str, Mutable: true}, {Name: "b", Type: bound}},
		ReturnType: bound,
	}
	gotFn, ok := derefType(fnType).(*FunctionDef)
	if !ok || gotFn == fnType {
		t.Fatalf("function with bound types must be rebuilt")
	}
	if gotFn.Parameters[0].Type != Str || !gotFn.Parameters[0].Mutable || gotFn.Parameters[1].Type != Int || gotFn.ReturnType != Int {
		t.Fatalf("rebuilt function = params %v return %v", gotFn.Parameters, gotFn.ReturnType)
	}
	if fnType.Parameters[1].Type != bound {
		t.Fatal("original function parameters must be untouched")
	}

	returnOnly := &FunctionDef{Name: "g", Parameters: []Parameter{{Name: "a", Type: Str}}, ReturnType: bound}
	gotReturnOnly := derefType(returnOnly).(*FunctionDef)
	if gotReturnOnly == returnOnly || gotReturnOnly.ReturnType != Int || gotReturnOnly.Parameters[0].Type != Str {
		t.Fatal("function with only a bound return type must be rebuilt with copied parameters")
	}
	if &gotReturnOnly.Parameters[0] == &returnOnly.Parameters[0] {
		t.Fatal("rebuilt function must not alias the original parameter slice")
	}
}
