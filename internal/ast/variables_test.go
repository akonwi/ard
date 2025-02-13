package ast

import (
	"testing"
)

func TestVariables(t *testing.T) {
	tests := []test{
		{
			name: "Declaring variables",
			input: `
				let name: Str = "Alice"
    		mut age: Int = 30
        mut temp: Float = 98.6
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
						Type:    IntType{},
						Value: NumLiteral{
							Value: "30",
						},
					},
					VariableDeclaration{
						Name:    "temp",
						Mutable: true,
						Type:    FloatType{},
						Value: NumLiteral{
							Value: "98.6",
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
		{
			name:  "Reassigning variables",
			input: `name = "Bob"`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					VariableAssignment{
						Target:   Identifier{Name: "name"},
						Operator: Assign,
						Value: StrLiteral{
							Value: `"Bob"`,
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}
