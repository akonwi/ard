package ast

import (
	"testing"
)

func TestVariables(t *testing.T) {
	tests := []test{
		{
			name:  "List elements must match declared type",
			input: `let strings: [Str] = [1, 2, 3]`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "strings",
						Type:    List{Element: StringType{}},
						Value: ListLiteral{
							Items: []Expression{
								NumLiteral{Value: "1"},
								NumLiteral{Value: "2"},
								NumLiteral{Value: "3"},
							},
						},
					},
				},
			},
		},
		{
			name:  "Valid list",
			input: `let numbers: [Num] = [1, 2, 3]`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "numbers",
						Type:    List{Element: NumberType{}},
						Value: ListLiteral{
							Items: []Expression{
								NumLiteral{Value: "1"},
								NumLiteral{Value: "2"},
								NumLiteral{Value: "3"},
							},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

// func TestLists(t *testing.T) {
// 	numList := MakeList(NumType)
// 	push_method := numList.GetProperty("push").(FunctionType)
// 	list_decl := VariableDeclaration{
// 		Mutable: true,
// 		Name:    "list",
// 		Type:    numList,
// 		Value: ListLiteral{
// 			Type: numList,
// 			Items: []Expression{
// 				NumLiteral{Value: "1"},
// 				NumLiteral{Value: "2"},
// 				NumLiteral{Value: "3"},
// 			},
// 		},
// 	}

// 	tests := []test{
// 		{
// 			name: "List size property",
// 			input: `
// 				mut list = [1,2,3]
// 				list.size`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					list_decl,
// 					MemberAccess{
// 						Target:     Identifier{Name: "list", Type: numList},
// 						AccessType: Instance,
// 						Member:     Identifier{Name: "size", Type: NumType},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Can call methods",
// 			input: `
// 				mut list = [1,2,3]
// 				list.push(4)`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					list_decl,
// 					MemberAccess{
// 						Target:     Identifier{Name: "list", Type: numList},
// 						AccessType: Instance,
// 						Member: FunctionCall{
// 							Name: "push",
// 							Args: []Expression{NumLiteral{Value: "4"}},
// 							Type: push_method,
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Cannot mutate an immutable list",
// 			input: `
// 						let list = [1,2,3]
// 						list.pop()`,
// 		},
// 		{
// 			name: ".map callback must have correct signature",
// 			input: `
// 				let list = [1,2,3]
// 				list.map((num: Str) { "foobar" })
// 				list.map((num) { "string" })`,
// 		},
// 	}

// 	runTests(t, tests)
// }
