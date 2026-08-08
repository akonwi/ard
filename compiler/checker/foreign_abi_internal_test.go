package checker

import "testing"

func TestFunctionCopiesPreserveParameterForeignABI(t *testing.T) {
	typeVar := &TypeVar{name: "$T"}
	original := &FunctionDef{
		Name:       "write",
		Parameters: []Parameter{{Name: "values", Type: &List{of: typeVar}, Mutable: true, ForeignABI: ForeignParameterDescriptorValue}},
		ReturnType: Void,
	}

	substituted, ok := substituteType(original, map[string]Type{"$T": Int}).(*FunctionDef)
	if !ok || substituted.Parameters[0].ForeignABI != ForeignParameterDescriptorValue {
		t.Fatalf("substituteType lost parameter ForeignABI: %#v", substituted)
	}

	replaced, ok := replaceGeneric(original, "$T", Int).(*FunctionDef)
	if !ok || replaced.Parameters[0].ForeignABI != ForeignParameterDescriptorValue {
		t.Fatalf("replaceGeneric lost parameter ForeignABI: %#v", replaced)
	}

	copied := copyFunctionWithTypeVarMap(original, map[string]*TypeVar{"$T": {name: "$T"}})
	if copied == nil || copied.Parameters[0].ForeignABI != ForeignParameterDescriptorValue {
		t.Fatalf("copyFunctionWithTypeVarMap lost parameter ForeignABI: %#v", copied)
	}
}
