package checker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/akonwi/ard/internal/ast"
	ts_ard "github.com/akonwi/tree-sitter-ard/bindings/go"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	ts "github.com/tree-sitter/go-tree-sitter"
)

var tsParser *ts.Parser

func init() {
	_tsParser, err := ts_ard.MakeParser()
	if err != nil {
		panic(err)
	}
	tsParser = _tsParser
}

type test struct {
	name        string
	input       string
	output      Program
	diagnostics []Diagnostic
}

var compareOptions = cmp.Options{
	cmpopts.SortMaps(func(a, b string) bool { return a < b }),
	cmpopts.IgnoreUnexported(Identifier{}),
}

func run(t *testing.T, tests []test) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := []byte(tt.input)
			tree := tsParser.Parse(source, nil)
			parser := ast.NewParser(source, tree)
			ast, err := parser.Parse()
			if err != nil {
				t.Fatal(fmt.Errorf("Error parsing input: %v", err))
			}
			program, diagnostics := Check(ast)
			if len(tt.output.Imports) > 0 {
				if diff := cmp.Diff(tt.output.Imports, program.Imports, compareOptions); diff != "" {
					t.Errorf("Program imports mismatch (-want +got):\n%s", diff)
				}
			}
			if len(tt.output.Statements) > 0 {
				if diff := cmp.Diff(tt.output.Statements, program.Statements, compareOptions); diff != "" {
					t.Errorf("Program statements mismatch (-want +got):\n%s", diff)
				}
			}

			if len(tt.diagnostics) > 0 || len(diagnostics) > 0 {
				if diff := cmp.Diff(tt.diagnostics, diagnostics, compareOptions); diff != "" {
					t.Errorf("Diagnostics mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestImports(t *testing.T) {
	run(t, []test{
		{
			name: "importing modules",
			input: strings.Join([]string{
				`use io/fs`,
				`use github.com/google/go-cmp/cmp`,
				`use github.com/tree-sitter/tree-sitter as ts`,
			}, "\n"),
			output: Program{
				Imports: map[string]Package{
					"fs": {
						Path: "io/fs",
					},
					"cmp": {
						Path: "github.com/google/go-cmp/cmp",
					},
					"ts": {
						Path: "github.com/tree-sitter/tree-sitter",
					},
				},
			},
		},
		{
			name: "name collisions are caught",
			input: strings.Join([]string{
				`use std/fs`,
				`use my/files as fs`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "[2:1] Duplicate package name: fs"},
			},
		},
	})
}

func TestLiterals(t *testing.T) {
	run(t, []test{
		{
			name: "primitive literals",
			input: strings.Join([]string{
				`"hello"`,
				"42",
				"false",
			}, "\n"),
			output: Program{
				Statements: []Statement{
					StrLiteral{
						Value: "hello",
					},
					NumLiteral{
						Value: 42,
					},
					BoolLiteral{
						Value: false,
					},
				},
			},
		},
		{
			name: "interpolated strings",
			input: strings.Join([]string{
				`let name = "world"`,
				`"Hello, {{name}}"`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Name:  "name",
						Value: StrLiteral{Value: "world"},
					},
					InterpolatedStr{
						Parts: []Expression{
							StrLiteral{Value: "Hello, "},
							Identifier{Name: "name"},
						},
					},
				},
			},
		},
	})
}

func TestVariables(t *testing.T) {
	run(t, []test{
		{
			name: "Declared types",
			input: strings.Join([]string{
				`let name: Str = "Alice"`,
				"let age: Num = 32",
				"let is_student: Bool = true`",
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Name:  "name",
						Value: StrLiteral{Value: "Alice"},
					},
					VariableBinding{
						Name:  "age",
						Value: NumLiteral{Value: 32},
					},
					VariableBinding{
						Name:  "is_student",
						Value: BoolLiteral{Value: true},
					},
				},
			},
		},
		{
			name: "Actual types should match declarations",
			input: strings.Join([]string{
				`let name: Str = "Alice"`,
				`let age: Num = "32"`,
				`let is_student: Bool = true`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "[2:16] Type mismatch: Expected Num, got Str"},
			},
		},
		{
			name: "Only mutable variables can be reassigned",
			input: strings.Join([]string{
				`let name: Str = "Alice"`,
				`name = "Bob"`,
				`mut other_name = "Bob"`,
				`other_name = "joe"`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "[2:1] Immutable variable: name"},
			},
		},
		{
			name:  "Reassigning types must match",
			input: `mut name = "Bob"` + "\n" + `name = 0`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "[2:8] Type mismatch: Expected Str, got Num"},
			},
		},
		{
			name:  "Cannot reassign undeclared variables",
			input: `name = "Bob"`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "[1:1] Undefined: name"},
			},
		},
		{
			name:  "Valid reassigments",
			input: `mut count = 0` + "\n" + `count = 1`,
			output: Program{
				Statements: []Statement{
					VariableBinding{Name: "count", Value: NumLiteral{Value: 0}},
					VariableAssignment{Name: "count", Value: NumLiteral{Value: 1}},
				},
			},
		},
		{
			name:  "Using variables",
			input: `let string_1 = "Hello"` + "\n" + `let string_2 = string_1`,
			output: Program{
				Statements: []Statement{
					VariableBinding{Name: "string_1", Value: StrLiteral{Value: "Hello"}},
					VariableBinding{Name: "string_2", Value: Identifier{Name: "string_1"}},
				},
			},
		},
	})
}

