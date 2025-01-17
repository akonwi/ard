package ast

import (
	"testing"
)

func TestVariableDeclarations(t *testing.T) {
	tests := []test{
		{
			name: "Valid variables",
			input: `
				let name: Str = "Alice"
    		mut age: Num = 30
      	let is_student: Bool = true`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					VariableDeclaration{
						Name:    "name",
						Mutable: false,
						Type:    StringType{},
						Value: StrLiteral{
							Value: `"Alice"`,
						},
					},
					VariableDeclaration{
						Name:    "age",
						Mutable: true,
						Type:    NumberType{},
						Value: NumLiteral{
							Value: "30",
						},
					},
					VariableDeclaration{
						Name:    "is_student",
						Mutable: false,
						Type:    BooleanType{},
						Value: BoolLiteral{
							Value: true,
						},
					},
				},
			},
		},
		// {
		// 	name:  "Valid empty map",
		// 	input: `mut entries: [Str:Num] = [:]`,
		// 	output: Program{
		// 		Imports: []Import{},
		// 		Statements: []Statement{
		// 			VariableDeclaration{
		// 				Mutable: true,
		// 				Name:    "entries",
		// 				Type: Map{
		// 					Key:   StringType{},
		// 					Value: NumberType{},
		// 				},
		// 				Value: MapLiteral{
		// 					Entries: []MapEntry{},
		// 					Type: MapType{
		// 						KeyType: StrType,
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// },
		// {
		// 	name:  "Valid map",
		// 	input: `mut name_to_counts: [Str:Num] = ["john":1, "jane":2, "jen":3]`,
		// 	output: Program{
		// 		Imports: []Import{},
		// 		Statements: []Statement{
		// 			VariableDeclaration{
		// 				Mutable: true,
		// 				Name:    "name_to_counts",
		// 				Type: Map{
		// 					Key:   StringType{},
		// 					Value: NumberType{},
		// 				},
		// 				Value: MapLiteral{
		// 					Entries: []MapEntry{
		// 						{Key: `"john"`, Value: NumLiteral{Value: "1"}},
		// 						{Key: `"jane"`, Value: NumLiteral{Value: "2"}},
		// 						{Key: `"jen"`, Value: NumLiteral{Value: "3"}},
		// 					},
		// 					Type: MapType{
		// 						KeyType:   StrType,
		// 						ValueType: NumType,
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// },
	}

	runTests(t, tests)
}

// func TestVariableTypeInference(t *testing.T) {
// 	tests := []test{
// 		{
// 			name:  "Inferred type",
// 			input: `let name = "Alice"`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableDeclaration{
// 						Mutable: false,
// 						Name:    "name",
// 						Type:    StringType{},
// 						Value: StrLiteral{
// 							Value: `"Alice"`,
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name:  "Inferred list",
// 			input: `let list = ["foo", "bar"]`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableDeclaration{
// 						Name:    "list",
// 						Mutable: false,
// 						Type:    List{Element: StringType{}},
// 						Value: ListLiteral{
// 							Type: ListType{ItemType: StrType},
// 							Items: []Expression{
// 								StrLiteral{Value: `"foo"`},
// 								StrLiteral{Value: `"bar"`},
// 							},
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name:  "Inferred map",
// 			input: `let map = ["foo":3]`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableDeclaration{
// 						Mutable: false,
// 						Name:    "map",
// 						Type:    Map{Key: StringType{}, Value: NumberType{}},
// 						Value: MapLiteral{
// 							Entries: []MapEntry{
// 								{Key: `"foo"`, Value: NumLiteral{Value: "3"}},
// 							},
// 							Type: MapType{KeyType: StrType, ValueType: NumType},
// 						},
// 					},
// 				},
// 			},
// 		},
// 		// {
// 		// 	name:  "Str mismatch",
// 		// 	input: `let name: Str = false`,
// 		// },
// 		// {
// 		// 	name:  "Num mismatch",
// 		// 	input: `let name: Num = "Alice"`,
// 		// },
// 		// {
// 		// 	name:  "Bool mismatch",
// 		// 	input: `let is_bool: Bool = "Alice"`,
// 		// },
// 	}

