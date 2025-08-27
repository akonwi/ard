package ast

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var compareOptions = cmp.Options{
	cmpopts.IgnoreUnexported(
		Identifier{},
		IntType{},
		FloatType{},
		StringType{},
		BooleanType{},
		VoidType{},
		List{},
		Map{},
		CustomType{},
		GenericType{},
		ResultType{},
		Try{},
		ExternalFunction{},
	),
	cmp.FilterPath(func(p cmp.Path) bool {
		return p.Last().String() == ".BaseNode" || p.Last().String() == ".Location"
	}, cmp.Ignore()),

	cmp.Comparer(func(x, y map[string]int) bool {
		if len(x) != len(y) {
			return false
		}
		for k, v1 := range x {
			if v2, ok := y[k]; !ok || v1 != v2 {
				return false
			}
		}
		return true
	}),
	cmp.Comparer(func(x, y token) bool {
		return x.kind == y.kind &&
			x.line == y.line &&
			x.column == y.column &&
			x.text == y.text
	}),
}

type test struct {
	name     string
	input    string
	output   Program
	wantErrs []string // Expected error messages
}

func runTests(t *testing.T, tests []test) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.input), "test.ard")

			// Validate errors if expected
			if len(tt.wantErrs) > 0 {
				if len(result.Errors) != len(tt.wantErrs) {
					t.Errorf("Expected %d errors, got %d: %v", len(tt.wantErrs), len(result.Errors), result.Errors)
					return
				}

				for i, wantErr := range tt.wantErrs {
					if !strings.Contains(result.Errors[i].Message, wantErr) {
						t.Errorf("Expected error %d to contain '%s', got '%s'", i, wantErr, result.Errors[i].Message)
					}
				}

				// For error cases, we don't validate AST structure (it may be partial/incomplete)
				t.Logf("Successfully validated %d expected errors", len(tt.wantErrs))
				return
			}

			// For success cases, ensure no errors occurred
			if len(result.Errors) > 0 {
				t.Fatalf("Expected no errors, got %d: %v", len(result.Errors), result.Errors)
			}

			// Validate AST structure (existing logic)
			ast := result.Program
			if ast == nil {
				t.Fatal("Expected program to be parsed, got nil")
			}

			if tt.output.Imports != nil {
				diff := cmp.Diff(tt.output.Imports, ast.Imports, compareOptions)
				if diff != "" {
					t.Errorf("Built AST does not match (-want +got):\n%s", diff)
				}
			}

			// allow nil statement arrays
			if tt.output.Statements != nil {
				diff := cmp.Diff(tt.output.Statements, ast.Statements, compareOptions)
				if diff != "" {
					t.Errorf("Built AST does not match (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestEmptyProgram(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Empty program",
			input: "",
			output: Program{
				Imports:    []Import{},
				Statements: []Statement{}},
		},
	})
}

func TestImportStatements(t *testing.T) {
	runTests(t, []test{
		{
			name: "importing modules",
			input: strings.Join([]string{
				`// comment`,
				`use ard/fs`,
				`use github.com/google/go-cmp/cmp`,
				`use github.com/tree-sitter/go-tree-sitter as ts`,
				`// comment`,
				`use github.com/tree-sitter/tree-sitter`,
			}, "\n"),
			output: Program{
				Imports: []Import{
					{
						Path: "ard/fs",
						Name: "fs",
					},
					{
						Path: "github.com/google/go-cmp/cmp",
						Name: "cmp",
					},
					{
						Path: "github.com/tree-sitter/go-tree-sitter",
						Name: "ts",
					},
					{
						Path: "github.com/tree-sitter/tree-sitter",
						Name: "tree_sitter",
					},
				},
			},
		},
	})
}

func TestIdentifiers(t *testing.T) {
	tests := []test{
		{
			name: "referencing variables",
			input: strings.Join([]string{
				`count`,
			}, "\n"),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&Identifier{Name: "count"},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestWhileLoop(t *testing.T) {
	runTests(t, []test{
		{
			name: "while loops",
			input: `
					while count <= 9 {}
					while has_more {}
					while {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&WhileLoop{
						Condition: &BinaryExpression{
							Left:     &Identifier{Name: "count"},
							Operator: LessThanOrEqual,
							Right:    &NumLiteral{Value: "9"},
						},
						Body: []Statement{},
					},
					&WhileLoop{
						Condition: &Identifier{Name: "has_more"},
						Body:      []Statement{},
					},
					&WhileLoop{
						Condition: nil,
						Body:      []Statement{},
					},
				},
			},
		},
	})
}

func TestIfAndElse(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Valid if statement",
			input: `if true {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&IfStatement{
						Condition: &BoolLiteral{Value: true},
						Body:      []Statement{},
						Else:      nil,
					},
				},
			},
		},
		{
			name:  "Complex condition",
			input: `if not foo.bar {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&IfStatement{
						Condition: &UnaryExpression{
							Operator: Not,
							Operand: &InstanceProperty{
								Target:   &Identifier{Name: "foo"},
								Property: Identifier{Name: "bar"},
							},
						},
						Body: []Statement{},
						Else: nil,
					},
				},
			},
		},
		{
			name: "Valid if-else",
			input: `
					if true {}
					else {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&IfStatement{
						Condition: &BoolLiteral{Value: true},
						Body:      []Statement{},
						Else: &IfStatement{
							Condition: nil,
							Body:      []Statement{},
						},
					},
				},
			},
		},
		{
			name: "Valid if-else if",
			input: `
					if true {}
					else if false {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&IfStatement{
						Condition: &BoolLiteral{Value: true},
						Body:      []Statement{},
						Else: &IfStatement{
							Condition: &BoolLiteral{Value: false},
							Body:      []Statement{},
						},
					},
				},
			},
		},
		{
			name: "Valid if-else-if-else",
			input: `
					if true {}
					else if false {}
					else {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&IfStatement{
						Condition: &BoolLiteral{Value: true},
						Body:      []Statement{},
						Else: &IfStatement{
							Condition: &BoolLiteral{Value: false},
							Body:      []Statement{},
							Else: &IfStatement{
								Condition: nil,
								Body:      []Statement{},
							},
						},
					},
				},
			},
		},
	})
}

