package javascript

import (
	"fmt"
	"strings"
	"testing"

	"github.com/akonwi/kon/ast"
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

func assertEquality(t *testing.T, got, want string) {
	t.Helper()
	diff := cmp.Diff(want, got)
	if diff != "" {
		t.Errorf("Generated code does not match (-want +got):\n%s", diff)
	}
}

type test struct {
	name, input, output string
}

func runTests(t *testing.T, tests []test) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := treeSitterParser.Parse([]byte(tt.input), nil)
			parser := ast.NewParser([]byte(tt.input), tree)
			ast, err := parser.Parse()
			if err != nil {
				t.Fatal(fmt.Errorf("Error parsing tree: %v", err))
			}

			js := GenerateJS(ast)

			if diff := cmp.Diff(tt.output, js, cmp.Transformer("SpaceRemover", strings.TrimSpace)); diff != "" {
				t.Errorf("Generated javascript does not match (-want +got):\n%s", diff)
			}
		})
	}
}

func TestVariableDeclaration(t *testing.T) {
	tests := []test{
		{
			name:   "mutable string",
			input:  `mut explicit: Str = "Alice"`,
			output: `let explicit = "Alice"`,
		},
		{
			name:   "immutable string",
			input:  `let explicit = "Alice"`,
			output: `const explicit = "Alice"`,
		},
		{
			name:   "mutable number",
			input:  `mut power = 200`,
			output: `let power = 200`,
		},
		{
			name:   "immutable number",
			input:  `let power = 200`,
			output: `const power = 200`,
		},
		{
			name:   "mutable boolean",
			input:  `mut is_valid = true`,
			output: `let is_valid = true`,
		},
		{
			name:   "immutable boolean",
			input:  `let is_valid = false`,
			output: `const is_valid = false`,
		},
	}

	runTests(t, tests)
}

func TestVariableAssignment(t *testing.T) {
	runTests(t, []test{
		{
			name: "string assignment",
			input: `
mut name = "Alice"
name = "Bob"`,
			output: `
let name = "Alice"
name = "Bob"`,
		},
	})
}

func TestExpressions(t *testing.T) {
	tests := []test{
		{
			name: "identifier",
			input: `
let x = 42
let y = x`,
			output: `
const x = 42
const y = x`,
		},
		{
			name:   "raw string",
			input:  `"foobar"`,
			output: `"foobar"`,
		},
		{
			name:   "interpolated string",
			input:  `"foobar {{ 42 }}"`,
			output: "`foobar ${42}`",
		},
		{
			name:   "list literal",
			input:  `[1, 2, 3]`,
			output: `[1, 2, 3]`,
		},
		{
			name:   "map literal",
			input:  `["jane": 1, "joe": 2]`,
			output: `new Map([["jane", 1], ["joe", 2]])`,
		},
	}

	runTests(t, tests)
}

func TestFunctionDeclaration(t *testing.T) {
	tests := []test{
		{
			name:   "noop",
			input:  `fn noop() {}`,
			output: "function noop() {}",
		},
		{
			name:   "with parameters",
			input:  `fn add(x: Num, y: Num) {}`,
			output: "function add(x, y) {}",
		},
		{
			name:   "with return type",
			input:  `fn add(x: Num, y: Num) Num {}`,
			output: "function add(x, y) {}",
		},
		// 		{
		// 			name:  "single statement body: return is implicit",
		// 			input: `fn add(x: Num, y: Num) Num { 42 }`,
		// 			output: `
		// function add(x, y) {
		// 	return 42
		// }`,
		// 		},
	}

	runTests(t, tests)
}
