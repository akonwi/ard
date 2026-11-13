package parse

import "testing"

func TestTupleTypes(t *testing.T) {
	runTests(t, []test{
		{
			name:  "variable annotation",
			input: `let pair: (Int, Str) = (1, "a")`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{
					Name: "pair",
					Type: &TupleType{Elements: []DeclaredType{&IntType{}, &StringType{}}},
					Value: &TupleLiteral{Items: []Expression{
						&NumLiteral{Value: "1"},
						&StrLiteral{Value: "a"},
					}},
				},
			}},
		},
		{
			name:  "nullable tuple",
			input: `let pair: (Int, Str)? = Maybe::new()`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{
					Name:  "pair",
					Type:  &TupleType{Elements: []DeclaredType{&IntType{}, &StringType{}}, nullable: true},
					Value: &StaticFunction{Target: &Identifier{Name: "Maybe"}, Function: FunctionCall{Name: "new", Args: []Argument{}, Comments: []Comment{}}},
				},
			}},
		},
		{
			name:  "tuple result sugar",
			input: `let pair: (Int, Str)!Str = Result::err("no")`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{
					Name:  "pair",
					Type:  &ResultType{Val: &TupleType{Elements: []DeclaredType{&IntType{}, &StringType{}}}, Err: &StringType{}},
					Value: &StaticFunction{Target: &Identifier{Name: "Result"}, Function: FunctionCall{Name: "err", Args: []Argument{{Value: &StrLiteral{Value: "no"}}}, Comments: []Comment{}}},
				},
			}},
		},
		{
			name:  "nested tuple and trailing comma",
			input: "let nested: ((Int, Str), [Bool],) = value",
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{
					Name: "nested",
					Type: &TupleType{Elements: []DeclaredType{
						&TupleType{Elements: []DeclaredType{&IntType{}, &StringType{}}},
						&List{Element: &BooleanType{}},
					}},
					Value: &Identifier{Name: "value"},
				},
			}},
		},
		{
			name:  "function type returning a tuple",
			input: `let split: fn(Str) (Str, Str) = cut`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{
					Name: "split",
					Type: &FunctionType{
						Params: []DeclaredType{&StringType{}},
						Return: &TupleType{Elements: []DeclaredType{&StringType{}, &StringType{}}},
					},
					Value: &Identifier{Name: "cut"},
				},
			}},
		},
		{
			name:  "single parenthesized type stays a grouped type",
			input: `let name: (Str)? = Maybe::new()`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{
					Name:  "name",
					Type:  &StringType{nullable: true},
					Value: &StaticFunction{Target: &Identifier{Name: "Maybe"}, Function: FunctionCall{Name: "new", Args: []Argument{}, Comments: []Comment{}}},
				},
			}},
		},
		{
			name:     "one-element tuple type",
			input:    `let single: (Int,) = value`,
			wantErrs: []string{"A tuple type needs at least two elements"},
		},
		{
			name:     "unclosed tuple type",
			input:    `let pair: (Int, Str = value`,
			wantErrs: []string{"Expected ')' after tuple type"},
		},
	})
}

func TestTupleLiterals(t *testing.T) {
	runTests(t, []test{
		{
			name:  "nested literal",
			input: `((1, 2), 3)`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&TupleLiteral{Items: []Expression{
					&TupleLiteral{Items: []Expression{&NumLiteral{Value: "1"}, &NumLiteral{Value: "2"}}},
					&NumLiteral{Value: "3"},
				}},
			}},
		},
		{
			name:  "multi-line literal with trailing comma",
			input: "let pair = (\n  1,\n  2,\n)",
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VariableDeclaration{
					Name:  "pair",
					Value: &TupleLiteral{Items: []Expression{&NumLiteral{Value: "1"}, &NumLiteral{Value: "2"}}},
				},
			}},
		},
		{
			name:  "single parenthesized expression stays a grouping",
			input: `(1 + 2) * 3`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&BinaryExpression{
					Operator: Multiply,
					Left:     &BinaryExpression{Operator: Plus, Left: &NumLiteral{Value: "1"}, Right: &NumLiteral{Value: "2"}},
					Right:    &NumLiteral{Value: "3"},
				},
			}},
		},
		{
			name:  "empty parentheses stay the void literal",
			input: `()`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&VoidLiteral{},
			}},
		},
		{
			name:  "tuple as a call argument",
			input: `use_pair((1, "a"))`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&FunctionCall{Name: "use_pair", Args: []Argument{{Value: &TupleLiteral{Items: []Expression{
					&NumLiteral{Value: "1"},
					&StrLiteral{Value: "a"},
				}}}}, Comments: []Comment{}},
			}},
		},
		{
			name:     "one-element tuple literal",
			input:    `let single = (1,)`,
			wantErrs: []string{"A tuple needs at least two elements"},
		},
		{
			name:     "missing comma between tuple elements",
			input:    `let pair = (1, 2 3)`,
			wantErrs: []string{"Expected ',' between tuple elements"},
		},
		{
			name:     "unclosed tuple literal",
			input:    `let pair = (1, 2`,
			wantErrs: []string{"Expected ')' to close tuple"},
		},
	})
}

func TestTupleDestructuring(t *testing.T) {
	runTests(t, []test{
		{
			name:  "let destructuring",
			input: `let (ctx, cancel) = pair`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&TupleDestructure{
					Names: []TupleBinding{{Name: "ctx"}, {Name: "cancel"}},
					Value: &Identifier{Name: "pair"},
				},
			}},
		},
		{
			name:  "mut destructuring with a discard",
			input: `mut (count, _, rest,) = triple()`,
			output: Program{Imports: []Import{}, Statements: []Statement{
				&TupleDestructure{
					Mutable: true,
					Names:   []TupleBinding{{Name: "count"}, {Name: "_"}, {Name: "rest"}},
					Value:   &FunctionCall{Name: "triple", Args: []Argument{}, Comments: []Comment{}},
				},
			}},
		},
		{
			name:     "single name",
			input:    `let (a) = pair`,
			wantErrs: []string{"Tuple destructuring needs at least two names"},
		},
		{
			name:     "nested pattern",
			input:    `let ((a, b), c) = nested`,
			wantErrs: []string{"Expected a name in tuple destructuring"},
		},
		{
			name:     "type annotation",
			input:    `let (a, b): (Int, Str) = pair`,
			wantErrs: []string{"Tuple destructuring cannot have a type annotation"},
		},
		{
			name:     "missing initializer",
			input:    `let (a, b)`,
			wantErrs: []string{"Expected '=' after tuple destructuring names"},
		},
	})
}
