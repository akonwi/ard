package gotarget

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
)

// renderFile renders the positionless AST produced by the Go target. Unlike
// go/printer, it does not need to preserve source positions, comments, or
// arbitrary user-authored layout.
func renderFile(file *ast.File) ([]byte, error) {
	p := sourceRenderer{}
	p.writeFile(file)
	if p.err != nil {
		return nil, p.err
	}
	return p.Bytes(), nil
}

type sourceRenderer struct {
	bytes.Buffer
	indent int
	err    error
}

func (p *sourceRenderer) fail(node any) {
	if p.err == nil {
		p.err = fmt.Errorf("unsupported generated Go AST node %T", node)
	}
}

func (p *sourceRenderer) writeByte(char byte) {
	if p.err == nil {
		_ = p.Buffer.WriteByte(char)
	}
}

func (p *sourceRenderer) writeString(value string) {
	if p.err == nil {
		_, _ = p.Buffer.WriteString(value)
	}
}

func (p *sourceRenderer) writeIndent() {
	for range p.indent {
		p.writeByte('\t')
	}
}

func (p *sourceRenderer) writeFile(file *ast.File) {
	if file == nil || file.Name == nil {
		p.fail(file)
		return
	}
	p.writeString("package ")
	p.writeString(file.Name.Name)
	p.writeString("\n\n")
	for i, decl := range file.Decls {
		p.writeDecl(decl)
		p.writeByte('\n')
		if i+1 < len(file.Decls) {
			p.writeByte('\n')
		}
	}
}

func (p *sourceRenderer) writeDecl(decl ast.Decl) {
	switch decl := decl.(type) {
	case *ast.GenDecl:
		p.writeGenDecl(decl)
	case *ast.FuncDecl:
		p.writeFuncDecl(decl)
	default:
		p.fail(decl)
	}
}

func (p *sourceRenderer) writeGenDecl(decl *ast.GenDecl) {
	p.writeString(decl.Tok.String())
	grouped := decl.Lparen.IsValid() || len(decl.Specs) != 1
	if !grouped {
		if len(decl.Specs) == 1 {
			p.writeByte(' ')
			p.writeSpec(decl.Specs[0])
		}
		return
	}
	p.writeString(" (\n")
	p.indent++
	for _, spec := range decl.Specs {
		p.writeIndent()
		p.writeSpec(spec)
		p.writeByte('\n')
	}
	p.indent--
	p.writeIndent()
	p.writeByte(')')
}

func (p *sourceRenderer) writeSpec(spec ast.Spec) {
	switch spec := spec.(type) {
	case *ast.ImportSpec:
		if spec.Name != nil {
			p.writeString(spec.Name.Name)
			p.writeByte(' ')
		}
		if spec.Path == nil {
			p.fail(spec)
			return
		}
		p.writeString(spec.Path.Value)
	case *ast.TypeSpec:
		p.writeString(spec.Name.Name)
		if spec.TypeParams != nil && len(spec.TypeParams.List) > 0 {
			p.writeFieldList(spec.TypeParams, '[', ']', true)
		}
		if spec.Assign.IsValid() {
			p.writeString(" = ")
		} else {
			p.writeByte(' ')
		}
		p.writeExpr(spec.Type, 0, false)
	case *ast.ValueSpec:
		p.writeIdentList(spec.Names)
		if spec.Type != nil {
			p.writeByte(' ')
			p.writeExpr(spec.Type, 0, false)
		}
		if len(spec.Values) > 0 {
			p.writeString(" = ")
			p.writeExprList(spec.Values)
		}
	default:
		p.fail(spec)
	}
}

func (p *sourceRenderer) writeFuncDecl(decl *ast.FuncDecl) {
	p.writeString("func ")
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		p.writeFieldList(decl.Recv, '(', ')', false)
		p.writeByte(' ')
	}
	p.writeString(decl.Name.Name)
	p.writeSignature(decl.Type)
	if decl.Body != nil {
		p.writeByte(' ')
		p.writeBlock(decl.Body)
	}
}

