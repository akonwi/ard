package ast

import (
	"testing"
)

func TestCommentsInStructFields(t *testing.T) {
	runTests(t, []test{
		{
			name: "Comments between struct fields",
			input: `
				struct Person {
					name: Str,
					// Comment between fields
					age: Int,
					// Another comment
					employed: Bool
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructDefinition{
						Name: Identifier{Name: "Person"},
						Fields: []StructField{
							{Name: Identifier{Name: "name"}, Type: &StringType{}},
							{Name: Identifier{Name: "age"}, Type: &IntType{}},
							{Name: Identifier{Name: "employed"}, Type: &BooleanType{}},
						},
						Comments: []Comment{
							{Value: "Comment between fields"},
							{Value: "Another comment"},
						},
					},
				},
			},
		},
		{
			name: "Comment after last struct field",
			input: `
				struct Person {
					name: Str,
					age: Int
					// Comment at end
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructDefinition{
						Name: Identifier{Name: "Person"},
						Fields: []StructField{
							{Name: Identifier{Name: "name"}, Type: &StringType{}},
							{Name: Identifier{Name: "age"}, Type: &IntType{}},
						},
						Comments: []Comment{
							{Value: "Comment at end"},
						},
					},
				},
			},
		},
	})
}

func TestCommentsInEnumVariants(t *testing.T) {
	runTests(t, []test{
		{
			name: "Comments between enum variants",
			input: `
				enum Color {
					Red,
					// Comment between variants
					Green,
					Blue
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&EnumDefinition{
						Name: "Color",
						Variants: []string{"Red", "Green", "Blue"},
						Comments: []Comment{
							{Value: "Comment between variants"},
						},
					},
				},
			},
		},
	})
}
