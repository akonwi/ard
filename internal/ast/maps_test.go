package ast

import "testing"

func TestMaps(t *testing.T) {
	runTests(t, []test{
		{
			name: "Instantiating maps",
			input: `
				let empty: [Str:Num] = [:]
			  let num_to_str: [Num:Str] = [1: "one", 2: "two", 3: "three"]
				`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "empty",
						Type:    Map{Key: StringType{}, Value: NumberType{}},
						Value: MapLiteral{
							Entries: []MapEntry{},
						},
					},
					VariableDeclaration{
						Mutable: false,
						Name:    "num_to_str",
						Type:    Map{Key: NumberType{}, Value: StringType{}},
						Value: MapLiteral{
							Entries: []MapEntry{
								{Key: NumLiteral{Value: "1"}, Value: StrLiteral{Value: `"one"`}},
								{Key: NumLiteral{Value: "2"}, Value: StrLiteral{Value: `"two"`}},
								{Key: NumLiteral{Value: "3"}, Value: StrLiteral{Value: `"three"`}},
							},
						},
					},
				},
			},
		},
	})
}
