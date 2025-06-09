package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestResults(t *testing.T) {
	run(t, []test{
		{
			name: "creating results",
			input: `
			fn divide(a: Int, b: Int) Result<Int, Str> {
			  match b == 0 {
			    true => Result::err("division by zero"),
			    false => Result::ok(a / b)
			  }
			}`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "divide",
							Parameters: []checker.Parameter{
								{Name: "a", Type: checker.Int},
								{Name: "b", Type: checker.Int},
							},
							ReturnType: checker.MakeResult(checker.Int, checker.Str),
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{
										Expr: &checker.BoolMatch{
											Subject: &checker.Equality{
												&checker.Variable{},
												&checker.IntLiteral{Value: 0},
											},
											True: &checker.Block{
												Stmts: []checker.Statement{
													{
														Expr: &checker.ModuleFunctionCall{
															Module: "ard/result",
															Call: &checker.FunctionCall{
																Name: "err",
																Args: []checker.Expression{
																	&checker.StrLiteral{Value: "division by zero"},
																},
															},
														},
													},
												},
											},
											False: &checker.Block{
												Stmts: []checker.Statement{
													{
														Expr: &checker.ModuleFunctionCall{
															Module: "ard/result",
															Call: &checker.FunctionCall{
																Name: "ok",
																Args: []checker.Expression{
																	&checker.IntDivision{
																		Left:  &checker.Variable{},
																		Right: &checker.Variable{},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Results must match declarations",
			input: `
			let result: Result<Int, Str> = Result::ok(true)
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: `Type mismatch: Expected Int, got Bool`},
			},
		},
		{
			name: "Results must match return declaration",
			input: `
			fn foo() Result<Int, Str> {
				Result::err(true)
			}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: `Type mismatch: Expected Result<Int, Str>, got Result<$Val, Bool>`},
			},
		},
		{
			name: "Result.or() unwraps with a default",
			input: `
			fn foo() Int {
				let res: Result<Int, Str> = Result::err("foo")
				res.or(10)
			}`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Matching on results",
			input: `
			use ard/io

			let res: Result<Int, Str> = Result::err("foo")
			match res {
				ok(num) => num,
				err => {
				  io::print("failed: " + err)
					-1
				}
			}`,
			output: &checker.Program{
				Imports: map[string]checker.Module{
					"ard/io": checker.IoPkg{},
				},
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Name: "res",
							Value: &checker.ModuleFunctionCall{
								Module: "ard/result",
								Call: &checker.FunctionCall{
									Name: "err",
									Args: []checker.Expression{&checker.StrLiteral{"foo"}},
								},
							},
						},
					},
					{
						Expr: &checker.ResultMatch{
							Subject: &checker.Variable{},
							Ok: &checker.Match{
								Pattern: &checker.Identifier{Name: "num"},
								Body: &checker.Block{Stmts: []checker.Statement{
									{Expr: &checker.Variable{}},
								}},
							},
							Err: &checker.Match{
								Pattern: &checker.Identifier{Name: "err"},
								Body: &checker.Block{Stmts: []checker.Statement{
									{Expr: &checker.ModuleFunctionCall{
										Module: "ard/io",
										Call: &checker.FunctionCall{
											Name: "print",
											Args: []checker.Expression{
												&checker.StrAddition{&checker.StrLiteral{"failed: "}, &checker.Variable{}},
											},
										},
									}},
									{Expr: &checker.Negation{&checker.IntLiteral{1}}},
								}},
							},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
	})
}

func TestTry(t *testing.T) {
	run(t, []test{
		{
			name: "trying a result",
			input: `
				fn do_stuff() Result<Int, Bool> {
					let res: Result<Int, Bool> = Result::ok(2)
					let num = try res
					res
				}`,
			output: &checker.Program{
				Imports: map[string]checker.Module{},
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name:       "do_stuff",
							Parameters: []checker.Parameter{},
							ReturnType: checker.MakeResult(checker.Int, checker.Bool),
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{
										Stmt: &checker.VariableDef{
											Name: "res",
											Value: &checker.ModuleFunctionCall{
												Module: "ard/result",
												Call: &checker.FunctionCall{
													Name: "ok",
													Args: []checker.Expression{&checker.IntLiteral{2}},
												},
											},
										},
									},
									{
										Stmt: &checker.VariableDef{
											Name:  "num",
											Value: &checker.TryOp{},
										},
									},
									{
										Expr: &checker.Variable{},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "the function return type must match the result",
			input: `
				fn do_stuff() Result<Str, Bool> {
					let res: Result<Int, Bool> = Result::ok(2)
					let num = try res
					Result::ok(num.to_str())
				}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Result<Str, Bool>, got Result<Int, Bool>"},
			},
		},
		{
			name: "try can only be used in functions",
			input: `
					let res: Result<Int, Bool> = Result::ok(2)
					let num = try res
				`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "The `try` keyword can only be used in a function body"},
			},
		},
	})
}
