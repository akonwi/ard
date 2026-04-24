package transpile

import (
	"go/ast"
	"go/token"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) inlineFuncCallAST(returnType checker.Type, body []ast.Stmt) (ast.Expr, error) {
	funcType := &ast.FuncType{}
	if returnType != nil && returnType != checker.Void {
		typeExpr, err := e.lowerTypeExpr(returnType)
		if err != nil {
			return nil, err
		}
		funcType.Results = funcResults(typeExpr)
	}
	return &ast.CallExpr{Fun: &ast.FuncLit{Type: funcType, Body: &ast.BlockStmt{List: body}}}, nil
}

func (e *emitter) lowerCopiedValueAST(value ast.Expr, t checker.Type) (ast.Expr, error) {
	switch typed := t.(type) {
	case *checker.List:
		typeExpr, err := e.lowerTypeExpr(typed)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{&ast.CallExpr{Fun: typeExpr, Args: []ast.Expr{ast.NewIdent("nil")}}, value}, Ellipsis: token.Pos(1)}, nil
	default:
		return value, nil
	}
}
