package ast

import (
	"strings"
	"testing"
)

func TestListLiterals(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Empty list",
			input: "[]",
			output: Program{
				Imports:    []Import{},
				Statements: []Statement{&ListLiteral{Items: []Expression{}}},
			},
		},
		{
			name:  "List with items",
			input: "[1,2,3,]",
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&ListLiteral{Items: []Expression{
						&NumLiteral{Value: "1"},
						&NumLiteral{Value: "2"},
						&NumLiteral{Value: "3"},
					}},
				},
			},
		},
	})
}

func TestMapLiterals(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Empty map",
			input: "[:]",
			output: Program{
				Imports:    []Import{},
				Statements: []Statement{&MapLiteral{Entries: []MapEntry{}}},
			},
		},
		{
			name:  "Map with entries",
			input: `[1:"one", 2:"two",]`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&MapLiteral{Entries: []MapEntry{
						{Key: &NumLiteral{Value: "1"}, Value: &StrLiteral{Value: "one"}},
						{Key: &NumLiteral{Value: "2"}, Value: &StrLiteral{Value: "two"}},
					}},
				},
			},
		},
	})
}

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
					&BinaryExpression{
						Operator: Plus,
						Left: &UnaryExpression{
							Operator: Minus,
							Operand: &NumLiteral{
								Value: `30`,
							},
						},
						Right: &NumLiteral{
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
					&BinaryExpression{
						Operator: Minus,
						Left: &NumLiteral{
							Value: `30`,
						},
						Right: &NumLiteral{
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
					&BinaryExpression{
						Operator: Divide,
						Left: &NumLiteral{
							Value: `30`,
						},
						Right: &NumLiteral{
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
					&BinaryExpression{
						Operator: Multiply,
						Left: &NumLiteral{
							Value: `30`,
						},
						Right: &NumLiteral{
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
					&BinaryExpression{
						Operator: Modulo,
						Left: &NumLiteral{
							Value: `3`,
						},
						Right: &NumLiteral{
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
					&BinaryExpression{
						Operator: GreaterThan,
						Left: &NumLiteral{
							Value: `30`,
						},
						Right: &NumLiteral{
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
					&BinaryExpression{
						Operator: LessThan,
						Left: &NumLiteral{
							Value: `30`,
						},
						Right: &NumLiteral{
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
					&BinaryExpression{
						Operator: LessThanOrEqual,
						Left: &NumLiteral{
							Value: `30`,
						},
						Right: &NumLiteral{
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
					&BinaryExpression{
						Operator: Equal,
						Left: &StrLiteral{
							Value: "Joe",
						},
						Right: &StrLiteral{
							Value: "Joe",
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
					&BinaryExpression{
						Operator: Equal,
						Left: &NumLiteral{
							Value: `1`,
						},
						Right: &NumLiteral{
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
					&BinaryExpression{
						Operator: Equal,
						Left: &BoolLiteral{
							Value: true,
						},
						Right: &BoolLiteral{
							Value: false,
						},
					},
				},
			},
		},

		// Test cases for not (foo == bar)
		{
			name:  "Valid string inequality checks",
			input: `not "Joe" == "Joe"`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&UnaryExpression{
						Operator: Not,
						Operand: &BinaryExpression{
							Operator: Equal,
							Left: &StrLiteral{
								Value: "Joe",
							},
							Right: &StrLiteral{
								Value: "Joe",
							},
						}},
				},
			},
		},
		{
			name:  "Valid number inequality checks",
			input: `not 1 == 1`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&UnaryExpression{
						Operator: Not,
						Operand: &BinaryExpression{
							Operator: Equal,
							Left: &NumLiteral{
								Value: `1`,
							},
							Right: &NumLiteral{
								Value: `1`,
							},
						},
					},
				},
			},
		},
		{
			name:  "Valid boolean inequality checks",
			input: `not true == false`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&UnaryExpression{
						Operator: Not,
						Operand: &BinaryExpression{
							Operator: Equal,
							Left: &BoolLiteral{
								Value: true,
							},
							Right: &BoolLiteral{
								Value: false,
							},
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
					&BinaryExpression{
						Operator: And,
						Left: &BoolLiteral{
							Value: true,
						},
						Right: &BoolLiteral{
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
					&BinaryExpression{
						Operator: Or,
						Left: &BoolLiteral{
							Value: true,
						},
						Right: &BoolLiteral{
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
					&RangeExpression{
						Start: &NumLiteral{
							Value: `1`,
						},
						End: &NumLiteral{
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
			name:  "Parenthesized expression",
			input: `(30 + 20) * 2`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&BinaryExpression{
						Operator: Multiply,
						Left: &BinaryExpression{
							Operator: Plus,
							Left: &NumLiteral{
								Value: `30`,
							},
							Right: &NumLiteral{
								Value: `20`,
							},
						},
						Right: &NumLiteral{
							Value: `2`,
						},
					},
				},
			},
		},
		{
			name:  "Multiplication precedence",
			input: `30 + 20 * x`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&BinaryExpression{
						Operator: Plus,
						Left:     &NumLiteral{Value: `30`},
						Right: &BinaryExpression{
							Operator: Multiply,
							Left:     &NumLiteral{Value: `20`},
							Right:    &Identifier{Name: "x"},
						},
					},
				},
			},
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
				`100.to_str()`,
			}, "\n"),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&InstanceProperty{
						Target: &StrLiteral{
							Value: "string",
						},
						Property: Identifier{
							Name: "size",
						},
					},
					&InstanceMethod{
						Target: &StrLiteral{
							Value: "string",
						},
						Method: FunctionCall{
							Name: "at",
							Args: []Expression{&NumLiteral{Value: "0"}},
						},
					},
					&InstanceProperty{
						Target: &Identifier{
							Name: "some_string",
						},
						Property: Identifier{
							Name: "size",
						},
					},
					&InstanceMethod{
						Target: &Identifier{
							Name: "some_string",
						},
						Method: FunctionCall{
							Name: "at",
							Args: []Expression{&NumLiteral{Value: "0"}},
						},
					},
					&InstanceProperty{
						Target: &InstanceMethod{
							Target: &Identifier{
								Name: "name",
							},
							Method: FunctionCall{
								Name: "take",
								Args: []Expression{&NumLiteral{Value: "3"}},
							},
						},
						Property: Identifier{Name: "size"},
					},
					&InstanceMethod{
						Target: &NumLiteral{
							Value: "100",
						},
						Method: FunctionCall{
							Name: "to_str",
							Args: []Expression{},
						},
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
					&StaticProperty{
						Target:   &Identifier{Name: "Color"},
						Property: Identifier{Name: "blue"},
					},
				},
			},
		},
	})
}

func TestInterpolatedStrings(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Interpolated string",
			input: `"Hello, {name}"`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&InterpolatedStr{
						Chunks: []Expression{
							&StrLiteral{Value: "Hello, "},
							&Identifier{Name: "name"},
						},
					},
				},
			},
		},
	})
}

func TestAndOrs(t *testing.T) {
	runTests(t, []test{
		{
			name:  "And",
			input: `true and false`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&BinaryExpression{
						Operator: And,
						Left:     &BoolLiteral{Value: true},
						Right:    &BoolLiteral{Value: false},
					},
				},
			},
		},
		{
			name:  "Or",
			input: `true or false`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&BinaryExpression{
						Operator: Or,
						Left:     &BoolLiteral{Value: true},
						Right:    &BoolLiteral{Value: false},
					},
				},
			},
		},
		{
			name:  "Not or not",
			input: `(not true) or (not false)`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&BinaryExpression{
						Operator: Or,
						Left:     &UnaryExpression{Operator: Not, Operand: &BoolLiteral{Value: true}},
						Right:    &UnaryExpression{Operator: Not, Operand: &BoolLiteral{Value: false}},
					},
				},
			},
		},
	})
}