func (p *sourceRenderer) writeSignature(function *ast.FuncType) {
	if function == nil {
		p.fail(function)
		return
	}
	if function.TypeParams != nil && len(function.TypeParams.List) > 0 {
		p.writeFieldList(function.TypeParams, '[', ']', false)
	}
	p.writeFieldList(function.Params, '(', ')', false)
	if function.Results == nil || len(function.Results.List) == 0 {
		return
	}
	if len(function.Results.List) == 1 && len(function.Results.List[0].Names) == 0 {
		p.writeByte(' ')
		p.writeExpr(function.Results.List[0].Type, 0, false)
		return
	}
	p.writeByte(' ')
	p.writeFieldList(function.Results, '(', ')', false)
}

func (p *sourceRenderer) writeFieldList(list *ast.FieldList, open, close byte, typeParams bool) {
	p.writeByte(open)
	if list != nil {
		for i, field := range list.List {
			if i > 0 {
				p.writeString(", ")
			}
			p.writeField(field, false)
		}
		if typeParams && list.NumFields() == 1 && combinesWithTypeParamName(list.List[0].Type) {
			p.writeByte(',')
		}
	}
	p.writeByte(close)
}

func (p *sourceRenderer) writeField(field *ast.Field, interfaceField bool) {
	if len(field.Names) > 0 {
		p.writeIdentList(field.Names)
		if function, ok := field.Type.(*ast.FuncType); ok && interfaceField {
			p.writeSignature(function)
		} else if field.Type != nil {
			p.writeByte(' ')
			p.writeExpr(field.Type, 0, false)
		}
	} else if field.Type != nil {
		p.writeExpr(field.Type, 0, false)
	}
	if field.Tag != nil {
		p.writeByte(' ')
		p.writeString(field.Tag.Value)
	}
}

func (p *sourceRenderer) writeIdentList(idents []*ast.Ident) {
	for i, ident := range idents {
		if i > 0 {
			p.writeString(", ")
		}
		p.writeString(ident.Name)
	}
}

