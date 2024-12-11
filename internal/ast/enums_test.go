package ast

import (
	"fmt"
	"testing"
)

var traffic_light_code = `
enum Color {
	Red,
	Green,
	Yellow
}`

var traffic_light_enum = EnumType{
	Name: "Color",
	Variants: []string{
		"Red",
		"Green",
		"Yellow",
	},
}

func TestEnumDefinitions(t *testing.T) {
	tests := []test{
		{
			name:  "Valid basic enum",
			input: traffic_light_code,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					EnumDefinition{
						Type: traffic_light_enum,
					},
				},
			},
			diagnostics: []Diagnostic{},
		},
	}

	runTests(t, tests)
}

func TestEnums(t *testing.T) {
	colorEnum := EnumType{Name: "Color",
		Variants: []string{"Black", "Grey"},
	}
	tests := []test{
		{
			name: "Valid enum variant access",
			input: `
				enum Color { Black, Grey }
				Color::Black`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					EnumDefinition{
						Type: colorEnum,
					},
					MemberAccess{
						Target:     Identifier{Name: "Color", Type: colorEnum},
						AccessType: Static,
						Member:     Identifier{Name: "Black", Type: colorEnum},
					},
				},
			},
		},
		{
			name: "Invalid enum variant access",
			input: `
					enum Color { Black, Grey }
					Color::Blue`,
			diagnostics: []Diagnostic{{Msg: "'Blue' is not a variant of 'Color' enum"}},
		},
		{
			name: "Assigning a variant to a variable",
			input: `
				enum Color { Black, Grey }
				let favorite: Color = Color::Black`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					EnumDefinition{
						Type: colorEnum,
					},
					VariableDeclaration{
						Mutable: false,
						Name:    "favorite",
						Type:    CustomType{Name: "Color"},
						Value: MemberAccess{
							Target:     Identifier{Name: "Color", Type: colorEnum},
							AccessType: Static,
							Member:     Identifier{Name: "Black", Type: colorEnum},
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
			name: "Matching must be exhaustive",
			input: fmt.Sprintf(`%v
				let light = Color::Red
				match light {
					Color::Red => "Stop",
					Color::Yellow => "Yield"
				}`, traffic_light_code),
			diagnostics: []Diagnostic{
				{Msg: "Missing case for 'Color::Green'"},
			},
		},
		{
			name: "Each case must return the same type",
			input: fmt.Sprintf(`%v
				let light = Color::Red
				match light {
					Color::Red => "Stop",
					Color::Yellow => "Yield",
					Color::Green => 100
				}`, traffic_light_code),
			diagnostics: []Diagnostic{
				{Msg: "Type mismatch: expected Str, got Num"},
			},
		},
		{
			name: "Valid matching",
			input: fmt.Sprintf(`%v
				let light = Color::Red
				match light {
					Color::Red => "Stop",
					Color::Yellow => "Yield",
					Color::Green => "Go"
				}`, traffic_light_code),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					EnumDefinition{
						Type: traffic_light_enum,
					},
					VariableDeclaration{
						Mutable: false,
						Name:    "light",
						Type:    CustomType{Name: "Color"},
						Value: MemberAccess{
							Target:     Identifier{Name: "Color", Type: traffic_light_enum},
							AccessType: Static,
							Member:     Identifier{Name: "Red", Type: traffic_light_enum},
						},
					},
					MatchExpression{
						Subject: Identifier{Name: "light", Type: traffic_light_enum},
						Cases: []MatchCase{
							{
								Pattern: MemberAccess{
									Target:     Identifier{Name: "Color", Type: traffic_light_enum},
									AccessType: Static,
									Member:     Identifier{Name: "Red", Type: traffic_light_enum},
								},
								Body: []Statement{StrLiteral{Value: `"Stop"`}},
								Type: StrType,
							},
							{
								Pattern: MemberAccess{
									Target:     Identifier{Name: "Color", Type: traffic_light_enum},
									AccessType: Static,
									Member:     Identifier{Name: "Yellow", Type: traffic_light_enum},
								},
								Body: []Statement{StrLiteral{Value: `"Yield"`}},
								Type: StrType,
							},
							{
								Pattern: MemberAccess{
									Target:     Identifier{Name: "Color", Type: traffic_light_enum},
									AccessType: Static,
									Member:     Identifier{Name: "Green", Type: traffic_light_enum},
								},
								Body: []Statement{StrLiteral{Value: `"Go"`}},
								Type: StrType,
							},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}
