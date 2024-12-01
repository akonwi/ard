package ast

import (
	"testing"

	checker "github.com/akonwi/kon/checker"
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
				Statements: []Statement{
					VariableDeclaration{
						Name:    "name",
						Mutable: false,
						Type:    checker.StrType,
						Value: StrLiteral{
							Value: `"Alice"`,
						},
					},
					VariableDeclaration{
						Name:    "age",
						Mutable: true,
						Type:    checker.NumType,
						Value: NumLiteral{
							Value: "30",
						},
					},
					VariableDeclaration{
						Name:    "is_student",
						Mutable: false,
						Type:    checker.BoolType,
						Value: BoolLiteral{
							Value: true,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "empty lists need to be explicitly typed",
			input: `let numbers = []`,
			diagnostics: []checker.Diagnostic{
				{Msg: "Empty lists need a declared type"},
			},
		},
		{
			name:  "List with mixed types",
			input: `let numbers = [1, "two", false]`,
			diagnostics: []checker.Diagnostic{
				{Msg: "List elements must be of the same type"},
			},
		},
		{
			name:  "List elements must match declared type",
			input: `let strings: [Str] = [1, 2, 3]`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "strings",
						Type:    &checker.ListType{ItemType: checker.StrType},
						Value: ListLiteral{
							Type: checker.ListType{ItemType: checker.NumType},
							Items: []Expression{
								NumLiteral{Value: "1"},
								NumLiteral{Value: "2"},
								NumLiteral{Value: "3"},
							},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{Msg: "Type mismatch: expected [Str], got [Num]"},
			},
		},
		{
			name:  "Valid list",
			input: `let numbers: [Num] = [1, 2, 3]`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "numbers",
						Type:    &checker.ListType{ItemType: checker.NumType},
						Value: ListLiteral{
							Type: checker.ListType{ItemType: checker.NumType},
							Items: []Expression{
								NumLiteral{Value: "1"},
								NumLiteral{Value: "2"},
								NumLiteral{Value: "3"},
							},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Valid empty map",
			input: `mut entries: [Str:Num] = [:]`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: true,
						Name:    "entries",
						Type: checker.MapType{
							KeyType:   checker.StrType,
							ValueType: checker.NumType,
						},
						Value: MapLiteral{
							Entries: map[StrLiteral]Expression{},
							Type: checker.MapType{
								KeyType: checker.StrType,
							},
						},
					},
				},
			},
		},
		{
			name:        "Empty maps require explicit type",
			input:       `mut entries = [:]`,
			diagnostics: []checker.Diagnostic{{Msg: "Empty maps need a declared type"}},
		},
		{
			name:  "Valid map",
			input: `mut name_to_counts: [Str:Num] = ["john":1, "jane":2, "jen":3]`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: true,
						Name:    "name_to_counts",
						Type: checker.MapType{
							KeyType:   checker.StrType,
							ValueType: checker.NumType,
						},
						Value: MapLiteral{
							Entries: map[StrLiteral]Expression{
								{Value: `"john"`}: NumLiteral{Value: "1"},
								{Value: `"jane"`}: NumLiteral{Value: "2"},
								{Value: `"jen"`}:  NumLiteral{Value: "3"},
							},
							Type: checker.MapType{
								KeyType:   checker.StrType,
								ValueType: checker.NumType,
							},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
	}

	runTests(t, tests)
}

func TestVariableTypeInference(t *testing.T) {
	tests := []test{
		{
			name:  "Inferred type",
			input: `let name = "Alice"`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "name",
						Type:    checker.StrType,
						Value: StrLiteral{
							Value: `"Alice"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Inferred list",
			input: `let list = ["foo", "bar"]`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Name:    "list",
						Mutable: false,
						Type:    checker.ListType{ItemType: checker.StrType},
						Value: ListLiteral{
							Type: checker.ListType{ItemType: checker.StrType},
							Items: []Expression{
								StrLiteral{Value: `"foo"`},
								StrLiteral{Value: `"bar"`},
							},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Inferred map",
			input: `let map = ["foo":3]`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "map",
						Type:    checker.MapType{KeyType: checker.StrType, ValueType: checker.NumType},
						Value: MapLiteral{
							Entries: map[StrLiteral]Expression{
								{Value: `"foo"`}: NumLiteral{Value: "3"},
							},
							Type: checker.MapType{KeyType: checker.StrType, ValueType: checker.NumType},
						},
					},
				},
			},
		},
		{
			name:  "Str mismatch",
			input: `let name: Str = false`,
			diagnostics: []checker.Diagnostic{
				{
					Msg: "Type mismatch: expected Str, got Bool",
				},
			},
		},
		{
			name:  "Num mismatch",
			input: `let name: Num = "Alice"`,
			diagnostics: []checker.Diagnostic{
				{
					Msg: "Type mismatch: expected Num, got Str",
				},
			},
		},
		{
			name:  "Bool mismatch",
			input: `let is_bool: Bool = "Alice"`,
			diagnostics: []checker.Diagnostic{
				{
					Msg: "Type mismatch: expected Bool, got Str",
				},
			},
		},
	}

	runTests(t, tests)
}

func TestVariableAssignment(t *testing.T) {
	tests := []test{
		{
			name: "Valid Str variable reassignment",
			input: `
				mut name = "Alice"
				name = "Bob"`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: true,
						Name:    "name",
						Type:    checker.StrType,
						Value:   StrLiteral{Value: `"Alice"`},
					},
					VariableAssignment{
						Name:     "name",
						Operator: Assign,
						Value: StrLiteral{
							Value: `"Bob"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Immutable Str variable reassignment",
			input: `
				let name = "Alice"
				name = "Bob"`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "name",
						Type:    checker.StrType,
						Value:   StrLiteral{Value: `"Alice"`},
					},
					VariableAssignment{
						Name:     "name",
						Operator: Assign,
						Value: StrLiteral{
							Value: `"Bob"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "'name' is not mutable",
				},
			},
		},
		{
			name: "Invalid Str variable reassignment",
			input: `
				mut name = "Alice"
				name = 500`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: true,
						Name:    "name",
						Type:    checker.StrType,
						Value:   StrLiteral{Value: `"Alice"`},
					},
					VariableAssignment{
						Name:     "name",
						Operator: Assign,
						Value: NumLiteral{
							Value: `500`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "Expected a 'Str' and received 'Num'",
				},
			},
		},
		{
			name:  "Unknown variable reassignment",
			input: `name = "Bob"`,
			output: Program{
				Statements: []Statement{
					VariableAssignment{
						Name:     "name",
						Operator: Assign,
						Value: StrLiteral{
							Value: `"Bob"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "Undefined: 'name'",
				},
			},
		},
		{
			name: "Valid Num increment assignment",
			input: `
				mut count = 0
				count =+ 2`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: true,
						Name:    "count",
						Type:    checker.NumType,
						Value:   NumLiteral{Value: `0`},
					},
					VariableAssignment{
						Name:     "count",
						Operator: Increment,
						Value: NumLiteral{
							Value: `2`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Cannot increment an immutable variable",
			input: `
				let count = 0
				count =+ 2`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "count",
						Type:    checker.NumType,
						Value:   NumLiteral{Value: `0`},
					},
					VariableAssignment{
						Name:     "count",
						Operator: Increment,
						Value: NumLiteral{
							Value: `2`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "'count' is not mutable",
				},
			},
		},
		{
			name: "Valid decrement",
			input: `
				mut count = 0
				count =- 2`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: true,
						Name:    "count",
						Type:    checker.NumType,
						Value:   NumLiteral{Value: `0`},
					},
					VariableAssignment{
						Name:     "count",
						Operator: Decrement,
						Value: NumLiteral{
							Value: `2`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Invalid decrement",
			input: `
						mut name = "joe"
						name =- 2`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: true,
						Name:    "name",
						Type:    checker.StrType,
						Value:   StrLiteral{Value: `"joe"`},
					},
					VariableAssignment{
						Name:     "name",
						Operator: Decrement,
						Value: NumLiteral{
							Value: `2`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "'=-' can only be used with 'Num'",
				},
			},
		},
		{
			name: "Cannot decrement an immutable variable",
			input: `
				let count = 0
				count =- 2`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "count",
						Type:    checker.NumType,
						Value:   NumLiteral{Value: `0`},
					},
					VariableAssignment{
						Name:     "count",
						Operator: Decrement,
						Value: NumLiteral{
							Value: `2`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "'count' is not mutable",
				},
			},
		},
	}

	runTests(t, tests)
}
