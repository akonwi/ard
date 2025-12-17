package checker_test

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/ast"
	checker "github.com/akonwi/ard/checker"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type test struct {
	name        string
	input       string
	output      *checker.Program
	diagnostics []checker.Diagnostic
}

var compareOptions = cmp.Options{
	cmpopts.SortMaps(func(a, b string) bool { return a < b }),
	cmpopts.IgnoreUnexported(
		checker.Any{},
		checker.Diagnostic{},
		checker.EnumVariant{},
		checker.ExternalFunctionDef{},
		checker.FiberExecution{},
		checker.FunctionCall{},
		checker.FunctionDef{},
		checker.Identifier{},
		checker.If{},
		checker.InstanceMethod{},
		checker.InstanceProperty{},
		checker.ListLiteral{},
		checker.List{},
		checker.MapLiteral{},
		checker.ModuleFunctionCall{},
		checker.ModuleStructInstance{},
		checker.ModuleSymbol{},
		checker.Result{},
		checker.Statement{},
		checker.StructInstance{},
		checker.TryOp{},
		checker.UserModule{},
		checker.Variable{},
		checker.VariableDef{},
		checker.And{},
		checker.BoolLiteral{},
		checker.BoolMatch{},
		checker.ConditionalMatch{},
		checker.CopyExpression{},
		checker.EnumMatch{},
		checker.Equality{},
		checker.FloatAddition{},
		checker.FloatDivision{},
		checker.FloatGreater{},
		checker.FloatGreaterEqual{},
		checker.FloatLess{},
		checker.FloatLessEqual{},
		checker.FloatLiteral{},
		checker.FloatMultiplication{},
		checker.FloatSubtraction{},
		checker.IntAddition{},
		checker.IntDivision{},
		checker.IntGreater{},
		checker.IntGreaterEqual{},
		checker.IntLess{},
		checker.IntLessEqual{},
		checker.IntLiteral{},
		checker.IntMatch{},
		checker.IntModulo{},
		checker.IntMultiplication{},
		checker.IntSubtraction{},
		checker.Negation{},
		checker.Not{},
		checker.OptionMatch{},
		checker.Or{},
		checker.Panic{},
		checker.ResultMatch{},
		checker.StrAddition{},
		checker.StrLiteral{},
		checker.TemplateStr{},
		checker.UnionMatch{},
		checker.VoidLiteral{},
	),
}

func run(t *testing.T, tests []test) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ast.Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("Parse errors: %v", result.Errors[0].Message)
			}
			ast := result.Program
			c := checker.New("test.ard", ast, nil)
			c.Check()
			if len(tt.diagnostics) > 0 || c.HasErrors() {
				if diff := cmp.Diff(tt.diagnostics, c.Diagnostics(), compareOptions); diff != "" {
					t.Fatalf("Diagnostics mismatch (-want +got):\n%s", diff)
				}
			}

			if tt.output != nil {
				if len(tt.output.Imports) > 0 {
					if diff := cmp.Diff(tt.output.Imports, c.Module().Program().Imports, compareOptions); diff != "" {
						t.Fatalf("Program imports mismatch (-want +got):\n%s", diff)
					}
				}
				if len(tt.output.Statements) > 0 {
					if diff := cmp.Diff(tt.output.Statements, c.Module().Program().Statements, compareOptions); diff != "" {
						t.Fatalf("Program statements mismatch (-want +got):\n%s", diff)
					}
				}
			}
		})
	}
}

func TestImports(t *testing.T) {
	run(t, []test{
		{
			name:  "importing modules",
			input: `use ard/io`,
			output: &checker.Program{
				Imports: map[string]checker.Module{},
			},
		},
		{
			name: "errors when importing unknowns from standard lib",
			input: strings.Join([]string{
				`use ard/foobar`,
			}, "\n"),
			output: &checker.Program{},
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Unknown module: ard/foobar",
				},
			},
		},
		{
			name: "name collisions are caught",
			input: strings.Join([]string{
				`use ard/fs`,
				`use ard/io as fs`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Warn, Message: "[2:1] Duplicate import: fs"},
			},
		},
	})
}

