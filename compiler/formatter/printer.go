package formatter

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/akonwi/ard/parse"
)

const indentWidth = 2

type printer struct {
	maxLineWidth int
}

func newPrinter(maxLineWidth int) printer {
	return printer{maxLineWidth: maxLineWidth}
}

func (p printer) program(program *parse.Program) string {
	if program == nil {
		return ""
	}

	lines := make([]string, 0)
	importLines := p.renderImports(program.Imports)
	if len(importLines) > 0 {
		lines = append(lines, importLines...)
	}

	if len(program.Statements) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		var previous parse.Statement
		haveRendered := false
		for _, statement := range program.Statements {
			if statement == nil {
				continue
			}
			if haveRendered && hasSourceBlankLine(previous, statement) {
				lines = append(lines, "")
			}
			lines = append(lines, p.renderStatement(statement, 0)...)
			previous = statement
			haveRendered = true
		}
	}

	return strings.Join(lines, "\n")
}

func (p printer) renderImports(imports []parse.Import) []string {
	if len(imports) == 0 {
		return nil
	}

	groups := map[int][]parse.Import{0: {}, 1: {}, 2: {}}
	for _, item := range imports {
		groups[importGroup(item.Path)] = append(groups[importGroup(item.Path)], item)
	}
	for _, key := range []int{0, 1, 2} {
		sort.Slice(groups[key], func(i, j int) bool {
			return groups[key][i].Path < groups[key][j].Path
		})
	}

	lines := make([]string, 0, len(imports)+2)
	for _, key := range []int{0, 1, 2} {
		if len(groups[key]) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		for _, item := range groups[key] {
			lines = append(lines, p.renderImport(item))
		}
	}

	return lines
}

func (p printer) renderImport(item parse.Import) string {
	defaultName := defaultImportName(item.Path)
	if item.Name != "" && item.Name != defaultName {
		return fmt.Sprintf("use %s as %s", item.Path, item.Name)
	}
	return fmt.Sprintf("use %s", item.Path)
}

func importGroup(importPath string) int {
	if strings.HasPrefix(importPath, "ard/") {
		return 0
	}
	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		return 2
	}
	return 1
}

func defaultImportName(importPath string) string {
	leaf := path.Base(importPath)
	leaf = strings.ReplaceAll(leaf, "-", "_")
	leaf = strings.ReplaceAll(leaf, ".", "_")
	return leaf
}

func (p printer) renderStatement(statement parse.Statement, indent int) []string {
	if statement == nil {
		return nil
	}

	switch node := statement.(type) {
	case *parse.Comment:
		return []string{p.indent(indent) + p.renderComment(node.Value)}
	case *parse.VariableDeclaration:
		return p.renderVariableDeclarationLines(node, indent)
	case *parse.VariableAssignment:
		return p.renderVariableAssignmentLines(node, indent)
	case *parse.FunctionDeclaration:
		return p.renderFunctionDeclaration(node, indent, false)
	case *parse.StaticFunctionDeclaration:
		return p.renderStaticFunctionDeclaration(node, indent)
	case *parse.ExternalFunction:
		return []string{p.indent(indent) + p.renderExternalFunction(node)}
	case *parse.StructDefinition:
		return p.renderStructDefinition(node, indent)
	case *parse.TraitDefinition:
		return p.renderTraitDefinition(node, indent)
	case *parse.ImplBlock:
		return p.renderImplBlock(node, indent)
	case *parse.TraitImplementation:
		return p.renderTraitImplementation(node, indent)
	case *parse.EnumDefinition:
		return p.renderEnumDefinition(node, indent)
	case *parse.WhileLoop:
		return p.renderWhileLoop(node, indent)
	case *parse.ForInLoop:
		return p.renderForInLoop(node, indent)
	case *parse.RangeLoop:
		return p.renderRangeLoop(node, indent)
	case *parse.ForLoop:
		return p.renderForLoop(node, indent)
	case *parse.IfStatement:
		return p.renderIfStatement(node, indent)
	case *parse.Break:
		return []string{p.indent(indent) + "break"}
	case *parse.TypeDeclaration:
		return []string{p.indent(indent) + p.renderTypeDeclaration(node)}
	default:
		if expr, ok := statement.(parse.Expression); ok {
			return p.indentLines(p.renderExpression(expr, 0), indent)
		}
		return []string{p.indent(indent) + statement.String()}
	}
}

func (p printer) renderVariableDeclarationLines(node *parse.VariableDeclaration, indent int) []string {
	binding := "let"
	if node.Mutable {
		binding = "mut"
	}
	prefix := binding + " " + node.Name
	if node.Type != nil {
		prefix += ": " + p.renderType(node.Type)
	}
	prefix += " = "
	valueLines := strings.Split(p.renderExpression(node.Value, 0), "\n")
	lines := []string{p.indent(indent) + prefix + valueLines[0]}
	for i := 1; i < len(valueLines); i++ {
		lines = append(lines, p.indent(indent)+valueLines[i])
	}
	return lines
}

func (p printer) renderVariableAssignmentLines(node *parse.VariableAssignment, indent int) []string {
	operator, value := p.assignmentParts(node)
	prefix := p.renderExpression(node.Target, 0) + " " + operator + " "
	valueLines := strings.Split(p.renderExpression(value, 0), "\n")
	lines := []string{p.indent(indent) + prefix + valueLines[0]}
	for i := 1; i < len(valueLines); i++ {
		lines = append(lines, p.indent(indent)+valueLines[i])
	}
	return lines
}

