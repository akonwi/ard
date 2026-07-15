package checker

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestFunctionTypeTransformationsPreserveParameterMetadata(t *testing.T) {
	span := SourceSpan{FilePath: "api.ard", Location: parse.Location{Start: parse.Point{Row: 2, Col: 10}}}
	parameter := Parameter{
		Name:       "value",
		Type:       &TypeVar{name: "T"},
		Mutable:    true,
		Loc:        span.Location,
		declaredAt: span,
		Variadic:   true,
	}
	fn := &FunctionDef{Name: "consume", GenericParams: []string{"T"}, Parameters: []Parameter{parameter}, ReturnType: Void}

	assertMetadata := func(t *testing.T, got Parameter) {
		t.Helper()
		if got.Name != parameter.Name || got.Mutable != parameter.Mutable || got.Loc != parameter.Loc || got.declaredAt != parameter.declaredAt || got.Variadic != parameter.Variadic {
			t.Fatalf("parameter metadata lost: %#v", got)
		}
	}

	t.Run("replace generic", func(t *testing.T) {
		got := replaceGeneric(fn, "T", Int).(*FunctionDef).Parameters[0]
		assertMetadata(t, got)
	})

	t.Run("substitute type", func(t *testing.T) {
		got := substituteType(fn, map[string]Type{"T": Int}).(*FunctionDef).Parameters[0]
		assertMetadata(t, got)
	})

	t.Run("copy type variables", func(t *testing.T) {
		got := copyFunctionWithTypeVarMap(fn, map[string]*TypeVar{"T": {name: "T"}}).Parameters[0]
		assertMetadata(t, got)
	})
}