func TestPrimitiveLiterals(t *testing.T) {
	run(t, []test{
		{
			name: "primitive literals",
			input: strings.Join([]string{
				`"hello"`,
				"42",
				"24.8",
				"true",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.StrLiteral{Value: "hello"},
					},
					{Expr: &checker.IntLiteral{
						Value: 42,
					}},
					{Expr: &checker.FloatLiteral{Value: 24.8}},
					{Expr: &checker.BoolLiteral{
						Value: true,
					}},
				},
			},
		},
		{
			name: "interpolated strings",
			input: strings.Join([]string{
				`let name = "world"`,
				`"Hello, {name}"`,
				`"Hello, {3}"`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{Name: "name", Value: &checker.StrLiteral{Value: "world"}},
					},
					{
						Expr: &checker.TemplateStr{
							Chunks: []checker.Expression{
								&checker.StrLiteral{Value: "Hello, "},
								&checker.Variable{},
							},
						},
					},
					{
						Expr: &checker.TemplateStr{
							Chunks: []checker.Expression{
								&checker.StrLiteral{Value: "Hello, "},
								&checker.IntLiteral{Value: 3},
							},
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
				"let age: Int = 32",
				"let temp: Float = 98.6",
				"mut is_student: Bool = true",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "name",
							Value:   &checker.StrLiteral{Value: "Alice"},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "age",
							Value:   &checker.IntLiteral{Value: 32},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "temp",
							Value:   &checker.FloatLiteral{Value: 98.6},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "is_student",
							Value:   &checker.BoolLiteral{Value: true},
						},
					},
				},
			},
		},
		{
			name: "Inferred types",
			input: strings.Join([]string{
				`let name = "Alice"`,
				"let age = 32",
				"let temp = 98.6",
				"mut is_student = true",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "name",
							Value:   &checker.StrLiteral{Value: "Alice"},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "age",
							Value:   &checker.IntLiteral{Value: 32},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "temp",
							Value:   &checker.FloatLiteral{Value: 98.6},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "is_student",
							Value:   &checker.BoolLiteral{Value: true},
						},
					},
				},
			},
		},
		{
			name: "Actual types should match declarations",
			input: strings.Join([]string{
				`let name: Str = "Alice"`,
				`let age: Int = "32"`,
				`let is_student: Bool = true`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
			},
		},
		{
			name:  "Int literals are not inferred as Float",
			input: `let temp: Float = 98`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Float, got Int"},
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
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "name",
							Value:   &checker.StrLiteral{Value: "Alice"},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "other_name",
							Value:   &checker.StrLiteral{Value: "Bob"},
						},
					},
					{
						Stmt: &checker.Reassignment{
							Target: &checker.Variable{},
							Value:  &checker.StrLiteral{Value: "joe"},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Immutable variable: name"},
			},
		},
		{
			name:  "Reassigning types must match",
			input: strings.Join([]string{`mut name = "Bob"`, `name = 0`}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
		{
			name:  "Cannot reassign undeclared variables",
			input: `name = "Bob"`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined: name"},
			},
		},
		{
			name:  "Using variables",
			input: strings.Join([]string{`let string_1 = "Hello"`, `let string_2 = string_1`}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "string_1",
							Value:   &checker.StrLiteral{Value: "Hello"},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "string_2",
							Value:   &checker.Variable{},
						},
					},
				},
			},
		},
	})
}

func TestInstanceProperties(t *testing.T) {
	run(t, []test{
		{
			name: "valid instance members",
			input: strings.Join([]string{
				`"foobar".size`,
				`let name = "Alice"`,
				`name.size`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.InstanceProperty{
							Subject:  &checker.StrLiteral{Value: "foobar"},
							Property: "size",
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "name",
							Value:   &checker.StrLiteral{Value: "Alice"},
						},
					},
					{
						Expr: &checker.InstanceProperty{
							Subject:  &checker.Variable{},
							Property: "size",
						},
					},
				},
			},
		},
		{
			name: "Undefined instance members are errors",
			input: strings.Join([]string{
				`"foo".length`,
				`let name = "joe"`,
				`name.len`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: `Undefined: "foo".length`},
				{Kind: checker.Error, Message: "Undefined: name.len"},
			},
		},
	})
}

func TestUnaryExpressions(t *testing.T) {
	run(t, []test{
		{
			name: "Negative numbers",
			input: `(-10)
							(-10.0)`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{Expr: &checker.Negation{Value: &checker.IntLiteral{Value: 10}}},
					{Expr: &checker.Negation{Value: &checker.FloatLiteral{Value: 10.0}}},
				},
			},
		},
		{
			name:  "Minus operator must be on numbers",
			input: `-true`,
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Only numbers can be negated with '-'",
				},
			},
		},
		{
			name:  "Boolean negation",
			input: `not true`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.Not{Value: &checker.BoolLiteral{Value: true}},
					},
				},
			},
		},
		{
			name:  "Bang operator must be on booleans",
			input: `not "string"`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Only booleans can be negated with 'not'"},
			},
		},
	})
}

