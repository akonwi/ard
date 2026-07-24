package checker

import "testing"

func TestFunctionCopiesPreserveForeignABI(t *testing.T) {
	typeVar := &TypeVar{name: "$T"}
	original := &FunctionDef{
		Name:       "write",
		Parameters: []Parameter{{Name: "values", Type: &List{of: typeVar}, Mutable: true}},
		ReturnType: Void,
		ForeignABI: true,
	}

	substituted, ok := substituteType(original, map[string]Type{"$T": Int}).(*FunctionDef)
	if !ok || !substituted.ForeignABI {
		t.Fatalf("substituteType lost ForeignABI: %#v", substituted)
	}

	replaced, ok := replaceGeneric(original, "$T", Int).(*FunctionDef)
	if !ok || !replaced.ForeignABI {
		t.Fatalf("replaceGeneric lost ForeignABI: %#v", replaced)
	}

	copied := copyFunctionWithTypeVarMap(original, map[string]*TypeVar{"$T": {name: "$T"}})
	if copied == nil || !copied.ForeignABI {
		t.Fatalf("copyFunctionWithTypeVarMap lost ForeignABI: %#v", copied)
	}
}