func (p printer) renderVariableDeclaration(node *parse.VariableDeclaration) string {
	binding := "let"
	if node.Mutable {
		binding = "mut"
	}
	if node.Type != nil {
		return fmt.Sprintf("%s %s: %s = %s", binding, node.Name, p.renderType(node.Type), p.renderExpression(node.Value, 0))
	}
	return fmt.Sprintf("%s %s = %s", binding, node.Name, p.renderExpression(node.Value, 0))
}

func (p printer) renderVariableAssignment(node *parse.VariableAssignment) string {
	operator, value := p.assignmentParts(node)
	return fmt.Sprintf("%s %s %s", p.renderExpression(node.Target, 0), operator, p.renderExpression(value, 0))
}

func (p printer) assignmentParts(node *parse.VariableAssignment) (string, parse.Expression) {
	if node.Operator == parse.Increment {
		return "=+", node.Value
	}
	if node.Operator == parse.Decrement {
		return "=-", node.Value
	}

	if node.Operator == parse.Assign {
		if bin, ok := node.Value.(*parse.BinaryExpression); ok {
			if sameLocation(bin.Left, node.Target) {
				if bin.Operator == parse.Plus {
					return "=+", bin.Right
				}
				if bin.Operator == parse.Minus {
					return "=-", bin.Right
				}
			}
		}
	}

	return "=", node.Value
}

func sameLocation(left parse.Expression, right parse.Expression) bool {
	l := left.GetLocation()
	r := right.GetLocation()
	return l.Start.Row == r.Start.Row && l.Start.Col == r.Start.Col && l.End.Row == r.End.Row && l.End.Col == r.End.Col
}

func (p printer) renderFunctionDeclaration(node *parse.FunctionDeclaration, indent int, traitSignatureOnly bool) []string {
	prefix := ""
	if node.Private {
		prefix += "private "
	}
	header := prefix + "fn "
	if node.Mutates {
		header += "mut "
	}
	header += node.Name
	header += p.renderTypeParams(node.TypeParams)
	header += p.renderParameterList(node.Parameters, indent, header)
	if node.ReturnType != nil {
		header += " " + p.renderType(node.ReturnType)
	}

	if traitSignatureOnly {
		return []string{p.indent(indent) + header}
	}

	return p.renderBlockWithHeader(header, node.Body, indent)
}

func (p printer) renderStaticFunctionDeclaration(node *parse.StaticFunctionDeclaration, indent int) []string {
	prefix := ""
	if node.Private {
		prefix += "private "
	}
	header := prefix + "fn "
	if node.Mutates {
		header += "mut "
	}
	header += p.renderExpression(&node.Path, 0)
	header += p.renderTypeParams(node.TypeParams)
	header += p.renderParameterList(node.Parameters, indent, header)
	if node.ReturnType != nil {
		header += " " + p.renderType(node.ReturnType)
	}
	return p.renderBlockWithHeader(header, node.Body, indent)
}

func (p printer) renderExternalFunction(node *parse.ExternalFunction) string {
	prefix := ""
	if node.Private {
		prefix = "private "
	}
	header := prefix + "extern fn " + node.Name + p.renderTypeParams(node.TypeParams)
	header += p.renderParameterList(node.Parameters, 0, header)
	if node.ReturnType != nil {
		header += " " + p.renderType(node.ReturnType)
	}
	header += " = " + strconv.Quote(node.ExternalBinding)
	return header
}

func (p printer) renderStructDefinition(node *parse.StructDefinition, indent int) []string {
	prefix := ""
	if node.Private {
		prefix = "private "
	}
	header := prefix + "struct " + node.Name.Name
	if len(node.Fields) == 0 && len(node.Comments) == 0 {
		return []string{p.indent(indent) + header + " {}"}
	}

	lines := []string{p.indent(indent) + header + " {"}
	commentIndex := 0
	for _, field := range node.Fields {
		fieldRow := field.Name.Location.Start.Row
		for commentIndex < len(node.Comments) && (fieldRow == 0 || node.Comments[commentIndex].Location.Start.Row < fieldRow) {
			lines = append(lines, p.indent(indent+1)+p.renderComment(node.Comments[commentIndex].Value))
			commentIndex++
		}
		lines = append(lines, p.indent(indent+1)+fmt.Sprintf("%s: %s,", field.Name.Name, p.renderType(field.Type)))
	}
	for ; commentIndex < len(node.Comments); commentIndex++ {
		lines = append(lines, p.indent(indent+1)+p.renderComment(node.Comments[commentIndex].Value))
	}
	lines = append(lines, p.indent(indent)+"}")
	return lines
}

func (p printer) renderEnumDefinition(node *parse.EnumDefinition, indent int) []string {
	prefix := ""
	if node.Private {
		prefix = "private "
	}
	header := prefix + "enum " + node.Name
	if len(node.Variants) == 0 && len(node.Comments) == 0 {
		return []string{p.indent(indent) + header + " {}"}
	}
	lines := []string{p.indent(indent) + header + " {"}
	for _, comment := range node.Comments {
		lines = append(lines, p.indent(indent+1)+p.renderComment(comment.Value))
	}
	for _, variant := range node.Variants {
		if variant.Value == nil {
			lines = append(lines, p.indent(indent+1)+variant.Name+",")
		} else {
			lines = append(lines, p.indent(indent+1)+fmt.Sprintf("%s = %s,", variant.Name, p.renderExpression(variant.Value, 0)))
		}
	}
	lines = append(lines, p.indent(indent)+"}")
	return lines
}

