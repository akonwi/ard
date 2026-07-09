package parse

import "testing"

func TestFixedArrayTypes(t *testing.T) {
	runTests(t, []test{
		{
			name:  "variable annotation",
			input: `let bytes: [Byte; 3] = [1, 2, 3]`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{
					Name: "bytes",
					Type: &FixedArray{Element: &CustomType{Name: "Byte"}, Length: 3},
					Value: &ListLiteral{Items: []Expression{
						&NumLiteral{Value: "1"},
						&NumLiteral{Value: "2"},
						&NumLiteral{Value: "3"},
					}},
				},
			}},
		},
		{
			name:  "length may use numeric separators",
			input: `let bytes: [Byte; 1_000] = []`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{
					Name:  "bytes",
					Type:  &FixedArray{Element: &CustomType{Name: "Byte"}, Length: 1000},
					Value: &ListLiteral{Items: []Expression{}},
				},
			}},
		},
		{
			name:  "nullable fixed array",
			input: `let bytes: [Byte; 0]? = Maybe::new()`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{
					Name:  "bytes",
					Type:  &FixedArray{Element: &CustomType{Name: "Byte"}, Length: 0, nullable: true},
					Value: &StaticFunction{Target: &Identifier{Name: "Maybe"}, Function: FunctionCall{Name: "new", Args: []Argument{}, Comments: []Comment{}}},
				},
			}},
		},
		{
			name:  "fixed array result sugar",
			input: `let bytes: [Byte; 32]!Str = Result::ok([0])`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{
					Name:  "bytes",
					Type:  &ResultType{Val: &FixedArray{Element: &CustomType{Name: "Byte"}, Length: 32}, Err: &StringType{}},
					Value: &StaticFunction{Target: &Identifier{Name: "Result"}, Function: FunctionCall{Name: "ok", Args: []Argument{{Value: &ListLiteral{Items: []Expression{&NumLiteral{Value: "0"}}}}}, Comments: []Comment{}}},
				},
			}},
		},
		{
			name:     "missing fixed array length",
			input:    `let bytes: [Byte;] = []`,
			wantErrs: []string{"Expected fixed array length"},
		},
	})
}