func TestIntMath(t *testing.T) {
	tests := []test{
		{
			name: "Adding Ints",
			input: strings.Join([]string{
				"1 + 2",
				"3 + -4",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.IntAddition{
							Left:  &checker.IntLiteral{Value: 1},
							Right: &checker.IntLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.IntAddition{
							Left:  &checker.IntLiteral{Value: 3},
							Right: &checker.Negation{Value: &checker.IntLiteral{Value: 4}},
						},
					},
				},
			},
		},
		{
			name: "Adding Floats",
			input: strings.Join([]string{
				"1.0 + 2.0",
				"3.0 + -4.5",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FloatAddition{
							Left:  &checker.FloatLiteral{Value: 1},
							Right: &checker.FloatLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.FloatAddition{
							Left:  &checker.FloatLiteral{Value: 3},
							Right: &checker.Negation{Value: &checker.FloatLiteral{Value: 4.5}},
						},
					},
				},
			},
		},
		{
			name: "Adding Strs",
			input: strings.Join([]string{
				`"hello" + "world"`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.StrAddition{
							Left:  &checker.StrLiteral{Value: "hello"},
							Right: &checker.StrLiteral{Value: "world"},
						},
					},
				},
			},
		},
		{
			name: "Subtracting Ints",
			input: strings.Join([]string{
				"1 - 2",
				"3 - -4",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.IntSubtraction{
							Left:  &checker.IntLiteral{Value: 1},
							Right: &checker.IntLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.IntSubtraction{
							Left:  &checker.IntLiteral{Value: 3},
							Right: &checker.Negation{Value: &checker.IntLiteral{Value: 4}},
						},
					},
				},
			},
		},
		{
			name: "Subtracting Floats",
			input: strings.Join([]string{
				"1.0 - 2.0",
				"3.0 - -4.5",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FloatSubtraction{
							Left:  &checker.FloatLiteral{Value: 1},
							Right: &checker.FloatLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.FloatSubtraction{
							Left:  &checker.FloatLiteral{Value: 3},
							Right: &checker.Negation{Value: &checker.FloatLiteral{Value: 4.5}},
						},
					},
				},
			},
		},
		{
			name: "Multiplying Ints",
			input: strings.Join([]string{
				"1 * 2",
				"3 * -4",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.IntMultiplication{
							Left:  &checker.IntLiteral{Value: 1},
							Right: &checker.IntLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.IntMultiplication{
							Left:  &checker.IntLiteral{Value: 3},
							Right: &checker.Negation{Value: &checker.IntLiteral{Value: 4}},
						},
					},
				},
			},
		},
		{
			name: "Multiplying Floats",
			input: strings.Join([]string{
				"1.0 * 2.0",
				"3.0 * -4.5",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FloatMultiplication{
							Left:  &checker.FloatLiteral{Value: 1},
							Right: &checker.FloatLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.FloatMultiplication{
							Left:  &checker.FloatLiteral{Value: 3},
							Right: &checker.Negation{Value: &checker.FloatLiteral{Value: 4.5}},
						},
					},
				},
			},
		},
		{
			name: "Dividing Ints",
			input: strings.Join([]string{
				"10 / 2",
				"15 / -3",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.IntDivision{
							Left:  &checker.IntLiteral{Value: 10},
							Right: &checker.IntLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.IntDivision{
							Left:  &checker.IntLiteral{Value: 15},
							Right: &checker.Negation{Value: &checker.IntLiteral{Value: 3}},
						},
					},
				},
			},
		},
		{
			name: "Dividing Floats",
			input: strings.Join([]string{
				"10.0 / 2.0",
				"15.0 / -3.0",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FloatDivision{
							Left:  &checker.FloatLiteral{Value: 10},
							Right: &checker.FloatLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.FloatDivision{
							Left:  &checker.FloatLiteral{Value: 15},
							Right: &checker.Negation{Value: &checker.FloatLiteral{Value: 3}},
						},
					},
				},
			},
		},
		{
			name: "Modulo Ints",
			input: strings.Join([]string{
				"10 % 3",
				"15 % -4",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.IntModulo{
							Left:  &checker.IntLiteral{Value: 10},
							Right: &checker.IntLiteral{Value: 3},
						},
					},
					{
						Expr: &checker.IntModulo{
							Left:  &checker.IntLiteral{Value: 15},
							Right: &checker.Negation{Value: &checker.IntLiteral{Value: 4}},
						},
					},
				},
			},
		},
		{
			name:  "Modulo Floats",
			input: "10.0 % 3.0",
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "The '%' operator can only be used for Int"},
			},
		},
		{
			name: "Greater than for Ints",
			input: strings.Join([]string{
				"1 > 2",
				"3 > -4",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.IntGreater{
							Left:  &checker.IntLiteral{Value: 1},
							Right: &checker.IntLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.IntGreater{
							Left:  &checker.IntLiteral{Value: 3},
							Right: &checker.Negation{Value: &checker.IntLiteral{Value: 4}},
						},
					},
				},
			},
		},
		{
			name: "Greater than or equal for Ints",
			input: strings.Join([]string{
				"1 >= 2",
				"3 >= -4",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.IntGreaterEqual{
							Left:  &checker.IntLiteral{Value: 1},
							Right: &checker.IntLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.IntGreaterEqual{
							Left:  &checker.IntLiteral{Value: 3},
							Right: &checker.Negation{Value: &checker.IntLiteral{Value: 4}},
						},
					},
				},
			},
		},
		{
			name: "Greater than for Floats",
			input: strings.Join([]string{
				"1.0 > 2.0",
				"3.0 > -4.5",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FloatGreater{
							Left:  &checker.FloatLiteral{Value: 1.0},
							Right: &checker.FloatLiteral{Value: 2.0},
						},
					},
					{
						Expr: &checker.FloatGreater{
							Left:  &checker.FloatLiteral{Value: 3.0},
							Right: &checker.Negation{Value: &checker.FloatLiteral{Value: 4.5}},
						},
					},
				},
			},
		},
		{
			name: "Greater than or equal for Floats",
			input: strings.Join([]string{
				"1.0 >= 2.0",
				"3.0 >= -4.5",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FloatGreaterEqual{
							Left:  &checker.FloatLiteral{Value: 1.0},
							Right: &checker.FloatLiteral{Value: 2.0},
						},
					},
					{
						Expr: &checker.FloatGreaterEqual{
							Left:  &checker.FloatLiteral{Value: 3.0},
							Right: &checker.Negation{Value: &checker.FloatLiteral{Value: 4.5}},
						},
					},
				},
			},
		},
		{
			name: "Less than for Ints",
			input: strings.Join([]string{
				"1 < 2",
				"3 < -4",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.IntLess{
							Left:  &checker.IntLiteral{Value: 1},
							Right: &checker.IntLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.IntLess{
							Left:  &checker.IntLiteral{Value: 3},
							Right: &checker.Negation{Value: &checker.IntLiteral{Value: 4}},
						},
					},
				},
			},
		},
		{
			name: "Less than or equal for Ints",
			input: strings.Join([]string{
				"1 <= 2",
				"3 <= -4",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.IntLessEqual{
							Left:  &checker.IntLiteral{Value: 1},
							Right: &checker.IntLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.IntLessEqual{
							Left:  &checker.IntLiteral{Value: 3},
							Right: &checker.Negation{Value: &checker.IntLiteral{Value: 4}},
						},
					},
				},
			},
		},
		{
			name: "Less than for Floats",
			input: strings.Join([]string{
				"1.0 < 2.0",
				"3.0 < -4.5",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FloatLess{
							Left:  &checker.FloatLiteral{Value: 1.0},
							Right: &checker.FloatLiteral{Value: 2.0},
						},
					},
					{
						Expr: &checker.FloatLess{
							Left:  &checker.FloatLiteral{Value: 3.0},
							Right: &checker.Negation{Value: &checker.FloatLiteral{Value: 4.5}},
						},
					},
				},
			},
		},
		{
			name: "Less than or equal for Floats",
			input: strings.Join([]string{
				"1.0 <= 2.0",
				"3.0 <= -4.5",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FloatLessEqual{
							Left:  &checker.FloatLiteral{Value: 1.0},
							Right: &checker.FloatLiteral{Value: 2.0},
						},
					},
					{
						Expr: &checker.FloatLessEqual{
							Left:  &checker.FloatLiteral{Value: 3.0},
							Right: &checker.Negation{Value: &checker.FloatLiteral{Value: 4.5}},
						},
					},
				},
			},
		},
	}

	run(t, tests)
}