func (p printer) renderTraitDefinition(node *parse.TraitDefinition, indent int) []string {
	prefix := ""
	if node.Private {
		prefix = "private "
	}
	header := prefix + "trait " + node.Name.Name
	if len(node.Methods) == 0 && len(node.Comments) == 0 {
		return []string{p.indent(indent) + header + " {}"}
	}
	lines := []string{p.indent(indent) + header + " {"}
	for _, comment := range node.Comments {
		lines = append(lines, p.indent(indent+1)+p.renderComment(comment.Value))
	}
	for _, method := range node.Methods {
		lines = append(lines, p.renderFunctionDeclaration(&method, indent+1, true)...)
	}
	lines = append(lines, p.indent(indent)+"}")
	return lines
}

func (p printer) renderImplBlock(node *parse.ImplBlock, indent int) []string {
	header := "impl " + node.Target.Name
	if len(node.Methods) == 0 && len(node.Comments) == 0 {
		return []string{p.indent(indent) + header + " {}"}
	}
	lines := []string{p.indent(indent) + header + " {"}
	for _, comment := range node.Comments {
		lines = append(lines, p.indent(indent+1)+p.renderComment(comment.Value))
	}
	for i, method := range node.Methods {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, p.renderFunctionDeclaration(&method, indent+1, false)...)
	}
	lines = append(lines, p.indent(indent)+"}")
	return lines
}

func (p printer) renderTraitImplementation(node *parse.TraitImplementation, indent int) []string {
	header := fmt.Sprintf("impl %s for %s", p.renderExpression(node.Trait, 0), node.ForType.Name)
	if len(node.Methods) == 0 {
		return []string{p.indent(indent) + header + " {}"}
	}
	lines := []string{p.indent(indent) + header + " {"}
	for i, method := range node.Methods {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, p.renderFunctionDeclaration(&method, indent+1, false)...)
	}
	lines = append(lines, p.indent(indent)+"}")
	return lines
}

func (p printer) renderTypeDeclaration(node *parse.TypeDeclaration) string {
	prefix := ""
	if node.Private {
		prefix = "private "
	}
	parts := make([]string, 0, len(node.Type))
	for _, item := range node.Type {
		parts = append(parts, p.renderType(item))
	}
	return fmt.Sprintf("%stype %s = %s", prefix, node.Name.Name, strings.Join(parts, " | "))
}

func (p printer) renderWhileLoop(node *parse.WhileLoop, indent int) []string {
	header := "while"
	if node.Condition != nil {
		header += " " + p.renderExpression(node.Condition, 0)
	}
	return p.renderBlockWithHeader(header, node.Body, indent)
}

func (p printer) renderRangeLoop(node *parse.RangeLoop, indent int) []string {
	header := fmt.Sprintf("for %s", node.Cursor.Name)
	if node.Cursor2.Name != "" {
		header += fmt.Sprintf(", %s", node.Cursor2.Name)
	}
	header += fmt.Sprintf(" in %s..%s", p.renderExpression(node.Start, 0), p.renderExpression(node.End, 0))
	return p.renderBlockWithHeader(header, node.Body, indent)
}

func (p printer) renderForInLoop(node *parse.ForInLoop, indent int) []string {
	header := fmt.Sprintf("for %s", node.Cursor.Name)
	if node.Cursor2.Name != "" {
		header += fmt.Sprintf(", %s", node.Cursor2.Name)
	}
	header += " in " + p.renderExpression(node.Iterable, 0)
	return p.renderBlockWithHeader(header, node.Body, indent)
}

func (p printer) renderForLoop(node *parse.ForLoop, indent int) []string {
	init := ""
	if node.Init != nil {
		binding := "let"
		if node.Init.Mutable {
			binding = "mut"
		}
		init = fmt.Sprintf("%s %s = %s", binding, node.Init.Name, p.renderExpression(node.Init.Value, 0))
	}
	condition := ""
	if node.Condition != nil {
		condition = p.renderExpression(node.Condition, 0)
	}
	increment := ""
	if node.Incrementer != nil {
		if incrementExpr, ok := node.Incrementer.(parse.Expression); ok {
			increment = p.renderExpression(incrementExpr, 0)
		} else if inc, ok := node.Incrementer.(*parse.VariableAssignment); ok {
			increment = p.renderVariableAssignment(inc)
		}
	}
	header := fmt.Sprintf("for %s; %s; %s", init, condition, increment)
	return p.renderBlockWithHeader(header, node.Body, indent)
}

func (p printer) renderIfStatement(node *parse.IfStatement, indent int) []string {
	if node.Condition == nil {
		return p.renderBlockWithHeader("else", node.Body, indent)
	}

	lines := []string{p.indent(indent) + "if " + p.renderExpression(node.Condition, 0) + " {"}
	lines = append(lines, p.renderStatements(node.Body, indent+1)...)

	next := node.Else
	for next != nil {
		elseIf, ok := next.(*parse.IfStatement)
		if !ok {
			break
		}
		if elseIf.Condition == nil {
			lines = append(lines, p.indent(indent)+"} else {")
			lines = append(lines, p.renderStatements(elseIf.Body, indent+1)...)
			lines = append(lines, p.indent(indent)+"}")
			return lines
		}
		lines = append(lines, p.indent(indent)+"} else if "+p.renderExpression(elseIf.Condition, 0)+" {")
		lines = append(lines, p.renderStatements(elseIf.Body, indent+1)...)
		next = elseIf.Else
	}

	if next != nil {
		lines = append(lines, p.indent(indent)+"} else {")
		lines = append(lines, p.renderStatement(next, indent+1)...)
		lines = append(lines, p.indent(indent)+"}")
		return lines
	}

	lines = append(lines, p.indent(indent)+"}")
	return lines
}

