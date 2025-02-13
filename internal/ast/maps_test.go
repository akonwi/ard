package ast

import "testing"

func TestMaps(t *testing.T) {
	runTests(t, []test{
		{
			name: "Instantiating maps",
			input: `
				let empty: [Str:Int] = [:]
			  let num_to_str: [Int:Str] = [1: "one", 2: "two", 3: "three"]
				`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "empty",
						Type:    Map{Key: StringType{}, Value: IntType{}},
						Value: MapLiteral{
							Entries: []MapEntry{},
						},
					},
					VariableDeclaration{
						Mutable: false,
						Name:    "num_to_str",
						Type:    Map{Key: IntType{}, Value: StringType{}},
						Value: MapLiteral{
							Entries: []MapEntry{
								{Key: IntLiteral{Value: "1"}, Value: StrLiteral{Value: `"one"`}},
								{Key: IntLiteral{Value: "2"}, Value: StrLiteral{Value: `"two"`}},
								{Key: IntLiteral{Value: "3"}, Value: StrLiteral{Value: `"three"`}},
							},
						},
					},
				},
			},
		},
	})
}
