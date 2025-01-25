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
	cmpopts.IgnoreUnexported(
		Identifier{},
		FunctionCall{},
		Package{},
		Diagnostic{},
		ListLiteral{},
		MapLiteral{},
		MatchCase{},
		Block{},
		EnumVariant{},
		StructInstance{},
		IfStatement{},
		Struct{},
		function{},
	),
}

func run(t *testing.T, tests []test) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := []byte(tt.input)
			tree := tsParser.Parse(source, nil)
			parser := ast.NewParser(source, tree)
			ast, err := parser.Parse()
			if err != nil {
				t.Fatalf("Error parsing input: %v", err)
			}
			program, diagnostics := Check(ast)
			if len(tt.diagnostics) > 0 || len(diagnostics) > 0 {
				if diff := cmp.Diff(tt.diagnostics, diagnostics, compareOptions); diff != "" {
					t.Fatalf("Diagnostics mismatch (-want +got):\n%s", diff)
				}
			}
			if len(tt.output.Imports) > 0 {
				if diff := cmp.Diff(tt.output.Imports, program.Imports, compareOptions); diff != "" {
					t.Fatalf("Program imports mismatch (-want +got):\n%s", diff)
				}
			}
			if len(tt.output.Statements) > 0 {
				if diff := cmp.Diff(tt.output.Statements, program.Statements, compareOptions); diff != "" {
					t.Fatalf("Program statements mismatch (-want +got):\n%s", diff)
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
				`let num = 3`,
				`"Hello, {{num}}"`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Mut:   false,
						Name:  "name",
						Value: StrLiteral{Value: "world"},
					},
					InterpolatedStr{
						Parts: []Expression{
							StrLiteral{Value: "Hello, "},
							Identifier{Name: "name"},
						},
					},
					VariableBinding{
						Mut:   false,
						Name:  "num",
						Value: NumLiteral{Value: 3},
					},
					InterpolatedStr{
						Parts: []Expression{
							StrLiteral{Value: "Hello, "},
							nil,
						},
					},
				},
			},
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Type mismatch: Expected Str, got Num"},
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
				"let is_student: Bool = true",
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Mut:   false,
						Name:  "name",
						Value: StrLiteral{Value: "Alice"},
					},
					VariableBinding{
						Mut:   false,
						Name:  "age",
						Value: NumLiteral{Value: 32},
					},
					VariableBinding{
						Mut:   false,
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
				{Kind: Error, Message: "Type mismatch: Expected Num, got Str"},
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
				{Kind: Error, Message: "Immutable variable: name"},
			},
		},
		{
			name:  "Reassigning types must match",
			input: `mut name = "Bob"` + "\n" + `name = 0`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Type mismatch: Expected Str, got Num"},
			},
		},
		{
			name:  "Cannot reassign undeclared variables",
			input: `name = "Bob"`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Undefined: name"},
			},
		},
		{
			name:  "Valid reassigments",
			input: `mut count = 0` + "\n" + `count = 1`,
			output: Program{
				Statements: []Statement{
					VariableBinding{Mut: true, Name: "count", Value: NumLiteral{Value: 0}},
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
						Mut:   false,
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
				{Kind: Error, Message: "Undefined: \"foo\".length"},
				{Kind: Error, Message: "Undefined: name.len"},
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
					Message: "The '-' operator can only be used on numbers",
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
				{Kind: Error, Message: "The '!' operator can only be used on booleans"},
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
					{Kind: Error, Message: fmt.Sprintf("Invalid operation: Num %s Bool", c.op)},
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
						Mut:  false,
						Name: "never",
						Value: BinaryExpr{
							Op:    And,
							Left:  BoolLiteral{Value: true},
							Right: BoolLiteral{Value: false},
						},
					},
					VariableBinding{
						Mut:  false,
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
					VariableBinding{Mut: false, Name: "is_on", Value: BoolLiteral{Value: true}},
					IfStatement{
						Condition: Identifier{Name: "is_on"},
						Body: []Statement{
							VariableBinding{Mut: false, Name: "foo", Value: StrLiteral{Value: "bar"}},
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
				{Kind: Error, Message: "If conditions must be boolean expressions"},
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
					VariableBinding{Mut: false, Name: "is_on", Value: BoolLiteral{Value: true}},
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
							VariableBinding{Mut: false, Name: "foo", Value: StrLiteral{Value: "bar"}},
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
					VariableBinding{Mut: false, Name: "is_on", Value: BoolLiteral{Value: true}},
					IfStatement{
						Condition: Identifier{Name: "is_on"},
						Body: []Statement{
							VariableBinding{Mut: false, Name: "foo", Value: StrLiteral{Value: "bar"}},
						},
						Else: IfStatement{
							Body: []Statement{
								VariableBinding{Mut: false, Name: "foo", Value: StrLiteral{Value: "baz"}},
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
					VariableBinding{Mut: false, Name: "is_on", Value: BoolLiteral{Value: true}},
					IfStatement{
						Condition: Identifier{Name: "is_on"},
						Body: []Statement{
							VariableBinding{Mut: false, Name: "foo", Value: StrLiteral{Value: "bar"}},
						},
						Else: IfStatement{
							Condition: BinaryExpr{
								Op:    GreaterThan,
								Left:  NumLiteral{Value: 1},
								Right: NumLiteral{Value: 2},
							},
							Body: []Statement{
								VariableBinding{Mut: false, Name: "foo", Value: StrLiteral{Value: "baz"}},
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
					VariableBinding{Mut: false, Name: "is_on", Value: BoolLiteral{Value: true}},
					IfStatement{
						Condition: Identifier{Name: "is_on"},
						Body: []Statement{
							VariableBinding{Mut: false, Name: "foo", Value: StrLiteral{Value: "bar"}},
						},
						Else: IfStatement{
							Condition: BinaryExpr{
								Op:    GreaterThan,
								Left:  NumLiteral{Value: 1},
								Right: NumLiteral{Value: 2},
							},
							Body: []Statement{
								VariableBinding{Mut: false, Name: "foo", Value: StrLiteral{Value: "baz"}},
							},
							Else: IfStatement{
								Body: []Statement{
									VariableBinding{Mut: false, Name: "foo", Value: StrLiteral{Value: "qux"}},
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
					VariableBinding{Mut: true, Name: "count", Value: NumLiteral{Value: 0}},
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
				{Kind: Error, Message: "Invalid range: Num..Bool"},
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
					VariableBinding{Mut: false, Name: "string", Value: StrLiteral{Value: "hello"}},
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
			name: "Iterating over a list",
			input: strings.Join([]string{
				`for i in [1,2,3] {}`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					ForIn{
						Cursor: Identifier{Name: "i"},
						Iterable: ListLiteral{
							Elements: []Expression{
								NumLiteral{Value: 1},
								NumLiteral{Value: 2},
								NumLiteral{Value: 3},
							},
						},
						Body: []Statement{},
					},
				},
			},
		},
		{
			name:  "Cannot iterate over a boolean",
			input: `for b in false {}`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Cannot iterate over a Bool"},
			},
		},
		{
			name: "Iterating over a list of structs",
			input: strings.Join([]string{
				`struct Shape { height: Num, width: Num }`,
				`for shape in [Shape{height: 1, width: 2}, Shape{height: 2, width: 2}] {}`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					&Struct{
						Name: "Shape",
						Fields: map[string]Type{
							"height": Num{},
							"width":  Num{},
						},
					},
					ForIn{
						Cursor: Identifier{Name: "shape"},
						Iterable: ListLiteral{
							Elements: []Expression{
								StructInstance{
									Name: "Shape",
									Fields: map[string]Expression{
										"height": NumLiteral{Value: 1},
										"width":  NumLiteral{Value: 2},
									},
								},
								StructInstance{
									Name: "Shape",
									Fields: map[string]Expression{
										"height": NumLiteral{Value: 2},
										"width":  NumLiteral{Value: 2},
									},
								},
							},
						},
						Body: []Statement{},
					},
				},
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
			output: Program{
				Statements: []Statement{
					VariableBinding{Mut: true, Name: "count", Value: NumLiteral{Value: 10}},
					WhileLoop{
						Condition: BinaryExpr{
							Op:    GreaterThan,
							Left:  Identifier{Name: "count"},
							Right: NumLiteral{Value: 0},
						},
						Body: []Statement{
							VariableAssignment{
								Name: "count",
								Value: BinaryExpr{
									Op:    Sub,
									Left:  Identifier{Name: "count"},
									Right: NumLiteral{Value: 1},
								},
							},
						},
					},
				},
			},
		},
	})
}

func TestFunctions(t *testing.T) {
	run(t, []test{
		{
			name:  "Empty function",
			input: `fn noop() {}` + "\n" + `noop()`,
			output: Program{
				Statements: []Statement{
					FunctionDeclaration{
						Name:       "noop",
						Parameters: []Parameter{},
						Body:       []Statement{},
						Return:     Void{},
					},
					FunctionCall{
						Name: "noop",
						Args: []Expression{},
					},
				},
			},
		},
		{
			name: "Return type is not inferred",
			input: strings.Join([]string{
				`fn get_msg() { "Hello, world!" }`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					FunctionDeclaration{
						Name:       "get_msg",
						Parameters: []Parameter{},
						Body: []Statement{
							StrLiteral{Value: "Hello, world!"},
						},
						Return: Void{},
					},
				},
			},
		},
		{
			name: "Can't use return value of non-returning function",
			input: strings.Join([]string{
				`fn get_msg() { "Hello, world!" }`,
				`let msg = get_msg()`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: fmt.Sprintf("Cannot assign a void value")},
			},
		},
		{
			name: "Explicit return type",
			input: strings.Join([]string{
				`fn get_msg() Str { "Hello, world!" }`,
				`let msg = get_msg()`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					FunctionDeclaration{
						Name:       "get_msg",
						Parameters: []Parameter{},
						Body: []Statement{
							StrLiteral{Value: "Hello, world!"},
						},
						Return: Str{},
					},
					VariableBinding{
						Mut:  false,
						Name: "msg",
						Value: FunctionCall{
							Name: "get_msg",
							Args: []Expression{},
						}},
				},
			},
		},
		{
			name: "Implementation should match declared return type",
			input: strings.Join([]string{
				`fn get_msg() Str { 200 }`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Type mismatch: Expected Str, got Num"},
			},
		},
		{
			name: "Function with parameters",
			input: strings.Join([]string{
				`fn greet(person: Str) Str { "hello {{person}}" }`,
				`greet("joe")`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					FunctionDeclaration{
						Name: "greet",
						Parameters: []Parameter{
							{Name: "person", Type: Str{}},
						},
						Return: Str{},
						Body: []Statement{
							InterpolatedStr{
								Parts: []Expression{
									StrLiteral{Value: "hello "},
									Identifier{Name: "person"},
								},
							},
						},
					},
					FunctionCall{
						Name: "greet",
						Args: []Expression{StrLiteral{Value: "joe"}},
					},
				},
			},
		},
		{
			name: "Function calls must have correct arguments",
			input: strings.Join([]string{
				`fn greet(person: Str) { "hello {{person}}" }`,
				`greet(101)`,
				`fn add(a: Num, b: Num) { a + b }`,
				`add(2)`,
				`add(1, "two")`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Type mismatch: Expected Str, got Num"},
				{Kind: Error, Message: "Incorrect number of arguments: Expected 2, got 1"},
				{Kind: Error, Message: "Type mismatch: Expected Num, got Str"},
			},
		},
		{
			name: "Anonymous functions",
			input: strings.Join([]string{
				`let add = (a: Num, b: Num) Num { a + b }`,
				`let eight: Num = add(3, 5)`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Mut:  false,
						Name: "add",
						Value: FunctionLiteral{
							Parameters: []Parameter{
								{Name: "a", Type: Num{}},
								{Name: "b", Type: Num{}},
							},
							Return: Num{},
							Body: []Statement{
								BinaryExpr{
									Op:    Add,
									Left:  Identifier{Name: "a"},
									Right: Identifier{Name: "b"},
								},
							},
						},
					},
					VariableBinding{
						Mut:   false,
						Name:  "eight",
						Value: FunctionCall{Name: "add", Args: []Expression{NumLiteral{Value: 3}, NumLiteral{Value: 5}}},
					},
				},
			},
		},
	})
}

func TestCallingPackageMethods(t *testing.T) {
	run(t, []test{
		{
			name: "io.print",
			input: strings.Join([]string{
				`use ard/io`,
				`io.print("Hello World")`,
			}, "\n"),
			output: Program{
				Imports: map[string]Package{
					"io": {
						Path: "ard/io",
					},
				},
				Statements: []Statement{
					PackageAccess{
						Package: Package{Path: "ard/io"},
						Property: FunctionCall{
							Name: "print",
							Args: []Expression{
								StrLiteral{Value: "Hello World"},
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
			input: `let empty: [Num] = []`,
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Mut:   false,
						Name:  "empty",
						Value: ListLiteral{Elements: []Expression{}},
					},
				},
			},
		},
		{
			name:  "Empty lists must have declared type",
			input: `let empty = []`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Empty lists need an explicit type"},
			},
		},
		{
			name:  "Lists cannot have mixed types",
			input: `let numbers = [1, "two", false]`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Type mismatch: Expected Num, got Str"},
				{Kind: Error, Message: "Type mismatch: Expected Num, got Bool"},
			},
		},
		{
			name:  "A valid list",
			input: `[1,2,3]`,
			output: Program{
				Statements: []Statement{
					ListLiteral{
						Elements: []Expression{
							NumLiteral{Value: 1},
							NumLiteral{Value: 2},
							NumLiteral{Value: 3},
						},
					},
				},
			},
		},
		{
			name: "List API",
			input: strings.Join([]string{
				`[1].size`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					InstanceProperty{
						Subject: ListLiteral{
							Elements: []Expression{NumLiteral{Value: 1}},
						},
						Property: Identifier{Name: "size"},
					},
				},
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
			output: Program{
				Statements: []Statement{
					Enum{
						Name:     "Color",
						Variants: []string{"Red", "Yellow", "Green"},
					},
				},
			},
		},
		{
			name:  "Enums must have at least one variant",
			input: `enum Color {}`,
			diagnostics: []Diagnostic{
				{
					Kind:    Error,
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
			diagnostics: []Diagnostic{
				{
					Kind:    Error,
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
				`Color.green`,
				`let choice: Color = Color::green`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					Enum{
						Name:     "Color",
						Variants: []string{"blue", "green", "purple"},
					},
					VariableBinding{
						Mut:  false,
						Name: "choice",
						Value: EnumVariant{
							Enum:    "Color",
							Variant: "green",
							Value:   1,
						},
					},
				},
			},
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Undefined: Color::onyx"},
				{Kind: Error, Message: "Undefined: Color.green"},
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
			output: Program{
				Statements: []Statement{
					Enum{
						Name:     "Direction",
						Variants: []string{"up", "down"},
					},
					VariableBinding{
						Mut:   false,
						Name:  "dir",
						Value: EnumVariant{Enum: "Direction", Variant: "down", Value: 1},
					},
					EnumMatch{
						Subject: Identifier{
							Name: "dir",
						},
						Cases: []Block{
							{
								Body: []Statement{
									StrLiteral{Value: "north"},
								},
							},
							{
								Body: []Statement{
									StrLiteral{Value: "south"},
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
			diagnostics: []Diagnostic{
				{
					Kind:    Error,
					Message: "Incomplete match: missing case for 'Direction::left'",
				},
				{
					Kind:    Error,
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
			diagnostics: []Diagnostic{
				{
					Kind:    Error,
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
			diagnostics: []Diagnostic{
				{
					Kind:    Error,
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
			output: Program{
				Statements: []Statement{
					Enum{
						Name:     "Direction",
						Variants: []string{"up", "down", "left", "right"},
					},
					VariableBinding{
						Mut:   false,
						Name:  "dir",
						Value: EnumVariant{Enum: "Direction", Variant: "down", Value: 1},
					},
					EnumMatch{
						Subject: Identifier{
							Name: "dir",
						},
						Cases: []Block{
							{
								Body: []Statement{
									StrLiteral{Value: "north"},
								},
							},
							{
								Body: []Statement{
									StrLiteral{Value: "south"},
								},
							},
						},
						CatchAll: MatchCase{
							Pattern: nil,
							Body: []Statement{
								StrLiteral{Value: "lateral"},
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
				`let is_big = "foo".size > 20`,
				`match is_big {`,
				`  true => "big",`,
				`  false => "smol"`,
				`}`,
			}, "\n"),
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Mut:  false,
						Name: "is_big",
						Value: BinaryExpr{
							Op: GreaterThan,
							Left: InstanceProperty{
								Subject:  StrLiteral{Value: "foo"},
								Property: Identifier{Name: "size"},
							},
							Right: NumLiteral{Value: 20},
						},
					},
					BoolMatch{
						Subject: Identifier{
							Name: "is_big",
						},
						True: Block{
							Body: []Statement{
								StrLiteral{Value: "big"},
							},
						},
						False: Block{
							Body: []Statement{
								StrLiteral{Value: "smol"},
							},
						},
					},
				},
			},
		},
		{
			name: "Matching on booleans should be exhaustive",
			input: strings.Join([]string{
				`let is_big = "foo".size > 20`,
				`match is_big {`,
				`  true => "big",`,
				`}`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{
					Kind:    Error,
					Message: "Incomplete match: Missing case for 'false'",
				},
			},
		},
		{
			name: "Duplicate cases are caught",
			input: strings.Join([]string{
				`let is_big = "foo".size > 20`,
				`match is_big {`,
				`  true => "big",`,
				`  true => "big",`,
				`  false => "smol",`,
				`}`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{
					Kind:    Error,
					Message: "Duplicate case: 'true'",
				},
			},
		},
		{
			name: "Each case must return the same type",
			input: strings.Join([]string{
				`let is_big = "foo".size > 20`,
				`match is_big {`,
				`  true => "big",`,
				`  false => 21,`,
				`}`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{
					Kind:    Error,
					Message: "Type mismatch: Expected Str, got Num",
				},
			},
		},
		{
			name: "Cannot use a catch-all case",
			input: strings.Join([]string{
				`let is_big = "foo".size > 20`,
				`match is_big {`,
				`  true => "big",`,
				`  _ => "smol"`,
				`}`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Catch-all case is not allowed for boolean matches"},
			},
		},
	})
}

func TestOptionals(t *testing.T) {
	optionPkg := Package{Path: "ard/option"}
	run(t, []test{
		{
			name: "Declaring an empty optional",
			input: `
				use ard/option
				mut name: Str? = option.make()`,
			output: Program{
				Imports: map[string]Package{
					"option": optionPkg,
				},
				Statements: []Statement{
					VariableBinding{
						Mut:  true,
						Name: "name",
						Value: PackageAccess{
							Package:  optionPkg,
							Property: FunctionCall{Name: "make", Args: []Expression{}},
						},
					},
				},
			},
		},
		{
			name: "Updating an optional",
			input: `
				use ard/option
				mut name: Str? = option.make()
				name.some("Bob")
				name = "Alice"
				name.none()`,
			output: Program{
				Imports: map[string]Package{
					"option": optionPkg,
				},
				Statements: []Statement{
					VariableBinding{
						Mut:  true,
						Name: "name",
						Value: PackageAccess{
							Package:  optionPkg,
							Property: FunctionCall{Name: "make", Args: []Expression{}},
						},
					},
					InstanceProperty{
						Subject:  Identifier{Name: "name"},
						Property: FunctionCall{Name: "some", Args: []Expression{StrLiteral{Value: "Bob"}}},
					},
					InstanceProperty{
						Subject:  Identifier{Name: "name"},
						Property: FunctionCall{Name: "none", Args: []Expression{}},
					},
				},
			},
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Type mismatch: Expected Str?, got Str"},
			},
		},
		{
			name: "Matching an optional",
			input: `
				use ard/io
				use ard/option

				mut name: Str? = option.make()
				name.some("Bob")
				match name {
				  it => io.print("name is {{it}}"),
					_ => io.print("no name ):")
				}`,
			output: Program{
				Imports: map[string]Package{
					"io":     {Path: "ard/io"},
					"option": optionPkg,
				},
				Statements: []Statement{
					VariableBinding{
						Mut:  true,
						Name: "name",
						Value: PackageAccess{
							Package:  optionPkg,
							Property: FunctionCall{Name: "make", Args: []Expression{}},
						},
					},
					InstanceProperty{
						Subject:  Identifier{Name: "name"},
						Property: FunctionCall{Name: "some", Args: []Expression{StrLiteral{Value: "Bob"}}},
					},
					OptionMatch{
						Subject: Identifier{Name: "name"},
						Some: MatchCase{
							Pattern: Identifier{Name: "it"},
							Body: []Statement{
								PackageAccess{
									Package: Package{Path: "ard/io"},
									Property: FunctionCall{
										Name: "print",
										Args: []Expression{
											InterpolatedStr{Parts: []Expression{StrLiteral{Value: "name is "}, Identifier{Name: "it"}}}}},
								},
							},
						},
						None: Block{
							Body: []Statement{
								PackageAccess{
									Package: Package{Path: "ard/io"},
									Property: FunctionCall{
										Name: "print",
										Args: []Expression{StrLiteral{Value: "no name ):"}},
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

func TestTypeUnions(t *testing.T) {
	run(t, []test{
		{
			name: "Valid type union",
			input: `
				type Alias = Bool
			  type Printable = Num|Str
				let a: Printable = "foo"
				let b: Alias = true
				let list: [Printable] = [1, "two", 3]`,
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Mut:   false,
						Name:  "a",
						Value: StrLiteral{Value: "foo"},
					},
					VariableBinding{
						Mut:   false,
						Name:  "b",
						Value: BoolLiteral{Value: true},
					},
					VariableBinding{
						Mut:  false,
						Name: "list",
						Value: ListLiteral{
							Elements: []Expression{
								NumLiteral{Value: 1},
								StrLiteral{Value: "two"},
								NumLiteral{Value: 3},
							},
						},
					},
				},
			},
		},
		{
			name: "Errors when types don't match",
			input: `
					  type Printable = Num|Str
						fn print(p: Printable) {}
						print(true)`,
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Type mismatch: Expected Num|Str, got Bool"},
			},
		},
		{
			name: "Matching behavior on type unions",
			input: `
				type Printable = Num|Str|Bool
				let a: Printable = "foo"
				match a {
				  Num => "number",
					Str => "string",
					_ => "other"
				}`,
			output: Program{
				Statements: []Statement{
					VariableBinding{
						Mut:   false,
						Name:  "a",
						Value: StrLiteral{Value: "foo"},
					},
					UnionMatch{
						Subject: Identifier{Name: "a"},
						Cases: map[Type]Block{
							Num{}: {Body: []Statement{StrLiteral{Value: "number"}}},
							Str{}: {Body: []Statement{StrLiteral{Value: "string"}}},
						},
						CatchAll: Block{Body: []Statement{StrLiteral{Value: "other"}}},
					},
				},
			},
		},
	})
}
