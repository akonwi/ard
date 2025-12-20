package parse

import (
	"testing"
)

func TestComments(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Single line comment",
			input: "// this is a comment",
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&Comment{Value: "// this is a comment"},
				},
			},
		},
		{
			name:  "Inline comment",
			input: "let x = 200 // this is a comment",
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name:  "x",
						Value: &NumLiteral{Value: "200"},
					},
					&Comment{Value: "// this is a comment"},
				},
			},
		},
	})
}

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
						Name:     "Color",
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

func TestCommentsInFunctionParameters(t *testing.T) {
	runTests(t, []test{
		{
			name: "Comments between function parameters",
			input: `fn add(x: Int,
					// Comment between parameters
					y: Int) Int {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "add",
						Parameters: []Parameter{
							{Name: "x", Type: &IntType{}},
							{Name: "y", Type: &IntType{}},
						},
						ReturnType: &IntType{},
						Comments: []Comment{
							{Value: "Comment between parameters"},
						},
						Body: []Statement{},
					},
				},
			},
		},
	})
}

func TestCommentsInStructLiterals(t *testing.T) {
	runTests(t, []test{
		{
			name: "Comments between struct literal properties",
			input: `Person{
					name: "Alice",
					// Comment between properties
					age: 30,
					employed: true
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructInstance{
						Name: Identifier{Name: "Person"},
						Properties: []StructValue{
							{Name: Identifier{Name: "name"}, Value: &StrLiteral{Value: "Alice"}},
							{Name: Identifier{Name: "age"}, Value: &NumLiteral{Value: "30"}},
							{Name: Identifier{Name: "employed"}, Value: &BoolLiteral{Value: true}},
						},
						Comments: []Comment{
							{Value: "Comment between properties"},
						},
					},
				},
			},
		},
	})
}

func TestInlineCommentsInStructLiterals(t *testing.T) {
	runTests(t, []test{
		{
			name: "Inline comments after struct literal properties",
			input: `Person{
					name: "Alice",     // Person name
					age: 30,           // Age in years
					employed: true     // Job status
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructInstance{
						Name: Identifier{Name: "Person"},
						Properties: []StructValue{
							{Name: Identifier{Name: "name"}, Value: &StrLiteral{Value: "Alice"}},
							{Name: Identifier{Name: "age"}, Value: &NumLiteral{Value: "30"}},
							{Name: Identifier{Name: "employed"}, Value: &BoolLiteral{Value: true}},
						},
						Comments: []Comment{
							{Value: "Person name"},
							{Value: "Age in years"},
							{Value: "Job status"},
						},
					},
				},
			},
		},
	})
}
