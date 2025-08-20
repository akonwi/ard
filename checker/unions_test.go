package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestTypeUnions(t *testing.T) {
	run(t, []test{
		{
			name: "Compatible types to a union",
			input: `
				type Alias = Bool
			  type Printable = Int|Str
				let a: Printable = "foo"
				let b: Alias = true
				let list: [Printable] = [1, "two", 3]`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "a",
							Value:   &checker.StrLiteral{Value: "foo"},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "b",
							Value:   &checker.BoolLiteral{Value: true},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "list",
							Value: &checker.ListLiteral{
								Elements: []checker.Expression{
									&checker.IntLiteral{Value: 1},
									&checker.StrLiteral{Value: "two"},
									&checker.IntLiteral{Value: 3},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Using unions in return declarations",
			input: `
				struct InvalidField { name: Str, message: Str }
				type Error = InvalidField | Str

				fn do_stuff() Error {
					"unknown failure"
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Using unions as err result",
			input: `
				struct InvalidField { name: Str, message: Str }
				type Error = InvalidField | Str

				fn do_stuff() Bool!Error {
					Result::err(InvalidField{ name: "foo", message: "bar" })
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Using unions as ok result",
			input: `
				type Success = Bool | Int

				fn do_stuff() Success!Str {
					Result::ok(true)
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Errors for an incompatible type",
			input: `
					  type Printable = Int|Str
						fn print(p: Printable) {}
						print(true)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Printable, got Bool"},
			},
		},
		{
			name: "Matching behavior on type unions",
			input: `
				type Printable = Int|Str|Bool
				let a: Printable = "foo"
				match a {
				  Int => "number",
					Str => "string",
					_ => "other"
				}`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "a",
							Value:   &checker.StrLiteral{Value: "foo"},
						},
					},
					{
						Expr: &checker.UnionMatch{
							Subject: &checker.Variable{},
							TypeCases: map[string]*checker.Block{
								"Int": {
									Stmts: []checker.Statement{
										{Expr: &checker.StrLiteral{Value: "number"}},
									},
								},
								"Str": {
									Stmts: []checker.Statement{
										{Expr: &checker.StrLiteral{Value: "string"}},
									},
								},
							},
							CatchAll: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.StrLiteral{Value: "other"}},
								},
							},
						},
					},
				},
			},
		},
	})
}
