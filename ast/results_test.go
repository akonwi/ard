package ast

import "testing"

// this needs to be done better
func testLexAngleBrackets(t *testing.T) {
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
		{
			name: "Result sugar syntax with generic maybe type",
			input: `
			fn nullable(decoder: fn(Dynamic) $T![Error]) $T?![Error] {}
			`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "nullable",
						Parameters: []Parameter{
							{
								Name: "decoder",
								Type: &FunctionType{
									Params: []DeclaredType{&CustomType{Name: "Dynamic"}},
									Return: &ResultType{
										Val:      &GenericType{Name: "T", nullable: false},
										Err:      &List{Element: &CustomType{Name: "Error"}},
										nullable: false,
									},
								},
							},
						},
						ReturnType: &ResultType{
							Val:      &GenericType{Name: "T", nullable: true},
							Err:      &List{Element: &CustomType{Name: "Error"}},
							nullable: false,
						},
						Body: []Statement{},
					},
				},
			},
		},
		{
			name: "Result sugar with qualified types",
			input: `
			fn foo() db::Conn!Str {}
			fn foo() Bool!bar::Qux {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name:       "foo",
						Parameters: []Parameter{},
						ReturnType: &ResultType{
							Val: &CustomType{
								Name: "db::Conn",
								Type: StaticProperty{
									Target:   &Identifier{Name: "db"},
									Property: &Identifier{Name: "Conn"},
								}},
							Err: &StringType{},
						},
						Body: []Statement{},
					},
					&FunctionDeclaration{
						Name:       "foo",
						Parameters: []Parameter{},
						ReturnType: &ResultType{
							Val: &BooleanType{},
							Err: &CustomType{
								Name: "bar::Qux",
								Type: StaticProperty{
									Target:   &Identifier{Name: "bar"},
									Property: &Identifier{Name: "Qux"},
								}},
						},
						Body: []Statement{},
					},
				},
			},
		},
		{
			name:  "Result sugar with list types",
			input: `fn foo() [Int]!Str {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name:       "foo",
						Parameters: []Parameter{},
						ReturnType: &ResultType{
							Val: &List{Element: &IntType{}},
							Err: &StringType{},
						},
						Body: []Statement{},
					},
				},
			},
		},
		{
			name:  "Result sugar with map types",
			input: `fn foo() [Int:Str]!Bool {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name:       "foo",
						Parameters: []Parameter{},
						ReturnType: &ResultType{
							Val: &Map{Key: &IntType{}, Value: &StringType{}},
							Err: &BooleanType{},
						},
						Body: []Statement{},
					},
				},
			},
		},
		{
			name: "Result sugar with complex nested types",
			input: `fn foo() [User]!Str {}
			fn bar() [Int:[User]]!Bool {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name:       "foo",
						Parameters: []Parameter{},
						ReturnType: &ResultType{
							Val: &List{Element: &CustomType{Name: "User"}},
							Err: &StringType{},
						},
						Body: []Statement{},
					},
					&FunctionDeclaration{
						Name:       "bar",
						Parameters: []Parameter{},
						ReturnType: &ResultType{
							Val: &Map{Key: &IntType{}, Value: &List{Element: &CustomType{Name: "User"}}},
							Err: &BooleanType{},
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
							Name:     "get_result",
							Args:     []Argument{},
							Comments: []Comment{},
						},
					},
				},
			},
		},
	})
}
