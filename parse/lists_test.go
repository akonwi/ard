package parse

import (
	"testing"
)

func TestLists(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Explicitly typed list",
			input: `let strings: [Str] = [1, 2, 3]`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Mutable: false,
						Name:    "strings",
						Type:    &List{Element: &StringType{}},
						Value: &ListLiteral{
							Items: []Expression{
								&NumLiteral{Value: "1"},
								&NumLiteral{Value: "2"},
								&NumLiteral{Value: "3"},
							},
						},
					},
				},
			},
		},
		{
			name: "List with variables",
			input: `
				let numbers: [Int] = [1, 2, 3, four]`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Mutable: false,
						Name:    "numbers",
						Type:    &List{Element: &IntType{}},
						Value: &ListLiteral{
							Items: []Expression{
								&NumLiteral{Value: "1"},
								&NumLiteral{Value: "2"},
								&NumLiteral{Value: "3"},
								&Identifier{Name: "four"},
							},
						},
					},
				},
			},
		},
	})
}
