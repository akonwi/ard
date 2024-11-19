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

var compareOptions = cmp.Options{
	cmp.FilterPath(func(p cmp.Path) bool {
		return p.Last().String() == ".BaseNode"
	}, cmp.Ignore()),
}

func TestEmptyProgram(t *testing.T) {
	sourceCode := []byte("")
	tree := treeSitterParser.Parse(sourceCode, nil)
	ast, err := NewParser(sourceCode).Parse(tree)
	if err != nil {
		t.Fatalf("Error parsing tree: %v", err)
	}

	if diff := cmp.Diff(&Program{Statements: []Statement{}}, ast, compareOptions); diff != "" {
		t.Errorf("AST does not match (-want +got):\n%s", diff)
	}
}

func TestVariableDeclarations(t *testing.T) {
	input := `
    let name: Str = "Alice"
    mut age: Num = 30
    let is_student = true`
	expected := &Program{
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
	}

	tree := treeSitterParser.Parse([]byte(input), nil)
	got, err := NewParser([]byte(input)).Parse(tree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diff := cmp.Diff(expected, got, compareOptions); diff != "" {
		t.Errorf("AST mismatch (-want +got):\n%s", diff)
	}
}
