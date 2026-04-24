package transpile

import (
	"go/ast"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) lowerPanicExprAST(message checker.Expression, resultType checker.Type) (ast.Expr, bool, error) {
	msg, ok, err := e.lowerExprAST(message)
	if err != nil || !ok {
		return nil, ok, err
	}
	funcType := &ast.FuncType{}
	if resultType != nil && resultType != checker.Void {
		typeExpr, err := e.lowerTypeExpr(resultType)
		if err != nil {
			return nil, false, err
		}
		funcType.Results = funcResults(typeExpr)
	}
	return &ast.CallExpr{Fun: &ast.FuncLit{Type: funcType, Body: &ast.BlockStmt{List: []ast.Stmt{
		&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{msg}}},
	}}}}, true, nil
}

func (e *emitter) lowerIfExprAST(expr *checker.If) (ast.Expr, bool, error) {
	if expr == nil {
		return nil, false, nil
	}
	resultType := expr.Type()
	typeExpr, err := e.lowerTypeExpr(resultType)
	if err != nil {
		return nil, false, err
	}
	stmt, ok, err := e.lowerIfStatementAST(expr, resultType, true)
	if err != nil || !ok {
		return nil, ok, err
	}
	return &ast.CallExpr{Fun: &ast.FuncLit{Type: &ast.FuncType{Results: funcResults(typeExpr)}, Body: &ast.BlockStmt{List: []ast.Stmt{stmt}}}}, true, nil
}