func TestMemberAccess(t *testing.T) {
	run(t, []test{
		{
			name: "valid instance members",
			input: strings.Join([]string{
				`"foobar".size`,
				`let name = "Alice"`,
				`name.size`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					InstanceProperty{
						Subject:  StrLiteral{Value: "foobar"},
						Property: Identifier{Name: "size"},
					},
					VariableBinding{
						Name:  "name",
						Value: StrLiteral{Value: "Alice"},
					},
					InstanceProperty{
						Subject:  Identifier{Name: "name"},
						Property: Identifier{Name: "size"},
					},
				},
			},
		},
		{
			name: "Undefined instance members",
			input: strings.Join([]string{
				`"foo".length`,
				`let name = "joe"`,
				`name.len`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "[1:7] Undefined: \"foo\".length"},
				{Kind: Error, Message: "[3:6] Undefined: name.len"},
			},
		},
	})
}

func TestUnaryExpressions(t *testing.T) {
	run(t, []test{
		{
			name:  "Negative numbers",
			input: `-10`,
			output: Program{
				Statements: []Statement{
					Negation{Value: NumLiteral{Value: 10}},
				},
			},
		},
		{
			name:  "Minus operator must be on numbers",
			input: `-true`,
			diagnostics: []Diagnostic{
				{
					Kind:    Error,
					Message: "[1:2] The '-' operator can only be used on numbers",
				},
			},
		},
		{
			name:  "Boolean negation",
			input: `!true`,
			output: Program{
				Statements: []Statement{
					Not{Value: BoolLiteral{Value: true}},
				},
			},
		},
		{
			name:  "Bang operator must be on booleans",
			input: `!"string"`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "[1:2] The '!' operator can only be used on booleans"},
			},
		},
	})
}

func TestNumberOperations(t *testing.T) {
	cases := []struct {
		name string
		op   BinaryOperator
	}{
		{"Addition", Add},
		{"Subtraction", Sub},
		{"Multiplication", Mul},
		{"Division", Div},
		{"Modulo", Mod},
		{"Greater than", GreaterThan},
		{"Greater than or equal", GreaterThanOrEqual},
		{"Less than", LessThan},
		{"Less than or equal", LessThanOrEqual},
	}
	tests := []test{}
	for _, c := range cases {
		tests = append(tests, test{
			name:  c.name,
			input: fmt.Sprintf("1 %s 2", c.op) + "\n" + fmt.Sprintf("3 %s -4", c.op),
			output: Program{
				Statements: []Statement{
					BinaryExpr{
						Op:    c.op,
						Left:  NumLiteral{Value: 1},
						Right: NumLiteral{Value: 2},
					},
					BinaryExpr{
						Op:    c.op,
						Left:  NumLiteral{Value: 3},
						Right: Negation{Value: NumLiteral{Value: 4}},
					},
				},
			},
		},
			test{
				name:  c.name + " with wrong types",
				input: fmt.Sprintf("1 %s true", c.op),
				diagnostics: []Diagnostic{
					{Kind: Error, Message: fmt.Sprintf("[1:1] Invalid operation: Num %s Bool", c.op)},
				},
			})
	}

	run(t, tests)
}

