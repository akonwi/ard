package go_backend

import "go/ast"

func selectorExpr(x ast.Expr, sel string) ast.Expr {
	return &ast.SelectorExpr{X: x, Sel: ast.NewIdent(sel)}
}

func indexExpr(base ast.Expr, args []ast.Expr) ast.Expr {
	switch len(args) {
	case 0:
		return base
	case 1:
		return &ast.IndexExpr{X: base, Index: args[0]}
	default:
		return &ast.IndexListExpr{X: base, Indices: args}
	}
}

func astCall(fun ast.Expr, typeArgs []ast.Expr, args []ast.Expr) ast.Expr {
	if len(typeArgs) > 0 {
		fun = indexExpr(fun, typeArgs)
	}
	return &ast.CallExpr{Fun: fun, Args: args}
}
