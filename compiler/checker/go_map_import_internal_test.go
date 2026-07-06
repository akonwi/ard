package checker

import (
	"go/token"
	"go/types"
	"testing"
)

func TestFunctionDefFromGoSignatureMarksMapParametersMutable(t *testing.T) {
	goMap := types.NewMap(types.Typ[types.String], types.Typ[types.Int])
	sig := types.NewSignatureType(nil, nil, nil, types.NewTuple(types.NewParam(token.NoPos, nil, "m", goMap)), nil, false)

	fn, reason := functionDefFromGoSignature("Use", sig)
	if reason != "" {
		t.Fatalf("unexpected unsupported signature: %s", reason)
	}
	if len(fn.Parameters) != 1 {
		t.Fatalf("got %d params, want 1", len(fn.Parameters))
	}
	if !fn.Parameters[0].Mutable {
		t.Fatal("Go map parameter was not marked mutable")
	}
	m, ok := fn.Parameters[0].Type.(*Map)
	if !ok {
		t.Fatalf("param type = %T, want *Map", fn.Parameters[0].Type)
	}
	if !equalTypes(m.Key(), Str) || !equalTypes(m.Value(), Int) {
		t.Fatalf("param type = %s, want [Str:Int]", m)
	}
}

func TestFunctionDefFromGoSignatureMarksNamedMapParametersMutable(t *testing.T) {
	goMap := types.NewMap(types.Typ[types.String], types.Typ[types.Int])
	namedMap := types.NewNamed(types.NewTypeName(token.NoPos, nil, "Headers", nil), goMap, nil)
	sig := types.NewSignatureType(nil, nil, nil, types.NewTuple(types.NewParam(token.NoPos, nil, "m", namedMap)), nil, false)

	fn, reason := functionDefFromGoSignature("Use", sig)
	if reason != "" {
		t.Fatalf("unexpected unsupported signature: %s", reason)
	}
	if len(fn.Parameters) != 1 {
		t.Fatalf("got %d params, want 1", len(fn.Parameters))
	}
	if !fn.Parameters[0].Mutable {
		t.Fatal("named Go map parameter was not marked mutable")
	}
	foreign, ok := fn.Parameters[0].Type.(*ForeignType)
	if !ok {
		t.Fatalf("param type = %T, want *ForeignType", fn.Parameters[0].Type)
	}
	if foreign.MapKey == nil || foreign.MapValue == nil {
		t.Fatalf("named map foreign type missing map key/value: %#v", foreign)
	}
}

func TestPointerToNamedGoMapDoesNotExposeMapMethods(t *testing.T) {
	goMap := types.NewMap(types.Typ[types.String], types.Typ[types.Int])
	namedMap := types.NewNamed(types.NewTypeName(token.NoPos, nil, "Headers", nil), goMap, nil)

	typ, reason := typeFromGo(types.NewPointer(namedMap))
	if reason != "" {
		t.Fatalf("unexpected unsupported pointer map: %s", reason)
	}
	foreign, ok := typ.(*ForeignType)
	if !ok {
		t.Fatalf("type = %T, want *ForeignType", typ)
	}
	if !foreign.Pointer {
		t.Fatal("expected pointer foreign type")
	}
	if foreign.MapKey != nil || foreign.MapValue != nil {
		t.Fatalf("pointer foreign map exposed map methods: key=%v value=%v", foreign.MapKey, foreign.MapValue)
	}
}

func TestMapSetReturnsVoid(t *testing.T) {
	m := MakeMap(Str, Int)
	set, ok := m.get("set").(*FunctionDef)
	if !ok {
		t.Fatalf("map set method = %T, want *FunctionDef", m.get("set"))
	}
	if set.ReturnType != Void {
		t.Fatalf("map set return = %s, want Void", set.ReturnType)
	}
	remove, ok := m.get("remove").(*FunctionDef)
	if !ok {
		t.Fatalf("map remove method = %T, want *FunctionDef", m.get("remove"))
	}
	if remove.ReturnType != Void || !remove.Mutates {
		t.Fatalf("map remove = return %s mutates %v, want Void mutating", remove.ReturnType, remove.Mutates)
	}
}

func TestTypeFromGoMapsUnnamedMapsToArdMaps(t *testing.T) {
	goMap := types.NewMap(types.Typ[types.String], types.NewSlice(types.Typ[types.Int]))

	typ, reason := typeFromGo(goMap)
	if reason != "" {
		t.Fatalf("unexpected unsupported map: %s", reason)
	}
	m, ok := typ.(*Map)
	if !ok {
		t.Fatalf("type = %T, want *Map", typ)
	}
	if !equalTypes(m.Key(), Str) {
		t.Fatalf("key type = %s, want Str", m.Key())
	}
	if _, ok := m.Value().(*List); !ok {
		t.Fatalf("value type = %T, want *List", m.Value())
	}
}
