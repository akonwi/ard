package air

import (
	"strings"
	"testing"
)

func TestValidateRejectsUnrepresentableJSONFieldName(t *testing.T) {
	program := &Program{Types: []TypeInfo{
		{ID: 1, Kind: TypeStr, Name: "Str"},
		{
			ID:   2,
			Kind: TypeStruct,
			Name: "Example",
			Fields: []FieldInfo{{
				Name:  "value",
				Type:  1,
				Index: 0,
				JSON:  JSONFieldInfo{Name: "a,b", HasName: true},
			}},
		},
	}}

	err := Validate(program)
	if err == nil || !strings.Contains(err.Error(), "unrepresentable JSON field name") {
		t.Fatalf("Validate error = %v, want unrepresentable JSON field name", err)
	}
}
