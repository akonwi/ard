package checker

import "testing"

func TestMaps(t *testing.T) {
	run(t, []test{
		{
			name:  "Valid map instantiation",
			input: `let ages: [Str:Int] = ["ard":0, "go":15] `,
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Mut:  false,
						Name: "ages",
						Value: MapLiteral{
							Entries: map[Expression]Expression{
								StrLiteral{Value: "ard"}: IntLiteral{Value: 0},
								StrLiteral{Value: "go"}:  IntLiteral{Value: 15},
							},
						},
					},
				},
			},
		},
		{
			name:  "Inferring types with initial values",
			input: `let ages = ["ard":0, "go":15]`,
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Mut:  false,
						Name: "ages",
						Value: MapLiteral{
							Entries: map[Expression]Expression{
								StrLiteral{Value: "ard"}: IntLiteral{Value: 0},
								StrLiteral{Value: "go"}:  IntLiteral{Value: 15},
							},
						},
					},
				},
			},
		},
		{
			name:  "Empty maps need an explicit type",
			input: `let empty = [:]`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Empty maps need an explicit type"},
			},
		},
		{
			name:  "Initial entries must match the declared type",
			input: `let ages: [Str:Int] = [1:1, "two":true]`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Type mismatch: Expected Str, got Int"},
				{Kind: Error, Message: "Type mismatch: Expected Int, got Bool"},
			},
		},
		{
			name:  "In order to infer, all entries must have the same type",
			input: `let peeps = ["joe":true, "jack":100]`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Map error: All entries must have the same type"},
			},
		},
	})
}
