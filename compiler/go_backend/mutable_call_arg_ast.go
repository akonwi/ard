package go_backend

import (
	"go/ast"
	"go/token"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) lowerMutableCallArgAST(arg checker.Expression, param checker.Parameter) (ast.Expr, bool, error) {
	if !mutableParamNeedsPointer(param.Type) {
		return e.lowerValueForTypeAST(arg, param.Type)
	}
	typeExpr, err := e.lowerTypeExpr(param.Type)
	if err != nil {
		return nil, false, err
	}
	wrapAddress := func(value ast.Expr) ast.Expr {
		return &ast.CallExpr{Fun: &ast.FuncLit{Type: &ast.FuncType{Results: funcResults(&ast.StarExpr{X: typeExpr})}, Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("value")}, Tok: token.DEFINE, Rhs: []ast.Expr{value}},
			&ast.ReturnStmt{Results: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent("value")}}},
		}}}}
	}
	switch v := arg.(type) {
	case *checker.CopyExpression:
		value, ok, err := e.lowerExprAST(v)
		if err != nil || !ok {
			return nil, ok, err
		}
		return wrapAddress(value), true, nil
	case *checker.Identifier:
		resolved := e.resolveLocal(v.Name)
		if e.isPointerLocal(v.Name) {
			return ast.NewIdent(resolved), true, nil
		}
		return &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(resolved)}, true, nil
	case checker.Variable:
		resolved := e.resolveLocal(v.Name())
		if e.isPointerLocal(v.Name()) {
			return ast.NewIdent(resolved), true, nil
		}
		return &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(resolved)}, true, nil
	case *checker.Variable:
		resolved := e.resolveLocal(v.Name())
		if e.isPointerLocal(v.Name()) {
			return ast.NewIdent(resolved), true, nil
		}
		return &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(resolved)}, true, nil
	default:
		target, ok, err := e.lowerAssignmentTargetAST(arg)
		if err == nil && ok {
			return &ast.UnaryExpr{Op: token.AND, X: target}, true, nil
		}
		value, ok, err := e.lowerExprAST(arg)
		if err != nil || !ok {
			return nil, ok, err
		}
		return wrapAddress(value), true, nil
	}
}
