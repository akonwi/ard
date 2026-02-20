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
	return p.renderDocAtIndent(p.renderStatementDoc(statement), indent)
}

func (p printer) renderStatementDoc(statement parse.Statement) doc {
	if statement == nil {
		return dText("")
	}

	switch node := statement.(type) {
	case *parse.Comment:
		return dText(p.renderComment(node.Value))
	case *parse.VariableDeclaration:
		return p.renderVariableDeclarationDoc(node)
	case *parse.VariableAssignment:
		return p.renderVariableAssignmentDoc(node)
	case *parse.FunctionDeclaration:
		return p.renderFunctionDeclarationDoc(node, false)
	case *parse.StaticFunctionDeclaration:
		return p.renderStaticFunctionDeclarationDoc(node)
	case *parse.ExternalFunction:
		return p.renderExternalFunctionDoc(node)
	case *parse.StructDefinition:
		return p.renderStructDefinitionDoc(node)
	case *parse.TraitDefinition:
		return p.renderTraitDefinitionDoc(node)
	case *parse.ImplBlock:
		return p.renderImplBlockDoc(node)
	case *parse.TraitImplementation:
		return p.renderTraitImplementationDoc(node)
	case *parse.EnumDefinition:
		return p.renderEnumDefinitionDoc(node)
	case *parse.WhileLoop:
		return p.renderWhileLoopDoc(node)
	case *parse.ForInLoop:
		return p.renderForInLoopDoc(node)
	case *parse.RangeLoop:
		return p.renderRangeLoopDoc(node)
	case *parse.ForLoop:
		return p.renderForLoopDoc(node)
	case *parse.IfStatement:
		return p.renderIfStatementDoc(node)
	case *parse.Break:
		return dText("break")
	case *parse.TypeDeclaration:
		return p.renderTypeDeclarationDoc(node)
	default:
		if expr, ok := statement.(parse.Expression); ok {
			return dText(p.renderExpression(expr, 0))
		}
		return dText(statement.String())
	}
}

func (p printer) renderVariableDeclarationDoc(node *parse.VariableDeclaration) doc {
	binding := "let"
	if node.Mutable {
		binding = "mut"
	}
	prefix := binding + " " + node.Name
	if node.Type != nil {
		prefix += ": " + p.renderType(node.Type)
	}
	prefix += " = "
	return dConcat(dText(prefix), p.renderExpressionValueDoc(node.Value, 0))
}

func (p printer) renderVariableAssignmentDoc(node *parse.VariableAssignment) doc {
	operator, value := p.assignmentParts(node)
	prefix := p.renderExpression(node.Target, 0) + " " + operator + " "
	return dConcat(dText(prefix), p.renderExpressionValueDoc(value, 0))
}