func (p printer) renderBlockWithHeader(header string, statements []parse.Statement, indent int) []string {
	if len(statements) == 0 {
		return []string{p.indent(indent) + header + " {}"}
	}
	lines := []string{p.indent(indent) + header + " {"}
	lines = append(lines, p.renderStatements(statements, indent+1)...)
	lines = append(lines, p.indent(indent)+"}")
	return lines
}

func (p printer) renderStatements(statements []parse.Statement, indent int) []string {
	if len(statements) == 0 {
		return nil
	}
	lines := make([]string, 0)
	var previous parse.Statement
	haveRendered := false
	for _, statement := range statements {
		if statement == nil {
			continue
		}
		if haveRendered && hasSourceBlankLine(previous, statement) {
			lines = append(lines, "")
		}
		rendered := p.renderStatement(statement, indent)
		if len(rendered) == 0 {
			continue
		}
		lines = append(lines, rendered...)
		previous = statement
		haveRendered = true
	}
	return lines
}

func (p printer) renderTypeParams(params []string) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, 0, len(params))
	for _, item := range params {
		parts = append(parts, "$"+item)
	}
	return "<" + strings.Join(parts, ", ") + ">"
}

func (p printer) renderParameterList(params []parse.Parameter, indent int, header string) string {
	parts := make([]string, 0, len(params))
	for _, parameter := range params {
		part := ""
		if parameter.Mutable {
			part += "mut "
		}
		part += parameter.Name + ": " + p.renderType(parameter.Type)
		parts = append(parts, part)
	}
	oneLine := "(" + strings.Join(parts, ", ") + ")"
	if len(parts) == 0 || len(header)+len(oneLine) <= p.maxLineWidth {
		return oneLine
	}

	lines := []string{"("}
	for _, part := range parts {
		lines = append(lines, p.indent(indent+1)+part+",")
	}
	lines = append(lines, p.indent(indent)+")")
	return strings.Join(lines, "\n")
}

func (p printer) renderType(declared parse.DeclaredType) string {
	switch node := declared.(type) {
	case *parse.StringType:
		return maybeNullable("Str", node.IsNullable())
	case *parse.IntType:
		return maybeNullable("Int", node.IsNullable())
	case *parse.FloatType:
		return maybeNullable("Float", node.IsNullable())
	case *parse.BooleanType:
		return maybeNullable("Bool", node.IsNullable())
	case *parse.VoidType:
		return maybeNullable("Void", node.IsNullable())
	case *parse.GenericType:
		return maybeNullable("$"+node.Name, node.IsNullable())
	case *parse.CustomType:
		name := node.Name
		if node.Type.Target != nil {
			name = p.renderExpression(&node.Type, 0)
		}
		if len(node.TypeArgs) > 0 {
			args := make([]string, 0, len(node.TypeArgs))
			for _, arg := range node.TypeArgs {
				args = append(args, p.renderType(arg))
			}
			name += "<" + strings.Join(args, ", ") + ">"
		}
		return maybeNullable(name, node.IsNullable())
	case *parse.List:
		return maybeNullable("["+p.renderType(node.Element)+"]", node.IsNullable())
	case *parse.Map:
		return maybeNullable("["+p.renderType(node.Key)+": "+p.renderType(node.Value)+"]", node.IsNullable())
	case *parse.ResultType:
		name := p.renderType(node.Val) + "!" + p.renderType(node.Err)
		return maybeNullable(name, node.IsNullable())
	case *parse.FunctionType:
		params := make([]string, 0, len(node.Params))
		for i, item := range node.Params {
			param := p.renderType(item)
			if i < len(node.ParamMutability) && node.ParamMutability[i] {
				param = "mut " + param
			}
			params = append(params, param)
		}
		name := "fn(" + strings.Join(params, ", ") + ")"
		if node.Return != nil {
			name += " " + p.renderType(node.Return)
		}
		return maybeNullable(name, node.IsNullable())
	default:
		return declared.GetName()
	}
}

func maybeNullable(base string, nullable bool) string {
	if nullable {
		return base + "?"
	}
	return base
}

