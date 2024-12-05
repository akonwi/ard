package javascript

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/akonwi/kon/ast"
	"github.com/akonwi/kon/checker"
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
	g.writeIndent()
	if decl.Mutable {
		g.write("let ")
	} else {
		g.write("const ")
	}

	g.write("%s = ", decl.Name)
	g.generateExpression(decl.Value)
	g.write("\n")
}

func (g *jsGenerator) generateFunctionDeclaration(decl ast.FunctionDeclaration) {
	g.writeIndent()
	g.write("function %s", decl.Name)
	g.write("(")
	for i, param := range decl.Parameters {
		if i > 0 {
			g.write(", ")
		}
		g.write(param.Name)
	}
	g.write(") ")

	if len(decl.Body) == 0 {
		g.write("{}\n")
	} else {
		g.writeLine("{")
		g.indent()
		for i, statement := range decl.Body {
			if i == len(decl.Body)-1 {
				if expr, ok := statement.(ast.Expression); ok {
					g.writeIndent()
					g.write("return ")
					g.generateExpression(expr)
					g.write("\n")
					continue
				}
			} else {
				g.generateStatement(statement)
			}
		}
		g.dedent()
		g.writeLine("}")
	}
}

func (g *jsGenerator) generateAnonymousFunction(decl ast.AnonymousFunction) {
	g.write("(")
	for i, param := range decl.Parameters {
		if i > 0 {
			g.write(", ")
		}
		g.write(param.Name)
	}
	g.write(") => {")

	if len(decl.Body) == 0 {
		g.write("}")
		return
	}

	g.write("\n")
	g.indent()
	for i, statement := range decl.Body {
		if i == len(decl.Body)-1 {
			if expr, ok := statement.(ast.Expression); ok {
				g.writeIndent()
				g.write("return ")
				g.generateExpression(expr)
				g.write("\n")
				continue
			}
		} else {
			g.generateStatement(statement)
		}
	}
	g.dedent()
	g.write("}")
}

func resolveOperator(operator ast.Operator) string {
	switch operator {
	case ast.Assign:
		return "="
	case ast.Equal:
		return "==="
	case ast.NotEqual:
		return "!=="
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
	case ast.Modulo:
		return "%"
	case ast.Or:
		return "||"
	case ast.And:
		return "&&"
	case ast.LessThan:
		return "<"
	case ast.LessThanOrEqual:
		return "<="
	case ast.GreaterThan:
		return ">"
	case ast.GreaterThanOrEqual:
		return ">="
	case ast.Bang:
		return "!"
	default:
		panic(fmt.Errorf("Unresolved operator: %v", operator))
	}
}

func (g *jsGenerator) generateVariableAssignment(assignment ast.VariableAssignment) {
	g.write("%s %s ", assignment.Name, resolveOperator(assignment.Operator))
	g.generateExpression(assignment.Value)
	g.write("\n")
}

func (g *jsGenerator) generateEnumDefinition(enum ast.EnumDefinition) {
	g.write("const %s = Object.freeze({\n", enum.Type.Name)
	g.indent()
	for index, name := range enum.Type.Variants {
		if index > 0 {
			g.write(",\n")
		}
		g.writeIndent()
		g.write("%s: Object.freeze({ index: %d })", name, index)
	}
	g.dedent()
	g.write("\n})")
}

func (g *jsGenerator) generateWhileLoop(loop ast.WhileLoop) {
	g.write("while (")
	g.generateExpression(loop.Condition)
	g.write(") {\n")
	g.indent()
	for _, statement := range loop.Body {
		g.generateStatement(statement)
	}
	g.dedent()
	g.write("}\n")
}

func (g *jsGenerator) generateForLoop(loop ast.ForLoop) {
	if rangeExpr, ok := loop.Iterable.(ast.RangeExpression); ok {
		g.writeIndent()
		g.write("for (let %s = ", loop.Cursor.Name)
		g.generateExpression(rangeExpr.Start)
		g.write("; ")
		g.write("%s < ", loop.Cursor.Name)
		g.generateExpression(rangeExpr.End)
		g.write("; ")
		g.write("%s++) {\n", loop.Cursor.Name)
		goto print_body_and_close
	}

	if primitive, ok := loop.Iterable.GetType().(checker.PrimitiveType); ok {
		if primitive == checker.BoolType {
			panic("Cannot iterate over a boolean")
		}

		g.writeIndent()
		if primitive == checker.StrType {
			g.write("for (const %s of ", loop.Cursor.Name)
			g.generateExpression(loop.Iterable)
			g.write(") {\n")
		} else {
			g.write("for (let %s = 0", loop.Cursor.Name)
			g.write("; %s < ", loop.Cursor.Name)
			g.generateExpression(loop.Iterable)
			g.write("; %s++) {\n", loop.Cursor.Name)
		}
		goto print_body_and_close
	}

	if _, ok := loop.Iterable.GetType().(checker.ListType); ok {
		g.writeIndent()
		g.write("for (const %s of ", loop.Cursor.Name)
		g.generateExpression(loop.Iterable)
		g.write(") {\n")

		goto print_body_and_close
	}

	panic(fmt.Errorf("Cannot loop over %s", loop.Iterable))

print_body_and_close:
	g.indent()
	for _, statement := range loop.Body {
		g.generateStatement(statement)
	}
	g.dedent()
	g.writeIndent()
	g.writeLine("}")
}

