package ast

import (
	"testing"

	checker "github.com/akonwi/kon/checker"
)

func TestEnumDefinitions(t *testing.T) {
	color_enum := checker.EnumType{
		Name: "Color",
		Variants: map[string]int{
			"Red":    0,
			"Green":  1,
			"Yellow": 2,
		},
	}
	tests := []test{
		{
			name: "Valid basic enum",
			input: `
			enum Color {
				Red,
				Green,
				Yellow
			}`,
			output: &Program{
				Statements: []Statement{
					&EnumDefinition{
						Type: color_enum,
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
	}

	runTests(t, tests)
}

func TestEnums(t *testing.T) {
	colorEnum := checker.EnumType{Name: "Color",
		Variants: map[string]int{"Black": 0, "Grey": 1},
	}
	tests := []test{
		{
			name: "Valid enum variant access",
			input: `
				enum Color { Black, Grey }
				Color::Black`,
			output: &Program{
				Statements: []Statement{
					&EnumDefinition{
						Type: colorEnum,
					},
					&MemberAccess{
						Target:     Identifier{Name: "Color", Type: colorEnum},
						AccessType: Static,
						Member:     &Identifier{Name: "Black", Type: colorEnum},
					},
				},
			},
		},
		{
			name: "Invalid enum variant access",
			input: `
					enum Color { Black, Grey }
					Color::Blue`,
			diagnostics: []checker.Diagnostic{{Msg: "'Blue' is not a variant of 'Color' enum"}},
		},
		{
			name: "Assigning a variant to a variable",
			input: `
				enum Color { Black, Grey }
				let favorite: Color = Color::Black`,
			output: &Program{
				Statements: []Statement{
					&EnumDefinition{
						Type: colorEnum,
					},
					&VariableDeclaration{
						Mutable: false,
						Name:    "favorite",
						Type:    colorEnum,
						Value: &MemberAccess{
							Target:     Identifier{Name: "Color", Type: colorEnum},
							AccessType: Static,
							Member:     &Identifier{Name: "Black", Type: colorEnum},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}
