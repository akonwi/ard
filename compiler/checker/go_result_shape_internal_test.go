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
	stringType := types.Typ[types.String]

	tests := []struct {
		name       string
		results    *types.Tuple
		want       ForeignResultShape
		wantReturn string
	}{
		{name: "direct", results: types.NewTuple(types.NewParam(token.NoPos, nil, "value", intType)), want: ForeignResultDirect, wantReturn: "Int"},
		{name: "error only", results: types.NewTuple(types.NewParam(token.NoPos, nil, "err", errorType)), want: ForeignResultErrorOnly, wantReturn: "Void!Error"},
		{name: "value and error", results: types.NewTuple(types.NewParam(token.NoPos, nil, "value", intType), types.NewParam(token.NoPos, nil, "err", errorType)), want: ForeignResultValueError, wantReturn: "Int!Error"},
		{name: "value and bool", results: types.NewTuple(types.NewParam(token.NoPos, nil, "value", intType), types.NewParam(token.NoPos, nil, "ok", boolType)), want: ForeignResultValueBool, wantReturn: "Int?"},
		{name: "two values", results: types.NewTuple(types.NewParam(token.NoPos, nil, "quo", intType), types.NewParam(token.NoPos, nil, "rem", stringType)), want: ForeignResultTuple, wantReturn: "(Int, Str)"},
		{name: "error first is not the error convention", results: types.NewTuple(types.NewParam(token.NoPos, nil, "err", errorType), types.NewParam(token.NoPos, nil, "value", intType)), want: ForeignResultTuple, wantReturn: "(Error, Int)"},
		{name: "bool first is not comma-ok", results: types.NewTuple(types.NewParam(token.NoPos, nil, "ok", boolType), types.NewParam(token.NoPos, nil, "value", stringType)), want: ForeignResultTuple, wantReturn: "(Bool, Str)"},
		{name: "values and error", results: types.NewTuple(types.NewParam(token.NoPos, nil, "r", intType), types.NewParam(token.NoPos, nil, "size", stringType), types.NewParam(token.NoPos, nil, "err", errorType)), want: ForeignResultTupleError, wantReturn: "(Int, Str)!Error"},
		{name: "values and trailing bool stay a plain tuple", results: types.NewTuple(types.NewParam(token.NoPos, nil, "before", stringType), types.NewParam(token.NoPos, nil, "after", stringType), types.NewParam(token.NoPos, nil, "found", boolType)), want: ForeignResultTuple, wantReturn: "(Str, Str, Bool)"},
		{name: "bool before the error is an element", results: types.NewTuple(types.NewParam(token.NoPos, nil, "line", stringType), types.NewParam(token.NoPos, nil, "isPrefix", boolType), types.NewParam(token.NoPos, nil, "err", errorType)), want: ForeignResultTupleError, wantReturn: "(Str, Bool)!Error"},
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
			if got := fn.ReturnType.String(); got != tt.wantReturn {
				t.Fatalf("return type = %s, want %s", got, tt.wantReturn)
			}
		})
	}
}
