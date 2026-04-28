package ardgo

import "testing"

func TestLazyJSONRejectsInvalidSyntax(t *testing.T) {
	tests := []string{
		`{"a":1,}`,
		`[1 2]`,
		`{"a":01}`,
		`{"a":"\x"}`,
		"{\"a\":\"\n\"}",
		`{"a":tru}`,
	}
	for _, input := range tests {
		if result := JsonToDynamicExtern(input); result.ok {
			t.Fatalf("JsonToDynamicExtern(%s) unexpectedly succeeded", input)
		}
	}
}

func TestLazyJSONRejectsDuplicateNames(t *testing.T) {
	tests := []string{
		`{"a":1,"a":2}`,
		`{"outer":{"a":1,"a":2}}`,
		`{"a":1,"\u0061":2}`,
	}
	for _, input := range tests {
		if result := JsonToDynamicExtern(input); result.ok {
			t.Fatalf("JsonToDynamicExtern(%s) unexpectedly succeeded", input)
		}
	}
}

func TestLazyJSONDecodesRequestedFields(t *testing.T) {
	rawResult := JsonToDynamicExtern(`{"units":[1,2,3],"counts":{"a":4,"b":5}}`)
	if !rawResult.ok {
		t.Fatalf("JsonToDynamicExtern failed: %s", rawResult.err)
	}
	switch rawResult.value.(type) {
	case jsonDynamic, *jsonObjectDynamic:
	default:
		t.Fatalf("expected lazy JSON dynamic, got %T", rawResult.value)
	}

	unitsField := ExtractFieldExtern(rawResult.value, "units")
	if !unitsField.ok {
		t.Fatalf("ExtractFieldExtern failed: %s", unitsField.err)
	}
	units := DecodeIntListErrorsExtern[builtinDecodeError](unitsField.value)
	if !units.ok || len(units.value) != 3 || units.value[0] != 1 || units.value[2] != 3 {
		t.Fatalf("unexpected units decode: %#v", units)
	}

	countsField := ExtractFieldExtern(rawResult.value, "counts")
	if !countsField.ok {
		t.Fatalf("ExtractFieldExtern failed: %s", countsField.err)
	}
	counts := DecodeStringIntMapErrorsExtern[builtinDecodeError](countsField.value)
	if !counts.ok || len(counts.value) != 2 || counts.value["a"] != 4 || counts.value["b"] != 5 {
		t.Fatalf("unexpected counts decode: %#v", counts)
	}
}

func TestLazyJSONFallsBackForGenericDynamicValue(t *testing.T) {
	rawResult := JsonToDynamicExtern(`{"items":[{"name":"a"}]}`)
	if !rawResult.ok {
		t.Fatalf("JsonToDynamicExtern failed: %s", rawResult.err)
	}
	parsed := builtinDynamicValue(rawResult.value)
	if _, ok := parsed.(map[string]any); !ok {
		t.Fatalf("expected fallback parse to map[string]any, got %T", parsed)
	}
}
