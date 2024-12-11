package ast

import (
	"testing"
)

func TestUnaryExpressions(t *testing.T) {
	tests := []test{
		{
			name:  "Valid negation",
			input: `let negative_number = -30`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					VariableDeclaration{
						Name:    "negative_number",
						Mutable: false,
						Type:    NumberType{},
						Value: UnaryExpression{
							Operator: Minus,
							Operand: NumLiteral{
								Value: `30`,
							}},
					},
				},
			},
			diagnostics: []Diagnostic{},
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
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid subtraction",
			input: `30 - 12`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid division",
			input: `30 / 6`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid multiplication",
			input: `30 * 10`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid modulo",
			input: `3 % 9`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid greater than",
			input: `30 > 12`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid less than",
			input: `30 < 12`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid less than or equal",
			input: `30 <= 12`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid string equality checks",
			input: `"Joe" == "Joe"`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid number equality checks",
			input: `1 == 1`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid boolean equality checks",
			input: `true == false`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},

		// Test cases for the '!=' operator
		{
			name:  "Valid string inequality checks",
			input: `"Joe" != "Joe"`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid number inequality checks",
			input: `1 != 1`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid boolean inequality checks",
			input: `true != false`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		// logic operator checks
		{
			name:  "Valid use of 'and' operator",
			input: `true and false`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Valid use of 'or' operator",
			input: `true or false`,
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},

		// range operator
		{
			name:  "Valid use of range operator",
			input: "1..10",
			output: Program{
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
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
				Imports: []Import{},
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
			diagnostics: []Diagnostic{},
		},
		{
			name:  "Invalid parenthesized expression",
			input: `30 + (20 * "fizz")`,
			diagnostics: []Diagnostic{
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
				Imports: []Import{},
				Statements: []Statement{
					MemberAccess{
						Target: StrLiteral{
							Value: `"string"`,
						},
						AccessType: Instance,
						Member: Identifier{
							Name: "size",
							Type: NumType,
						},
					},
				},
			},
		},
	})
}
