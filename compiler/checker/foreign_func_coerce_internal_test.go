package checker

import "testing"

// foreignFuncCoerces must flag compatibility that relies on the named/unnamed
// Go func coercion in either direction, so Go interface conformance can
// exclude it (generated method signatures must match the interface exactly).
func TestForeignFuncCoerces(t *testing.T) {
	signature := &FunctionDef{Name: "CancelFunc", Parameters: []Parameter{}, ReturnType: Void}
	named := &ForeignType{Target: "go", Namespace: "context", Qualifier: "context", Name: "CancelFunc", Underlying: signature}
	plain := &FunctionDef{Name: "<function>", Parameters: []Parameter{}, ReturnType: Void}

	if !foreignFuncCoerces(named, plain) {
		t.Fatal("expected unnamed-to-named func coercion to be flagged")
	}
	if !foreignFuncCoerces(plain, named) {
		t.Fatal("expected named-to-unnamed func coercion to be flagged")
	}
	if foreignFuncCoerces(plain, plain) {
		t.Fatal("plain function types do not coerce")
	}
	scalar := &ForeignType{Target: "go", Namespace: "time", Qualifier: "time", Name: "Month", Underlying: Int}
	if foreignFuncCoerces(plain, scalar) || foreignFuncCoerces(scalar, plain) {
		t.Fatal("non-func foreign types do not func-coerce")
	}
	pointer := &ForeignType{Target: "go", Namespace: "context", Qualifier: "context", Name: "CancelFunc", Underlying: signature, Pointer: true}
	if foreignFuncCoerces(pointer, plain) {
		t.Fatal("pointer foreign types do not func-coerce")
	}
}
