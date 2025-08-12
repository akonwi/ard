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
			fn divide(a: Int, b: Int) Int!Str {
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
			let result: Int!Str = Result::ok(true)
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: `Type mismatch: Expected Int, got Bool`},
			},
		},
		{
			name: "Results must match return declaration",
			input: `
			fn foo() Int!Str {
				Result::err(true)
			}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: `Type mismatch: Expected Int!Str, got $Val!Bool`},
			},
		},
		{
			name: "Result.or() unwraps with a default",
			input: `
			fn foo() Int {
				let res: Int!Str = Result::err("foo")
				res.or(10)
			}`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Matching on results",
			input: `
			use ard/io

			let res: Int!Str = Result::err("foo")
			match res {
				ok(num) => num,
				err => {
				  io::print("failed: " + err)
					-1
				}
			}`,
			output: &checker.Program{
				Imports: map[string]checker.Module{
					"ard/io": &checker.UserModule{}, // UserModule from embedded system
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
			name: "trying a result with compatible error types",
			input: `
				fn do_stuff() Int!Bool {
					let res: Int!Bool = Result::ok(2)
					let num = try res
					Result::ok(num)
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
										Expr: &checker.ModuleFunctionCall{
											Module: "ard/result",
											Call: &checker.FunctionCall{
												Name: "ok",
												Args: []checker.Expression{&checker.Variable{}},
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
			name: "try works when error types match even if value types differ",
			input: `
				fn do_stuff() Str!Bool {
					let res: Int!Bool = Result::ok(2)
					let num = try res
					Result::ok(num.to_str())
				}`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "try can only be used in functions",
			input: `
					let res: Int!Bool = Result::ok(2)
					let num = try res
				`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "The `try` keyword can only be used in a function body"},
			},
		},
		{
			name: "try without catch requires Result return type",
			input: `
				fn test_func() Str {
					let res: Int!Str = Result::ok(42)
					let num = try res
					num.to_str()
				}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "try without catch clause requires function to return a Result type"},
			},
		},
		{
			name: "try-catch block must return compatible type",
			input: `
				fn test_func() Str {
					try Result::err("error") -> err {
						42
					}
				}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
		{
			name: "try-catch function must have correct signature",
			input: `
				fn bad_handler(code: Int) Str {
					"Error: {code}"
				}
				fn test_func() Str {
					try Result::err("error") -> bad_handler
				}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Void"},
			},
		},
		{
			name: "try-catch function must exist",
			input: `
				fn test_func() Str {
					try Result::err("error") -> nonexistent_func
				}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined function: nonexistent_func"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Void"},
			},
		},
		{
			name: "try can only be used on Result types",
			input: `
				fn test_func() Str {
					let val = "hello"
					try val
				}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "try can only be used on Result types, got: Str"},
			},
		},
		{
			name: "try with mismatched error types",
			input: `
				fn test_func() Int!Int {
					let res: Int!Str = Result::ok(42)
					let int = try res
					Result::ok(int)
				}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Error type mismatch: Expected Int, got Str"},
			},
		},
	})
}

func TestTryInMatchBlocks(t *testing.T) {
	run(t, []test{
		{
			name: "try works in enum match arms",
			input: `
				enum Status { active, inactive }
				
				fn get_result() Int!Str {
					Result::ok(42)
				}
				
				fn process_status(status: Status) Int!Str {
					match status {
						Status::active => {
							let value = try get_result()
							Result::ok(value + 1)
						}
						Status::inactive => Result::err("inactive")
					}
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "try works in maybe match arms",
			input: `
				fn get_result() Int!Str {
					Result::ok(42)
				}
				
				fn process_maybe(maybe_val: Int?) Int!Str {
					match maybe_val {
						val => {
							let result = try get_result()
							Result::ok(result + val)
						}
						_ => Result::err("no value")
					}
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "try with catch works in match arms",
			input: `
				fn risky_operation() Str!Str {
					Result::err("failed")
				}
				
				fn process_with_catch(flag: Bool) Str {
					match flag {
						true => {
							try risky_operation() -> err {
								"caught error: {err}"
							}
						}
						false => "no operation"
					}
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "try in nested match blocks",
			input: `
				enum Status { active, inactive }
				
				fn get_result() Int!Str {
					Result::ok(42)
				}
				
				fn process_nested(status: Status, maybe_val: Int?) Int!Str {
					match status {
						Status::active => {
							match maybe_val {
								val => {
									let result = try get_result()
									Result::ok(result + val)
								}
								_ => Result::err("no value")
							}
						}
						Status::inactive => Result::err("inactive")
					}
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "try still requires compatible return type in match arms",
			input: `
				fn get_result() Int!Bool {
					Result::ok(42)
				}
				
				fn wrong_error_type(flag: Bool) Int!Str {
					match flag {
						true => {
							let value = try get_result()  // Error: Bool vs Str mismatch
							Result::ok(value)
						}
						false => Result::ok(0)
					}
				}
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Error type mismatch: Expected Str, got Bool"},
			},
		},
	})
}
