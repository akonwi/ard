package ast

import (
	"fmt"
	"testing"

	checker "github.com/akonwi/kon/checker"
)

func TestStructDefinitions(t *testing.T) {
	emptyStruct := checker.StructType{
		Name:   "Box",
		Fields: map[string]checker.Type{},
	}
	personStruct := checker.StructType{
		Name: "Person",
		Fields: map[string]checker.Type{
			"name":     checker.StrType,
			"age":      checker.NumType,
			"employed": checker.BoolType,
		},
	}

	tests := []test{
		{
			name: "An empty struct",
			input: `
				struct Box {}`,
			output: &Program{
				Statements: []Statement{
					&StructDefinition{
						Type: emptyStruct,
					},
				},
			},
		},
		{
			name: "A valid struct",
			input: `
				struct Person {
					name: Str,
					age: Num,
					employed: Bool
				}`,
			output: &Program{
				Statements: []Statement{
					&StructDefinition{
						Type: personStruct,
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestInstantiatingStructs(t *testing.T) {
	emptyStruct := checker.StructType{
		Name:   "Box",
		Fields: map[string]checker.Type{},
	}

	personStructCode := `
		struct Person {
			name: Str,
			age: Num,
			employed: Bool
		}`
	personStruct := checker.StructType{
		Name: "Person",
		Fields: map[string]checker.Type{
			"name":     checker.StrType,
			"age":      checker.NumType,
			"employed": checker.BoolType,
		},
	}
	tests := []test{
		{
			name: "Instantiating a field-less struct",
			input: `
				struct Box {}
				Box{}`,
			output: &Program{
				Statements: []Statement{
					&StructDefinition{
						Type: emptyStruct,
					},
					StructInstance{
						Type:       emptyStruct,
						Properties: map[string]Expression{},
					},
				},
			},
		},
		{
			name: "Instantiating with field errors",
			input: fmt.Sprintf(`%s
				Person { name: 23, employed: true, size: "xl"  }
			`, personStructCode),
			diagnostics: []checker.Diagnostic{
				{Msg: "Type mismatch: expected Str, got Num"},
				{Msg: "'size' is not a field of 'Person'"},
				{Msg: "Missing field 'age' in struct 'Person'"},
			},
		},
		{
			name: "Correctly instantiating a struct with fields",
			input: fmt.Sprintf(`%s
				Person { name: "John", age: 23, employed: true }
			`, personStructCode),
			output: &Program{
				Statements: []Statement{
					&StructDefinition{
						Type: personStruct,
					},
					StructInstance{
						Type: personStruct,
						Properties: map[string]Expression{
							"name":     &StrLiteral{Value: `"John"`},
							"age":      &NumLiteral{Value: "23"},
							"employed": &BoolLiteral{Value: true},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestStructFieldAccess(t *testing.T) {
	personStructCode := `
		struct Person {
			name: Str,
			age: Num,
			employed: Bool
		}`
	personStruct := checker.StructType{
		Name: "Person",
		Fields: map[string]checker.Type{
			"name":     checker.StrType,
			"age":      checker.NumType,
			"employed": checker.BoolType,
		},
	}
	tests := []test{
		{
			name: "Valid field access",
			input: fmt.Sprintf(`%s
				let person = Person { name: "Bobby", age: 12, employed: false }
				person.name
				person.age
				person.employed`, personStructCode),
			output: &Program{
				Statements: []Statement{
					&StructDefinition{Type: personStruct},
					&VariableDeclaration{
						Mutable: false,
						Name:    "person",
						Type:    personStruct,
						Value: StructInstance{
							Type: personStruct,
							Properties: map[string]Expression{
								"name":     &StrLiteral{Value: `"Bobby"`},
								"age":      &NumLiteral{Value: "12"},
								"employed": &BoolLiteral{Value: false},
							},
						},
					},
					MemberAccess{
						Target:     Identifier{Name: "person", Type: personStruct},
						AccessType: Instance,
						Member:     Identifier{Name: "name", Type: checker.StrType},
					},
					MemberAccess{
						Target:     Identifier{Name: "person", Type: personStruct},
						AccessType: Instance,
						Member:     Identifier{Name: "age", Type: checker.NumType},
					},
					MemberAccess{
						Target:     Identifier{Name: "person", Type: personStruct},
						AccessType: Instance,
						Member:     Identifier{Name: "employed", Type: checker.BoolType},
					},
				},
			},
		},
		{
			name: "Accessing non-existent fields",
			input: fmt.Sprintf(`%s
				let person = Person { name: "Bobby", age: 12, employed: false }
				person.foobar`, personStructCode),
			diagnostics: []checker.Diagnostic{
				{Msg: "No field 'foobar' in 'Person' struct"},
			},
		},
	}

	runTests(t, tests)
}
