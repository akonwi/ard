package javascript

import (
	"strings"
	"testing"

	tree_sitter_kon "github.com/akonwi/tree-sitter-kon/bindings/go"
	"github.com/google/go-cmp/cmp"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

var language *tree_sitter.Language
var parser *tree_sitter.Parser

func getParser() *tree_sitter.Parser {
	if language == nil {
		language = tree_sitter.NewLanguage(tree_sitter_kon.Language())
		parser = tree_sitter.NewParser()
		parser.SetLanguage(language)
	}
	return parser
}

func assertEquality(t *testing.T, got, want string) {
	t.Helper()
	diff := cmp.Diff(want, got)
	if diff != "" {
		t.Errorf("Generated code does not match (-want +got):\n%s", diff)
	}
}

func testVariableDeclaration(t *testing.T) {
	sourceCode := []byte(`
mut name: String = "Alice"
let age: Num = 30
let is_student = true`)

	tree := getParser().Parse(sourceCode, nil)
	js := GenerateJS(sourceCode, tree)
	assertEquality(t, js, strings.TrimLeft(`
let name = "Alice"
const age = 30
const is_student = true
`, "\n"))
}

func testFunctionDeclaration(t *testing.T) {
	sourceCode := []byte(`
fn get_hello() {
 "Hello, world!"
}`)
	js := GenerateJS(sourceCode, getParser().Parse(sourceCode, nil))
	assertEquality(t, js, strings.TrimLeft(`
function get_hello() {
	return "Hello, world!"
}`, "\n"))
}