func (p printer) renderExpression(expression parse.Expression, parentPrecedence int) string {
	if expression == nil {
		return ""
	}

	switch node := expression.(type) {
	case *parse.Identifier:
		return node.Name
	case parse.Identifier:
		return node.Name
	case *parse.StrLiteral:
		return strconv.Quote(node.Value)
	case parse.StrLiteral:
		return strconv.Quote(node.Value)
	case *parse.InterpolatedStr:
		return p.renderInterpolatedString(node)
	case parse.InterpolatedStr:
		copy := node
		return p.renderInterpolatedString(&copy)
	case *parse.NumLiteral:
		return node.Value
	case parse.NumLiteral:
		return node.Value
	case *parse.BoolLiteral:
		if node.Value {
			return "true"
		}
		return "false"
	case parse.BoolLiteral:
		if node.Value {
			return "true"
		}
		return "false"
	case *parse.VoidLiteral:
		return "()"
	case parse.VoidLiteral:
		return "()"
	case *parse.UnaryExpression:
		return p.renderUnary(node, parentPrecedence)
	case parse.UnaryExpression:
		copy := node
		return p.renderUnary(&copy, parentPrecedence)
	case *parse.BinaryExpression:
		return p.renderBinary(node, parentPrecedence)
	case parse.BinaryExpression:
		copy := node
		return p.renderBinary(&copy, parentPrecedence)
	case *parse.ChainedComparison:
		parts := make([]string, 0, len(node.Operands)*2)
		for i, operand := range node.Operands {
			if i > 0 {
				parts = append(parts, p.operatorString(node.Operators[i-1]))
			}
			parts = append(parts, p.renderExpression(operand, precedenceCompare))
		}
		return strings.Join(parts, " ")
	case *parse.RangeExpression:
		return p.renderExpression(node.Start, precedenceCompare) + ".." + p.renderExpression(node.End, precedenceCompare)
	case parse.RangeExpression:
		return p.renderExpression(node.Start, precedenceCompare) + ".." + p.renderExpression(node.End, precedenceCompare)
	case *parse.ListLiteral:
		return p.renderListLiteral(node)
	case parse.ListLiteral:
		copy := node
		return p.renderListLiteral(&copy)
	case *parse.MapLiteral:
		return p.renderMapLiteral(node)
	case parse.MapLiteral:
		copy := node
		return p.renderMapLiteral(&copy)
	case *parse.StructInstance:
		return p.renderStructInstance(node)
	case parse.StructInstance:
		copy := node
		return p.renderStructInstance(&copy)
	case *parse.FunctionCall:
		return p.renderFunctionCall(node)
	case parse.FunctionCall:
		copy := node
		return p.renderFunctionCall(&copy)
	case *parse.InstanceProperty:
		if id, ok := node.Target.(*parse.Identifier); ok && id.Name == "@" {
			return "@" + node.Property.Name
		}
		return p.renderExpression(node.Target, precedenceCall) + "." + node.Property.Name
	case parse.InstanceProperty:
		copy := node
		return p.renderExpression(&copy, parentPrecedence)
	case *parse.InstanceMethod:
		if id, ok := node.Target.(*parse.Identifier); ok && id.Name == "@" {
			return "@" + p.renderFunctionCall(&node.Method)
		}
		return p.renderExpression(node.Target, precedenceCall) + "." + p.renderFunctionCall(&node.Method)
	case parse.InstanceMethod:
		copy := node
		return p.renderExpression(&copy, parentPrecedence)
	case *parse.StaticProperty:
		return p.renderExpression(node.Target, precedenceCall) + "::" + p.renderExpression(node.Property, precedenceCall)
	case parse.StaticProperty:
		copy := node
		return p.renderExpression(&copy, parentPrecedence)
	case *parse.StaticFunction:
		return p.renderExpression(node.Target, precedenceCall) + "::" + p.renderFunctionCall(&node.Function)
	case parse.StaticFunction:
		copy := node
		return p.renderExpression(&copy, parentPrecedence)
	case *parse.MatchExpression:
		return p.renderMatchExpression(node)
	case *parse.ConditionalMatchExpression:
		return p.renderConditionalMatchExpression(node)
	case *parse.AnonymousFunction:
		head := "fn" + p.renderParameterList(node.Parameters, 0, "fn")
		if node.ReturnType != nil {
			head += " " + p.renderType(node.ReturnType)
		}
		if len(node.Body) == 0 {
			return head + " {}"
		}
		lines := []string{head + " {"}
		for _, line := range p.renderStatements(node.Body, 1) {
			lines = append(lines, line)
		}
		lines = append(lines, "}")
		return strings.Join(lines, "\n")
	case *parse.Try:
		text := "try " + p.renderExpression(node.Expression, precedenceUnary)
		if node.CatchVar != nil {
			text += " -> " + node.CatchVar.Name
			if len(node.CatchBlock) == 0 {
				text += " {}"
			} else {
				lines := []string{text + " {"}
				for _, line := range p.renderStatements(node.CatchBlock, 1) {
					lines = append(lines, line)
				}
				lines = append(lines, "}")
				return strings.Join(lines, "\n")
			}
		}
		return text
	case *parse.BlockExpression:
		if len(node.Statements) == 0 {
			return "{}"
		}
		lines := []string{"{"}
		for _, line := range p.renderStatements(node.Statements, 1) {
			lines = append(lines, line)
		}
		lines = append(lines, "}")
		return strings.Join(lines, "\n")
	default:
		return expression.String()
	}
}

func (p printer) renderInterpolatedString(value *parse.InterpolatedStr) string {
	var builder strings.Builder
	builder.WriteByte('"')
	p.writeInterpolatedChunks(&builder, value.Chunks)
	builder.WriteByte('"')
	return builder.String()
}

func (p printer) writeInterpolatedChunks(builder *strings.Builder, chunks []parse.Expression) {
	for _, chunk := range chunks {
		if literal, ok := chunk.(*parse.StrLiteral); ok {
			builder.WriteString(escapeInterpolatedText(literal.Value))
			continue
		}
		if nested, ok := chunk.(*parse.InterpolatedStr); ok {
			p.writeInterpolatedChunks(builder, nested.Chunks)
			continue
		}
		builder.WriteByte('{')
		builder.WriteString(p.renderExpression(chunk, 0))
		builder.WriteByte('}')
	}
}

