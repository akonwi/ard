package ast

import (
	"fmt"
	"testing"
)

var colorCode = `
enum Color {
	Red,
	Green,
	Yellow
}`

var colorVariants = []string{
	"Red",
	"Green",
	"Yellow",
}

func TestEnumDefinitions(t *testing.T) {
	tests := []test{
		{
			name:  "Valid basic enum",
			input: colorCode,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					EnumDefinition{
						Name:     "Color",
						Variants: []string{"Red", "Green", "Yellow"},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestEnums(t *testing.T) {
	tests := []test{
		{
			name: "Valid enum variant access",
			input: fmt.Sprintf(`%s
				Color::Red`, colorCode),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					EnumDefinition{
						Name:     "Color",
						Variants: colorVariants,
					},
					StaticProperty{
						Target:   Identifier{Name: "Color"},
						Property: Identifier{Name: "Red"},
					},
				},
			},
		},
		{
			name: "Assigning a variant to a variable",
			input: fmt.Sprintf(`%s
				let favorite: Color = Color::Green`, colorCode),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					EnumDefinition{
						Name:     "Color",
						Variants: colorVariants,
					},
					VariableDeclaration{
						Mutable: false,
						Name:    "favorite",
						Type:    CustomType{Name: "Color"},
						Value: StaticProperty{
							Target:   Identifier{Name: "Color"},
							Property: Identifier{Name: "Green"},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestMatchingOnEnums(t *testing.T) {
	tests := []test{
		{
			name: "Valid matching",
			input: fmt.Sprintf(`%v
				let light = Color::Red
				match light {
					Color::Red => "Stop",
					Color::Yellow => "Yield",
					Color::Green => "Go"
				}`, colorCode),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					EnumDefinition{
						Name:     "Color",
						Variants: colorVariants,
					},
					VariableDeclaration{
						Mutable: false,
						Name:    "light",
						Value: StaticProperty{
							Target:   Identifier{Name: "Color"},
							Property: Identifier{Name: "Red"},
						},
					},
					MatchExpression{
						Subject: Identifier{Name: "light"},
						Cases: []MatchCase{
							{
								Pattern: StaticProperty{
									Target:   Identifier{Name: "Color"},
									Property: Identifier{Name: "Red"},
								},
								Body: []Statement{StrLiteral{Value: `"Stop"`}},
							},
							{
								Pattern: StaticProperty{
									Target:   Identifier{Name: "Color"},
									Property: Identifier{Name: "Yellow"},
								},
								Body: []Statement{StrLiteral{Value: `"Yield"`}},
							},
							{
								Pattern: StaticProperty{
									Target:   Identifier{Name: "Color"},
									Property: Identifier{Name: "Green"},
								},
								Body: []Statement{StrLiteral{Value: `"Go"`}},
							},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}