func (p printer) renderExpressionValueDoc(expression parse.Expression, parentPrecedence int) doc {
	switch node := expression.(type) {
	case *parse.AnonymousFunction:
		return p.renderAnonymousFunctionDoc(node)
	case *parse.Try:
		return p.renderTryDoc(node)
	case *parse.BlockExpression:
		return p.renderBlockExpressionDoc(node)
	default:
		return dText(p.renderExpression(expression, parentPrecedence))
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

func (p printer) renderFunctionDeclarationDoc(node *parse.FunctionDeclaration, traitSignatureOnly bool) doc {
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
	header += p.renderParameterList(node.Parameters, 0, header)
	if node.ReturnType != nil {
		header += " " + p.renderType(node.ReturnType)
	}

	if traitSignatureOnly {
		return dText(header)
	}

	return p.renderBlockDoc(header, node.Body)
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

func (p printer) renderStaticFunctionDeclarationDoc(node *parse.StaticFunctionDeclaration) doc {
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
	header += p.renderParameterList(node.Parameters, 0, header)
	if node.ReturnType != nil {
		header += " " + p.renderType(node.ReturnType)
	}
	return p.renderBlockDoc(header, node.Body)
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

func (p printer) renderExternalFunctionDoc(node *parse.ExternalFunction) doc {
	return dText(p.renderExternalFunction(node))
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

func (p printer) renderStructDefinitionDoc(node *parse.StructDefinition) doc {
	prefix := ""
	if node.Private {
		prefix = "private "
	}
	header := prefix + "struct " + node.Name.Name
	if len(node.Fields) == 0 && len(node.Comments) == 0 {
		return dText(header + " {}")
	}

	items := make([]doc, 0, len(node.Fields)+len(node.Comments))
	commentIndex := 0
	for _, field := range node.Fields {
		fieldRow := field.Name.Location.Start.Row
		for commentIndex < len(node.Comments) && (fieldRow == 0 || node.Comments[commentIndex].Location.Start.Row < fieldRow) {
			items = append(items, dText(p.renderComment(node.Comments[commentIndex].Value)))
			commentIndex++
		}
		items = append(items, dText(fmt.Sprintf("%s: %s,", field.Name.Name, p.renderType(field.Type))))
	}
	for ; commentIndex < len(node.Comments); commentIndex++ {
		items = append(items, dText(p.renderComment(node.Comments[commentIndex].Value)))
	}

	body := dJoin(dHardLine(), items)
	return dGroup(dConcat(
		dText(header+" {"),
		dIndent(dConcat(dHardLine(), body)),
		dHardLine(),
		dText("}"),
	))
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

func (p printer) renderEnumDefinitionDoc(node *parse.EnumDefinition) doc {
	prefix := ""
	if node.Private {
		prefix = "private "
	}
	header := prefix + "enum " + node.Name
	if len(node.Variants) == 0 && len(node.Comments) == 0 {
		return dText(header + " {}")
	}

	items := make([]doc, 0, len(node.Variants)+len(node.Comments))
	for _, comment := range node.Comments {
		items = append(items, dText(p.renderComment(comment.Value)))
	}
	for _, variant := range node.Variants {
		if variant.Value == nil {
			items = append(items, dText(variant.Name+","))
		} else {
			items = append(items, dText(fmt.Sprintf("%s = %s,", variant.Name, p.renderExpression(variant.Value, 0))))
		}
	}
	body := dJoin(dHardLine(), items)
	return dGroup(dConcat(
		dText(header+" {"),
		dIndent(dConcat(dHardLine(), body)),
		dHardLine(),
		dText("}"),
	))
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

func (p printer) renderTraitDefinitionDoc(node *parse.TraitDefinition) doc {
	prefix := ""
	if node.Private {
		prefix = "private "
	}
	header := prefix + "trait " + node.Name.Name
	if len(node.Methods) == 0 && len(node.Comments) == 0 {
		return dText(header + " {}")
	}

	items := p.interleaveMethodDocs(node.Methods, node.Comments, true)
	body := dJoin(dHardLine(), items)
	return dGroup(dConcat(
		dText(header+" {"),
		dIndent(dConcat(dHardLine(), body)),
		dHardLine(),
		dText("}"),
	))
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

func (p printer) renderImplBlockDoc(node *parse.ImplBlock) doc {
	header := "impl " + node.Target.Name
	if len(node.Methods) == 0 && len(node.Comments) == 0 {
		return dText(header + " {}")
	}

	items := p.interleaveMethodDocs(node.Methods, node.Comments, false)
	body := dJoin(dHardLine(), items)
	return dGroup(dConcat(
		dText(header+" {"),
		dIndent(dConcat(dHardLine(), body)),
		dHardLine(),
		dText("}"),
	))
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

func (p printer) renderTraitImplementationDoc(node *parse.TraitImplementation) doc {
	header := fmt.Sprintf("impl %s for %s", p.renderExpression(node.Trait, 0), node.ForType.Name)
	if len(node.Methods) == 0 {
		return dText(header + " {}")
	}

	body := dJoin(dHardLine(), p.interleaveMethodDocs(node.Methods, nil, false))
	return dGroup(dConcat(
		dText(header+" {"),
		dIndent(dConcat(dHardLine(), body)),
		dHardLine(),
		dText("}"),
	))
}

func (p printer) interleaveMethodDocs(methods []parse.FunctionDeclaration, comments []parse.Comment, traitSignatureOnly bool) []doc {
	if len(methods) == 0 && len(comments) == 0 {
		return nil
	}

	items := make([]doc, 0, len(methods)+len(comments)*2)
	lastKind := ""
	commentIndex := 0
	for _, method := range methods {
		methodStart := method.Location.Start.Row
		for commentIndex < len(comments) && (methodStart == 0 || comments[commentIndex].Location.Start.Row < methodStart) {
			if lastKind == "method" {
				items = append(items, dText(""))
			}
			items = append(items, dText(p.renderComment(comments[commentIndex].Value)))
			lastKind = "comment"
			commentIndex++
		}

		if lastKind == "method" {
			items = append(items, dText(""))
		}
		items = append(items, p.renderFunctionDeclarationDoc(&method, traitSignatureOnly))
		lastKind = "method"

	}

	for ; commentIndex < len(comments); commentIndex++ {
		if lastKind == "method" {
			items = append(items, dText(""))
		}
		items = append(items, dText(p.renderComment(comments[commentIndex].Value)))
		lastKind = "comment"
	}

	return items
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

func (p printer) renderTypeDeclarationDoc(node *parse.TypeDeclaration) doc {
	return dText(p.renderTypeDeclaration(node))
}

func (p printer) renderWhileLoop(node *parse.WhileLoop, indent int) []string {
	header := "while"
	if node.Condition != nil {
		header += " " + p.renderExpression(node.Condition, 0)
	}
	return p.renderBlockWithHeader(header, node.Body, indent)
}

func (p printer) renderWhileLoopDoc(node *parse.WhileLoop) doc {
	header := "while"
	if node.Condition != nil {
		header += " " + p.renderExpression(node.Condition, 0)
	}
	return p.renderBlockDoc(header, node.Body)
}

func (p printer) renderRangeLoop(node *parse.RangeLoop, indent int) []string {
	header := fmt.Sprintf("for %s", node.Cursor.Name)
	if node.Cursor2.Name != "" {
		header += fmt.Sprintf(", %s", node.Cursor2.Name)
	}
	header += fmt.Sprintf(" in %s..%s", p.renderExpression(node.Start, 0), p.renderExpression(node.End, 0))
	return p.renderBlockWithHeader(header, node.Body, indent)
}

func (p printer) renderRangeLoopDoc(node *parse.RangeLoop) doc {
	header := fmt.Sprintf("for %s", node.Cursor.Name)
	if node.Cursor2.Name != "" {
		header += fmt.Sprintf(", %s", node.Cursor2.Name)
	}
	header += fmt.Sprintf(" in %s..%s", p.renderExpression(node.Start, 0), p.renderExpression(node.End, 0))
	return p.renderBlockDoc(header, node.Body)
}

func (p printer) renderForInLoop(node *parse.ForInLoop, indent int) []string {
	header := fmt.Sprintf("for %s", node.Cursor.Name)
	if node.Cursor2.Name != "" {
		header += fmt.Sprintf(", %s", node.Cursor2.Name)
	}
	header += " in " + p.renderExpression(node.Iterable, 0)
	return p.renderBlockWithHeader(header, node.Body, indent)
}

func (p printer) renderForInLoopDoc(node *parse.ForInLoop) doc {
	header := fmt.Sprintf("for %s", node.Cursor.Name)
	if node.Cursor2.Name != "" {
		header += fmt.Sprintf(", %s", node.Cursor2.Name)
	}
	header += " in " + p.renderExpression(node.Iterable, 0)
	return p.renderBlockDoc(header, node.Body)
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
		if inc, ok := node.Incrementer.(*parse.VariableAssignment); ok {
			increment = p.renderVariableAssignment(inc)
		} else if inc, ok := node.Incrementer.(parse.VariableAssignment); ok {
			copy := inc
			increment = p.renderVariableAssignment(&copy)
		} else if incrementExpr, ok := node.Incrementer.(parse.Expression); ok {
			increment = p.renderExpression(incrementExpr, 0)
		}
	}
	header := fmt.Sprintf("for %s; %s; %s", init, condition, increment)
	return p.renderBlockWithHeader(header, node.Body, indent)
}

func (p printer) renderForLoopDoc(node *parse.ForLoop) doc {
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
		if inc, ok := node.Incrementer.(*parse.VariableAssignment); ok {
			increment = p.renderVariableAssignment(inc)
		} else if inc, ok := node.Incrementer.(parse.VariableAssignment); ok {
			copy := inc
			increment = p.renderVariableAssignment(&copy)
		} else if incrementExpr, ok := node.Incrementer.(parse.Expression); ok {
			increment = p.renderExpression(incrementExpr, 0)
		}
	}
	header := fmt.Sprintf("for %s; %s; %s", init, condition, increment)
	return p.renderBlockDoc(header, node.Body)
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

func (p printer) renderIfStatementDoc(node *parse.IfStatement) doc {
	if node.Condition == nil {
		return p.renderBlockDoc("else", node.Body)
	}

	head := "if " + p.renderExpression(node.Condition, 0)
	current := p.renderBlockDoc(head, node.Body)
	if node.Else == nil {
		return current
	}

	if elseIf, ok := node.Else.(*parse.IfStatement); ok {
		if elseIf.Condition == nil {
			return dConcat(current, dText(" "), p.renderBlockDoc("else", elseIf.Body))
		}
		return dConcat(current, dText(" else "), p.renderIfStatementDoc(elseIf))
	}

	return dConcat(current, dText(" else "), p.renderBlockDoc("else", []parse.Statement{node.Else}))
}

func (p printer) renderBlockDoc(header string, statements []parse.Statement) doc {
	if len(statements) == 0 {
		return dText(header + " {}")
	}

	return dGroup(dConcat(
		dText(header+" {"),
		dIndent(dConcat(dHardLine(), p.renderStatementsDoc(statements))),
		dHardLine(),
		dText("}"),
	))
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
	return p.renderDocAtIndent(p.renderStatementsDoc(statements), indent)
}

func (p printer) renderStatementsDoc(statements []parse.Statement) doc {
	if len(statements) == 0 {
		return dText("")
	}

	parts := make([]doc, 0, len(statements)*2)
	var previous parse.Statement
	haveRendered := false
	for _, statement := range statements {
		if statement == nil {
			continue
		}
		if haveRendered {
			parts = append(parts, dHardLine())
			if hasSourceBlankLine(previous, statement) {
				parts = append(parts, dHardLine())
			}
		}
		parts = append(parts, p.renderStatementDoc(statement))
		previous = statement
		haveRendered = true
	}
	if len(parts) == 0 {
		return dText("")
	}
	return dConcat(parts...)
}

func (p printer) renderDocAtIndent(document doc, indent int) []string {
	rendered := p.printDoc(document)
	if rendered == "" {
		return nil
	}
	parts := strings.Split(rendered, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, p.indent(indent)+part)
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
	return p.printDoc(p.renderExpressionDoc(expression, parentPrecedence))
}

func (p printer) renderExpressionDoc(expression parse.Expression, parentPrecedence int) doc {
	if expression == nil {
		return dText("")
	}

	switch node := expression.(type) {
	case *parse.Identifier:
		return dText(node.Name)
	case parse.Identifier:
		return dText(node.Name)
	case *parse.StrLiteral:
		return dText(strconv.Quote(node.Value))
	case parse.StrLiteral:
		return dText(strconv.Quote(node.Value))
	case *parse.InterpolatedStr:
		return p.renderInterpolatedStringDoc(node)
	case parse.InterpolatedStr:
		copy := node
		return p.renderInterpolatedStringDoc(&copy)
	case *parse.NumLiteral:
		return dText(node.Value)
	case parse.NumLiteral:
		return dText(node.Value)
	case *parse.BoolLiteral:
		if node.Value {
			return dText("true")
		}
		return dText("false")
	case parse.BoolLiteral:
		if node.Value {
			return dText("true")
		}
		return dText("false")
	case *parse.VoidLiteral:
		return dText("()")
	case parse.VoidLiteral:
		return dText("()")
	case *parse.UnaryExpression:
		return dText(p.renderUnary(node, parentPrecedence))
	case parse.UnaryExpression:
		copy := node
		return dText(p.renderUnary(&copy, parentPrecedence))
	case *parse.BinaryExpression:
		return dText(p.renderBinary(node, parentPrecedence))
	case parse.BinaryExpression:
		copy := node
		return dText(p.renderBinary(&copy, parentPrecedence))
	case *parse.ChainedComparison:
		parts := make([]string, 0, len(node.Operands)*2)
		for i, operand := range node.Operands {
			if i > 0 {
				parts = append(parts, p.operatorString(node.Operators[i-1]))
			}
			parts = append(parts, p.renderExpression(operand, precedenceCompare))
		}
		return dText(strings.Join(parts, " "))
	case *parse.RangeExpression:
		return dConcat(p.renderExpressionDoc(node.Start, precedenceCompare), dText(".."), p.renderExpressionDoc(node.End, precedenceCompare))
	case parse.RangeExpression:
		return dConcat(p.renderExpressionDoc(node.Start, precedenceCompare), dText(".."), p.renderExpressionDoc(node.End, precedenceCompare))
	case *parse.ListLiteral:
		return p.renderListLiteralDoc(node)
	case parse.ListLiteral:
		copy := node
		return p.renderListLiteralDoc(&copy)
	case *parse.MapLiteral:
		return p.renderMapLiteralDoc(node)
	case parse.MapLiteral:
		copy := node
		return p.renderMapLiteralDoc(&copy)
	case *parse.StructInstance:
		return p.renderStructInstanceDoc(node)
	case parse.StructInstance:
		copy := node
		return p.renderStructInstanceDoc(&copy)
	case *parse.FunctionCall:
		return p.renderFunctionCallDoc(node)
	case parse.FunctionCall:
		copy := node
		return p.renderFunctionCallDoc(&copy)
	case *parse.InstanceProperty:
		if id, ok := node.Target.(*parse.Identifier); ok && id.Name == "@" {
			return dText("@" + node.Property.Name)
		}
		return dConcat(p.renderExpressionDoc(node.Target, precedenceCall), dText("."+node.Property.Name))
	case parse.InstanceProperty:
		copy := node
		return p.renderExpressionDoc(&copy, parentPrecedence)
	case *parse.InstanceMethod:
		if id, ok := node.Target.(*parse.Identifier); ok && id.Name == "@" {
			return dConcat(dText("@"), p.renderFunctionCallDoc(&node.Method))
		}
		return dConcat(p.renderExpressionDoc(node.Target, precedenceCall), dText("."), p.renderFunctionCallDoc(&node.Method))
	case parse.InstanceMethod:
		copy := node
		return p.renderExpressionDoc(&copy, parentPrecedence)
	case *parse.StaticProperty:
		return dConcat(p.renderExpressionDoc(node.Target, precedenceCall), dText("::"), p.renderExpressionDoc(node.Property, precedenceCall))
	case parse.StaticProperty:
		copy := node
		return p.renderExpressionDoc(&copy, parentPrecedence)
	case *parse.StaticFunction:
		return dConcat(p.renderExpressionDoc(node.Target, precedenceCall), dText("::"), p.renderFunctionCallDoc(&node.Function))
	case parse.StaticFunction:
		copy := node
		return p.renderExpressionDoc(&copy, parentPrecedence)
	case *parse.MatchExpression:
		return p.renderMatchExpressionDoc(node)
	case *parse.ConditionalMatchExpression:
		return p.renderConditionalMatchExpressionDoc(node)
	case *parse.AnonymousFunction:
		return p.renderAnonymousFunctionDoc(node)
	case *parse.Try:
		return p.renderTryDoc(node)
	case *parse.BlockExpression:
		return p.renderBlockExpressionDoc(node)
	default:
		return dText(expression.String())
	}
}

func (p printer) renderAnonymousFunctionDoc(node *parse.AnonymousFunction) doc {
	header := "fn" + p.renderParameterList(node.Parameters, 0, "fn")
	if node.ReturnType != nil {
		header += " " + p.renderType(node.ReturnType)
	}
	if len(node.Body) == 0 {
		return dText(header + " {}")
	}
	return dGroup(dConcat(
		dText(header+" {"),
		dIndent(dConcat(dHardLine(), p.renderStatementsDoc(node.Body))),
		dHardLine(),
		dText("}"),
	))
}

func (p printer) renderTryDoc(node *parse.Try) doc {
	prefix := "try " + p.renderExpression(node.Expression, precedenceUnary)
	if node.CatchVar == nil {
		return dText(prefix)
	}
	prefix += " -> " + node.CatchVar.Name
	if len(node.CatchBlock) == 0 {
		return dText(prefix + " {}")
	}
	return dGroup(dConcat(
		dText(prefix+" {"),
		dIndent(dConcat(dHardLine(), p.renderStatementsDoc(node.CatchBlock))),
		dHardLine(),
		dText("}"),
	))
}

func (p printer) renderBlockExpressionDoc(node *parse.BlockExpression) doc {
	if len(node.Statements) == 0 {
		return dText("{}")
	}
	return dGroup(dConcat(
		dText("{"),
		dIndent(dConcat(dHardLine(), p.renderStatementsDoc(node.Statements))),
		dHardLine(),
		dText("}"),
	))
}

func (p printer) renderInterpolatedString(value *parse.InterpolatedStr) string {
	return p.printDoc(p.renderInterpolatedStringDoc(value))
}

func (p printer) renderInterpolatedStringDoc(value *parse.InterpolatedStr) doc {
	var builder strings.Builder
	builder.WriteByte('"')
	p.writeInterpolatedChunks(&builder, value.Chunks)
	builder.WriteByte('"')
	return dText(builder.String())
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
	return p.printDoc(p.renderListLiteralDoc(list))
}

func (p printer) renderListLiteralDoc(list *parse.ListLiteral) doc {
	items := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, p.renderExpression(item, 0))
	}
	if len(items) == 0 {
		return dText("[]")
	}

	itemDocs := make([]doc, 0, len(items))
	for _, item := range items {
		itemDocs = append(itemDocs, dText(item))
	}
	body := dJoin(dConcat(dText(","), dLine()), itemDocs)
	body = dConcat(body, dIfBreak(dText(","), dText("")))

	if len(list.Comments) > 0 {
		commentDocs := make([]doc, 0, len(list.Comments))
		for _, comment := range list.Comments {
			commentDocs = append(commentDocs, dText(p.renderComment(comment.Value)))
		}
		body = dConcat(dJoin(dHardLine(), commentDocs), dHardLine(), body)
	}

	return dGroup(dConcat(
		dText("["),
		dIndent(dConcat(dSoftLine(), body)),
		dSoftLine(),
		dText("]"),
	))
}

func (p printer) renderMapLiteral(m *parse.MapLiteral) string {
	return p.printDoc(p.renderMapLiteralDoc(m))
}

func (p printer) renderMapLiteralDoc(m *parse.MapLiteral) doc {
	if len(m.Entries) == 0 {
		return dText("[:]")
	}

	parts := make([]string, 0, len(m.Entries))
	for _, entry := range m.Entries {
		parts = append(parts, p.renderExpression(entry.Key, 0)+": "+p.renderExpression(entry.Value, 0))
	}

	partDocs := make([]doc, 0, len(parts))
	for _, part := range parts {
		partDocs = append(partDocs, dText(part))
	}
	body := dJoin(dConcat(dText(","), dLine()), partDocs)
	body = dConcat(body, dIfBreak(dText(","), dText("")))
	if len(m.Comments) > 0 {
		commentDocs := make([]doc, 0, len(m.Comments))
		for _, comment := range m.Comments {
			commentDocs = append(commentDocs, dText(p.renderComment(comment.Value)))
		}
		body = dConcat(dJoin(dHardLine(), commentDocs), dHardLine(), body)
	}

	return dGroup(dConcat(
		dText("["),
		dIndent(dConcat(dSoftLine(), body)),
		dSoftLine(),
		dText("]"),
	))
}

func (p printer) renderStructInstance(node *parse.StructInstance) string {
	return p.printDoc(p.renderStructInstanceDoc(node))
}

func (p printer) renderStructInstanceDoc(node *parse.StructInstance) doc {
	if len(node.Properties) == 0 && len(node.Comments) == 0 {
		return dText(node.Name.Name + "{}")
	}

	parts := make([]string, 0, len(node.Properties))
	for _, property := range node.Properties {
		parts = append(parts, property.Name.Name+": "+p.renderExpression(property.Value, 0))
	}
	oneLine := node.Name.Name + "{" + strings.Join(parts, ", ") + "}"
	if len(node.Properties) <= 2 && len(node.Comments) == 0 && len(oneLine) <= p.maxLineWidth {
		return dText(oneLine)
	}

	items := make([]doc, 0, len(parts)+len(node.Comments))
	for _, comment := range node.Comments {
		items = append(items, dText(p.renderComment(comment.Value)))
	}
	for _, part := range parts {
		items = append(items, dText(part+","))
	}
	body := dJoin(dHardLine(), items)

	return dConcat(
		dText(node.Name.Name+"{"),
		dIndent(dConcat(dHardLine(), body)),
		dHardLine(),
		dText("}"),
	)
}

func (p printer) renderFunctionCall(node *parse.FunctionCall) string {
	return p.printDoc(p.renderFunctionCallDoc(node))
}

func (p printer) renderFunctionCallDoc(node *parse.FunctionCall) doc {
	head := node.Name
	if len(node.TypeArgs) > 0 {
		types := make([]string, 0, len(node.TypeArgs))
		for _, item := range node.TypeArgs {
			types = append(types, p.renderType(item))
		}
		head += "<" + strings.Join(types, ", ") + ">"
	}
	argDocs := make([]doc, 0, len(node.Args))
	for _, arg := range node.Args {
		prefix := ""
		if arg.Name != "" {
			prefix += arg.Name + ": "
		}
		if arg.Mutable {
			prefix += "mut "
		}
		argDocs = append(argDocs, dConcat(dText(prefix), p.renderExpressionValueDoc(arg.Value, 0)))
	}
	if len(argDocs) == 0 && len(node.Comments) == 0 {
		return dText(head + "()")
	}

	body := dJoin(dConcat(dText(","), dLine()), argDocs)
	body = dConcat(body, dIfBreak(dText(","), dText("")))

	if len(node.Comments) > 0 {
		commentDocs := make([]doc, 0, len(node.Comments))
		for _, comment := range node.Comments {
			commentDocs = append(commentDocs, dText(p.renderComment(comment.Value)))
		}
		if len(argDocs) > 0 {
			body = dConcat(dJoin(dHardLine(), commentDocs), dHardLine(), body)
		} else {
			body = dJoin(dHardLine(), commentDocs)
		}
	}

	return dGroup(dConcat(
		dText(head+"("),
		dIndent(dConcat(dSoftLine(), body)),
		dSoftLine(),
		dText(")"),
	))
}

func (p printer) renderMatchExpression(node *parse.MatchExpression) string {
	return p.printDoc(p.renderMatchExpressionDoc(node))
}

func (p printer) renderMatchExpressionDoc(node *parse.MatchExpression) doc {
	caseDocs := make([]doc, 0, len(node.Cases)+len(node.Comments))
	for _, comment := range node.Comments {
		caseDocs = append(caseDocs, dText(p.renderComment(comment.Value)))
	}
	for _, matchCase := range node.Cases {
		caseDocs = append(caseDocs, dConcat(p.renderMatchCaseDoc(matchCase), dText(",")))
	}

	body := dText("")
	if len(caseDocs) > 0 {
		body = dJoin(dHardLine(), caseDocs)
	}

	return dGroup(dConcat(
		dText("match "+p.renderExpression(node.Subject, 0)+" {"),
		dIndent(dConcat(dHardLine(), body)),
		dHardLine(),
		dText("}"),
	))
}

func (p printer) renderConditionalMatchExpression(node *parse.ConditionalMatchExpression) string {
	return p.printDoc(p.renderConditionalMatchExpressionDoc(node))
}

func (p printer) renderConditionalMatchExpressionDoc(node *parse.ConditionalMatchExpression) doc {
	caseDocs := make([]doc, 0, len(node.Cases)+len(node.Comments))
	for _, comment := range node.Comments {
		caseDocs = append(caseDocs, dText(p.renderComment(comment.Value)))
	}
	for _, matchCase := range node.Cases {
		caseDocs = append(caseDocs, dConcat(p.renderConditionalMatchCaseDoc(matchCase), dText(",")))
	}

	body := dText("")
	if len(caseDocs) > 0 {
		body = dJoin(dHardLine(), caseDocs)
	}

	return dGroup(dConcat(
		dText("match {"),
		dIndent(dConcat(dHardLine(), body)),
		dHardLine(),
		dText("}"),
	))
}

func (p printer) renderConditionalMatchCaseDoc(matchCase parse.ConditionalMatchCase) doc {
	pattern := "_"
	if matchCase.Condition != nil {
		pattern = p.renderExpression(matchCase.Condition, 0)
	}
	if len(matchCase.Body) == 0 {
		return dText(pattern + " => ()")
	}
	if len(matchCase.Body) == 1 {
		if expr, ok := matchCase.Body[0].(parse.Expression); ok {
			if canInlineMatchExpression(expr) {
				rendered := p.renderExpression(expr, 0)
				if !strings.Contains(rendered, "\n") {
					line := pattern + " => " + rendered
					if len(line) <= p.maxLineWidth {
						return dText(line)
					}
				}
			}
		}
	}

	return dGroup(dConcat(
		dText(pattern+" => {"),
		dIndent(dConcat(dHardLine(), p.renderStatementsDoc(matchCase.Body))),
		dHardLine(),
		dText("}"),
	))
}

func (p printer) renderMatchCaseDoc(matchCase parse.MatchCase) doc {
	pattern := p.renderExpression(matchCase.Pattern, 0)
	if len(matchCase.Body) == 0 {
		return dText(pattern + " => ()")
	}
	if len(matchCase.Body) == 1 {
		if expr, ok := matchCase.Body[0].(parse.Expression); ok {
			if canInlineMatchExpression(expr) {
				rendered := p.renderExpression(expr, 0)
				if !strings.Contains(rendered, "\n") {
					line := pattern + " => " + rendered
					if len(line) <= p.maxLineWidth {
						return dText(line)
					}
				}
			}
		}
	}

	return dGroup(dConcat(
		dText(pattern+" => {"),
		dIndent(dConcat(dHardLine(), p.renderStatementsDoc(matchCase.Body))),
		dHardLine(),
		dText("}"),
	))
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
