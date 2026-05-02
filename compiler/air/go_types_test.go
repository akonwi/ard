package air

import (
	"strings"
	"testing"
)

func TestGenerateGoStructDeclarations(t *testing.T) {
	program := lowerSource(t, `
		struct User {
			id: Int,
			name: Str,
			email: Str?,
			scores: [Int],
			attrs: [Str:Str],
			status: Int!Str,
		}
	`)

	gotBytes, err := GenerateGoStructDeclarations(program, GoTypeOptions{
		RuntimeQualifier: "ardrt",
	})
	if err != nil {
		t.Fatalf("GenerateGoStructDeclarations error: %v", err)
	}

	want := `type User struct {
	Attrs  map[string]string
	Email  ardrt.Maybe[string]
	Id     int
	Name   string
	Scores []int
	Status ardrt.Result[int, string]
}
`
	if got := string(gotBytes); got != want {
		t.Fatalf("generated Go structs:\n%s\nwant:\n%s", got, want)
	}
}

func TestGenerateGoStructDeclarationsRejectsInvalidFieldType(t *testing.T) {
	program := &Program{
		Types: []TypeInfo{{
			ID:   1,
			Kind: TypeStruct,
			Name: "Bad",
			Fields: []FieldInfo{{
				Name:  "missing",
				Type:  99,
				Index: 0,
			}},
		}},
	}

	_, err := GenerateGoStructDeclarations(program, GoTypeOptions{})
	if err == nil {
		t.Fatalf("GenerateGoStructDeclarations succeeded, want invalid type error")
	}
	if !strings.Contains(err.Error(), "invalid type id 99") {
		t.Fatalf("error = %q, want invalid type id", err)
	}
}