// 	runTests(t, tests)
// }

// func TestVariableAssignment(t *testing.T) {
// 	tests := []test{
// 		{
// 			name: "Valid Str variable reassignment",
// 			input: `
// 				mut name = "Alice"
// 				name = "Bob"`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableDeclaration{
// 						Mutable: true,
// 						Name:    "name",
// 						Value:   StrLiteral{Value: `"Alice"`},
// 					},
// 					VariableAssignment{
// 						Name:     "name",
// 						Operator: Assign,
// 						Value: StrLiteral{
// 							Value: `"Bob"`,
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Immutable Str variable reassignment",
// 			input: `
// 				let name = "Alice"
// 				name = "Bob"`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableDeclaration{
// 						Mutable: false,
// 						Name:    "name",
// 						Type:    StringType{},
// 						Value:   StrLiteral{Value: `"Alice"`},
// 					},
// 					VariableAssignment{
// 						Name:     "name",
// 						Operator: Assign,
// 						Value: StrLiteral{
// 							Value: `"Bob"`,
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Invalid Str variable reassignment",
// 			input: `
// 				mut name = "Alice"
// 				name = 500`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableDeclaration{
// 						Mutable: true,
// 						Name:    "name",
// 						Type:    StringType{},
// 						Value:   StrLiteral{Value: `"Alice"`},
// 					},
// 					VariableAssignment{
// 						Name:     "name",
// 						Operator: Assign,
// 						Value: NumLiteral{
// 							Value: `500`,
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name:  "Unknown variable reassignment",
// 			input: `name = "Bob"`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableAssignment{
// 						Name:     "name",
// 						Operator: Assign,
// 						Value: StrLiteral{
// 							Value: `"Bob"`,
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Valid Num increment assignment",
// 			input: `
// 				mut count = 0
// 				count =+ 2`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableDeclaration{
// 						Mutable: true,
// 						Name:    "count",
// 						Type:    NumberType{},
// 						Value:   NumLiteral{Value: `0`},
// 					},
// 					VariableAssignment{
// 						Name:     "count",
// 						Operator: Increment,
// 						Value: NumLiteral{
// 							Value: `2`,
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Cannot increment an immutable variable",
// 			input: `
// 				let count = 0
// 				count =+ 2`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableDeclaration{
// 						Mutable: false,
// 						Name:    "count",
// 						Type:    NumberType{},
// 						Value:   NumLiteral{Value: `0`},
// 					},
// 					VariableAssignment{
// 						Name:     "count",
// 						Operator: Increment,
// 						Value: NumLiteral{
// 							Value: `2`,
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Valid decrement",
// 			input: `
// 				mut count = 0
// 				count =- 2`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableDeclaration{
// 						Mutable: true,
// 						Name:    "count",
// 						Type:    NumberType{},
// 						Value:   NumLiteral{Value: `0`},
// 					},
// 					VariableAssignment{
// 						Name:     "count",
// 						Operator: Decrement,
// 						Value: NumLiteral{
// 							Value: `2`,
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Invalid decrement",
// 			input: `
// 						mut name = "joe"
// 						name =- 2`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableDeclaration{
// 						Mutable: true,
// 						Name:    "name",
// 						Type:    StringType{},
// 						Value:   StrLiteral{Value: `"joe"`},
// 					},
// 					VariableAssignment{
// 						Name:     "name",
// 						Operator: Decrement,
// 						Value: NumLiteral{
// 							Value: `2`,
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Cannot decrement an immutable variable",
// 			input: `
// 				let count = 0
// 				count =- 2`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					VariableDeclaration{
// 						Mutable: false,
// 						Name:    "count",
// 						Type:    NumberType{},
// 						Value:   NumLiteral{Value: `0`},
// 					},
// 					VariableAssignment{
// 						Name:     "count",
// 						Operator: Decrement,
// 						Value: NumLiteral{
// 							Value: `2`,
// 						},
// 					},
// 				},
// 			},
// 		},
// 	}

// 	runTests(t, tests)
// }
