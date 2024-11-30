package ast

import (
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

func testUsingStructs(t *testing.T) {
	emptyStruct := checker.StructType{
		Name:   "Box",
		Fields: map[string]checker.Type{},
	}
	// personStruct := checker.StructType{
	// 	Name: "Person",
	// 	Fields: map[string]checker.Type{
	// 		"name": checker.StrType,
	// 		"age":  checker.NumType,
	// 	},
	// }
	tests := []test{
		{
			name: "Instantiating a field-less struct",
			input: `
				enum Box {}
				Box::new()`,
			output: &Program{
				Statements: []Statement{
					&StructDefinition{
						Type: emptyStruct,
					},
					&MemberAccess{
						Target:     Identifier{Name: "Box", Type: emptyStruct},
						AccessType: Static,
						Member:     &Identifier{Name: "new", Type: checker.FunctionType{}},
					},
				},
			},
		},
	}

	runTests(t, tests)
}
