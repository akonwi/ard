package checker

import (
	"go/types"
	"testing"
)

func TestConstTypeFromGoRejectsUnsupportedConstantType(t *testing.T) {
	if _, reason := constTypeFromGo(types.Typ[types.UntypedComplex]); reason == "" {
		t.Fatal("expected unsupported untyped complex constant reason")
	}
}