func (p *sourceRenderer) writeStmt(stmt ast.Stmt) {
	switch stmt := stmt.(type) {
	case *ast.BlockStmt:
		p.writeBlock(stmt)
	case *ast.DeclStmt:
		p.writeDecl(stmt.Decl)
	case *ast.AssignStmt:
		p.writeExprList(stmt.Lhs)
		p.writeByte(' ')
		p.writeString(stmt.Tok.String())
		p.writeByte(' ')
		p.writeExprList(stmt.Rhs)
	case *ast.ExprStmt:
		p.writeExpr(stmt.X, 0, false)
	case *ast.ReturnStmt:
		p.writeString("return")
		if len(stmt.Results) > 0 {
			p.writeByte(' ')
			p.writeExprList(stmt.Results)
		}
	case *ast.IfStmt:
		p.writeString("if ")
		if stmt.Init != nil {
			p.writeStmt(stmt.Init)
			p.writeString("; ")
		}
		p.writeByte('(')
		p.writeExpr(stmt.Cond, 0, false)
		p.writeString(") ")
		p.writeBlock(stmt.Body)
		if stmt.Else != nil {
			p.writeString(" else ")
			p.writeStmt(stmt.Else)
		}
	case *ast.ForStmt:
		p.writeString("for")
		if stmt.Init != nil || stmt.Post != nil {
			p.writeByte(' ')
			if stmt.Init != nil {
				p.writeStmt(stmt.Init)
			}
			p.writeString("; ")
			if stmt.Cond != nil {
				p.writeExpr(stmt.Cond, 0, false)
			}
			p.writeString("; ")
			if stmt.Post != nil {
				p.writeStmt(stmt.Post)
			}
		} else if stmt.Cond != nil {
			p.writeString(" (")
			p.writeExpr(stmt.Cond, 0, false)
			p.writeByte(')')
		}
		p.writeByte(' ')
		p.writeBlock(stmt.Body)
	case *ast.RangeStmt:
		p.writeString("for ")
		if stmt.Key != nil {
			p.writeExpr(stmt.Key, 0, false)
			if stmt.Value != nil {
				p.writeString(", ")
				p.writeExpr(stmt.Value, 0, false)
			}
			p.writeByte(' ')
			p.writeString(stmt.Tok.String())
			p.writeByte(' ')
		}
		p.writeString("range ")
		p.writeExpr(stmt.X, 0, false)
		p.writeByte(' ')
		p.writeBlock(stmt.Body)
	case *ast.BranchStmt:
		p.writeString(stmt.Tok.String())
		if stmt.Label != nil {
			p.writeByte(' ')
			p.writeString(stmt.Label.Name)
		}
	case *ast.LabeledStmt:
		p.writeString(stmt.Label.Name)
		p.writeString(":\n")
		p.writeIndent()
		p.writeStmt(stmt.Stmt)
	case *ast.SwitchStmt:
		p.writeString("switch")
		if stmt.Init != nil {
			p.writeByte(' ')
			p.writeStmt(stmt.Init)
			p.writeByte(';')
		}
		if stmt.Tag != nil {
			p.writeString(" (")
			p.writeExpr(stmt.Tag, 0, false)
			p.writeByte(')')
		}
		p.writeByte(' ')
		p.writeBlock(stmt.Body)
	case *ast.TypeSwitchStmt:
		p.writeString("switch")
		if stmt.Init != nil {
			p.writeByte(' ')
			p.writeStmt(stmt.Init)
			p.writeByte(';')
		}
		if stmt.Assign != nil {
			p.writeByte(' ')
			p.writeStmt(stmt.Assign)
		}
		p.writeByte(' ')
		p.writeBlock(stmt.Body)
	case *ast.SelectStmt:
		p.writeString("select ")
		p.writeBlock(stmt.Body)
	case *ast.SendStmt:
		p.writeExpr(stmt.Chan, 0, false)
		p.writeString(" <- ")
		p.writeExpr(stmt.Value, 0, false)
	case *ast.GoStmt:
		p.writeString("go ")
		p.writeExpr(stmt.Call, 0, false)
	case *ast.DeferStmt:
		p.writeString("defer ")
		p.writeExpr(stmt.Call, 0, false)
	case *ast.CaseClause:
		p.writeCaseClause(stmt)
	case *ast.CommClause:
		p.writeCommClause(stmt)
	case *ast.IncDecStmt:
		p.writeExpr(stmt.X, 0, false)
		p.writeString(stmt.Tok.String())
	case *ast.EmptyStmt:
		p.writeByte(';')
	default:
		p.fail(stmt)
	}
}

func (p *sourceRenderer) writeBlock(block *ast.BlockStmt) {
	p.writeString("{\n")
	p.indent++
	for _, stmt := range block.List {
		p.writeIndent()
		p.writeStmt(stmt)
		p.writeByte('\n')
	}
	p.indent--
	p.writeIndent()
	p.writeByte('}')
}

func (p *sourceRenderer) writeCaseClause(clause *ast.CaseClause) {
	if len(clause.List) == 0 {
		p.writeString("default:")
	} else {
		p.writeString("case ")
		p.writeExprList(clause.List)
		p.writeByte(':')
	}
	p.indent++
	for _, stmt := range clause.Body {
		p.writeByte('\n')
		p.writeIndent()
		p.writeStmt(stmt)
	}
	p.indent--
}

func (p *sourceRenderer) writeCommClause(clause *ast.CommClause) {
	if clause.Comm == nil {
		p.writeString("default:")
	} else {
		p.writeString("case ")
		p.writeStmt(clause.Comm)
		p.writeByte(':')
	}
	p.indent++
	for _, stmt := range clause.Body {
		p.writeByte('\n')
		p.writeIndent()
		p.writeStmt(stmt)
	}
	p.indent--
}

func (p *sourceRenderer) writeExprList(expressions []ast.Expr) {
	for i, expression := range expressions {
		if i > 0 {
			p.writeString(", ")
		}
		p.writeExpr(expression, 0, false)
	}
}

