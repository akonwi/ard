package javascript

import "strings"

type jsExpr interface {
	jsExprNode()
}

type jsRawExprIR struct {
	Code string
}

type jsCallExprIR struct {
	Callee string
	Args   []string
}

type jsArrayExprIR struct {
	Items []string
}

type jsMemberExprIR struct {
	Object   string
	Property string
}

type jsNewExprIR struct {
	Ctor string
	Args []string
}

type jsUnaryExprIR struct {
	Op    string
	Value jsExpr
}

type jsBinaryExprIR struct {
	Left  jsExpr
	Op    string
	Right jsExpr
}

type jsStmt interface {
	jsStmtNode()
}

type jsRawStmtIR struct {
	Code string
}

type jsExprStmtIR struct {
	Expr string
}

type jsVarDeclStmtIR struct {
	Keyword  string
	Name     string
	Value    string
	HasValue bool
}

type jsAssignStmtIR struct {
	Left  string
	Right string
}

type jsReturnStmtIR struct {
	Value string
}

type jsThrowStmtIR struct {
	Value string
}

type jsIfStmtIR struct {
	Condition string
	Then      []jsStmt
	Else      []jsStmt
}

func (jsRawExprIR) jsExprNode()    {}
func (jsCallExprIR) jsExprNode()   {}
func (jsArrayExprIR) jsExprNode()  {}
func (jsMemberExprIR) jsExprNode() {}
func (jsNewExprIR) jsExprNode()    {}
func (jsUnaryExprIR) jsExprNode()  {}
func (jsBinaryExprIR) jsExprNode() {}

func (jsRawStmtIR) jsStmtNode()     {}
func (jsExprStmtIR) jsStmtNode()    {}
func (jsVarDeclStmtIR) jsStmtNode() {}
func (jsAssignStmtIR) jsStmtNode()  {}
func (jsReturnStmtIR) jsStmtNode()  {}
func (jsThrowStmtIR) jsStmtNode()   {}
func (jsIfStmtIR) jsStmtNode()      {}

func rawJSExpr(code string) jsExpr {
	return jsRawExprIR{Code: strings.TrimSpace(code)}
}

func rawJSStmt(code string) jsStmt {
	return jsRawStmtIR{Code: strings.TrimSpace(code)}
}

func renderJSExpr(expr jsExpr) string {
	switch expr := expr.(type) {
	case jsRawExprIR:
		return expr.Code
	case jsCallExprIR:
		return renderJSDoc(jsCallDoc(expr.Callee, expr.Args))
	case jsArrayExprIR:
		return renderJSDoc(jsArrayDoc(expr.Items))
	case jsMemberExprIR:
		return expr.Object + "." + expr.Property
	case jsNewExprIR:
		return renderJSDoc(jsCallDoc("new "+expr.Ctor, expr.Args))
	case jsUnaryExprIR:
		return "(" + expr.Op + renderJSExpr(expr.Value) + ")"
	case jsBinaryExprIR:
		return "(" + renderJSExpr(expr.Left) + " " + expr.Op + " " + renderJSExpr(expr.Right) + ")"
	default:
		return ""
	}
}

func renderJSStmt(stmt jsStmt) string {
	switch stmt := stmt.(type) {
	case jsRawStmtIR:
		return stmt.Code
	case jsExprStmtIR:
		return renderJSDoc(jsExprStmtDoc(stmt.Expr))
	case jsVarDeclStmtIR:
		if !stmt.HasValue {
			return stmt.Keyword + " " + stmt.Name + ";"
		}
		return renderJSDoc(jsVarDeclDoc(stmt.Keyword, stmt.Name, stmt.Value))
	case jsAssignStmtIR:
		return renderJSDoc(jsAssignDoc(stmt.Left, stmt.Right))
	case jsReturnStmtIR:
		return renderJSDoc(jsReturnDoc(stmt.Value))
	case jsThrowStmtIR:
		return renderJSDoc(jsThrowDoc(stmt.Value))
	case jsIfStmtIR:
		var elseDoc jsDoc
		if len(stmt.Else) > 0 {
			elseDoc = jsBareBlockDoc(renderJSStmts(stmt.Else))
		}
		return renderJSDoc(jsIfDoc(stmt.Condition, renderJSStmts(stmt.Then), elseDoc))
	default:
		return ""
	}
}

func renderJSStmts(stmts []jsStmt) string {
	parts := make([]string, 0, len(stmts))
	for _, stmt := range stmts {
		text := strings.TrimSpace(renderJSStmt(stmt))
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}