func TestEqualityComparisons(t *testing.T) {
	run(t, []test{
		{
			name: "Equality between primitives",
			input: strings.Join([]string{
				"1 == 2",
				"1 != 2",
				"true == false",
				"true != false",
				`"hello" == "world"`,
				`"hello" != "world"`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					BinaryExpr{
						Op:    Equal,
						Left:  NumLiteral{Value: 1},
						Right: NumLiteral{Value: 2},
					},
					BinaryExpr{
						Op:    NotEqual,
						Left:  NumLiteral{Value: 1},
						Right: NumLiteral{Value: 2},
					},
					BinaryExpr{
						Op:    Equal,
						Left:  BoolLiteral{Value: true},
						Right: BoolLiteral{Value: false},
					},
					BinaryExpr{
						Op:    NotEqual,
						Left:  BoolLiteral{Value: true},
						Right: BoolLiteral{Value: false},
					},
					BinaryExpr{
						Op:    Equal,
						Left:  StrLiteral{Value: "hello"},
						Right: StrLiteral{Value: "world"},
					},
					BinaryExpr{
						Op:    NotEqual,
						Left:  StrLiteral{Value: "hello"},
						Right: StrLiteral{Value: "world"},
					},
				},
			},
		},
	})
}

func TestBooleanOperations(t *testing.T) {
	run(t, []test{
		{
			name:  "Boolean operations",
			input: "let never = true and false" + "\n" + "let always = true or false",
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Name: "never",
						Value: BinaryExpr{
							Op:    And,
							Left:  BoolLiteral{Value: true},
							Right: BoolLiteral{Value: false},
						},
					},
					VariableBinding{
						Name: "always",
						Value: BinaryExpr{
							Op:    Or,
							Left:  BoolLiteral{Value: true},
							Right: BoolLiteral{Value: false},
						},
					},
				},
			},
		},
	})
}

func TestParenthesizedExpressions(t *testing.T) {
	run(t, []test{
		{
			name:  "arithmatic",
			input: "(30 + 20) * 4",
			output: Program{
				Statements: []Statement{
					BinaryExpr{
						Op: Mul,
						Left: BinaryExpr{
							Op:    Add,
							Left:  NumLiteral{Value: 30},
							Right: NumLiteral{Value: 20},
						},
						Right: NumLiteral{Value: 4},
					},
				},
			},
		},
		{
			name:  "logical",
			input: "(true and true) or (true and false)",
			output: Program{
				Statements: []Statement{
					BinaryExpr{
						Op: Or,
						Left: BinaryExpr{
							Op:    And,
							Left:  BoolLiteral{Value: true},
							Right: BoolLiteral{Value: true},
						},
						Right: BinaryExpr{
							Op:    And,
							Left:  BoolLiteral{Value: true},
							Right: BoolLiteral{Value: false},
						},
					},
				},
			},
		},
	})
}

