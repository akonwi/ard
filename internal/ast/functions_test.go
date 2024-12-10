package ast

import (
	"testing"

	"github.com/akonwi/ard/internal/checker"
)

func TestFunctionDeclaration(t *testing.T) {
	tests := []test{
		{
			name:  "Empty function",
			input: `fn empty() {}`,
			output: Program{
				Statements: []Statement{
					FunctionDeclaration{
						Name:       "empty",
						Parameters: []Parameter{},
						ReturnType: checker.VoidType,
						Body:       []Statement{},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Inferred function return type",
			input: `fn get_msg() { "Hello, world!" }`,
			output: Program{
				Statements: []Statement{
					FunctionDeclaration{
						Name:       "get_msg",
						Parameters: []Parameter{},
						ReturnType: checker.StrType,
						Body: []Statement{
							StrLiteral{
								Value: `"Hello, world!"`,
							},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Function with a parameter and declared return type",
			input: `fn greet(person: Str) Str { "hello" }`,
			output: Program{
				Statements: []Statement{
					FunctionDeclaration{
						Name: "greet",
						Parameters: []Parameter{
							{
								Name: "person",
								Type: checker.StrType,
							},
						},
						ReturnType: checker.StrType,
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
			diagnostics: []checker.Diagnostic{
				{
					Msg: "Type mismatch: expected Str, got Void",
				},
			},
		},
		{
			name:  "Function with two parameters",
			input: `fn add(x: Num, y: Num) Num { 10 }`,
			output: Program{
				Statements: []Statement{
					FunctionDeclaration{
						Name: "add",
						Parameters: []Parameter{
							{
								Name: "x",
								Type: checker.NumType,
							},
							{
								Name: "y",
								Type: checker.NumType,
							},
						},
						ReturnType: checker.NumType,
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
	get_name := checker.FunctionType{
		Name:       "get_name",
		Mutates:    false,
		Parameters: []checker.Type{},
		ReturnType: checker.StrType,
	}
	greet := checker.FunctionType{
		Name:       "greet",
		Mutates:    false,
		Parameters: []checker.Type{checker.StrType},
		ReturnType: checker.StrType,
	}
	add := checker.FunctionType{
		Name: "add",
		Parameters: []checker.Type{
			checker.NumType,
			checker.NumType,
		},
		ReturnType: checker.NumType,
	}

	tests := []test{
		{
			name: "Valid function call with no arguments",
			input: `
				fn get_name() Str { "name" }
				get_name()`,
			output: Program{
				Statements: []Statement{
					FunctionDeclaration{
						Name:       "get_name",
						Parameters: []Parameter{},
						ReturnType: get_name.ReturnType,
						Body:       []Statement{StrLiteral{Value: `"name"`}},
					},
					FunctionCall{
						Name: "get_name",
						Args: []Expression{},
						Type: get_name,
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Providing arguments when none are expected",
			input: `
				fn get_name() Str { "name" }
				get_name("bo")
			`,
			diagnostics: []checker.Diagnostic{
				{Msg: "Expected 0 arguments, got 1"},
			},
		},
		{
			name: "Valid function call with one argument",
			input: `
				fn greet(name: Str) Str { "hello" }
				greet("Alice")`,
			output: Program{
				Statements: []Statement{
					FunctionDeclaration{
						Name: greet.Name,
						Parameters: []Parameter{
							{Name: "name", Type: checker.StrType},
						},
						ReturnType: greet.ReturnType,
						Body:       []Statement{StrLiteral{Value: `"hello"`}},
					},
					FunctionCall{
						Name: "greet",
						Args: []Expression{
							StrLiteral{Value: `"Alice"`},
						},
						Type: greet,
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Valid function call with two arguments",
			input: `
				fn add(x: Num, y: Num) Num { x + y }
				add(1, 2)`,
			output: Program{
				Statements: []Statement{
					FunctionDeclaration{
						Name: add.Name,
						Parameters: []Parameter{
							{Name: "x", Type: checker.NumType},
							{Name: "y", Type: checker.NumType},
						},
						ReturnType: add.ReturnType,
						Body: []Statement{
							BinaryExpression{
								Left:     Identifier{Name: "x", Type: checker.NumType},
								Operator: Plus,
								Right:    Identifier{Name: "y", Type: checker.NumType},
							},
						},
					},
					FunctionCall{
						Name: "add",
						Args: []Expression{
							NumLiteral{Value: "1"},
							NumLiteral{Value: "2"},
						},
						Type: add,
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Wrong argument type",
			input: `
				fn add(x: Num, y: Num) Num { x + y }
				add(1, "two")`,
			diagnostics: []checker.Diagnostic{
				{
					Msg: "Type mismatch: expected Num, got Str",
				},
			},
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
				Statements: []Statement{
					AnonymousFunction{
						Parameters: []Parameter{},
						ReturnType: checker.StrType,
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
				Statements: []Statement{
					AnonymousFunction{
						Parameters: []Parameter{
							{Name: "name", Type: checker.StrType},
						},
						ReturnType: checker.StrType,
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
				Statements: []Statement{
					AnonymousFunction{
						Parameters: []Parameter{
							{Name: "name", Type: checker.StrType},
						},
						ReturnType: checker.StrType,
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
