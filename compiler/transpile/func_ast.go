package transpile

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) lowerExternFunctionDeclNode(def *checker.ExternalFunctionDef) (ast.Decl, error) {
	order, paramsMap, constraints := signatureTypeParams(def.Parameters, def.ReturnType)
	var decl ast.Decl
	err := e.withFreshLocals(func() error {
		return e.withTypeParams(paramsMap, func() error {
			e.pushScope()
			defer e.popScope()

			paramFields := make([]*ast.Field, 0, len(def.Parameters))
			argExprs := make([]ast.Expr, 0, len(def.Parameters)+1)
			argExprs = append(argExprs, &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(def.ExternalBinding)})
			for _, param := range def.Parameters {
				typeExpr, err := e.lowerTypeExpr(param.Type)
				if err != nil {
					return fmt.Errorf("extern function %s: %w", def.Name, err)
				}
				name := e.bindLocal(param.Name)
				paramFields = append(paramFields, &ast.Field{
					Names: []*ast.Ident{ast.NewIdent(name)},
					Type:  typeExpr,
				})
				argExprs = append(argExprs, ast.NewIdent(name))
			}

			funcType := &ast.FuncType{
				TypeParams: typeParamFieldList(order, paramsMap, constraints),
				Params:     &ast.FieldList{List: paramFields},
			}
			if def.ReturnType != checker.Void {
				returnType, err := e.lowerTypeExpr(def.ReturnType)
				if err != nil {
					return fmt.Errorf("extern function %s: %w", def.Name, err)
				}
				funcType.Results = funcResults(returnType)
			}

			call := &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "CallExtern"), Args: argExprs}
			resultName := ast.NewIdent("result")
			if def.ReturnType == checker.Void {
				resultName = ast.NewIdent("_")
			}
			errName := ast.NewIdent("err")
			body := []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{resultName, errName}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
				&ast.IfStmt{
					Cond: &ast.BinaryExpr{X: ast.NewIdent("err"), Op: token.NEQ, Y: ast.NewIdent("nil")},
					Body: &ast.BlockStmt{List: []ast.Stmt{
						&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{ast.NewIdent("err")}}},
					}},
				},
			}
			if def.ReturnType != checker.Void {
				returnType, err := e.lowerTypeExpr(def.ReturnType)
				if err != nil {
					return err
				}
				coerceFun := indexExpr(selectorExpr(ast.NewIdent(helperImportAlias), "CoerceExtern"), []ast.Expr{returnType})
				body = append(body, &ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: coerceFun, Args: []ast.Expr{ast.NewIdent("result")}}}})
			}

			decl = &ast.FuncDecl{
				Name: ast.NewIdent(e.functionNames[def.Name]),
				Type: funcType,
				Body: &ast.BlockStmt{List: body},
			}
			return nil
		})
	})
	return decl, err
}