func TestIfStatements(t *testing.T) {
	run(t, []test{
		{
			name: "Simple if statement",
			input: strings.Join([]string{
				`let is_on = true`,
				`if is_on {
				  let foo = "bar"
				}`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{Name: "is_on", Value: BoolLiteral{Value: true}},
					IfStatement{
						Condition: Identifier{Name: "is_on"},
						Body: []Statement{
							VariableBinding{Name: "foo", Value: StrLiteral{Value: "bar"}},
						},
					},
				},
			},
		},
		{
			name: "The condition expression must be a boolean",
			input: strings.Join([]string{
				`if 20 {
				  let foo = "bar"
				}`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "[1:4] If conditions must be boolean expressions"},
			},
		},
		{
			name: "Compound conditions",
			input: strings.Join([]string{
				`let is_on = true`,
				`if is_on and 100 > 30 {
				  let foo = "bar"
				}`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{Name: "is_on", Value: BoolLiteral{Value: true}},
					IfStatement{
						Condition: BinaryExpr{
							Op:   And,
							Left: Identifier{Name: "is_on"},
							Right: BinaryExpr{
								Op:    GreaterThan,
								Left:  NumLiteral{Value: 100},
								Right: NumLiteral{Value: 30},
							},
						},
						Body: []Statement{
							VariableBinding{Name: "foo", Value: StrLiteral{Value: "bar"}},
						},
					},
				},
			},
		},
		{
			name: "With else clause",
			input: strings.Join([]string{
				`let is_on = true`,
				`if is_on {
				  let foo = "bar"
				} else {
				  let foo = "baz"
				}`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{Name: "is_on", Value: BoolLiteral{Value: true}},
					IfStatement{
						Condition: Identifier{Name: "is_on"},
						Body: []Statement{
							VariableBinding{Name: "foo", Value: StrLiteral{Value: "bar"}},
						},
						Else: IfStatement{
							Body: []Statement{
								VariableBinding{Name: "foo", Value: StrLiteral{Value: "baz"}},
							},
						},
					},
				},
			},
		},
		{
			name: "With else-if clause",
			input: strings.Join([]string{
				`let is_on = true`,
				`if is_on {
				  let foo = "bar"
				} else if 1 > 2 {
				  let foo = "baz"
				}`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{Name: "is_on", Value: BoolLiteral{Value: true}},
					IfStatement{
						Condition: Identifier{Name: "is_on"},
						Body: []Statement{
							VariableBinding{Name: "foo", Value: StrLiteral{Value: "bar"}},
						},
						Else: IfStatement{
							Condition: BinaryExpr{
								Op:    GreaterThan,
								Left:  NumLiteral{Value: 1},
								Right: NumLiteral{Value: 2},
							},
							Body: []Statement{
								VariableBinding{Name: "foo", Value: StrLiteral{Value: "baz"}},
							},
						},
					},
				},
			},
		},
		{
			name: "if-else-if-else",
			input: strings.Join([]string{
				`let is_on = true`,
				`if is_on {
				  let foo = "bar"
				} else if 1 > 2 {
				  let foo = "baz"
				} else {
					let foo = "qux"
				}`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{Name: "is_on", Value: BoolLiteral{Value: true}},
					IfStatement{
						Condition: Identifier{Name: "is_on"},
						Body: []Statement{
							VariableBinding{Name: "foo", Value: StrLiteral{Value: "bar"}},
						},
						Else: IfStatement{
							Condition: BinaryExpr{
								Op:    GreaterThan,
								Left:  NumLiteral{Value: 1},
								Right: NumLiteral{Value: 2},
							},
							Body: []Statement{
								VariableBinding{Name: "foo", Value: StrLiteral{Value: "baz"}},
							},
							Else: IfStatement{
								Body: []Statement{
									VariableBinding{Name: "foo", Value: StrLiteral{Value: "qux"}},
								},
							},
						},
					},
				},
			},
		},
	})
}

func TestForLoops(t *testing.T) {
	run(t, []test{
		{
			name: "Iterating over a range",
			input: strings.Join([]string{
				`mut count = 0`,
				`for i in 1..10 {`,
				`  count = count + i`,
				`}`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{Name: "count", Value: NumLiteral{Value: 0}},
					ForRange{
						Cursor: Identifier{Name: "i"},
						Start:  NumLiteral{Value: 1},
						End:    NumLiteral{Value: 10},
						Body: []Statement{
							VariableAssignment{
								Name: "count",
								Value: BinaryExpr{
									Op:    Add,
									Left:  Identifier{Name: "count"},
									Right: Identifier{Name: "i"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "The range must be between numbers",
			input: strings.Join([]string{
				`mut count = 0`,
				`for i in 1..true {`,
				`  count = count + i`,
				`}`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "[2:10] Invalid range: Num..Bool"},
			},
		},
		{
			name: "Iterating over a string",
			input: strings.Join([]string{
				`let string = "hello"`,
				`for c in string {`,
				`  c`,
				`}`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{Name: "string", Value: StrLiteral{Value: "hello"}},
					ForIn{
						Cursor:   Identifier{Name: "c"},
						Iterable: Identifier{Name: "string"},
						Body: []Statement{
							Identifier{Name: "c"},
						},
					},
				},
			},
		},
		{
			name: "Iterating up to a number, is sugar for 0..n",
			input: strings.Join([]string{
				`for i in 20 {`,
				`  i`,
				`}`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					ForRange{
						Cursor: Identifier{Name: "i"},
						Start:  NumLiteral{Value: 0},
						End:    NumLiteral{Value: 20},
						Body: []Statement{
							Identifier{Name: "i"},
						},
					},
				},
			},
		},
		{
			name:  "Cannot iterate over a boolean",
			input: `for b in false {}`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "[1:10] Cannot iterate over a Bool"},
			},
		},
	})
}