func TestForInLoops(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Number range",
			input: `for i in 1..10 {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&RangeLoop{
						Cursor: Identifier{Name: "i"},
						Start:  &NumLiteral{Value: "1"},
						End:    &NumLiteral{Value: "10"},
						Body:   []Statement{},
					},
				},
			},
		},
		{
			name:  "Iterating over a string",
			input: `for char in "foobar" {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&ForInLoop{
						Cursor: Identifier{Name: "char"},
						Iterable: &StrLiteral{
							Value: "foobar",
						},
						Body: []Statement{},
					},
				},
			},
		},
		{
			name:  "Iterating over a list",
			input: `for num in [1, 2] {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&ForInLoop{
						Cursor: Identifier{Name: "num"},
						Iterable: &ListLiteral{
							Items: []Expression{
								&NumLiteral{Value: "1"},
								&NumLiteral{Value: "2"},
							},
						},
						Body: []Statement{},
					},
				},
			},
		},
		{
			name:  "Iterating over a map",
			input: `for key, val in map {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&ForInLoop{
						Cursor:   Identifier{Name: "key"},
						Cursor2:  Identifier{Name: "val"},
						Iterable: &Identifier{Name: "map"},
						Body:     []Statement{},
					},
				},
			},
		},
		{
			name:  "Iterating over a list of struct literals",
			input: `for shape in [Shape{height: 1, width: 2}, Shape{height: 2, width: 2}] {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&ForInLoop{
						Cursor: Identifier{Name: "shape"},
						Iterable: &ListLiteral{
							Items: []Expression{
								&StructInstance{
									Name: Identifier{Name: "Shape"},
									Properties: []StructValue{
										{Name: Identifier{Name: "height"}, Value: &NumLiteral{Value: "1"}},
										{Name: Identifier{Name: "width"}, Value: &NumLiteral{Value: "2"}},
									},
								},
								&StructInstance{
									Name: Identifier{Name: "Shape"},
									Properties: []StructValue{
										{Name: Identifier{Name: "height"}, Value: &NumLiteral{Value: "2"}},
										{Name: Identifier{Name: "width"}, Value: &NumLiteral{Value: "2"}},
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

func TestForLoops(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Basic",
			input: `for mut i = 0; i < 10; i =+ 1 {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&ForLoop{
						Init: &VariableDeclaration{
							Mutable: true,
							Name:    "i",
							Value:   &NumLiteral{Value: "0"},
						},
						Condition: &BinaryExpression{
							Operator: LessThan,
							Left:     &Identifier{Name: "i"},
							Right:    &NumLiteral{Value: "10"},
						},
						Incrementer: &VariableAssignment{
							Target:   &Identifier{Name: "i"},
							Operator: Assign,
							Value: &BinaryExpression{
								Operator: Plus,
								Left:     &Identifier{Name: "i"},
								Right:    &NumLiteral{Value: "1"},
							},
						},
						Body: []Statement{},
					},
				},
			},
		},
	})
}

func TestTypeUnion(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Type union",
			input: `private type Value = Int | Bool`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&TypeDeclaration{
						Private: true,
						Name:    Identifier{Name: "Value"},
						Type:    []DeclaredType{&IntType{}, &BooleanType{}},
					},
				},
			},
		},
	})
}

func TestStaticPaths(t *testing.T) {
	runTests(t, []test{
		{
			name:  "deep",
			input: `http::Response::new(200, "ok")`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StaticFunction{
						Target: &StaticProperty{
							Target:   &Identifier{Name: "http"},
							Property: &Identifier{Name: "Response"},
						},
						Function: FunctionCall{
							Name: "new",
							Args: []Argument{
								{Name: "", Value: &NumLiteral{Value: "200"}},
								{Name: "", Value: &StrLiteral{Value: "ok"}},
							},
						},
					},
				},
			},
		},
	})
}
