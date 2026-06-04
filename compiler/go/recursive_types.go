package gotarget

import (
	"go/ast"
	"go/token"

	"github.com/akonwi/ard/air"
)

func structFieldByName(typ air.TypeInfo, name string) (air.FieldInfo, bool) {
	for _, field := range typ.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return air.FieldInfo{}, false
}

func recursiveNullableValueAsPointer(l *lowerer, maybeType air.TypeID, expr ast.Expr) ast.Expr {
	if !validTypeID(l.program, maybeType) {
		return expr
	}
	maybeInfo := l.program.Types[maybeType-1]
	if maybeInfo.Kind != air.TypeMaybe || !validTypeID(l.program, maybeInfo.Elem) {
		return expr
	}
	maybeExpr := mustTypeExpr(l, maybeType)
	elemExpr := mustTypeExpr(l, maybeInfo.Elem)
	param := ast.NewIdent("value")
	return &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{param}, Type: maybeExpr}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: elemExpr}}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.IfStmt{
				Cond: &ast.UnaryExpr{Op: token.NOT, X: &ast.SelectorExpr{X: param, Sel: ast.NewIdent("Some")}},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("nil")}}}},
			},
			&ast.ReturnStmt{Results: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: &ast.SelectorExpr{X: param, Sel: ast.NewIdent("Value")}}}},
		}},
	}, Args: []ast.Expr{expr}}
}

func recursiveNullableFieldAsMaybe(l *lowerer, maybeType air.TypeID, fieldExpr ast.Expr) ast.Expr {
	maybeExpr := mustTypeExpr(l, maybeType)
	return &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: maybeExpr}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: fieldExpr, Op: token.EQL, Y: ast.NewIdent("nil")},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CompositeLit{Type: maybeExpr}}}}},
			},
			&ast.ReturnStmt{Results: []ast.Expr{&ast.CompositeLit{Type: maybeExpr, Elts: []ast.Expr{
				&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: &ast.StarExpr{X: fieldExpr}},
				&ast.KeyValueExpr{Key: ast.NewIdent("Some"), Value: ast.NewIdent("true")},
			}}}},
		}},
	}}
}
