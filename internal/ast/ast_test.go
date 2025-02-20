package ast

import (
	"fmt"
	"strings"
	"testing"

	tree_sitter_ard "github.com/akonwi/tree-sitter-ard/bindings/go"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

var tsParser *tree_sitter.Parser
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
}

func init() {
	_tsParser, err := tree_sitter_ard.MakeParser()
	if err != nil {
		panic(err)
	}
	tsParser = _tsParser
}

type test struct {
	name   string
	input  string
	output Program
}

func runTests(t *testing.T, tests []test) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := tsParser.Parse([]byte(tt.input), nil)
			parser := NewParser([]byte(tt.input), tree)
			ast, err := parser.Parse()
			if err != nil {
				t.Fatal(fmt.Errorf("Error parsing tree: %v", err))
			}

			if len(tt.output.Statements) > 0 {
				diff := cmp.Diff(tt.output, ast, compareOptions)
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
				`use io/fs`,
				`use github.com/google/go-cmp/cmp`,
				`use github.com/tree-sitter/go-tree-sitter as ts`,
				`use github.com/tree-sitter/tree-sitter`,
			}, "\n"),
			output: Program{
				Imports: []Import{
					{
						Path: "fmt",
						Name: "fmt",
					},
					{
						Path: "io/fs",
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

func TestIdentifiers(t *testing.T) {
	tests := []test{
		{
			name: "referencing variables",
			input: strings.Join([]string{
				`let count = 10`,
				`count <= 10`,
			}, "\n"),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "count",
						Value:   NumLiteral{Value: "10"},
					},
					BinaryExpression{
						Left:     Identifier{Name: "count"},
						Operator: LessThanOrEqual,
						Right:    NumLiteral{Value: "10"},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestWhileLoop(t *testing.T) {
	tests := []test{
		{
			name: "while loop",
			input: `
				while count <= 9 {
					count =+ 1
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					WhileLoop{
						Condition: BinaryExpression{
							Left:     Identifier{Name: "count"},
							Operator: LessThanOrEqual,
							Right:    NumLiteral{Value: "9"},
						},
						Body: []Statement{
							VariableAssignment{
								Target:   Identifier{Name: "count"},
								Operator: Increment,
								Value:    NumLiteral{Value: "1"},
							},
						},
					},
				},
			},
		},
		{
			name: "With non-boolean condition",
			input: `
						while 9 - 7 {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					WhileLoop{
						Condition: BinaryExpression{
							Left:     NumLiteral{Value: "9"},
							Operator: Minus,
							Right:    NumLiteral{Value: "7"},
						},
						Body: []Statement{},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestIfAndElse(t *testing.T) {
	tests := []test{
		{
			name:  "Valid if statement",
			input: `if true {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					IfStatement{
						Condition: BoolLiteral{Value: true},
						Body:      []Statement{},
						Else:      nil,
					},
				},
			},
		},
		{
			name:  "Invalid condition expression",
			input: `if 20 - 1 {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					IfStatement{
						Condition: BinaryExpression{
							Left:     NumLiteral{Value: "20"},
							Operator: Minus,
							Right:    NumLiteral{Value: "1"},
						},
						Body: []Statement{},
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
					IfStatement{
						Condition: BoolLiteral{Value: true},
						Body:      []Statement{},
						Else: IfStatement{
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
					IfStatement{
						Condition: BoolLiteral{Value: true},
						Body:      []Statement{},
						Else: IfStatement{
							Condition: BoolLiteral{Value: false},
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
					IfStatement{
						Condition: BoolLiteral{Value: true},
						Body:      []Statement{},
						Else: IfStatement{
							Condition: BoolLiteral{Value: false},
							Body:      []Statement{},
							Else: IfStatement{
								Condition: nil,
								Body:      []Statement{},
							},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestForInLoops(t *testing.T) {
	tests := []test{
		{
			name:  "Number range",
			input: `for i in 1..10 {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					RangeLoop{
						Cursor: Identifier{Name: "i"},
						Start:  NumLiteral{Value: "1"},
						End:    NumLiteral{Value: "10"},
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
					ForInLoop{
						Cursor: Identifier{Name: "char"},
						Iterable: StrLiteral{
							Value: `"foobar"`,
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
					ForInLoop{
						Cursor: Identifier{Name: "num"},
						Iterable: ListLiteral{
							Items: []Expression{
								NumLiteral{Value: "1"},
								NumLiteral{Value: "2"},
							},
						},
						Body: []Statement{},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestForLoops(t *testing.T) {
	runTests(t, []test{
		{
			name:  "For loop",
			input: `for mut i = 0; i < 10; i =+ 1 {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					ForLoop{
						Init: VariableDeclaration{
							Mutable: true,
							Name:    "i",
							Value:   NumLiteral{Value: "0"},
						},
						Condition: BinaryExpression{
							Operator: LessThan,
							Left:     Identifier{Name: "i"},
							Right:    NumLiteral{Value: "10"},
						},
						Incrementer: VariableAssignment{
							Target:   Identifier{Name: "i"},
							Operator: Increment,
							Value:    NumLiteral{Value: "1"},
						},
						Body: []Statement{},
					},
				},
			},
		},
	})
}

func TestInterpolatedStrings(t *testing.T) {
	tests := []test{
		{
			name: "Interpolated string",
			input: `
			let name = "world"
			"Hello, {{name}}"`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					VariableDeclaration{
						Mutable: false,
						Name:    "name",
						Value:   StrLiteral{Value: `"world"`},
					},
					InterpolatedStr{
						Chunks: []Expression{
							StrLiteral{Value: "Hello, "},
							Identifier{Name: "name"},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestComments(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Single line comment",
			input: "// this is a comment",
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					Comment{Value: "// this is a comment"},
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
					TypeDeclaration{
						Name: Identifier{Name: "Value"},
						Type: []DeclaredType{IntType{}, BooleanType{}},
					},
				},
			},
		},
	})
}
