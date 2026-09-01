package gotarget

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
)

func TestLowerForeignCallRequiresResultShapeForAdaptedReturn(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{
		{ID: 1, Kind: air.TypeInt, Name: "Int"},
		{ID: 2, Kind: air.TypeStr, Name: "Str"},
		{ID: 3, Kind: air.TypeResult, Name: "Result<Int, Str>", Value: 1, Error: 2},
	}}
	lowerer := &lowerer{program: program, runtimeHelpers: map[string]bool{}}
	expr := air.Expr{
		Kind: air.ExprForeignCall,
		Type: 3,
		Payload: &air.ForeignExprPayload{
			Target:    "go",
			Namespace: "example.com/service",
			Qualifier: "service",
			Symbol:    "Load",
		},
	}

	if _, err := lowerer.lowerForeignCall(air.Function{}, expr); err == nil || !strings.Contains(err.Error(), "missing its result shape") {
		t.Fatalf("lowerForeignCall error = %v, want missing result shape", err)
	}

	expr.ForeignPayload().ResultShape = air.ForeignResultValueError
	if _, err := lowerer.lowerForeignCall(air.Function{}, expr); err != nil {
		t.Fatalf("lowerForeignCall with explicit result shape: %v", err)
	}
}
