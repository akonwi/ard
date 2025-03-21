package ast

import (
	"fmt"
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
		List{},
		Map{},
		CustomType{}),
	cmp.FilterPath(func(p cmp.Path) bool {
		return p.Last().String() == ".BaseNode" || p.Last().String() == ".Range"
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
	name   string
	input  string
	output Program
}

func runTests(t *testing.T, tests []test) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatal(fmt.Errorf("Error parsing tree: %v", err))
			}

			diff := cmp.Diff(&tt.output, ast, compareOptions)
			if diff != "" {
				t.Errorf("Built AST does not match (-want +got):\n%s", diff)
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
				`use ard/fs`,
				`use github.com/google/go-cmp/cmp`,
				`use github.com/tree-sitter/go-tree-sitter as ts`,
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
				Statements: []Statement{},
			},
		},
	})
}

func TestVariableDeclarations(t *testing.T) {
	tests := []test{
		{
			name: "declaring variables",
			input: strings.Join([]string{
				`let count = 10`,
				`mut boolean: Bool = false`,
			}, "\n"),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Mutable: false,
						Name:    "count",
						Value:   &NumLiteral{Value: "10"},
					},
					&VariableDeclaration{
						Mutable: true,
						Name:    "boolean",
						Type:    BooleanType{},
						Value:   &BoolLiteral{Value: false},
					},
				},
			},
		},
	}

	runTests(t, tests)
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
			name: "while loop",
			input: `
					while count <= 9 {
					}`,
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

func TestComments(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Single line comment",
			input: "// this is a comment",
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&Comment{Value: "// this is a comment"},
				},
			},
		},
		{
			name:  "Block comment",
			input: "/* this is comment line 1" + "\n" + "this is a comment line 2*/",
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&Comment{Value: "/* this is comment line 1" + "\n" + "this is a comment line 2*/"},
				},
			},
		},
	})
}

func TestTypeUnion(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Type union",
			input: `type Value = Int | Bool`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&TypeDeclaration{
						Name: Identifier{Name: "Value"},
						Type: []DeclaredType{IntType{}, BooleanType{}},
					},
				},
			},
		},
	})
}
