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
					FunctionDeclaration{
						Name:       "empty",
						Parameters: []Parameter{},
						Body:       []Statement{},
					},
				},
			},
		},
		{
			name:  "Inferred function return type",
			input: `fn get_msg() { "Hello, world!" }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					FunctionDeclaration{
						Name:       "get_msg",
						Parameters: []Parameter{},
						Body: []Statement{
							StrLiteral{
								Value: `"Hello, world!"`,
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
					FunctionDeclaration{
						Name: "greet",
						Parameters: []Parameter{
							{
								Name: "person",
								Type: StringType{},
							},
						},
						ReturnType: StringType{},
						Body: []Statement{
							StrLiteral{Value: `"hello"`},
						},
					},
				},
			},
		},
		{
			name:  "Function return must match declared return type",
			input: `fn greet(person: Str) Str { }`,
		},
		{
			name:  "Function with two parameters",
			input: `fn add(x: Num, y: Num) Num { 10 }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					FunctionDeclaration{
						Name: "add",
						Parameters: []Parameter{
							{
								Name: "x",
								Type: NumberType{},
							},
							{
								Name: "y",
								Type: NumberType{},
							},
						},
						ReturnType: NumberType{},
						Body: []Statement{
							NumLiteral{Value: "10"},
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
					FunctionDeclaration{
						Name:       "get_name",
						Parameters: []Parameter{},
						ReturnType: StringType{},
						Body:       []Statement{StrLiteral{Value: `"name"`}},
					},
					FunctionCall{
						Name: "get_name",
						Args: []Expression{},
					},
				},
			},
		},
		{
			name: "Providing arguments when none are expected",
			input: `
				fn get_name() Str { "name" }
				get_name("bo")
			`,
		},
		{
			name: "Valid function call with one argument",
			input: `
				fn greet(name: Str) Str { "hello" }
				greet("Alice")`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					FunctionDeclaration{
						Name: "greet",
						Parameters: []Parameter{
							{Name: "name", Type: StringType{}},
						},
						ReturnType: StringType{},
						Body:       []Statement{StrLiteral{Value: `"hello"`}},
					},
					FunctionCall{
						Name: "greet",
						Args: []Expression{
							StrLiteral{Value: `"Alice"`},
						},
					},
				},
			},
		},
		{
			name: "Valid function call with two arguments",
			input: `
				fn add(x: Num, y: Num) Num { x + y }
				add(1, 2)`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					FunctionDeclaration{
						Name: "add",
						Parameters: []Parameter{
							{Name: "x", Type: NumberType{}},
							{Name: "y", Type: NumberType{}},
						},
						ReturnType: NumberType{},
						Body: []Statement{
							BinaryExpression{
								Left:     Identifier{Name: "x", Type: NumType},
								Operator: Plus,
								Right:    Identifier{Name: "y", Type: NumType},
							},
						},
					},
					FunctionCall{
						Name: "add",
						Args: []Expression{
							NumLiteral{Value: "1"},
							NumLiteral{Value: "2"},
						},
					},
				},
			},
		},
		{
			name: "Wrong argument type",
			input: `
				fn add(x: Num, y: Num) Num { x + y }
				add(1, "two")`,
		},
	}

	runTests(t, tests)
}

func TestAnonymousFunctions(t *testing.T) {
	tests := []test{
		{
			name:  "Anonymous function",
			input: `() { "Hello, world!" }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					AnonymousFunction{
						Parameters: []Parameter{},
						Body: []Statement{
							StrLiteral{Value: `"Hello, world!"`},
						},
					},
				},
			},
		},
		{
			name:  "Anonymous function with a parameter",
			input: `(name: Str) { "Hello, name!" }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					AnonymousFunction{
						Parameters: []Parameter{
							{Name: "name", Type: StringType{}},
						},
						Body: []Statement{
							StrLiteral{Value: `"Hello, name!"`},
						},
					},
				},
			},
		},
		{
			name:  "Anonymous function with a parameter",
			input: `(name: Str) { "Hello, name!" }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					AnonymousFunction{
						Parameters: []Parameter{
							{Name: "name", Type: StringType{}},
						},
						Body: []Statement{
							StrLiteral{Value: `"Hello, name!"`},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}
