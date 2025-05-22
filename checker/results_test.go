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
														Expr: &checker.PackageFunctionCall{
															Package: "ard/result",
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
														Expr: &checker.PackageFunctionCall{
															Package: "ard/result",
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
	})
}