func TestEqualityComparisons(t *testing.T) {
	run(t, []test{
		{
			name: "Equality between primitives",
			input: strings.Join([]string{
				"1 == 2",
				"10.2 == 21.4",
				"true == false",
				`"hello" == "world"`,
				`1 == false`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.Equality{
							Left:   &checker.IntLiteral{Value: 1},
							Right:  &checker.IntLiteral{Value: 2},
						},
					},
					{
						Expr: &checker.Equality{
							Left:   &checker.FloatLiteral{Value: 10.2},
							Right:  &checker.FloatLiteral{Value: 21.4},
						},
					},
					{
						Expr: &checker.Equality{
							Left:   &checker.BoolLiteral{Value: true},
							Right:  &checker.BoolLiteral{Value: false},
						},
					},
					{
						Expr: &checker.Equality{
							Left:   &checker.StrLiteral{Value: "hello"},
							Right:  &checker.StrLiteral{Value: "world"},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Invalid: Int == Bool"},
			},
		},
	})
}

func TestBooleanOperations(t *testing.T) {
	run(t, []test{
		{
			name: "Boolean operations",
			input: strings.Join([]string{
				"let never = true and false",
				"let always = true or false",
				"let invalid = 5 and true",
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{Stmt: &checker.VariableDef{
						Name: "never",
						Value: &checker.And{
							Left:  &checker.BoolLiteral{Value: true},
							Right: &checker.BoolLiteral{Value: false},
						},
					}},
					{Stmt: &checker.VariableDef{
						Name: "always",
						Value: &checker.Or{
							Left:  &checker.BoolLiteral{Value: true},
							Right: &checker.BoolLiteral{Value: false},
						},
					}},
				},
			},
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "The 'and' operator can only be used between Bools"},
			},
		},
	})
}

