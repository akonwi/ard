package javascript

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker"
)

type jsGenerator struct {
	doc ast.Document
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

func (g *jsGenerator) generateStatement(statement ast.Statement, _isReturn ...bool) ast.Document {
	isReturn := len(_isReturn) > 0 && _isReturn[0]
	switch statement.(type) {
	case ast.StructDefinition: // skipped
	case ast.VariableDeclaration:
		decl := statement.(ast.VariableDeclaration)
		binding := "const"
		if decl.Mutable {
			binding = "let"
		}
		return ast.MakeDoc(fmt.Sprintf("%s %s = %s", binding, decl.Name, toJSExpression(decl.Value)))
	case ast.VariableAssignment:
		assignment := statement.(ast.VariableAssignment)
		return ast.MakeDoc(fmt.Sprintf(
			"%s %s %s",
			assignment.Name,
			resolveOperator(assignment.Operator),
			toJSExpression(assignment.Value),
		))
	case ast.FunctionDeclaration:
		decl := statement.(ast.FunctionDeclaration)
		params := make([]string, len(decl.Parameters))
		for i, param := range decl.Parameters {
			params[i] = param.Name
		}
		doc := ast.MakeDoc(fmt.Sprintf("function %s(%s) {", decl.Name, strings.Join(params, ", ")))
		for i, statement := range decl.Body {
			doc.Nest(g.generateStatement(statement, i == len(decl.Body)-1))
		}
		doc.Line("}")
		return doc
	case ast.EnumDefinition:
		{
			enum := statement.(ast.EnumDefinition)
			doc := ast.MakeDoc(fmt.Sprintf("const %s = Object.freeze({", enum.Type.Name))
			doc.Indent()
			for index, name := range enum.Type.Variants {
				content := fmt.Sprintf("%s: %d", name, index)
				if index < len(enum.Type.Variants)-1 {
					content += ","
				}
				doc.Line(content)
			}
			doc.Dedent()
			doc.Line("})")
			return doc
		}
	case ast.WhileLoop:
		{
			loop := statement.(ast.WhileLoop)
			doc := ast.MakeDoc(fmt.Sprintf("while (%s) {", toJSExpression(loop.Condition)))
			for _, statement := range loop.Body {
				doc.Nest(g.generateStatement(statement))
			}
			doc.Line("}")
			return doc
		}
	case ast.ForLoop:
		{
			doc := ast.MakeDoc("")
			loop := statement.(ast.ForLoop)
			if rangeExpr, ok := loop.Iterable.(ast.RangeExpression); ok {
				doc.Line(
					fmt.Sprintf(
						"for (let %s = %s; %s < %s; %s++) {",
						loop.Cursor.Name,
						toJSExpression(rangeExpr.Start),
						loop.Cursor.Name,
						toJSExpression(rangeExpr.End),
						loop.Cursor.Name,
					))
				goto print_body_and_close
			}

			if primitive, ok := loop.Iterable.GetType().(checker.PrimitiveType); ok {
				if primitive == checker.BoolType {
					panic("Cannot iterate over a boolean")
				}

				if primitive == checker.StrType {
					doc.Line(fmt.Sprintf("for (const %s of %s) {", loop.Cursor.Name, toJSExpression(loop.Iterable)))
				} else {
					doc.Line(
						fmt.Sprintf(
							"for (let %s = 0; %s < %s; %s++) {",
							loop.Cursor.Name,
							loop.Cursor.Name,
							toJSExpression(loop.Iterable),
							loop.Cursor.Name,
						),
					)
				}
				goto print_body_and_close
			}

			if _, ok := loop.Iterable.GetType().(checker.ListType); ok {
				doc.Line(fmt.Sprintf("for (const %s of %s) {", loop.Cursor.Name, toJSExpression(loop.Iterable)))
				goto print_body_and_close
			}

			panic(fmt.Errorf("Cannot loop over %s", loop.Iterable))

		print_body_and_close:
			for _, statement := range loop.Body {
				doc.Nest(g.generateStatement(statement))
			}
			doc.Line("}")
			return doc
		}
	case ast.IfStatement:
		{
			doc := ast.MakeDoc("")
			stmt := statement.(ast.IfStatement)
			if stmt.Condition != nil {
				doc.Line(fmt.Sprintf("if (%s) {", toJSExpression(stmt.Condition)))
			} else {
				start := stmt.TSNode.StartPosition()
				panic(fmt.Errorf("[%d:%d] Condition is required for if statement", start.Row, start.Column))
			}

			for _, statement := range stmt.Body {
				doc.Nest(g.generateStatement(statement))
			}

			if stmt.Else != nil {
				doc.Append(g.generateElseStatement(stmt.Else.(ast.IfStatement)))
			} else {
				doc.Line("}")
			}

			return doc
		}
	case ast.Comment:
		return ast.MakeDoc(statement.(ast.Comment).Value)
	default:
		if expr, ok := statement.(ast.Expression); ok {
			js := toJSExpression(expr, true)
			if isReturn {
				return ast.MakeDoc("return " + js)
			} else {
				return ast.MakeDoc(js)
			}
		}
		panic(fmt.Errorf("Unhandled statement node: [%s] - %s\n", reflect.TypeOf(statement), statement))
	}
	return ast.MakeDoc("")
}

func (g *jsGenerator) generateElseStatement(stmt ast.IfStatement) ast.Document {
	doc := ast.MakeDoc("")
	if stmt.Condition != nil {
		doc.Line(fmt.Sprintf("} else if (%s) {", toJSExpression(stmt.Condition)))
	} else {
		doc.Line("} else {")
	}

	body := ast.MakeDoc("")
	for _, statement := range stmt.Body {
		body.Append(g.generateStatement(statement))
	}

	doc.Nest(body)
	if stmt.Else != nil {
		doc.Append(g.generateElseStatement(stmt.Else.(ast.IfStatement)))
	} else {
		doc.Line("}")
	}
	return doc
}

// rather than futzing with the AST to avoid adding runtime models
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

func getJsFunctionCall(call ast.FunctionCall) ast.FunctionCall {
	if call.Name == "print" {
		call.Name = "console.log"
	}

	return call
}

func GenerateJS(program ast.Program) string {
	generator := jsGenerator{
		doc: ast.MakeDoc(""),
	}

	for _, statement := range program.Statements {
		generator.doc.Append(generator.generateStatement(statement))
	}

	return strings.ReplaceAll(generator.doc.String(), "%%", "%")
}

func toJSExpression(node ast.Expression, _isStatement ...bool) string {
	isStatement := len(_isStatement) > 0 && _isStatement[0]
	switch node.(type) {
	case ast.Identifier:
		return node.(ast.Identifier).Name
	case ast.StrLiteral:
		return node.(ast.StrLiteral).Value
	case ast.InterpolatedStr:
		{
			str := node.(ast.InterpolatedStr)
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
	case ast.NumLiteral:
		return node.(ast.NumLiteral).Value
	case ast.BoolLiteral:
		return fmt.Sprintf("%v", node.(ast.BoolLiteral).Value)
	case ast.ListLiteral:
		{
			list := node.(ast.ListLiteral)
			items := make([]string, len(list.Items))
			for i, item := range list.Items {
				items[i] = toJSExpression(item)
			}
			return fmt.Sprintf("[%s]", strings.Join(items, ", "))
		}
	case ast.MapLiteral:
		{
			m := node.(ast.MapLiteral)
			entries := make([]string, len(m.Entries))
			for i, entry := range m.Entries {
				entries[i] = fmt.Sprintf(`[%s, %s]`, entry.Key, toJSExpression(entry.Value))
			}
			return fmt.Sprintf("new Map([%s])", strings.Join(entries, ", "))
		}
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
		doc := ast.MakeDoc(fmt.Sprintf("(%s) => {", strings.Join(params, ", ")))
		for i, statement := range fn.Body {
			doc.Nest((&jsGenerator{}).generateStatement(statement, i == len(fn.Body)-1))
		}
		doc.Line("}")
		return doc.String()
	case ast.StructInstance:
		instance := node.(ast.StructInstance)
		props := make([]string, len(instance.Properties))
		for i, entry := range instance.Properties {
			props[i] = fmt.Sprintf("%s: %s", entry.Name, toJSExpression(entry.Value))
		}
		return fmt.Sprintf("{%s}", strings.Join(props, ", "))
	case ast.FunctionCall:
		call := getJsFunctionCall(node.(ast.FunctionCall))
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
		{
			expr := node.(ast.MatchExpression)
			armsDoc := ast.MakeDoc("")
			for _, arm := range expr.Cases {
				armsDoc.Line(
					fmt.Sprintf(
						"if (%s === %s) {",
						toJSExpression(expr.Subject),
						toJSExpression(arm.Pattern),
					))

				for i, statement := range arm.Body {
					armsDoc.Nest((&jsGenerator{}).generateStatement(statement, i == len(arm.Body)-1))
				}
				armsDoc.Line("}")
			}
			iife := ast.MakeDoc("(() => {")
			iife.Nest(armsDoc)
			iife.Line("})()")
			if isStatement {
				return iife.String() + ";"
			}
			return iife.String()
		}
	default:
		return node.String()
	}
}
