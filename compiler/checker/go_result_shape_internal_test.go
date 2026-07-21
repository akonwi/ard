package checker

import (
	"go/token"
	"go/types"
	"testing"
)

func TestForeignResultShapeZeroValueIsUnknown(t *testing.T) {
	var shape ForeignResultShape
	if shape != ForeignResultUnknown {
		t.Fatalf("zero result shape = %v, want unknown", shape)
	}
}

func TestFunctionDefFromGoSignatureRecordsResultShape(t *testing.T) {
	errorType := types.Universe.Lookup("error").Type()
	intType := types.Typ[types.Int]
	boolType := types.Typ[types.Bool]

	tests := []struct {
		name    string
		results *types.Tuple
		want    ForeignResultShape
	}{
		{name: "direct", results: types.NewTuple(types.NewParam(token.NoPos, nil, "value", intType)), want: ForeignResultDirect},
		{name: "error only", results: types.NewTuple(types.NewParam(token.NoPos, nil, "err", errorType)), want: ForeignResultErrorOnly},
		{name: "value and error", results: types.NewTuple(types.NewParam(token.NoPos, nil, "value", intType), types.NewParam(token.NoPos, nil, "err", errorType)), want: ForeignResultValueError},
		{name: "value and bool", results: types.NewTuple(types.NewParam(token.NoPos, nil, "value", intType), types.NewParam(token.NoPos, nil, "ok", boolType)), want: ForeignResultValueBool},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := types.NewSignatureType(nil, nil, nil, nil, tt.results, false)
			fn, reason := functionDefFromGoSignature("Load", sig)
			if reason != "" {
				t.Fatalf("unexpected unsupported signature: %s", reason)
			}
			if got := fn.ForeignResultShape; got != tt.want {
				t.Fatalf("result shape = %v, want %v", got, tt.want)
			}
		})
	}
}
