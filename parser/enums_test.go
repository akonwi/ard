package parser

import (
	"testing"
)

var colorCode = `enum Color {
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
		{
			name:  "Private enum",
			input: `private ` + colorCode,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&EnumDefinition{
						Name:     "Color",
						Variants: []string{"Red", "Green", "Yellow"},
						Private:  true,
					},
				},
			},
		},
		// Error cases
		{
			name:     "Missing enum name",
			input:    "enum { A, B }",
			wantErrs: []string{"Expected name after 'enum'"},
		},
		{
			name:     "Missing opening brace",
			input:    "enum Color A, B }",
			wantErrs: []string{"Expected '{'"},
		},
		{
			name:     "Empty first variant",
			input:    "enum Color { , B }",
			wantErrs: []string{}, // Should gracefully skip empty variants
		},
		{
			name:     "Empty middle variant",
			input:    "enum Color { A, , C }",
			wantErrs: []string{}, // Should gracefully skip empty variants
		},
		{
			name:     "Empty enum",
			input:    "enum Color { }",
			wantErrs: []string{}, // Should work fine
		},
		{
			name:     "Trailing comma works",
			input:    "enum Color { A, B, }",
			wantErrs: []string{}, // Should work fine
		},
	})
}

func TestMatchingOnEnums(t *testing.T) {
	runTests(t, []test{
		{
			name: "Valid matching",
			input: `
					match light {
						Color::Yellow => "Yield",
						Color::Green => { "Go" },
						_ => "Stop",
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
									Property: &Identifier{Name: "Yellow"},
								},
								Body: []Statement{&StrLiteral{Value: "Yield"}},
							},
							{
								Pattern: &StaticProperty{
									Target:   &Identifier{Name: "Color"},
									Property: &Identifier{Name: "Green"},
								},
								Body: []Statement{&StrLiteral{Value: "Go"}},
							},
							{
								Pattern: &Identifier{Name: "_"},
								Body:    []Statement{&StrLiteral{Value: "Stop"}},
							},
						},
					},
				},
			},
		},
	})
}
