package vm

import (
	"testing"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
)

func TestParseTypeNameNestedFunction(t *testing.T) {
	parsed, err := parseTypeName("fn (fn(Int, Int) Int, Str) Bool")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	fn, ok := parsed.(*checker.FunctionDef)
	if !ok {
		t.Fatalf("expected function type, got %T", parsed)
	}
	if len(fn.Parameters) != 2 {
		t.Fatalf("expected 2 params, got %d", len(fn.Parameters))
	}
	inner, ok := fn.Parameters[0].Type.(*checker.FunctionDef)
	if !ok {
		t.Fatalf("expected nested function param, got %T", fn.Parameters[0].Type)
	}
	if len(inner.Parameters) != 2 {
		t.Fatalf("expected nested function to have 2 params, got %d", len(inner.Parameters))
	}
}

func TestStructTypeForHydratesFieldsFromProgramMetadata(t *testing.T) {
	program := bytecode.Program{
		Types: []bytecode.TypeEntry{
			{ID: 1, Name: "Person"},
			{ID: 2, Name: "Int"},
			{ID: 3, Name: "Str"},
		},
		Structs: []bytecode.StructTypeEntry{{
			TypeID: 1,
			Name:   "Person",
			Fields: []bytecode.StructFieldEntry{
				{Name: "age", TypeID: 2},
				{Name: "name", TypeID: 3},
			},
		}},
	}

	resolved, err := New(program).structTypeFor(1)
	if err != nil {
		t.Fatalf("unexpected structTypeFor error: %v", err)
	}
	if resolved.Name != "Person" {
		t.Fatalf("expected struct name Person, got %s", resolved.Name)
	}
	if got := resolved.Fields["age"]; got != checker.Int {
		t.Fatalf("expected age field type Int, got %v", got)
	}
	if got := resolved.Fields["name"]; got != checker.Str {
		t.Fatalf("expected name field type Str, got %v", got)
	}
}