func TestParenthesizedExpressions(t *testing.T) {
	run(t, []test{
		{
			name:  "arithmatic",
			input: "(30 + 20) * 4",
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.IntMultiplication{
							Left:  &checker.IntAddition{Left: &checker.IntLiteral{Value: 30}, Right: &checker.IntLiteral{Value: 20}},
							Right: &checker.IntLiteral{Value: 4},
						},
					},
				},
			},
		},
		{
			name:  "logical",
			input: "(true and true) or (true and false)",
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.Or{
							Left:  &checker.And{Left: &checker.BoolLiteral{Value: true}, Right: &checker.BoolLiteral{Value: true}},
							Right: &checker.And{Left: &checker.BoolLiteral{Value: true}, Right: &checker.BoolLiteral{Value: false}},
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
				  "on"
				}`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: false,
							Name:    "is_on",
							Value:   &checker.BoolLiteral{Value: true},
						},
					},
					{
						Expr: &checker.If{
							Condition: &checker.Variable{},
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.StrLiteral{Value: "on"}},
								},
							},
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
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "If conditions must be boolean expressions"},
			},
		},
		{
			name: "Else clause",
			input: strings.Join([]string{
				`if true {
				  "bar"
				} else {
				  "baz"
				}`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.If{
							Condition: &checker.BoolLiteral{Value: true},
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.StrLiteral{Value: "bar"}},
								},
							},
							Else: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.StrLiteral{Value: "baz"}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Else If clause",
			input: strings.Join([]string{
				`if true {
				  "bar"
				} else if false {
				  "baz"
				} else {
				  "qux"
				}`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.If{
							Condition: &checker.BoolLiteral{Value: true},
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.StrLiteral{Value: "bar"}},
								},
							},
							ElseIf: &checker.If{
								Condition: &checker.BoolLiteral{Value: false},
								Body: &checker.Block{
									Stmts: []checker.Statement{
										{Expr: &checker.StrLiteral{Value: "baz"}},
									},
								},
							},
							Else: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.StrLiteral{Value: "qux"}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Branches must have consistent return type",
			input: strings.Join([]string{
				"if true {",
				"  1",
				"} else {",
				"  false",
				"}",
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "All branches must have the same result type"},
			},
		},
	})
}

func TestForLoops(t *testing.T) {
	run(t, []test{
		{
			name: "Iterating over a numeric range",
			input: strings.Join([]string{
				`mut count = 0`,
				`for i in 1..10 {`,
				`  count = count + i`,
				`}`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "count",
							Value:   &checker.IntLiteral{Value: 0},
						},
					},
					{
						Stmt: &checker.ForIntRange{
							Cursor: "i",
							Start:  &checker.IntLiteral{Value: 1},
							End:    &checker.IntLiteral{Value: 10},
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{
										Stmt: &checker.Reassignment{
											Target: &checker.Variable{},
											Value: &checker.IntAddition{
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
		{
			name: "The range must be between numbers",
			input: strings.Join([]string{
				`for i in 1..true {}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Invalid range: Int..Bool"},
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
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Name: "string",
							Value: &checker.StrLiteral{
								Value: "hello",
							},
						},
					},
					{Stmt: &checker.ForInStr{
						Cursor: "c",
						Value:  &checker.Variable{},
						Body: &checker.Block{
							Stmts: []checker.Statement{
								{Expr: &checker.Variable{}},
							},
						},
					}},
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
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.ForIntRange{
							Cursor: "i",
							Start:  &checker.IntLiteral{Value: 0},
							End:    &checker.IntLiteral{Value: 20},
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.Variable{}},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "Cannot iterate over a boolean",
			input: `for b in false {}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Cannot iterate over a Bool"},
			},
		},
	})
}

func TestLoopingOverMaps(t *testing.T) {
	run(t, []test{
		{
			name: "valid",
			input: `
			for key, val in ["hello":5, "world": 5] {
				"{key} = {val}"
			}`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.ForInMap{
							Key: "key",
							Val: "val",
							Map: &checker.MapLiteral{
								Keys:   []checker.Expression{&checker.StrLiteral{Value: "hello"}, &checker.StrLiteral{Value: "world"}},
								Values: []checker.Expression{&checker.IntLiteral{Value: 5}, &checker.IntLiteral{Value: 5}},
							},
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{
										Expr: &checker.TemplateStr{
											Chunks: []checker.Expression{
												&checker.StrLiteral{},
												&checker.Variable{},
												&checker.TemplateStr{Chunks: []checker.Expression{
													&checker.StrLiteral{Value: " = "},
													&checker.Variable{},
												}},
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

func TestTraditionalForLoop(t *testing.T) {
	run(t, []test{
		{
			name: "Basic C-style for loop",
			input: strings.Join([]string{
				`for mut i = 0; i < 10; i = i + 1 {`,
				`  i`,
				`}`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.ForLoop{
							Init: &checker.VariableDef{
								Mutable: true,
								Name:    "i",
								Value:   &checker.IntLiteral{Value: 0},
							},
							Condition: &checker.IntLess{
								Left:  &checker.Variable{},
								Right: &checker.IntLiteral{Value: 10},
							},
							Update: &checker.Reassignment{
								Target: &checker.Variable{},
								Value: &checker.IntAddition{
									Left:  &checker.Variable{},
									Right: &checker.IntLiteral{Value: 1},
								},
							},
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.Variable{}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "For loop condition must be boolean",
			input: strings.Join([]string{
				`for mut i = 0; i; i = i + 1 {}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "For loop condition must be a boolean expression"},
			},
		},
	})
}