func (g *jsGenerator) generateStatement(statement ast.Statement) {
	switch statement.(type) {
	case ast.StructDefinition: // skipped
	case ast.VariableDeclaration:
		g.generateVariableDeclaration(statement.(ast.VariableDeclaration))
	case ast.VariableAssignment:
		g.generateVariableAssignment(statement.(ast.VariableAssignment))
	case ast.FunctionDeclaration:
		g.generateFunctionDeclaration(statement.(ast.FunctionDeclaration))
	case ast.EnumDefinition:
		g.generateEnumDefinition(statement.(ast.EnumDefinition))
	case ast.WhileLoop:
		g.generateWhileLoop(statement.(ast.WhileLoop))
	case ast.ForLoop:
		g.generateForLoop(statement.(ast.ForLoop))
	default:
		{
			if expr, ok := statement.(ast.Expression); ok {
				g.writeIndent()
				g.generateExpression(expr)
				g.write("\n")
			} else {
				panic(fmt.Errorf("Unhandled statement node: [%s] - %s\n", reflect.TypeOf(statement), statement))
			}
		}
	}
}

func (g *jsGenerator) generateStructInstance(instance ast.StructInstance) {
	g.write("{")
	if len(instance.Properties) > 0 {
		g.write(" ")
		for i, entry := range instance.Properties {
			if i > 0 {
				g.write(", ")
			} else {
				i++
			}
			g.write("%s: ", entry.Name)
			g.generateExpression(entry.Value)
		}
		g.write(" ")
	}
	g.write("}")
}

func (g *jsGenerator) generateExpression(expr ast.Expression) {
	switch expr.(type) {
	case ast.InterpolatedStr:
		g.write("`")
		for _, chunk := range expr.(ast.InterpolatedStr).Chunks {
			if _, ok := chunk.(ast.StrLiteral); ok {
				g.write(chunk.(ast.StrLiteral).Value)
			} else {
				g.write("${")
				g.generateExpression(chunk)
				g.write("}")
			}
		}
		g.write("`")
	case ast.StrLiteral:
		g.write(expr.(ast.StrLiteral).Value)
	case ast.NumLiteral:
		g.write(expr.(ast.NumLiteral).Value)
	case ast.BoolLiteral:
		g.write("%v", expr.(ast.BoolLiteral).Value)
	case ast.ListLiteral:
		g.write("[")
		for i, item := range expr.(ast.ListLiteral).Items {
			if i > 0 {
				g.write(", ")
			}
			g.generateExpression(item)
		}
		g.write("]")
	case ast.MapLiteral:
		g.write("new Map([")
		for i, entry := range expr.(ast.MapLiteral).Entries {
			if i > 0 {
				g.write(", ")
			}
			g.write("[")
			g.write(`%s, `, entry.Key)
			g.generateExpression(entry.Value)
			g.write("]")
		}
		g.write("])")
	case ast.Identifier:
		g.write("%s", expr.(ast.Identifier).Name)
	case ast.BinaryExpression:
		binary := expr.(ast.BinaryExpression)
		if binary.HasPrecedence {
			g.write("(")
		}
		g.generateExpression(binary.Left)
		g.write(" %s ", resolveOperator(binary.Operator))
		g.generateExpression(binary.Right)
		if binary.HasPrecedence {
			g.write(")")
		}
	case ast.UnaryExpression:
		unary := expr.(ast.UnaryExpression)
		g.write("%s", resolveOperator(unary.Operator))
		g.generateExpression(unary.Operand)
	case ast.AnonymousFunction:
		g.generateAnonymousFunction(expr.(ast.AnonymousFunction))
	case ast.StructInstance:
		g.generateStructInstance(expr.(ast.StructInstance))
	default:
		panic(fmt.Errorf("Unhandled expression node: [%s] - %s\n", reflect.TypeOf(expr), expr))
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
