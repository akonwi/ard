package checker_test

import (
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestFunctions(t *testing.T) {
	run(t, []test{
		{
			name: "Calling empty function",
			input: strings.Join(
				[]string{
					`fn noop() {}`,
					`noop()`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name:       "noop",
							Parameters: []checker.Parameter{},
							ReturnType: checker.Void,
							Body: &checker.Block{
								Stmts: []checker.Statement{},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "noop",
							Args: []checker.Expression{},
						},
					},
				},
			},
		},
		{
			name: "Calling function with parameters",
			input: strings.Join(
				[]string{
					`fn add(a: Int, b: Int) {}`,
					`add(1, 2)`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "add",
							Parameters: []checker.Parameter{
								{Name: "a", Type: checker.Int, Mutable: false},
								{Name: "b", Type: checker.Int, Mutable: false},
							},
							ReturnType: checker.Void,
							Body: &checker.Block{
								Stmts: []checker.Statement{},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "add",
							Args: []checker.Expression{
								&checker.IntLiteral{Value: 1},
								&checker.IntLiteral{Value: 2},
							},
						},
					},
				},
			},
		},
		{
			name: "Mutable parameters",
			input: strings.Join(
				[]string{
					`fn update(mut value: Int) {}`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "update",
							Parameters: []checker.Parameter{
								{Name: "value", Type: checker.Int, Mutable: true},
							},
							ReturnType: checker.Void,
							Body: &checker.Block{
								Stmts: []checker.Statement{},
							},
						},
					},
				},
			},
		},
		{
			name: "Functions should return the declared return type",
			input: strings.Join(
				[]string{
					`fn add(a: Int, b: Int) Int { false }`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Bool"},
			},
		},
		{
			name: "A functiton with a declared return type should have a final expression that satisfies it",
			input: strings.Join(
				[]string{
					`fn add(a: Int, b: Int) Void!Bool {
						let foo = false
					}`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Void!Bool, got Void"},
			},
		},
		{
			name: "Functions can declare Void in return type",
			input: strings.Join(
				[]string{
					`fn no_return() Void { true }`,
					`fn void_ok() Void!Bool { Result::err(true) }`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Type mismatch in function arguments",
			input: strings.Join(
				[]string{
					`fn greet(name: Str) {}`,
					`greet(42)`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
		{
			name: "Incorrect number of arguments",
			input: strings.Join(
				[]string{
					`fn add(a: Int, b: Int) {}`,
					`add(1)`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 2, got 1"},
			},
		},
	})
}

func TestCallingPackageFunctions(t *testing.T) {
	run(t, []test{
		{
			name: "Calling io::print",
			input: strings.Join([]string{
				`use ard/io`,
				`io::print("Hello World")`,
				`io::print(200)`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.ModuleFunctionCall{
							Module: "ard/io",
							Call: &checker.FunctionCall{
								Name: "print",
								Args: []checker.Expression{
									&checker.StrLiteral{Value: "Hello World"},
								},
							},
						},
					},
					{
						Expr: &checker.ModuleFunctionCall{
							Module: "ard/io",
							Call: &checker.FunctionCall{
								Name: "print",
								Args: []checker.Expression{
									&checker.IntLiteral{Value: 200},
								},
							},
						},
					},
				},
			},
		},
	})
}

func TestCallingInstanceMethods(t *testing.T) {
	run(t, []test{
		{
			name:  "Int.to_str()",
			input: `200.to_str()`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.InstanceMethod{
							Subject: &checker.IntLiteral{Value: 200},
							Method: &checker.FunctionCall{
								Name: "to_str",
								Args: []checker.Expression{},
							},
						},
					},
				},
			},
		},
	})
}

func TestNamedArguments(t *testing.T) {
	run(t, []test{
		{
			name: "Named arguments in correct order",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) Str { "hello" }`,
					`greet(name: "Alice", age: 25)`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "greet",
							Parameters: []checker.Parameter{
								{Name: "name", Type: checker.Str, Mutable: false},
								{Name: "age", Type: checker.Int, Mutable: false},
							},
							ReturnType: checker.Str,
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.StrLiteral{Value: "hello"}},
								},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "greet",
							Args: []checker.Expression{
								&checker.StrLiteral{Value: "Alice"},
								&checker.IntLiteral{Value: 25},
							},
						},
					},
				},
			},
		},
		{
			name: "Named arguments in reverse order",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) Str { "hello" }`,
					`greet(age: 42, name: "Bob")`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "greet",
							Parameters: []checker.Parameter{
								{Name: "name", Type: checker.Str, Mutable: false},
								{Name: "age", Type: checker.Int, Mutable: false},
							},
							ReturnType: checker.Str,
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.StrLiteral{Value: "hello"}},
								},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "greet",
							Args: []checker.Expression{
								&checker.StrLiteral{Value: "Bob"},
								&checker.IntLiteral{Value: 42},
							},
						},
					},
				},
			},
		},
		{
			name: "Named arguments with three parameters",
			input: strings.Join(
				[]string{
					`fn calculate(x: Int, y: Int, z: Int) Int { x }`,
					`calculate(z: 3, x: 1, y: 2)`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "calculate",
							Parameters: []checker.Parameter{
								{Name: "x", Type: checker.Int, Mutable: false},
								{Name: "y", Type: checker.Int, Mutable: false},
								{Name: "z", Type: checker.Int, Mutable: false},
							},
							ReturnType: checker.Int,
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.Variable{}},
								},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "calculate",
							Args: []checker.Expression{
								&checker.IntLiteral{Value: 1},
								&checker.IntLiteral{Value: 2},
								&checker.IntLiteral{Value: 3},
							},
						},
					},
				},
			},
		},
		{
			name: "Unknown parameter name",
			input: strings.Join(
				[]string{
					`fn greet(name: Str) {}`,
					`greet(unknown: "value")`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "unknown parameter name: unknown"},
			},
		},
		{
			name: "Parameter specified multiple times with positional and named",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) {}`,
					`greet("Alice", name: "Bob")`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "parameter name specified multiple times"},
			},
		},
		{
			name: "Parameter specified multiple times with named arguments",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) {}`,
					`greet(name: "Alice", name: "Bob")`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "parameter name specified multiple times"},
			},
		},
		{
			name: "Missing parameter with named arguments",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) {}`,
					`greet(name: "Alice")`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "missing argument for parameter: age"},
			},
		},
		{
			name: "Type mismatch with named arguments",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) {}`,
					`greet(name: 42, age: "hello")`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
		{
			name: "Named arguments with single parameter",
			input: strings.Join(
				[]string{
					`fn hello(name: Str) {}`,
					`hello(name: "World")`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "hello",
							Parameters: []checker.Parameter{
								{Name: "name", Type: checker.Str, Mutable: false},
							},
							ReturnType: checker.Void,
							Body: &checker.Block{
								Stmts: []checker.Statement{},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "hello",
							Args: []checker.Expression{
								&checker.StrLiteral{Value: "World"},
							},
						},
					},
				},
			},
		},
	})
}

func TestInferringEmptyCollectionArguments(t *testing.T) {
	run(t, []test{
		{
			name: "Empty array argument has no error",
			input: `
				fn greet(names: [Str]) {}
				greet(names: [])
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Empty map argument has no error",
			input: `
				fn foo(map: [Str:Int]) {}
				foo(map: [:])
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}

func TestUsingValidTypesForUnionArguments(t *testing.T) {
	run(t, []test{
		{
			name: "Map literal with union values",
			input: `
				use ard/sql

				fn foo(values: [Str : sql::Value]) {}
				foo(values: ["int":1, "str":"hey"])
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
