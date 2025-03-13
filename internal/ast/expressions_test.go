package ast

import (
	"strings"
	"testing"
)

func TestUnaryExpressions(t *testing.T) {
	tests := []test{
		{
			name:  "Valid numeric negation",
			input: `let negative_number = -30`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name:    "negative_number",
						Mutable: false,
						Value: &UnaryExpression{
							Operator: Minus,
							Operand: &NumLiteral{
								Value: `30`,
							}},
					},
				},
			},
		},
		{
			name:  "Valid boolean negation",
			input: `let nope = not true`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name:    "nope",
						Mutable: false,
						Value: &UnaryExpression{
							Operator: Not,
							Operand: &BoolLiteral{
								Value: true,
							},
						},
					},
				},
			},
		},
	}

	runTestsV2(t, tests)
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
		},
		{
			name:  "Invalid parenthesized expression",
			input: `30 + (20 * "fizz")`,
		},
	}

	runTests(t, tests)
}

func TestMemberAccess(t *testing.T) {
	runTests(t, []test{
		{
			name: "Accessing instance members",
			input: strings.Join([]string{
				`"string".size`,
				`"string".at(0)`,
				`some_string.size`,
				`some_string.at(0)`,
				"name.take(3).size",
			}, "\n"),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					InstanceProperty{
						Target: StrLiteral{
							Value: `"string"`,
						},
						Property: Identifier{
							Name: "size",
						},
					},
					InstanceMethod{
						Target: StrLiteral{
							Value: `"string"`,
						},
						Method: FunctionCall{
							Name: "at",
							Args: []Expression{NumLiteral{Value: "0"}},
						},
					},
					InstanceProperty{
						Target: Identifier{
							Name: "some_string",
						},
						Property: Identifier{
							Name: "size",
						},
					},
					InstanceMethod{
						Target: Identifier{
							Name: "some_string",
						},
						Method: FunctionCall{
							Name: "at",
							Args: []Expression{NumLiteral{Value: "0"}},
						},
					},
					InstanceProperty{
						Target: InstanceMethod{
							Target: Identifier{
								Name: "name",
							},
							Method: FunctionCall{
								Name: "take",
								Args: []Expression{NumLiteral{Value: "3"}},
							},
						},
						Property: Identifier{Name: "size"},
					},
				},
			},
		},
		{
			name:  "Accessing static members",
			input: strings.Join([]string{"Color::blue"}, "\n"),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					StaticProperty{
						Target:   Identifier{Name: "Color"},
						Property: Identifier{Name: "blue"},
					},
				},
			},
		},
	})
}
