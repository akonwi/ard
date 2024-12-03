package javascript

import (
	"fmt"
	"strings"

	"github.com/akonwi/kon/ast"
)

type jsGenerator struct {
	builder     strings.Builder
	indentLevel int
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

func (g *jsGenerator) generateVariableDeclaration(decl ast.VariableDeclaration) {
	if decl.Mutable {
		g.write("let ")
	} else {
		g.write("const ")
	}

	g.write("%s = ", decl.Name)
	g.generateExpression(decl.Value)
	g.writeLine("")
}

func (g *jsGenerator) generateFunctionDeclaration(decl ast.FunctionDeclaration) {
	g.write("function %s", decl.Name)
	g.write("(")
	for i, param := range decl.Parameters {
		if i > 0 {
			g.write(", ")
		}
		g.write("%s", param.Name)
	}
	g.write(") ")

	if len(decl.Body) == 0 {
		g.writeLine("{}")
	}
}

func resolveOperator(operator ast.Operator) string {
	switch operator {
	case ast.Assign:
		return "="
	case ast.Increment:
		return "+="
	case ast.Decrement:
		return "-="
	case ast.Multiply:
		return "*"
	case ast.Divide:
		return "/"
	case ast.Plus:
		return "+"
	case ast.Minus:
		return "-"
	default:
		panic(fmt.Errorf("Unresolved operator: %v", operator))
	}
}

func (g *jsGenerator) generateVariableAssignment(assignment ast.VariableAssignment) {
	g.write("%s %s ", assignment.Name, resolveOperator(assignment.Operator))
	g.generateExpression(assignment.Value)
}

func (g *jsGenerator) generateStatement(statement ast.Statement) {
	switch statement.(type) {
	case ast.VariableDeclaration:
		g.generateVariableDeclaration(statement.(ast.VariableDeclaration))
	case ast.VariableAssignment:
		g.generateVariableAssignment(statement.(ast.VariableAssignment))
	case ast.FunctionDeclaration:
		g.generateFunctionDeclaration(statement.(ast.FunctionDeclaration))
	default:
		{
			panic(fmt.Errorf("Unhandled statement node: %s\n", statement))
		}
	}
}

func (g *jsGenerator) generateExpression(expr ast.Expression) {
	switch expr.(type) {
	case ast.StrLiteral:
		g.write(expr.(ast.StrLiteral).Value)
	case ast.NumLiteral:
		g.write(expr.(ast.NumLiteral).Value)
	case ast.BoolLiteral:
		g.write("%v", expr.(ast.BoolLiteral).Value)
	default:
		panic(fmt.Errorf("Unhandled expression node: %s\n", expr))
	}
}

func GenerateJS(program ast.Program) string {
	generator := jsGenerator{
		builder:     strings.Builder{},
		indentLevel: 0,
	}

	for _, statement := range program.Statements {
		generator.generateStatement(statement)
	}

	return generator.builder.String()
}
