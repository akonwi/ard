package ast

import (
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
	runTests(t, []test{
		{
			name:  "Valid basic enum",
			input: colorCode,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&EnumDefinition{
						Name:     "Color",
						Variants: []string{"Red", "Green", "Yellow"},
					},
				},
			},
		},
	})
}

func TestMatchingOnEnums(t *testing.T) {
	runTests(t, []test{
		{
			name: "Valid matching",
			input: `
					match light {
						Color::Red => "Stop",
						Color::Yellow => "Yield",
						Color::Green => { "Go" },
					}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&MatchExpression{
						Subject: &Identifier{Name: "light"},
						Cases: []MatchCase{
							{
								Pattern: &StaticProperty{
									Target:   &Identifier{Name: "Color"},
									Property: Identifier{Name: "Red"},
								},
								Body: []Statement{&StrLiteral{Value: "Stop"}},
							},
							{
								Pattern: &StaticProperty{
									Target:   &Identifier{Name: "Color"},
									Property: Identifier{Name: "Yellow"},
								},
								Body: []Statement{&StrLiteral{Value: "Yield"}},
							},
							{
								Pattern: &StaticProperty{
									Target:   &Identifier{Name: "Color"},
									Property: Identifier{Name: "Green"},
								},
								Body: []Statement{&StrLiteral{Value: "Go"}},
							},
						},
					},
				},
			},
		},
	})
}
