package vm

import (
	"testing"

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