func TestWhileLoops(t *testing.T) {
	run(t, []test{
		{
			name: "Simple condition",
			input: strings.Join([]string{
				`mut count = 10`,
				`while count > 0 {`,
				`  count = count - 1`,
				`}`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "count",
							Value:   &checker.IntLiteral{Value: 10},
						},
					},
					{
						Stmt: &checker.WhileLoop{
							Condition: &checker.IntGreater{
								Left:  &checker.Variable{},
								Right: &checker.IntLiteral{Value: 0},
							},
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{
										Stmt: &checker.Reassignment{
											Target: &checker.Variable{},
											Value: &checker.IntSubtraction{
												Left:  &checker.Variable{},
												Right: &checker.IntLiteral{Value: 1},
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
			name: "While loop condition must be boolean",
			input: strings.Join([]string{
				`while 42 {`,
				`  42`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "While loop condition must be a boolean expression"},
			},
		},
		{
			name: "Complex condition",
			input: strings.Join([]string{
				`mut i = 0`,
				`mut j = 10`,
				`while i < 5 and j > 0 {`,
				`  i = i + 1`,
				`}`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "i",
							Value:   &checker.IntLiteral{Value: 0},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "j",
							Value:   &checker.IntLiteral{Value: 10},
						},
					},
					{
						Stmt: &checker.WhileLoop{
							Condition: &checker.And{
								Left: &checker.IntLess{
									Left:  &checker.Variable{},
									Right: &checker.IntLiteral{Value: 5},
								},
								Right: &checker.IntGreater{
									Left:  &checker.Variable{},
									Right: &checker.IntLiteral{Value: 0},
								},
							},
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{
										Stmt: &checker.Reassignment{
											Target: &checker.Variable{},
											Value: &checker.IntAddition{
												Left:  &checker.Variable{},
												Right: &checker.IntLiteral{Value: 1},
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

func TestMaybes(t *testing.T) {
	run(t, []test{
		{
			name: "Declaring nullables",
			input: `
				use ard/maybe
				mut name: Str? = maybe::none()
				mut name2 = maybe::some("Bob")`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "name",
							Value: &checker.ModuleFunctionCall{
								Module: "ard/maybe",
								Call: &checker.FunctionCall{
									Name: "none",
									Args: []checker.Expression{},
								},
							},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "name2",
							Value: &checker.ModuleFunctionCall{
								Module: "ard/maybe",
								Call: &checker.FunctionCall{
									Name: "some",
									Args: []checker.Expression{&checker.StrLiteral{Value: "Bob"}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Reassigning with nullables",
			input: `
				use ard/maybe
				mut name: Str? = maybe::some("Joe")
				name = maybe::some("Bob")
			  name = "Alice"
				name = maybe::none()`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "name",
							Value: &checker.ModuleFunctionCall{
								Module: "ard/maybe",
								Call: &checker.FunctionCall{
									Name: "some",
									Args: []checker.Expression{&checker.StrLiteral{Value: "Joe"}},
								},
							},
						},
					},
					{
						Stmt: &checker.Reassignment{
							Target: &checker.Variable{},
							Value: &checker.ModuleFunctionCall{
								Module: "ard/maybe",
								Call: &checker.FunctionCall{
									Name: "some",
									Args: []checker.Expression{&checker.StrLiteral{Value: "Bob"}},
								},
							},
						},
					},
					{
						Stmt: &checker.Reassignment{
							Target: &checker.Variable{},
							Value: &checker.ModuleFunctionCall{
								Module: "ard/maybe",
								Call: &checker.FunctionCall{
									Name: "none",
									Args: []checker.Expression{},
								},
							},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str?, got Str"},
			},
		},
		{
			name: "Matching on maybes",
			input: `
				use ard/io
				use ard/maybe

				mut name: Str? = maybe::none()
				match name {
				  value => io::print("name is {value}"),
					_ => io::print("no name")
				}`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "name",
							Value: &checker.ModuleFunctionCall{
								Module: "ard/maybe",
								Call: &checker.FunctionCall{
									Name: "none",
									Args: []checker.Expression{},
								},
							},
						},
					},
					{
						Expr: &checker.OptionMatch{
							Subject: &checker.Variable{},
							Some: &checker.Match{
								Pattern: &checker.Identifier{Name: "value"},
								Body: &checker.Block{
									Stmts: []checker.Statement{
										{
											Expr: &checker.ModuleFunctionCall{
												Module: "ard/io",
												Call: &checker.FunctionCall{
													Name: "print",
													Args: []checker.Expression{
														&checker.TemplateStr{
															Chunks: []checker.Expression{
																&checker.StrLiteral{Value: "name is "},
																&checker.Variable{},
															},
														},
													},
												},
											},
										},
									},
								},
							},
							None: &checker.Block{
								Stmts: []checker.Statement{
									{
										Expr: &checker.ModuleFunctionCall{
											Module: "ard/io",
											Call: &checker.FunctionCall{
												Name: "print",
												Args: []checker.Expression{
													&checker.StrLiteral{Value: "no name"},
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

func TestLists(t *testing.T) {
	run(t, []test{
		{
			name:  "Empty list",
			input: `let empty: [Int] = []`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{Stmt: &checker.VariableDef{
						Mutable: false,
						Name:    "empty",
						Value: &checker.ListLiteral{
							Elements: []checker.Expression{},
						},
					}},
				},
			},
		},
		{
			name: "Empty lists must have declared type",
			input: `
			let empty = List::new()
			let uninferred = []
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Empty lists need an explicit type"},
			},
		},
		{
			name:  "Lists cannot have mixed types",
			input: `let numbers = [1, "two", false]`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: A list can only contain values of single type"},
				{Kind: checker.Error, Message: "Type mismatch: A list can only contain values of single type"},
			},
		},
		{
			name:  "A valid list",
			input: `[1,2,3]`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.ListLiteral{
							Elements: []checker.Expression{
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
			name:  "Looping over a list",
			input: `for num,index in [1,2,3] { num }`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.ForInList{
							Cursor: "num",
							Index:  "index",
							List: &checker.ListLiteral{
								Elements: []checker.Expression{
									&checker.IntLiteral{Value: 1},
									&checker.IntLiteral{Value: 2},
									&checker.IntLiteral{Value: 3},
								},
							},
							Body: &checker.Block{
								Stmts: []checker.Statement{
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
			name: "List API",
			input: strings.Join([]string{
				`[1].size()`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.InstanceMethod{
							Subject: &checker.ListLiteral{Elements: []checker.Expression{&checker.IntLiteral{Value: 1}}},
							Method:  &checker.FunctionCall{Name: "size", Args: []checker.Expression{}},
						},
					},
				},
			},
		},
		{
			name: "An immutable list cannot be changed",
			input: `
			  let list = [1,2,3]
				list.push(4)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Cannot mutate immutable 'list' with '.push()'"},
			},
		},
	})
}

func TestMaps(t *testing.T) {
	run(t, []test{
		{
			name:  "Valid map instantiation",
			input: `let ages: [Str:Int] = ["ard":0, "go":15] `,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Name: "ages",
							Value: &checker.MapLiteral{
								Keys: []checker.Expression{
									&checker.StrLiteral{Value: "ard"},
									&checker.StrLiteral{Value: "go"},
								},
								Values: []checker.Expression{
									&checker.IntLiteral{Value: 0},
									&checker.IntLiteral{Value: 15},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "Inferring types with initial values",
			input: `let ages = ["ard":0, "go":15]`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Name: "ages",
							Value: &checker.MapLiteral{
								Keys: []checker.Expression{
									&checker.StrLiteral{Value: "ard"},
									&checker.StrLiteral{Value: "go"},
								},
								Values: []checker.Expression{
									&checker.IntLiteral{Value: 0},
									&checker.IntLiteral{Value: 15},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "Empty maps need an explicit type",
			input: `let empty = [:]`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Empty maps need an explicit type"},
			},
		},
		{
			name:  "Initial entries must match the declared type",
			input: `let ages: [Str:Int] = [1:1, "two":true]`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Bool"},
			},
		},
		{
			name:  "In order to infer, all entries must have the same type",
			input: `let peeps = ["joe":true, "jack":100]`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Map value type mismatch: Expected Bool, got Int"},
			},
		},
	})
}

func TestEnums(t *testing.T) {
	run(t, []test{
		{
			name: "Valid enum definition",
			input: `
				enum Color {
					Red,
					Yellow,
					Green
				}
			`,
			output: &checker.Program{
				Statements: []checker.Statement{},
			},
		},
		{
			name:  "Enums must have at least one variant",
			input: `enum Color {}`,
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Enums must have at least one variant",
				},
			},
		},
		{
			name: "Variants must be unique",
			input: strings.Join([]string{
				`enum Color {`,
				`  Blue,`,
				`  Green,`,
				`  Blue`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Duplicate variant: Blue",
				},
			},
		},
		{
			name: "Referencing a variant",
			input: strings.Join([]string{
				`enum Color {`,
				`  blue,`,
				`  green,`,
				`  purple`,
				`}`,
				`Color::onyx`,
				`Color.yellow`,
				`let choice: Color = Color::green`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Name:  "choice",
							Value: &checker.EnumVariant{Variant: 1},
						},
					},
				},
			},
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined: Color::onyx"},
				{Kind: checker.Error, Message: "Undefined: Color.yellow"},
			},
		},
	})
}

func TestMatchingOnEnums(t *testing.T) {
	run(t, []test{
		{
			name: "Matching on enums",
			input: strings.Join([]string{
				`enum Direction { up, down }`,
				`let dir = Direction::down`,
				"match dir {",
				`  Direction::up => "north",`,
				`  Direction::down => "south"`,
				`}`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Name:  "dir",
							Value: &checker.EnumVariant{Variant: 1},
						},
					},
					{
						Expr: &checker.EnumMatch{
							Subject: &checker.Variable{},
							Cases: []*checker.Block{
								{
									Stmts: []checker.Statement{
										{Expr: &checker.StrLiteral{Value: "north"}},
									},
								},
								{
									Stmts: []checker.Statement{
										{Expr: &checker.StrLiteral{Value: "south"}},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Matching on enums should be exhaustive",
			input: strings.Join([]string{
				`enum Direction { up, down, left, right }`,
				"let dir = Direction::down",
				`match dir {`,
				`  Direction::up => "north",`,
				`  Direction::down => "south"`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Incomplete match: missing case for 'Direction::left'",
				},
				{
					Kind:    checker.Error,
					Message: "Incomplete match: missing case for 'Direction::right'",
				},
			},
		},
		{
			name: "Duplicate cases are caught",
			input: strings.Join([]string{
				`enum Direction { up, down, left, right }`,
				"let dir = Direction::down",
				`match dir {`,
				`  Direction::up => "north",`,
				`  Direction::down => "south",`,
				`  Direction::left => "west",`,
				`  Direction::down => "south",`,
				`  Direction::right => "east"`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Duplicate case: Direction::down",
				},
			},
		},
		{
			name: "Each case must return the same type",
			input: strings.Join([]string{
				`enum Direction { up, down, left, right }`,
				"let dir = Direction::down",
				`match dir {`,
				`  Direction::up => "north",`,
				`  Direction::down => "south",`,
				`  Direction::left => false,`,
				`  Direction::right => "east"`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Type mismatch: Expected Str, got Bool",
				},
			},
		},
		{
			name: "A catch-all case can be provided",
			input: strings.Join([]string{
				`enum Direction { up, down, left, right }`,
				"let dir = Direction::down",
				`match dir {`,
				`  Direction::up => "north",`,
				`  Direction::down => "south",`,
				`  _ => "lateral"`,
				`}`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Name:  "dir",
							Value: &checker.EnumVariant{Variant: 1},
						},
					},
					{
						Expr: &checker.EnumMatch{
							Subject: &checker.Variable{},
							Cases: []*checker.Block{
								{
									Stmts: []checker.Statement{
										{Expr: &checker.StrLiteral{Value: "north"}},
									},
								},
								{
									Stmts: []checker.Statement{
										{Expr: &checker.StrLiteral{Value: "south"}},
									},
								},
								nil,
								nil,
							},
							CatchAll: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.StrLiteral{Value: "lateral"}},
								},
							},
						},
					},
				},
			},
		},
	})
}

func TestMatchingOnBooleans(t *testing.T) {
	run(t, []test{
		{
			name: "Matching on booleans",
			input: strings.Join([]string{
				`let is_big = "foo".size() > 20`,
				`match is_big {`,
				`  true => "big",`,
				`  false => "smol"`,
				`}`,
			}, "\n"),
			output: &checker.Program{
				Statements: []checker.Statement{},
			},
		},
		{
			name: "Matching on booleans should be exhaustive",
			input: strings.Join([]string{
				`let is_big = "foo".size() > 20`,
				`match is_big {`,
				`  true => "big",`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Incomplete match: Missing case for 'false'",
				},
			},
		},
		{
			name: "Duplicate cases are caught",
			input: strings.Join([]string{
				`let is_big = "foo".size() > 20`,
				`match is_big {`,
				`  true => "big",`,
				`  true => "big",`,
				`  false => "smol",`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Duplicate case: 'true'",
				},
			},
		},
		{
			name: "Each case must return the same type",
			input: strings.Join([]string{
				`let is_big = "foo".size() > 20`,
				`match is_big {`,
				`  true => "big",`,
				`  false => 21,`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Type mismatch: Expected Str, got Int",
				},
			},
		},
		{
			name: "Cannot use a catch-all case",
			input: strings.Join([]string{
				`let is_big = "foo".size() > 20`,
				`match is_big {`,
				`  true => "big",`,
				`  _ => "smol"`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Catch-all case is not allowed for boolean matches"},
			},
		},
	})
}

func TestMatchingOnInts(t *testing.T) {
	run(t, []test{
		{
			name: "Matching on booleans should be exhaustive",
			input: `
			  let int = 100
				match int {
				  -1 => "less",
				  0 => "equal",
					1 => "greater",
				}
			`,
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Incomplete match: missing catch-all case for Int match",
				},
			},
		},
	})
}

func TestGenerics(t *testing.T) {
	run(t, []test{
		{
			name:  "An identity function",
			input: `fn identity(of: $T) $T { of }`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name:       "identity",
							Parameters: []checker.Parameter{{Name: "of", Type: &checker.Any{}}},
							ReturnType: &checker.Any{},
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.Variable{}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Providing type arguments to static functions",
			input: `
				use ard/json
				let result = json::encode<Int>(42)
			`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Name: "result",
							Value: &checker.ModuleFunctionCall{
								Module: "ard/json",
								Call: &checker.FunctionCall{
									Name: "encode",
									Args: []checker.Expression{&checker.IntLiteral{Value: 42}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Providing type arguments to normal functions",
			input: `
			  fn identity(of: $T) $T { of }
				2 + identity<Int>(1)
			`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name:       "identity",
							Parameters: []checker.Parameter{{Name: "of", Type: &checker.Any{}}},
							ReturnType: &checker.Any{},
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.Variable{}},
								},
							},
						},
					},
					{
						Expr: &checker.IntAddition{
							Left: &checker.IntLiteral{Value: 2},
							Right: &checker.FunctionCall{
								Name: "identity",
								Args: []checker.Expression{&checker.IntLiteral{Value: 1}},
							},
						},
					},
				},
			},
		},
		{
			name: "Providing type arguments to methods",
			input: `
				use ard/json
			  struct Foo {
					body: Str
				}
				impl Foo {
				  fn bar() $T!Str {
					  json::encode<$T>(@body)
					}
				}
			  let foo = Foo{body: "200"}
				let num = foo.bar<Int>()
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}

func TestVoidLiteral(t *testing.T) {
	run(t, []test{
		{
			name:  "void literal",
			input: `let unit = ()`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{Name: "unit", Value: &checker.VoidLiteral{}},
					},
				},
			},
		},
	})
}