func (p *sourceRenderer) writeExpr(expression ast.Expr, parentPrecedence int, rightChild bool) {
	if expression == nil {
		p.fail(expression)
		return
	}
	precedence := expressionPrecedence(expression)
	parenthesize := precedence < parentPrecedence || rightChild && precedence == parentPrecedence
	if parenthesize {
		p.writeByte('(')
	}
	switch expression := expression.(type) {
	case *ast.Ident:
		p.writeString(expression.Name)
	case *ast.BasicLit:
		p.writeString(expression.Value)
	case *ast.BinaryExpr:
		opPrecedence := expression.Op.Precedence()
		p.writeExpr(expression.X, opPrecedence, false)
		p.writeByte(' ')
		p.writeString(expression.Op.String())
		p.writeByte(' ')
		p.writeExpr(expression.Y, opPrecedence, true)
	case *ast.UnaryExpr:
		p.writeString(expression.Op.String())
		if _, nested := expression.X.(*ast.UnaryExpr); nested {
			p.writeByte('(')
			p.writeExpr(expression.X, 0, false)
			p.writeByte(')')
		} else {
			p.writeExpr(expression.X, token.UnaryPrec, false)
		}
	case *ast.ParenExpr:
		p.writeByte('(')
		p.writeExpr(expression.X, 0, false)
		p.writeByte(')')
	case *ast.SelectorExpr:
		p.writePostfixBase(expression.X)
		p.writeByte('.')
		p.writeString(expression.Sel.Name)
	case *ast.IndexExpr:
		p.writePostfixBase(expression.X)
		p.writeByte('[')
		p.writeExpr(expression.Index, 0, false)
		p.writeByte(']')
	case *ast.IndexListExpr:
		p.writePostfixBase(expression.X)
		p.writeByte('[')
		p.writeExprList(expression.Indices)
		p.writeByte(']')
	case *ast.SliceExpr:
		p.writePostfixBase(expression.X)
		p.writeByte('[')
		if expression.Low != nil {
			p.writeExpr(expression.Low, 0, false)
		}
		p.writeByte(':')
		if expression.High != nil {
			p.writeExpr(expression.High, 0, false)
		}
		if expression.Slice3 {
			p.writeByte(':')
			if expression.Max != nil {
				p.writeExpr(expression.Max, 0, false)
			}
		}
		p.writeByte(']')
	case *ast.CallExpr:
		p.writeCallTarget(expression.Fun)
		p.writeByte('(')
		p.writeExprList(expression.Args)
		if expression.Ellipsis.IsValid() {
			p.writeString("...")
		}
		p.writeByte(')')
	case *ast.StarExpr:
		p.writeByte('*')
		p.writeExpr(expression.X, token.UnaryPrec, false)
	case *ast.TypeAssertExpr:
		p.writePostfixBase(expression.X)
		p.writeString(".(")
		if expression.Type == nil {
			p.writeString("type")
		} else {
			p.writeExpr(expression.Type, 0, false)
		}
		p.writeByte(')')
	case *ast.CompositeLit:
		if expression.Type != nil {
			p.writeExpr(expression.Type, 0, false)
		}
		p.writeByte('{')
		p.writeExprList(expression.Elts)
		p.writeByte('}')
	case *ast.KeyValueExpr:
		p.writeExpr(expression.Key, 0, false)
		p.writeString(": ")
		p.writeExpr(expression.Value, 0, false)
	case *ast.FuncLit:
		p.writeString("func")
		p.writeSignature(expression.Type)
		p.writeByte(' ')
		p.writeBlock(expression.Body)
	case *ast.FuncType:
		p.writeString("func")
		p.writeSignature(expression)
	case *ast.StructType:
		p.writeString("struct")
		p.writeRecordFields(expression.Fields, false)
	case *ast.InterfaceType:
		p.writeString("interface")
		p.writeRecordFields(expression.Methods, true)
	case *ast.ArrayType:
		p.writeByte('[')
		if expression.Len != nil {
			p.writeExpr(expression.Len, 0, false)
		}
		p.writeByte(']')
		p.writeExpr(expression.Elt, token.UnaryPrec, false)
	case *ast.MapType:
		p.writeString("map[")
		p.writeExpr(expression.Key, 0, false)
		p.writeByte(']')
		p.writeExpr(expression.Value, token.UnaryPrec, false)
	case *ast.ChanType:
		switch expression.Dir {
		case ast.SEND | ast.RECV:
			p.writeString("chan ")
		case ast.SEND:
			p.writeString("chan<- ")
		case ast.RECV:
			p.writeString("<-chan ")
		default:
			p.fail(expression)
		}
		if nested, ok := expression.Value.(*ast.ChanType); ok && nested.Dir == ast.RECV {
			p.writeByte('(')
			p.writeExpr(nested, 0, false)
			p.writeByte(')')
		} else {
			p.writeExpr(expression.Value, token.UnaryPrec, false)
		}
	case *ast.Ellipsis:
		p.writeString("...")
		if expression.Elt != nil {
			p.writeExpr(expression.Elt, token.UnaryPrec, false)
		}
	default:
		p.fail(expression)
	}
	if parenthesize {
		p.writeByte(')')
	}
}

