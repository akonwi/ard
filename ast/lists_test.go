package ast

import (
	"testing"

	checker "github.com/akonwi/kon/checker"
)

func TestVariables(t *testing.T) {
	tests := []test{
		{
			name:  "empty lists need to be explicitly typed",
			input: `let numbers = []`,
			diagnostics: []checker.Diagnostic{
				{Msg: "Empty lists need a declared type"},
			},
		},
		{
			name:  "List with mixed types",
			input: `let numbers = [1, "two", false]`,
			diagnostics: []checker.Diagnostic{
				{Msg: "List elements must be of the same type"},
			},
		},
		{
			name:  "List elements must match declared type",
			input: `let strings: [Str] = [1, 2, 3]`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "strings",
						Type:    &checker.ListType{ItemType: checker.StrType},
						Value: ListLiteral{
							Type: checker.ListType{ItemType: checker.NumType},
							Items: []Expression{
								NumLiteral{Value: "1"},
								NumLiteral{Value: "2"},
								NumLiteral{Value: "3"},
							},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{Msg: "Type mismatch: expected [Str], got [Num]"},
			},
		},
		{
			name:  "Valid list",
			input: `let numbers: [Num] = [1, 2, 3]`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "numbers",
						Type:    &checker.ListType{ItemType: checker.NumType},
						Value: ListLiteral{
							Type: checker.ListType{ItemType: checker.NumType},
							Items: []Expression{
								NumLiteral{Value: "1"},
								NumLiteral{Value: "2"},
								NumLiteral{Value: "3"},
							},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
	}

	runTests(t, tests)
}

func TestListApi(t *testing.T) {
	numList := checker.MakeList(checker.NumType)
	push_method := numList.GetProperty("push").(checker.FunctionType)
	list_decl := VariableDeclaration{
		Mutable: true,
		Name:    "list",
		Type:    numList,
		Value: ListLiteral{
			Type: numList,
			Items: []Expression{
				NumLiteral{Value: "1"},
				NumLiteral{Value: "2"},
				NumLiteral{Value: "3"},
			},
		},
	}

	tests := []test{
		{
			name: "List size property",
			input: `
				mut list = [1,2,3]
				list.size`,
			output: Program{
				Statements: []Statement{
					list_decl,
					MemberAccess{
						Target:     Identifier{Name: "list", Type: numList},
						AccessType: Instance,
						Member:     Identifier{Name: "size", Type: checker.NumType},
					},
				},
			},
		},
		{
			name: "Can call methods",
			input: `
				mut list = [1,2,3]
				list.push(4)`,
			output: Program{
				Statements: []Statement{
					list_decl,
					MemberAccess{
						Target:     Identifier{Name: "list", Type: numList},
						AccessType: Instance,
						Member: FunctionCall{
							Name: "push",
							Args: []Expression{NumLiteral{Value: "4"}},
							Type: push_method,
						},
					},
				},
			},
		},
		{
			name: "Cannot mutate an immutable list",
			input: `
						let list = [1,2,3]
						list.pop()`,
			diagnostics: []checker.Diagnostic{
				{Msg: "Cannot mutate an immutable list"},
			},
		},
	}

	runTests(t, tests)
}
