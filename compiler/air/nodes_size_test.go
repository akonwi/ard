package air

import (
	"testing"
	"unsafe"
)

func TestExprSizeBudget(t *testing.T) {
	const maxExprBytes = 64
	if got := unsafe.Sizeof(Expr{}); got > maxExprBytes {
		t.Fatalf("Expr size = %d bytes, want at most %d", got, maxExprBytes)
	}
}

func TestExprPayloadShapeValidation(t *testing.T) {
	tests := []struct {
		name    string
		expr    Expr
		wantErr bool
	}{
		{name: "valid text", expr: Expr{Kind: ExprConstInt, Payload: &TextExprPayload{Value: "1"}}},
		{name: "incompatible", expr: Expr{Kind: ExprLoadLocal, Payload: &TextExprPayload{Value: "1"}}, wantErr: true},
		{name: "missing required", expr: Expr{Kind: ExprIntAdd}, wantErr: true},
		{name: "optional try catch", expr: Expr{Kind: ExprTryResult}},
		{name: "optional unary", expr: Expr{Kind: ExprToStr}},
		{name: "unknown kind", expr: Expr{Kind: ExprKind(255)}, wantErr: true},
		{name: "typed nil", expr: Expr{Kind: ExprCall, Payload: (*CallExprPayload)(nil)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExprPayload(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateExprPayload() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAggregateElementSizeBudgets(t *testing.T) {
	if got, want := unsafe.Sizeof(StructFieldValue{}), uintptr(88); got > want {
		t.Fatalf("StructFieldValue size = %d bytes, want at most %d", got, want)
	}
	if got, want := unsafe.Sizeof(MapEntry{}), uintptr(128); got > want {
		t.Fatalf("MapEntry size = %d bytes, want at most %d", got, want)
	}
}