func (p *sourceRenderer) writeCallTarget(expression ast.Expr) {
	parenthesize := false
	switch expression := expression.(type) {
	case *ast.FuncType:
		parenthesize = true
	case *ast.ChanType:
		parenthesize = expression.Dir == ast.RECV
	}
	if parenthesize {
		p.writeByte('(')
		p.writeExpr(expression, 0, false)
		p.writeByte(')')
		return
	}
	p.writeExpr(expression, token.HighestPrec, false)
}

func (p *sourceRenderer) writePostfixBase(expression ast.Expr) {
	if _, composite := expression.(*ast.CompositeLit); composite {
		p.writeByte('(')
		p.writeExpr(expression, 0, false)
		p.writeByte(')')
		return
	}
	p.writeExpr(expression, token.HighestPrec, false)
}

func (p *sourceRenderer) writeRecordFields(fields *ast.FieldList, interfaceFields bool) {
	p.writeByte('{')
	if fields != nil && len(fields.List) > 0 {
		p.writeByte(' ')
		for i, field := range fields.List {
			if i > 0 {
				p.writeString("; ")
			}
			p.writeField(field, interfaceFields)
		}
		p.writeByte(' ')
	}
	p.writeByte('}')
}

func combinesWithTypeParamName(expression ast.Expr) bool {
	switch expression := expression.(type) {
	case *ast.StarExpr:
		return !isTypeElement(expression.X)
	case *ast.BinaryExpr:
		return combinesWithTypeParamName(expression.X) && !isTypeElement(expression.Y)
	case *ast.ParenExpr:
		return !isTypeElement(expression.X)
	default:
		return false
	}
}

func isTypeElement(expression ast.Expr) bool {
	switch expression := expression.(type) {
	case *ast.ArrayType, *ast.StructType, *ast.FuncType, *ast.InterfaceType, *ast.MapType, *ast.ChanType:
		return true
	case *ast.UnaryExpr:
		return expression.Op == token.TILDE
	case *ast.BinaryExpr:
		return isTypeElement(expression.X) || isTypeElement(expression.Y)
	case *ast.ParenExpr:
		return isTypeElement(expression.X)
	default:
		return false
	}
}

func expressionPrecedence(expression ast.Expr) int {
	switch expression := expression.(type) {
	case *ast.BinaryExpr:
		return expression.Op.Precedence()
	case *ast.UnaryExpr, *ast.StarExpr:
		return token.UnaryPrec
	case *ast.SelectorExpr, *ast.IndexExpr, *ast.IndexListExpr, *ast.SliceExpr, *ast.CallExpr, *ast.TypeAssertExpr:
		return token.HighestPrec
	default:
		return token.HighestPrec + 1
	}
}
