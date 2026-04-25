package go_backend

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) lowerFunctionBodyBlock(stmts []checker.Statement, returnType checker.Type) (*ast.BlockStmt, error) {
	prevScopes := cloneLocalScopes(e.localScopes)
	prevPointerScopes := clonePointerScopes(e.pointerScopes)
	prevCounts := cloneLocalNameCounts(e.localNameCounts)
	prevTempCounter := e.tempCounter
	prevReturnType := e.fnReturnType
	defer func() {
		e.fnReturnType = prevReturnType
	}()
	e.fnReturnType = returnType
	if block, ok, err := e.lowerStatementsBlockAST(stmts, returnType); err != nil {
		if !errors.Is(err, errStructuredLoweringUnsupported) {
			return nil, err
		}
	} else if ok {
		return block, nil
	}
	e.localScopes = prevScopes
	e.pointerScopes = prevPointerScopes
	e.localNameCounts = prevCounts
	e.tempCounter = prevTempCounter
	e.fnReturnType = returnType
	return nil, errStructuredLoweringUnsupported
}

func (e *emitter) lowerFunctionDeclNode(def *checker.FunctionDef) (ast.Decl, error) {
	order, paramsMap, constraints := functionTypeParams(def)
	var decl ast.Decl
	err := e.withFreshLocals(func() error {
		return e.withTypeParams(paramsMap, func() error {
			e.pushScope()
			defer e.popScope()
			params, err := e.lowerBoundFunctionParamFields(def.Parameters)
			if err != nil {
				return fmt.Errorf("function %s: %w", def.Name, err)
			}
			returnType := effectiveFunctionReturnType(def)
			funcType := &ast.FuncType{
				TypeParams: typeParamFieldList(order, paramsMap, constraints),
				Params:     &ast.FieldList{List: params},
			}
			if returnType != checker.Void {
				resultType, err := e.lowerTypeExpr(returnType)
				if err != nil {
					return fmt.Errorf("function %s: %w", def.Name, err)
				}
				funcType.Results = funcResults(resultType)
			}
			body, err := e.lowerFunctionBodyBlock(def.Body.Stmts, returnType)
			if err != nil {
				return fmt.Errorf("function %s: %w", def.Name, err)
			}
			decl = &ast.FuncDecl{
				Name: ast.NewIdent(e.functionNames[def.Name]),
				Type: funcType,
				Body: body,
			}
			return nil
		})
	})
	return decl, err
}

func (e *emitter) lowerReceiverMethodDeclNode(typeName string, receiverType ast.Expr, typeParams map[string]string, method *checker.FunctionDef) (ast.Decl, error) {
	var decl ast.Decl
	err := e.withFreshLocals(func() error {
		return e.withTypeParams(typeParams, func() error {
			e.pushScope()
			defer e.popScope()
			recvType := receiverType
			if method.Mutates {
				recvType = &ast.StarExpr{X: recvType}
			}
			receiverName := e.bindLocal(method.Receiver)
			params, err := e.lowerBoundFunctionParamFields(method.Parameters)
			if err != nil {
				return fmt.Errorf("method %s.%s: %w", typeName, method.Name, err)
			}
			funcType := &ast.FuncType{Params: &ast.FieldList{List: params}}
			if method.ReturnType != checker.Void {
				resultType, err := e.lowerTypeExpr(method.ReturnType)
				if err != nil {
					return fmt.Errorf("method %s.%s: %w", typeName, method.Name, err)
				}
				funcType.Results = funcResults(resultType)
			}
			body, err := e.lowerFunctionBodyBlock(method.Body.Stmts, method.ReturnType)
			if err != nil {
				return fmt.Errorf("method %s.%s: %w", typeName, method.Name, err)
			}
			decl = &ast.FuncDecl{
				Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(receiverName)}, Type: recvType}}},
				Name: ast.NewIdent(goName(method.Name, !method.Private)),
				Type: funcType,
				Body: body,
			}
			return nil
		})
	})
	return decl, err
}

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