func escapeInterpolatedText(value string) string {
	quoted := strconv.Quote(value)
	quoted = strings.TrimPrefix(quoted, "\"")
	quoted = strings.TrimSuffix(quoted, "\"")
	quoted = strings.ReplaceAll(quoted, "{", `\{`)
	quoted = strings.ReplaceAll(quoted, "}", `\}`)
	return quoted
}

func (p printer) renderListLiteral(list *parse.ListLiteral) string {
	items := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, p.renderExpression(item, 0))
	}
	oneLine := "[" + strings.Join(items, ", ") + "]"
	if len(items) == 0 || len(oneLine) <= p.maxLineWidth {
		return oneLine
	}

	lines := []string{"["}
	for _, comment := range list.Comments {
		lines = append(lines, p.indent(1)+p.renderComment(comment.Value))
	}
	for _, item := range items {
		lines = append(lines, p.indent(1)+item+",")
	}
	lines = append(lines, "]")
	return strings.Join(lines, "\n")
}

func (p printer) renderMapLiteral(m *parse.MapLiteral) string {
	if len(m.Entries) == 0 {
		return "[:]"
	}

	parts := make([]string, 0, len(m.Entries))
	for _, entry := range m.Entries {
		parts = append(parts, p.renderExpression(entry.Key, 0)+": "+p.renderExpression(entry.Value, 0))
	}
	oneLine := "[" + strings.Join(parts, ", ") + "]"
	if len(parts) == 0 || len(oneLine) <= p.maxLineWidth {
		return oneLine
	}

	lines := []string{"["}
	for _, comment := range m.Comments {
		lines = append(lines, p.indent(1)+p.renderComment(comment.Value))
	}
	for _, part := range parts {
		lines = append(lines, p.indent(1)+part+",")
	}
	lines = append(lines, "]")
	return strings.Join(lines, "\n")
}

