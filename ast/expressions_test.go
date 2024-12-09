package ast

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestUnaryExpressions(t *testing.T) {
	tests := []test{
		{
			name:  "Valid negation",
			input: `let negative_number = -30`,
			output: Program{
				Statements: []Statement{
					VariableDeclaration{
						Name:    "negative_number",
						Mutable: false,
						Type:    checker.NumType,
						Value: UnaryExpression{
							Operator: Minus,
							Operand: NumLiteral{
								Value: `30`,
							}},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid negation",
			input: `-false`,
			output: Program{
				Statements: []Statement{
					UnaryExpression{
						Operator: Minus,
						Operand: BoolLiteral{
							Value: false,
						}},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '-' operator can only be used on 'Num'",
				},
			},
		},
	}

	runTests(t, tests)
}

func TestBinaryExpressions(t *testing.T) {
	tests := []test{
		{
			name:  "Valid addition",
			input: `-30 + 20`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Plus,
						Left: UnaryExpression{
							Operator: Minus,
							Operand: NumLiteral{
								Value: `30`,
							},
						},
						Right: NumLiteral{
							Value: "20",
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid addition",
			input: `30 + "f12"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Plus,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: StrLiteral{
							Value: `"f12"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '+' operator can only be used between instances of 'Num'",
				},
			},
		},
		{
			name:  "+ operator is only allowed on Num",
			input: `"foo" + "bar"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Plus,
						Left: StrLiteral{
							Value: `"foo"`,
						},
						Right: StrLiteral{
							Value: `"bar"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '+' operator can only be used between instances of 'Num'",
				},
			},
		},
		{
			name:  "Valid subtraction",
			input: `30 - 12`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Minus,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: NumLiteral{
							Value: `12`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid subtraction",
			input: `30 - "f12"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Minus,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: StrLiteral{
							Value: `"f12"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '-' operator can only be used between instances of 'Num'",
				},
			},
		},
		{
			name:  "Valid division",
			input: `30 / 6`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Divide,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: NumLiteral{
							Value: `6`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid division",
			input: `30 / "f12"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Divide,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: StrLiteral{
							Value: `"f12"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '/' operator can only be used between instances of 'Num'",
				},
			},
		},
		{
			name:  "Valid multiplication",
			input: `30 * 10`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Multiply,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: NumLiteral{
							Value: `10`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid multiplication",
			input: `30 * "f12"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Multiply,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: StrLiteral{
							Value: `"f12"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '*' operator can only be used between instances of 'Num'",
				},
			},
		},
		{
			name:  "Valid modulo",
			input: `3 % 9`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Modulo,
						Left: NumLiteral{
							Value: `3`,
						},
						Right: NumLiteral{
							Value: `9`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid modulo",
			input: `30 % "f12"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Modulo,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: StrLiteral{
							Value: `"f12"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '%' operator can only be used between instances of 'Num'",
				},
			},
		},
		{
			name:  "Valid greater than",
			input: `30 > 12`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: GreaterThan,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: NumLiteral{
							Value: `12`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid greater than",
			input: `30 > "f12"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: GreaterThan,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: StrLiteral{
							Value: `"f12"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '>' operator can only be used between instances of 'Num'",
				},
			},
		},
		{
			name:  "Valid greater than or equal",
			input: `30 >= 12`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: GreaterThanOrEqual,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: NumLiteral{
							Value: `12`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid greater than or equal",
			input: `30 >= "f12"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: GreaterThanOrEqual,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: StrLiteral{
							Value: `"f12"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '>=' operator can only be used between instances of 'Num'",
				},
			},
		},
		{
			name:  "Valid less than",
			input: `30 < 12`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: LessThan,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: NumLiteral{
							Value: `12`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid les than",
			input: `30 < "f12"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: LessThan,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: StrLiteral{
							Value: `"f12"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '<' operator can only be used between instances of 'Num'",
				},
			},
		},
		{
			name:  "Valid less than or equal",
			input: `30 <= 12`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: LessThanOrEqual,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: NumLiteral{
							Value: `12`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid less than or equal",
			input: `30 <= true`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: LessThanOrEqual,
						Left: NumLiteral{
							Value: `30`,
						},
						Right: BoolLiteral{
							Value: true,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '<=' operator can only be used between instances of 'Num'",
				},
			},
		},
		{
			name:  "Valid string equality checks",
			input: `"Joe" == "Joe"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Equal,
						Left: StrLiteral{
							Value: `"Joe"`,
						},
						Right: StrLiteral{
							Value: `"Joe"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid string equality check",
			input: `"Joe" == true`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Equal,
						Left: StrLiteral{
							Value: `"Joe"`,
						},
						Right: BoolLiteral{
							Value: true,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '==' operator can only be used between instances of 'Num', 'Str', or 'Bool'",
				},
			},
		},
		{
			name:  "Valid number equality checks",
			input: `1 == 1`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Equal,
						Left: NumLiteral{
							Value: `1`,
						},
						Right: NumLiteral{
							Value: `1`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid number equality checks",
			input: `1 == "eleventy"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Equal,
						Left: NumLiteral{
							Value: `1`,
						},
						Right: StrLiteral{
							Value: `"eleventy"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '==' operator can only be used between instances of 'Num', 'Str', or 'Bool'",
				},
			},
		},
		{
			name:  "Valid boolean equality checks",
			input: `true == false`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Equal,
						Left: BoolLiteral{
							Value: true,
						},
						Right: BoolLiteral{
							Value: false,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid boolean equality checks",
			input: `true == "eleventy"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Equal,
						Left: BoolLiteral{
							Value: true,
						},
						Right: StrLiteral{
							Value: `"eleventy"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '==' operator can only be used between instances of 'Num', 'Str', or 'Bool'",
				},
			},
		},

		// Test cases for the '!=' operator
		{
			name:  "Valid string inequality checks",
			input: `"Joe" != "Joe"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: NotEqual,
						Left: StrLiteral{
							Value: `"Joe"`,
						},
						Right: StrLiteral{
							Value: `"Joe"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid string inequality check",
			input: `"Joe" != true`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: NotEqual,
						Left: StrLiteral{
							Value: `"Joe"`,
						},
						Right: BoolLiteral{
							Value: true,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '!=' operator can only be used between instances of 'Num', 'Str', or 'Bool'",
				},
			},
		},
		{
			name:  "Valid number inequality checks",
			input: `1 != 1`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: NotEqual,
						Left: NumLiteral{
							Value: `1`,
						},
						Right: NumLiteral{
							Value: `1`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid number inequality checks",
			input: `1 != "eleventy"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: NotEqual,
						Left: NumLiteral{
							Value: `1`,
						},
						Right: StrLiteral{
							Value: `"eleventy"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '!=' operator can only be used between instances of 'Num', 'Str', or 'Bool'",
				},
			},
		},
		{
			name:  "Valid boolean inequality checks",
			input: `true != false`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: NotEqual,
						Left: BoolLiteral{
							Value: true,
						},
						Right: BoolLiteral{
							Value: false,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid boolean inequality checks",
			input: `true != "eleventy"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: NotEqual,
						Left: BoolLiteral{
							Value: true,
						},
						Right: StrLiteral{
							Value: `"eleventy"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '!=' operator can only be used between instances of 'Num', 'Str', or 'Bool'",
				},
			},
		},

		// logic operator checks
		{
			name:  "Valid use of 'and' operator",
			input: `true and false`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: And,
						Left: BoolLiteral{
							Value: true,
						},
						Right: BoolLiteral{
							Value: false,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Ivalid use of 'and' operator",
			input: `true and "eleventy"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: And,
						Left: BoolLiteral{
							Value: true,
						},
						Right: StrLiteral{
							Value: `"eleventy"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The 'and' operator can only be used between instances of 'Bool'",
				},
			},
		},
		{
			name:  "Valid use of 'or' operator",
			input: `true or false`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Or,
						Left: BoolLiteral{
							Value: true,
						},
						Right: BoolLiteral{
							Value: false,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Ivalid use of 'or' operator",
			input: `true or "eleventy"`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						Operator: Or,
						Left: BoolLiteral{
							Value: true,
						},
						Right: StrLiteral{
							Value: `"eleventy"`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The 'or' operator can only be used between instances of 'Bool'",
				},
			},
		},

		// range operator
		{
			name:  "Valid use of range operator",
			input: "1..10",
			output: Program{
				Statements: []Statement{
					RangeExpression{
						Start: NumLiteral{
							Value: `1`,
						},
						End: NumLiteral{
							Value: `10`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid use of range operator",
			input: `"fizz"..10`,
			output: Program{
				Statements: []Statement{
					RangeExpression{
						Start: StrLiteral{
							Value: `"fizz"`,
						},
						End: NumLiteral{
							Value: `10`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{{
				Msg: "A range must be between two Num",
			}},
		},
	}

	runTests(t, tests)
}

func TestParenthesizedExpressions(t *testing.T) {
	tests := []test{
		{
			name:  "Valid parenthesized expression",
			input: `(30 + 20) * 2`,
			output: Program{
				Statements: []Statement{
					BinaryExpression{
						HasPrecedence: false,
						Operator:      Multiply,
						Left: BinaryExpression{
							HasPrecedence: true,
							Operator:      Plus,
							Left: NumLiteral{
								Value: `30`,
							},
							Right: NumLiteral{
								Value: `20`,
							},
						},
						Right: NumLiteral{
							Value: `2`,
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "Invalid parenthesized expression",
			input: `30 + (20 * "fizz")`,
			diagnostics: []checker.Diagnostic{
				{
					Msg: "The '*' operator can only be used between instances of 'Num'",
				},
			},
		},
	}

	runTests(t, tests)
}

func TestMemberAccess(t *testing.T) {
	runTests(t, []test{
		{
			name:  "on string literals",
			input: `"string".size`,
			output: Program{
				Statements: []Statement{
					MemberAccess{
						Target: StrLiteral{
							Value: `"string"`,
						},
						AccessType: Instance,
						Member: Identifier{
							Name: "size",
							Type: checker.NumType,
						},
					},
				},
			},
		},
	})
}
