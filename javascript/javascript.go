package javascript

import (
	"fmt"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type jsGenerator struct {
	builder     strings.Builder
	indentLevel int
	sourceCode  []byte
}

func (g *jsGenerator) getText(node tree_sitter.Node) string {
	return string(g.sourceCode[node.StartByte():node.EndByte()])
}

func (g *jsGenerator) indent() {
	g.indentLevel++
}

func (g *jsGenerator) dedent() {
	if g.indentLevel > 0 {
		g.indentLevel--
	}
}

func (g *jsGenerator) writeIndent() {
	g.builder.WriteString(strings.Repeat("  ", g.indentLevel))
}

func (g *jsGenerator) write(format string, args ...interface{}) {
	g.builder.WriteString(fmt.Sprintf(format, args...))
}

func (g *jsGenerator) writeLine(line string, args ...interface{}) {
	g.writeIndent()
	g.builder.WriteString(fmt.Sprintf(line, args...))
	g.builder.WriteString("\n")
}

func (g *jsGenerator) generateVariableDeclaration(node *tree_sitter.Node) {
	if g.getText(*node.NamedChild(0)) == "mut" {
		g.write("let ")
	} else {
		g.write("const ")
	}

	g.writeLine("%s = %s", g.getText(*node.NamedChild(1)), g.generateExpression(node.ChildByFieldName("value")))
}

func (g *jsGenerator) generateFunctionDeclaration(node *tree_sitter.Node) {}

func (g *jsGenerator) generateExpression(node *tree_sitter.Node) string {
	return g.getText(*node.NamedChild(0))
}

func GenerateJS(sourceCode []byte, tree *tree_sitter.Tree) string {
	generator := jsGenerator{
		sourceCode:  sourceCode,
		builder:     strings.Builder{},
		indentLevel: 0,
	}

	var generate func(node *tree_sitter.Node)
	generate = func(node *tree_sitter.Node) {
		switch node.GrammarName() {
		case "variable_definition":
			generator.generateVariableDeclaration(node)
		case "function_definition":
			generator.generateFunctionDeclaration(node)
		default:
			{
				for i := range node.ChildCount() {
					generate(node.Child(i))
				}
			}
		}
	}

	generate(tree.RootNode())

	return generator.builder.String()
}
