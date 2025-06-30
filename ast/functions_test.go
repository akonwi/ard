package ast

import (
	"testing"
)

func TestFunctionDeclaration(t *testing.T) {
	tests := []test{
		{
			name:  "Empty function",
			input: `fn empty() {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name:       "empty",
						Parameters: []Parameter{},
						Body:       []Statement{},
					},
				},
			},
		},
		{
			name:  "Public function",
			input: `pub fn empty() {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Public:     true,
						Name:       "empty",
						Parameters: []Parameter{},
						Body:       []Statement{},
					},
				},
			},
		},
		{
			name:  "Function with generics",
			input: `fn decode(str: $In) $Out {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "decode",
						Parameters: []Parameter{
							{
								Name: "str",
								Type: &GenericType{Name: "In"},
							},
						},
						ReturnType: &GenericType{Name: "Out"},
						Body:       []Statement{},
					},
				},
			},
		},
		{
			name:  "Non-returning function",
			input: `fn get_msg() { "Hello, world!" }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name:       "get_msg",
						Parameters: []Parameter{},
						Body: []Statement{
							&StrLiteral{
								Value: "Hello, world!",
							},
						},
					},
				},
			},
		},
		{
			name:  "Function with a parameter and declared return type",
			input: `fn greet(person: Str) Str { "hello" }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "greet",
						Parameters: []Parameter{
							{
								Name: "person",
								Type: &StringType{},
							},
						},
						ReturnType: &StringType{},
						Body: []Statement{
							&StrLiteral{Value: "hello"},
						},
					},
				},
			},
		},
		{
			name:  "Function with two parameters",
			input: `fn add(x: Int, y: Int) Int { 10 }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "add",
						Parameters: []Parameter{
							{
								Name: "x",
								Type: &IntType{},
							},
							{
								Name: "y",
								Type: &IntType{},
							},
						},
						ReturnType: &IntType{},
						Body: []Statement{
							&NumLiteral{Value: "10"},
						},
					},
				},
			},
		},
		{
			name:  "Mutable parameter",
			input: `fn greet(mut person: Str) Str { }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "greet",
						Parameters: []Parameter{
							{
								Name:    "person",
								Type:    &StringType{},
								Mutable: true,
							},
						},
						ReturnType: &StringType{},
						Body:       []Statement{},
					},
				},
			},
		},
		{
			name:  "Static paths to types",
			input: `fn print(thing: Str::ToString) Str { }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "print",
						Parameters: []Parameter{
							{
								Name: "thing",
								Type: &CustomType{
									Type: StaticProperty{
										Target:   &Identifier{Name: "Str"},
										Property: &Identifier{Name: "ToString"},
									},
								},
							},
						},
						ReturnType: &StringType{},
						Body:       []Statement{},
					},
				},
			},
		},
		{
			name:  "Function with a qualified path",
			input: `fn Person::new(name: Str) Person { }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StaticFunctionDeclaration{
						Path: StaticProperty{Target: &Identifier{Name: "Person"}, Property: &Identifier{Name: "new"}},
						FunctionDeclaration: FunctionDeclaration{
							Parameters: []Parameter{
								{
									Name: "name",
									Type: &StringType{},
								},
							},
							ReturnType: &CustomType{Name: "Person"},
							Body:       []Statement{},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestFunctionCalls(t *testing.T) {
	tests := []test{
		{
			name: "Valid function call with no arguments",
			input: `
				fn get_name() Str { "name" }
				get_name()`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name:       "get_name",
						Parameters: []Parameter{},
						ReturnType: &StringType{},
						Body:       []Statement{&StrLiteral{Value: "name"}},
					},
					&FunctionCall{
						Name: "get_name",
						Args: []Expression{},
					},
				},
			},
		},
		{
			name: "Valid function call with one argument",
			input: `
				fn greet(name: Str) Str { "hello" }
				greet("Alice")`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "greet",
						Parameters: []Parameter{
							{Name: "name", Type: &StringType{}},
						},
						ReturnType: &StringType{},
						Body:       []Statement{&StrLiteral{Value: "hello"}},
					},
					&FunctionCall{
						Name: "greet",
						Args: []Expression{
							&StrLiteral{Value: "Alice"},
						},
					},
				},
			},
		},
		{
			name: "Valid function call with two arguments",
			input: `
				fn add(x: Int, y: Int) Int { x + y }
				add(1, 2)`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "add",
						Parameters: []Parameter{
							{Name: "x", Type: &IntType{}},
							{Name: "y", Type: &IntType{}},
						},
						ReturnType: &IntType{},
						Body: []Statement{
							&BinaryExpression{
								Left:     &Identifier{Name: "x"},
								Operator: Plus,
								Right:    &Identifier{Name: "y"},
							},
						},
					},
					&FunctionCall{
						Name: "add",
						Args: []Expression{
							&NumLiteral{Value: "1"},
							&NumLiteral{Value: "2"},
						},
					},
				},
			},
		},
		{
			name: "Calls that break after opening paren lines",
			input: `
				fn add(x: Int, y: Int) Int { x + y }
				add(
					1, 2)`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "add",
						Parameters: []Parameter{
							{Name: "x", Type: &IntType{}},
							{Name: "y", Type: &IntType{}},
						},
						ReturnType: &IntType{},
						Body: []Statement{
							&BinaryExpression{
								Left:     &Identifier{Name: "x"},
								Operator: Plus,
								Right:    &Identifier{Name: "y"},
							},
						},
					},
					&FunctionCall{
						Name: "add",
						Args: []Expression{
							&NumLiteral{Value: "1"},
							&NumLiteral{Value: "2"},
						},
					},
				},
			},
		},
		{
			name: "Calls that span multiple lines",
			input: `
				fn add(x: Int, y: Int) Int { x + y }
				add(
					1,
					2
				)`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "add",
						Parameters: []Parameter{
							{Name: "x", Type: &IntType{}},
							{Name: "y", Type: &IntType{}},
						},
						ReturnType: &IntType{},
						Body: []Statement{
							&BinaryExpression{
								Left:     &Identifier{Name: "x"},
								Operator: Plus,
								Right:    &Identifier{Name: "y"},
							},
						},
					},
					&FunctionCall{
						Name: "add",
						Args: []Expression{
							&NumLiteral{Value: "1"},
							&NumLiteral{Value: "2"},
						},
					},
				},
			},
		},
		{
			name: "Chaining calls across lines",
			input: `
			 	foo.bar()
					.baz()
				`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&InstanceMethod{
						Target: &InstanceMethod{
							Target: &Identifier{Name: "foo"},
							Method: FunctionCall{
								Name: "bar",
								Args: []Expression{},
							},
						},
						Method: FunctionCall{
							Name: "baz",
							Args: []Expression{},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestFunctionsWithGenerics(t *testing.T) {
	runTests(t, []test{
		{
			name:  "function def with generics",
			input: `fn identity(of: $T) $T { }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "identity",
						Parameters: []Parameter{
							{Name: "of", Type: &GenericType{Name: "T"}},
						},
						ReturnType: &GenericType{Name: "T"},
						Body:       []Statement{},
					},
				},
			},
		},
		{
			name:  "Static function call with generic type argument",
			input: `json::decode<Person>(str)`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StaticFunction{
						Target: &Identifier{Name: "json"},
						Function: FunctionCall{
							Name: "decode",
							TypeArgs: []DeclaredType{
								&CustomType{Name: "Person"},
							},
							Args: []Expression{
								&Identifier{Name: "str"},
							},
						},
					},
				},
			},
		},
	})
}

func TestAnonymousFunctions(t *testing.T) {
	tests := []test{
		{
			name:  "Anonymous function",
			input: `let lambda = fn() { "Hello, world!" }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name: "lambda",
						Value: &AnonymousFunction{
							Parameters: []Parameter{},
							Body: []Statement{
								&StrLiteral{Value: "Hello, world!"},
							},
						}},
				},
			},
		},
		{
			name:  "Anonymous function with a parameter",
			input: `fn(name: Str) { "Hello, name!" }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&AnonymousFunction{
						Parameters: []Parameter{
							{Name: "name", Type: &StringType{}},
						},
						Body: []Statement{
							&StrLiteral{Value: "Hello, name!"},
						},
					},
				},
			},
		},
		{
			name:  "Anonymous function with a parameter",
			input: `fn(name: Str) { "Hello, name!" }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&AnonymousFunction{
						Parameters: []Parameter{
							{Name: "name", Type: &StringType{}},
						},
						Body: []Statement{
							&StrLiteral{Value: "Hello, name!"},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}
