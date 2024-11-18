package javascript

import (
	"strings"
	"testing"

	tree_sitter_kon "github.com/akonwi/tree-sitter-kon/bindings/go"
	"github.com/google/go-cmp/cmp"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestVariableDeclaration(t *testing.T) {
	language := tree_sitter.NewLanguage(tree_sitter_kon.Language())
	if language == nil {
		t.Error("Error loading Kon grammar")
	}
	parser := tree_sitter.NewParser()
	parser.SetLanguage(language)
	sourceCode := []byte(`
mut name: String = "Alice"
let age: Num = 30
let is_student = true`)

	tree := parser.Parse(sourceCode, nil)
	js := GenerateJS(sourceCode, tree)
	assertEquality(t, js, strings.TrimLeft(`
let name = "Alice"
const age = 30
const is_student = true
`, "\n"))
}

func assertEquality(t *testing.T, got, want string) {
	t.Helper()
	diff := cmp.Diff(want, got)
	if diff != "" {
		t.Errorf("Generated code does not match (-want +got):\n%s", diff)
	}
}
