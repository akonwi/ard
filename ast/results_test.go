package ast

import "testing"

func TestLexAngleBrackets(t *testing.T) {
	runLexing(t, []lexTest{
		{
			name:  "Result type in return declaration",
			input: `fn foo() Result<Int, Str> {}`,
			want: []token{
				{kind: fn, line: 1, column: 1},
				{kind: identifier, line: 1, column: 4, text: "foo"},
				{kind: left_paren, line: 1, column: 7},
				{kind: right_paren, line: 1, column: 8},
				{kind: identifier, line: 1, column: 10, text: "Result"},
				{kind: less_than, line: 1, column: 16},
				{kind: identifier, line: 1, column: 17, text: "Int"},
				{kind: comma, line: 1, column: 20},
				{kind: identifier, line: 1, column: 22, text: "Str"},
				{kind: greater_than, line: 1, column: 25},
				{kind: left_brace, line: 1, column: 27},
				{kind: right_brace, line: 1, column: 28},
				{kind: eof},
			},
		},
	})
}

func TestResultTypeInSignature(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Result type in return declaration",
			input: `fn foo() Result<Int, Str> {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name:       "foo",
						Parameters: []Parameter{},
						ReturnType: &ResultType{
							Val: &IntType{},
							Err: &StringType{},
						},
						Body: []Statement{},
					},
				},
			},
		},
		{
			name:  "Result sugar syntax in return declaration",
			input: `fn foo() Int!Str {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name:       "foo",
						Parameters: []Parameter{},
						ReturnType: &ResultType{
							Val: &IntType{},
							Err: &StringType{},
						},
						Body: []Statement{},
					},
				},
			},
		},
		{
			name:  "Result sugar syntax with custom types",
			input: `fn foo() User!Error {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name:       "foo",
						Parameters: []Parameter{},
						ReturnType: &ResultType{
							Val: &CustomType{Name: "User"},
							Err: &CustomType{Name: "Error"},
						},
						Body: []Statement{},
					},
				},
			},
		},
		{
			name:  "Result sugar syntax with generic type",
			input: `fn foo() $T!Str {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name:       "foo",
						Parameters: []Parameter{},
						ReturnType: &ResultType{
							Val: &GenericType{Name: "T"},
							Err: &StringType{},
						},
						Body: []Statement{},
					},
				},
			},
		},
	})
}

func TestTryKeyword(t *testing.T) {
	runTests(t, []test{
		{
			name:  "try a result",
			input: `try get_result()`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&Try{
						Expression: &FunctionCall{
							Name: "get_result",
							Args: []Expression{},
						},
					},
				},
			},
		},
	})
}
