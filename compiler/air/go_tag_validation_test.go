package air

import (
	"strings"
	"testing"
)

func TestValidateRejectsInvalidGoStructTagMetadata(t *testing.T) {
	tests := []struct {
		name    string
		tags    []GoFieldTag
		message string
	}{
		{"empty key", []GoFieldTag{{Value: "value"}}, `invalid Go struct tag key ""`},
		{"space in key", []GoFieldTag{{Key: "bad key", Value: "value"}}, `invalid Go struct tag key "bad key"`},
		{"hyphen in key", []GoFieldTag{{Key: "bad-key", Value: "value"}}, `invalid Go struct tag key "bad-key"`},
		{"digit-prefixed key", []GoFieldTag{{Key: "9bad", Value: "value"}}, `invalid Go struct tag key "9bad"`},
		{"bare dollar key", []GoFieldTag{{Key: "$", Value: "value"}}, `invalid Go struct tag key "$"`},
		{"dollar-digit key", []GoFieldTag{{Key: "$1", Value: "value"}}, `invalid Go struct tag key "$1"`},
		{"reserved JSON key", []GoFieldTag{{Key: "json", Value: "value"}}, "reserved Go struct tag key json"},
		{"duplicate key", []GoFieldTag{{Key: "yaml", Value: "one"}, {Key: "yaml", Value: "two"}}, `duplicate Go struct tag key "yaml"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := &Program{Types: []TypeInfo{
				{ID: 1, Kind: TypeStr, Name: "Str"},
				{
					ID:   2,
					Kind: TypeStruct,
					Name: "Example",
					Fields: []FieldInfo{{
						Name:   "value",
						Type:   1,
						Index:  0,
						GoTags: tt.tags,
					}},
				},
			}}
			err := Validate(program)
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("Validate error = %v, want %q", err, tt.message)
			}
		})
	}
}