func (p printer) renderStructInstance(node *parse.StructInstance) string {
	if len(node.Properties) == 0 && len(node.Comments) == 0 {
		return node.Name.Name + "{}"
	}

	parts := make([]string, 0, len(node.Properties))
	for _, property := range node.Properties {
		parts = append(parts, property.Name.Name+": "+p.renderExpression(property.Value, 0))
	}
	oneLine := node.Name.Name + "{" + strings.Join(parts, ", ") + "}"
	if len(parts) <= 1 && len(oneLine) <= p.maxLineWidth {
		return oneLine
	}

	lines := []string{node.Name.Name + "{"}
	for _, comment := range node.Comments {
		lines = append(lines, p.indent(1)+p.renderComment(comment.Value))
	}
	for _, part := range parts {
		lines = append(lines, p.indent(1)+part+",")
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

func (p printer) renderFunctionCall(node *parse.FunctionCall) string {
	head := node.Name
	if len(node.TypeArgs) > 0 {
		types := make([]string, 0, len(node.TypeArgs))
		for _, item := range node.TypeArgs {
			types = append(types, p.renderType(item))
		}
		head += "<" + strings.Join(types, ", ") + ">"
	}
	args := make([]string, 0, len(node.Args))
	for _, arg := range node.Args {
		part := ""
		if arg.Name != "" {
			part += arg.Name + ": "
		}
		if arg.Mutable {
			part += "mut "
		}
		part += p.renderExpression(arg.Value, 0)
		args = append(args, part)
	}
	oneLine := head + "(" + strings.Join(args, ", ") + ")"
	if len(args) <= 1 && len(oneLine) <= p.maxLineWidth {
		return oneLine
	}
	if len(args) > 1 && len(oneLine) <= p.maxLineWidth {
		return oneLine
	}

	lines := []string{head + "("}
	for _, comment := range node.Comments {
		lines = append(lines, p.indent(1)+p.renderComment(comment.Value))
	}
	for _, arg := range args {
		lines = append(lines, p.indent(1)+arg+",")
	}
	lines = append(lines, ")")
	return strings.Join(lines, "\n")
}

func (p printer) renderMatchExpression(node *parse.MatchExpression) string {
	lines := []string{"match " + p.renderExpression(node.Subject, 0) + " {"}
	for _, comment := range node.Comments {
		lines = append(lines, p.indent(1)+p.renderComment(comment.Value))
	}
	for _, matchCase := range node.Cases {
		caseLines := p.renderMatchCase(matchCase, 1)
		if len(caseLines) > 0 {
			caseLines[len(caseLines)-1] += ","
		}
		lines = append(lines, caseLines...)
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

func (p printer) renderConditionalMatchExpression(node *parse.ConditionalMatchExpression) string {
	lines := []string{"match {"}
	for _, comment := range node.Comments {
		lines = append(lines, p.indent(1)+p.renderComment(comment.Value))
	}
	for _, matchCase := range node.Cases {
		caseLines := p.renderConditionalMatchCase(matchCase, 1)
		if len(caseLines) > 0 {
			caseLines[len(caseLines)-1] += ","
		}
		lines = append(lines, caseLines...)
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

func (p printer) renderConditionalMatchCase(matchCase parse.ConditionalMatchCase, indent int) []string {
	pattern := "_"
	if matchCase.Condition != nil {
		pattern = p.renderExpression(matchCase.Condition, 0)
	}
	if len(matchCase.Body) == 0 {
		return []string{p.indent(indent) + pattern + " => ()"}
	}
	if len(matchCase.Body) == 1 {
		if expr, ok := matchCase.Body[0].(parse.Expression); ok {
			if canInlineMatchExpression(expr) {
				rendered := p.renderExpression(expr, 0)
				if !strings.Contains(rendered, "\n") {
					line := pattern + " => " + rendered
					if len(line)+indent*indentWidth <= p.maxLineWidth {
						return []string{p.indent(indent) + line}
					}
				}
			}
		}
	}

	lines := []string{p.indent(indent) + pattern + " => {"}
	lines = append(lines, p.renderStatements(matchCase.Body, indent+1)...)
	lines = append(lines, p.indent(indent)+"}")
	return lines
}

func (p printer) renderMatchCase(matchCase parse.MatchCase, indent int) []string {
	pattern := p.renderExpression(matchCase.Pattern, 0)
	if len(matchCase.Body) == 0 {
		return []string{p.indent(indent) + pattern + " => ()"}
	}
	if len(matchCase.Body) == 1 {
		if expr, ok := matchCase.Body[0].(parse.Expression); ok {
			if canInlineMatchExpression(expr) {
				rendered := p.renderExpression(expr, 0)
				if !strings.Contains(rendered, "\n") {
					line := pattern + " => " + rendered
					if len(line)+indent*indentWidth <= p.maxLineWidth {
						return []string{p.indent(indent) + line}
					}
				}
			}
		}
	}

	lines := []string{p.indent(indent) + pattern + " => {"}
	lines = append(lines, p.renderStatements(matchCase.Body, indent+1)...)
	lines = append(lines, p.indent(indent)+"}")
	return lines
}

const (
	precedenceLowest = iota
	precedenceOr
	precedenceAnd
	precedenceCompare
	precedenceAdd
	precedenceMul
	precedenceUnary
	precedenceCall
)

func (p printer) renderUnary(node *parse.UnaryExpression, parentPrecedence int) string {
	operator := p.operatorString(node.Operator)
	if operator == "" {
		operator = "-"
	}
	operand := p.renderExpression(node.Operand, precedenceUnary)
	text := operator + operand
	if node.Operator == parse.Not {
		text = "not " + operand
	}
	if precedenceUnary < parentPrecedence {
		return "(" + text + ")"
	}
	return text
}

func (p printer) renderBinary(node *parse.BinaryExpression, parentPrecedence int) string {
	precedence := p.binaryPrecedence(node.Operator)
	left := p.renderExpression(node.Left, precedence)
	right := p.renderExpression(node.Right, precedence+1)
	operator := p.operatorString(node.Operator)
	separator := " "
	if node.Operator == parse.Range {
		separator = ""
	}
	text := left + separator + operator + separator + right
	if node.Operator == parse.Range {
		text = left + ".." + right
	}
	if precedence < parentPrecedence {
		return "(" + text + ")"
	}
	return text
}

func (p printer) binaryPrecedence(operator parse.Operator) int {
	switch operator {
	case parse.Or:
		return precedenceOr
	case parse.And:
		return precedenceAnd
	case parse.Equal, parse.NotEqual, parse.GreaterThan, parse.GreaterThanOrEqual, parse.LessThan, parse.LessThanOrEqual, parse.Range:
		return precedenceCompare
	case parse.Plus, parse.Minus:
		return precedenceAdd
	case parse.Multiply, parse.Divide, parse.Modulo:
		return precedenceMul
	default:
		return precedenceLowest
	}
}

func (p printer) operatorString(operator parse.Operator) string {
	switch operator {
	case parse.Bang:
		return "!"
	case parse.Minus:
		return "-"
	case parse.Decrement:
		return "=-"
	case parse.Plus:
		return "+"
	case parse.Increment:
		return "=+"
	case parse.Divide:
		return "/"
	case parse.Multiply:
		return "*"
	case parse.Modulo:
		return "%"
	case parse.GreaterThan:
		return ">"
	case parse.GreaterThanOrEqual:
		return ">="
	case parse.LessThan:
		return "<"
	case parse.LessThanOrEqual:
		return "<="
	case parse.Equal:
		return "=="
	case parse.NotEqual:
		return "!="
	case parse.And:
		return "and"
	case parse.Not:
		return "not"
	case parse.Or:
		return "or"
	case parse.Range:
		return ".."
	case parse.Assign:
		return "="
	default:
		return ""
	}
}

func (p printer) renderComment(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "//") {
		return trimmed
	}
	return "// " + trimmed
}

func (p printer) indent(level int) string {
	if level <= 0 {
		return ""
	}
	return strings.Repeat(" ", level*indentWidth)
}

func canInlineMatchExpression(expression parse.Expression) bool {
	switch expression.(type) {
	case *parse.VariableAssignment, parse.VariableAssignment, *parse.VariableDeclaration, parse.VariableDeclaration:
		return false
	default:
		return true
	}
}

func (p printer) indentLines(text string, indent int) []string {
	parts := strings.Split(text, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, p.indent(indent)+part)
	}
	return lines
}

func hasSourceBlankLine(previous parse.Statement, current parse.Statement) bool {
	if previous == nil || current == nil {
		return false
	}
	prevEnd := statementEndRow(previous)
	currStart := statementStartRow(current)
	if prevEnd <= 0 || currStart <= 0 {
		return false
	}
	return currStart-prevEnd > 1
}

func statementStartRow(statement parse.Statement) int {
	loc := statement.GetLocation()
	if loc.Start.Row > 0 {
		return loc.Start.Row
	}
	if loc.End.Row > 0 {
		return loc.End.Row
	}

	switch node := statement.(type) {
	case *parse.IfStatement:
		if node.Condition != nil {
			return expressionStartRow(node.Condition)
		}
		return statementsStartRow(node.Body)
	case *parse.WhileLoop:
		if node.Condition != nil {
			return expressionStartRow(node.Condition)
		}
		return statementsStartRow(node.Body)
	case *parse.ForInLoop:
		if node.Iterable != nil {
			return expressionStartRow(node.Iterable)
		}
		return statementsStartRow(node.Body)
	case *parse.RangeLoop:
		if node.Start != nil {
			return expressionStartRow(node.Start)
		}
		return statementsStartRow(node.Body)
	case *parse.ForLoop:
		if node.Init != nil {
			return statementStartRow(node.Init)
		}
		if node.Condition != nil {
			return expressionStartRow(node.Condition)
		}
		return statementsStartRow(node.Body)
	}

	if expr, ok := statement.(parse.Expression); ok {
		return expressionStartRow(expr)
	}
	return 0
}

func statementEndRow(statement parse.Statement) int {
	loc := statement.GetLocation()
	if loc.End.Row > 0 {
		return loc.End.Row
	}
	if loc.Start.Row > 0 {
		return loc.Start.Row
	}

	switch node := statement.(type) {
	case *parse.IfStatement:
		if node.Else != nil {
			if row := statementEndRow(node.Else); row > 0 {
				return row
			}
		}
		if row := statementsEndRow(node.Body); row > 0 {
			return row
		}
		if node.Condition != nil {
			return expressionEndRow(node.Condition)
		}
	case *parse.WhileLoop:
		if row := statementsEndRow(node.Body); row > 0 {
			return row
		}
		if node.Condition != nil {
			return expressionEndRow(node.Condition)
		}
	case *parse.ForInLoop:
		if row := statementsEndRow(node.Body); row > 0 {
			return row
		}
		if node.Iterable != nil {
			return expressionEndRow(node.Iterable)
		}
	case *parse.RangeLoop:
		if row := statementsEndRow(node.Body); row > 0 {
			return row
		}
		if node.End != nil {
			return expressionEndRow(node.End)
		}
	case *parse.ForLoop:
		if row := statementsEndRow(node.Body); row > 0 {
			return row
		}
		if node.Incrementer != nil {
			return statementEndRow(node.Incrementer)
		}
	}

	if expr, ok := statement.(parse.Expression); ok {
		return expressionEndRow(expr)
	}
	return 0
}

func statementsStartRow(statements []parse.Statement) int {
	for _, statement := range statements {
		if statement == nil {
			continue
		}
		if row := statementStartRow(statement); row > 0 {
			return row
		}
	}
	return 0
}

func statementsEndRow(statements []parse.Statement) int {
	for i := len(statements) - 1; i >= 0; i-- {
		statement := statements[i]
		if statement == nil {
			continue
		}
		if row := statementEndRow(statement); row > 0 {
			return row
		}
	}
	return 0
}

func expressionStartRow(expression parse.Expression) int {
	if expression == nil {
		return 0
	}
	loc := expression.GetLocation()
	if loc.Start.Row > 0 {
		return loc.Start.Row
	}
	if loc.End.Row > 0 {
		return loc.End.Row
	}

	switch node := expression.(type) {
	case *parse.InstanceMethod:
		if node.Target != nil {
			return expressionStartRow(node.Target)
		}
		return expressionStartRow(&node.Method)
	case *parse.StaticFunction:
		if node.Target != nil {
			return expressionStartRow(node.Target)
		}
		return expressionStartRow(&node.Function)
	case *parse.StaticProperty:
		if node.Target != nil {
			return expressionStartRow(node.Target)
		}
		if inner, ok := node.Property.(parse.Expression); ok {
			return expressionStartRow(inner)
		}
	case *parse.FunctionCall:
		for _, arg := range node.Args {
			if arg.Value != nil {
				return expressionStartRow(arg.Value)
			}
		}
	}
	return 0
}

func expressionEndRow(expression parse.Expression) int {
	if expression == nil {
		return 0
	}
	loc := expression.GetLocation()
	if loc.End.Row > 0 {
		return loc.End.Row
	}
	if loc.Start.Row > 0 {
		return loc.Start.Row
	}

	switch node := expression.(type) {
	case *parse.InstanceMethod:
		if row := expressionEndRow(&node.Method); row > 0 {
			return row
		}
		if node.Target != nil {
			return expressionEndRow(node.Target)
		}
	case *parse.StaticFunction:
		if row := expressionEndRow(&node.Function); row > 0 {
			return row
		}
		if node.Target != nil {
			return expressionEndRow(node.Target)
		}
	case *parse.StaticProperty:
		if inner, ok := node.Property.(parse.Expression); ok {
			return expressionEndRow(inner)
		}
		if node.Target != nil {
			return expressionEndRow(node.Target)
		}
	case *parse.FunctionCall:
		for i := len(node.Args) - 1; i >= 0; i-- {
			if node.Args[i].Value != nil {
				if row := expressionEndRow(node.Args[i].Value); row > 0 {
					return row
				}
			}
		}
	}
	return 0
}
