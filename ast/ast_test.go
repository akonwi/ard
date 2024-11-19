package ast

import (
	"testing"

	tree_sitter_kon "github.com/akonwi/tree-sitter-kon/bindings/go"
	"github.com/google/go-cmp/cmp"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

var treeSitterParser *tree_sitter.Parser

func init() {
	language := tree_sitter.NewLanguage(tree_sitter_kon.Language())
	treeSitterParser = tree_sitter.NewParser()
	treeSitterParser.SetLanguage(language)
}

func TestEmptyProgram(t *testing.T) {
	assertAST(t, []byte(""), &Program{Statements: []Statement{}})
}

func TestVariableDeclarations(t *testing.T) {
	assertAST(t, []byte(`
    let name: Str = "Alice"
    mut age: Num = 30
    let is_student = true`),
		&Program{
			Statements: []Statement{
				&VariableDeclaration{
					Name:    "name",
					Mutable: false,
					Value: &StrLiteral{
						Value: `"Alice"`,
					},
				},
				&VariableDeclaration{
					Name:    "age",
					Mutable: true,
					Value: &NumLiteral{
						Value: "30",
					},
				},
				&VariableDeclaration{
					Name:    "is_student",
					Mutable: false,
					Value: &BoolLiteral{
						Value: true,
					},
				},
			},
		},
	)
}

func TestVariableFunctionDeclaration(t *testing.T) {
	assertAST(t, []byte(`
		fn get_msg() {
			"Hello, world!"
		}
		fn greet(person: Str) Str {
		}
		fn add(x: Num, y: Num) Num {
		}
	`), &Program{
		Statements: []Statement{
			&FunctionDeclaration{
				Name:       "get_msg",
				Parameters: []Parameter{},
				Body: []Statement{
					&StrLiteral{
						Value: `"Hello, world!"`,
					},
				},
			},
			&FunctionDeclaration{
				Name: "greet",
				Parameters: []Parameter{
					{
						Name: "person",
					},
				},
				Body: []Statement{},
			},
			&FunctionDeclaration{
				Name: "add",
				Parameters: []Parameter{
					{
						Name: "x",
					},
					{
						Name: "y",
					},
				},
				Body: []Statement{},
			},
		},
	})
}

var compareOptions = cmp.Options{
	cmp.FilterPath(func(p cmp.Path) bool {
		return p.Last().String() == ".BaseNode"
	}, cmp.Ignore()),
}

func assertAST(t *testing.T, input []byte, want *Program) {
	t.Helper()

	tree := treeSitterParser.Parse(input, nil)
	ast, err := NewParser(input, tree).Parse()
	if err != nil {
		t.Fatalf("Error parsing tree: %v", err)
	}

	diff := cmp.Diff(want, ast, compareOptions)
	if diff != "" {
		t.Errorf("Generated code does not match (-want +got):\n%s", diff)
	}
}
