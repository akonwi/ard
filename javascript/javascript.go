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

func (g jsGenerator) getIndent() string {
	return strings.Repeat("  ", g.indentLevel)
}

func (g *jsGenerator) writeIndent() {
	g.builder.WriteString(g.getIndent())
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
	binding := "const"
	if decl.Mutable {
		binding = "let"
	}
	g.writeLine("%s %s = %s", binding, decl.Name, toJSExpression(decl.Value))
}

func (g *jsGenerator) generateFunctionDeclaration(decl ast.FunctionDeclaration) {
	params := make([]string, len(decl.Parameters))
	for i, param := range decl.Parameters {
		params[i] = param.Name
	}
	g.writeLine("function %s(%s) {", decl.Name, strings.Join(params, ", "))
	g.indent()
	for i, statement := range decl.Body {
		g.generateStatement(statement, i == len(decl.Body)-1)
	}
	g.dedent()
	g.writeLine("}")
}

func (g *jsGenerator) generateAnonymousFunction(decl ast.AnonymousFunction) {
	params := make([]string, len(decl.Parameters))
	for i, param := range decl.Parameters {
		params[i] = param.Name
	}
	g.write("(")
	g.write(") => {")

	if len(decl.Body) == 0 {
		g.write("}")
		return
	}

	g.write("\n")
	g.indent()
	for i, statement := range decl.Body {
		g.generateStatement(statement, i == len(decl.Body)-1)
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
		return "%%"
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
	g.writeLine("%s %s %s", assignment.Name, resolveOperator(assignment.Operator), toJSExpression(assignment.Value))
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
	g.write("\n})\n")
}

func (g *jsGenerator) generateWhileLoop(loop ast.WhileLoop) {
	g.writeLine("while (%s) {", toJSExpression(loop.Condition))
	g.indent()
	for _, statement := range loop.Body {
		g.generateStatement(statement)
	}
	g.dedent()
	g.write("}\n")
}

func (g *jsGenerator) generateForLoop(loop ast.ForLoop) {
	if rangeExpr, ok := loop.Iterable.(ast.RangeExpression); ok {
		g.writeLine(
			"for (let %s = %s; %s < %s; %s++) {",
			loop.Cursor.Name,
			toJSExpression(rangeExpr.Start),
			loop.Cursor.Name,
			toJSExpression(rangeExpr.End),
			loop.Cursor.Name,
		)
		goto print_body_and_close
	}

	if primitive, ok := loop.Iterable.GetType().(checker.PrimitiveType); ok {
		if primitive == checker.BoolType {
			panic("Cannot iterate over a boolean")
		}

		g.writeIndent()
		if primitive == checker.StrType {
			g.writeLine("for (const %s of %s) {", loop.Cursor.Name, toJSExpression(loop.Iterable))
		} else {
			g.writeLine(
				"for (let %s = 0; %s < %s; %s++) {",
				loop.Cursor.Name,
				loop.Cursor.Name,
				toJSExpression(loop.Iterable),
				loop.Cursor.Name,
			)
		}
		goto print_body_and_close
	}

	if _, ok := loop.Iterable.GetType().(checker.ListType); ok {
		g.writeLine("for (const %s of %s) {", loop.Cursor.Name, toJSExpression(loop.Iterable))
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

func (g *jsGenerator) generateIfStatement(stmt ast.IfStatement) {
	// if stmt.condition, build the 'if' statement
	// otherwise build the block following the 'else'
	if stmt.Condition != nil {
		g.writeLine("if (%s) {", toJSExpression(stmt.Condition))
	} else {
		g.writeLine("{")
	}

	g.indent()
	for _, statement := range stmt.Body {
		g.generateStatement(statement)
	}
	g.dedent()
	g.write("}")

	if stmt.Else != nil {
		g.write(" else ")
		g.generateStatement(stmt.Else)
	} else {
		g.write("\n")
	}
}

func (g *jsGenerator) generateStatement(statement ast.Statement, _isReturn ...bool) {
	isReturn := len(_isReturn) > 0 && _isReturn[0]
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
	case ast.IfStatement:
		g.generateIfStatement(statement.(ast.IfStatement))
	default:
		{
			if expr, ok := statement.(ast.Expression); ok {
				js := toJSExpression(expr, true)
				if isReturn {
					g.writeLine("return %s", js)
				} else {
					g.writeLine(js)
				}
			} else {
				panic(fmt.Errorf("Unhandled statement node: [%s] - %s\n", reflect.TypeOf(statement), statement))
			}
		}
	}
}

// rather than futzing with the AST, use a custom Str class JS built-in
func getJsMemberAccess(expr ast.MemberAccess) ast.MemberAccess {
	if expr.Target.GetType().String() == checker.StrType.String() {
		if expr.Member.(ast.Identifier).Name == "size" {
			return ast.MemberAccess{
				Target:     expr.Target,
				AccessType: expr.AccessType,
				Member:     ast.Identifier{Name: "length", Type: expr.Member.GetType()},
			}
		}
	}

	return expr
}

func (g *jsGenerator) generateExpression(expr ast.Expression, _isStatement ...bool) {
	switch expr.(type) {
	case ast.InterpolatedStr:
		g.write("`")
		for _, chunk := range expr.(ast.InterpolatedStr).Chunks {
			if _, ok := chunk.(ast.StrLiteral); ok {
				g.generateExpression(chunk.(ast.StrLiteral))
			} else {
				g.write("${")
				g.generateExpression(chunk)
				g.write("}")
			}
		}
		g.write("`")
	case ast.StrLiteral:
		g.write(toJSExpression(expr))
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
		g.write("%s%s", resolveOperator(unary.Operator), toJSExpression(unary.Operand))
	case ast.AnonymousFunction:
		g.generateAnonymousFunction(expr.(ast.AnonymousFunction))
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

func toJSExpression(node ast.Expression, _isStatement ...bool) string {
	isStatement := len(_isStatement) > 0 && _isStatement[0]
	switch node.(type) {
	case ast.Identifier:
		return node.(ast.Identifier).Name
	case ast.StrLiteral:
		return node.(ast.StrLiteral).Value
	case ast.InterpolatedStr:
		return interpolationToJs(node.(ast.InterpolatedStr))
	case ast.NumLiteral:
		return node.(ast.NumLiteral).Value
	case ast.BoolLiteral:
		return fmt.Sprintf("%v", node.(ast.BoolLiteral).Value)
	case ast.ListLiteral:
		return listLiteralToJs(node.(ast.ListLiteral))
	case ast.MapLiteral:
		return mapLiteralToJs(node.(ast.MapLiteral))
	case ast.BinaryExpression:
		binary := node.(ast.BinaryExpression)
		lhs := toJSExpression(binary.Left)
		op := resolveOperator(binary.Operator)
		rhs := toJSExpression(binary.Right)
		if binary.HasPrecedence {
			return "(" + lhs + " " + op + " " + rhs + ")"
		}
		return lhs + " " + op + " " + rhs
	case ast.UnaryExpression:
		unary := node.(ast.UnaryExpression)
		return resolveOperator(unary.Operator) + toJSExpression(unary.Operand)
	case ast.AnonymousFunction:
		fn := node.(ast.AnonymousFunction)
		params := make([]string, len(fn.Parameters))
		for i, param := range fn.Parameters {
			params[i] = param.Name
		}
		generator := jsGenerator{}
		generator.indent()
		for i, statement := range fn.Body {
			generator.generateStatement(statement, i == len(fn.Body)-1)
		}
		generator.dedent()
		return fmt.Sprintf("(%s) => {\n%s}", strings.Join(params, ", "), generator.builder.String())
	case ast.StructInstance:
		instance := node.(ast.StructInstance)
		props := make([]string, len(instance.Properties))
		for i, entry := range instance.Properties {
			props[i] = fmt.Sprintf("%s: %s", entry.Name, toJSExpression(entry.Value))
		}
		return fmt.Sprintf("{%s}", strings.Join(props, ", "))
	case ast.FunctionCall:
		call := node.(ast.FunctionCall)
		args := make([]string, len(call.Args))
		for i, arg := range call.Args {
			args[i] = toJSExpression(arg)
		}
		result := fmt.Sprintf("%s(%s)", call.Name, strings.Join(args, ", "))
		if isStatement {
			result += ";"
		}
		return result
	case ast.MemberAccess:
		expr := node.(ast.MemberAccess)
		jsExpr := getJsMemberAccess(expr)
		return fmt.Sprintf("%s.%s", toJSExpression(jsExpr.Target), toJSExpression(jsExpr.Member))
	case ast.MatchExpression:
		return matchExprToJs(node.(ast.MatchExpression), isStatement)
	default:
		return node.String()
	}
}

func interpolationToJs(str ast.InterpolatedStr) string {
	output := "`"
	for _, chunk := range str.Chunks {
		if _, ok := chunk.(ast.StrLiteral); ok {
			output += chunk.(ast.StrLiteral).Value
		} else {
			output += fmt.Sprintf("${%s}", toJSExpression(chunk))
		}
	}
	return output + "`"
}

func listLiteralToJs(list ast.ListLiteral) string {
	items := make([]string, len(list.Items))
	for i, item := range list.Items {
		items[i] = toJSExpression(item)
	}
	return fmt.Sprintf("[%s]", strings.Join(items, ", "))
}

func mapLiteralToJs(m ast.MapLiteral) string {
	entries := make([]string, len(m.Entries))
	for i, entry := range m.Entries {
		entries[i] = fmt.Sprintf(`[%s, %s]`, entry.Key, toJSExpression(entry.Value))
	}
	return fmt.Sprintf("new Map([%s])", strings.Join(entries, ", "))
}

func matchExprToJs(expr ast.MatchExpression, isStatement bool) string {
	g := jsGenerator{}
	g.indent()
	for _, arm := range expr.Cases {
		g.writeLine("if (%s === %s) {", toJSExpression(expr.Subject), toJSExpression(arm.Pattern))
		g.indent()
		for i, statement := range arm.Body {
			g.generateStatement(statement, i == len(arm.Body)-1)
		}
		g.dedent()
		g.writeLine("}")
	}
	g.indent()
	result := fmt.Sprintf("(() => {\n%s})()", g.builder.String())
	if isStatement {
		result += ";"
	}
	return result
}
