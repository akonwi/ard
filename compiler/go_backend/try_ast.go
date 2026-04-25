package go_backend

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) lowerTrySuccessValueAST(op *checker.TryOp, tempName string) (ast.Expr, error) {
	switch op.Kind {
	case checker.TryResult:
		return e.lowerCopiedValueAST(&ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tempName), "UnwrapOk")}, op.OkType)
	case checker.TryMaybe:
		return e.lowerCopiedValueAST(&ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tempName), "Expect"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("unreachable none in try success path")}}}, op.OkType)
	default:
		return nil, errStructuredLoweringUnsupported
	}
}

func (e *emitter) lowerTryCatchBlockAST(op *checker.TryOp, returnType checker.Type, tempName string) (*ast.BlockStmt, bool, error) {
	if op.CatchBlock == nil {
		return nil, false, errStructuredLoweringUnsupported
	}
	setup := func() ([]ast.Stmt, error) {
		if op.Kind != checker.TryResult || op.CatchVar == "" || op.CatchVar == "_" {
			return nil, nil
		}
		name := e.bindLocal(op.CatchVar)
		prefix := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(name)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tempName), "UnwrapErr")}}}}
		if !usesNameInStatements(op.CatchBlock.Stmts, op.CatchVar) {
			prefix = append(prefix, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(name)}})
		}
		return prefix, nil
	}
	return e.lowerMatchBranchAST(op.CatchBlock, returnType, setup)
}

func (e *emitter) lowerTryDefaultFailureAST(op *checker.TryOp, tempName string) ([]ast.Stmt, error) {
	switch op.Kind {
	case checker.TryResult:
		resultType, ok := e.fnReturnType.(*checker.Result)
		if !ok {
			return nil, errStructuredLoweringUnsupported
		}
		valType, err := e.lowerTypeArgExprWithOptions(resultType.Val(), e.typeParams, nil)
		if err != nil {
			return nil, err
		}
		errType, err := e.lowerTypeArgExprWithOptions(resultType.Err(), e.typeParams, nil)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{astCall(selectorExpr(ast.NewIdent(helperImportAlias), "Err"), []ast.Expr{valType, errType}, []ast.Expr{&ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tempName), "UnwrapErr")}})}}}, nil
	case checker.TryMaybe:
		maybeType, ok := e.fnReturnType.(*checker.Maybe)
		if !ok {
			return nil, errStructuredLoweringUnsupported
		}
		innerType, err := e.lowerTypeArgExprWithOptions(maybeType.Of(), e.typeParams, nil)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{astCall(selectorExpr(ast.NewIdent(helperImportAlias), "None"), []ast.Expr{innerType}, nil)}}}, nil
	default:
		return nil, errStructuredLoweringUnsupported
	}
}

func (e *emitter) lowerTryOpAST(op *checker.TryOp, returnType checker.Type, onSuccess func(ast.Expr) ([]ast.Stmt, error)) ([]ast.Stmt, bool, error) {
	if op == nil {
		return nil, false, nil
	}
	subject, ok, err := e.lowerExprAST(op.Expr())
	if err != nil || !ok {
		return nil, ok, err
	}
	tempName := e.nextTemp("Try")
	list := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(tempName)}, Tok: token.DEFINE, Rhs: []ast.Expr{subject}}}
	failureBody := &ast.BlockStmt{}
	switch {
	case op.CatchBlock != nil:
		failureBody, ok, err = e.lowerTryCatchBlockAST(op, returnType, tempName)
		if err != nil || !ok {
			return nil, ok, err
		}
		if op.CatchBlock.Type() == checker.Void {
			if returnType == nil || returnType == checker.Void {
				failureBody.List = append(failureBody.List, &ast.ReturnStmt{})
			} else {
				return nil, false, errStructuredLoweringUnsupported
			}
		}
		cond := ast.Expr(nil)
		if op.Kind == checker.TryResult {
			cond = &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tempName), "IsErr")}
		} else {
			cond = &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tempName), "IsNone")}
		}
		list = append(list, &ast.IfStmt{Cond: cond, Body: failureBody})
	default:
		body, err := e.lowerTryDefaultFailureAST(op, tempName)
		if err != nil {
			return nil, false, err
		}
		cond := ast.Expr(nil)
		if op.Kind == checker.TryResult {
			cond = &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tempName), "IsErr")}
		} else {
			cond = &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tempName), "IsNone")}
		}
		list = append(list, &ast.IfStmt{Cond: cond, Body: &ast.BlockStmt{List: body}})
	}
	if onSuccess != nil {
		successValue, err := e.lowerTrySuccessValueAST(op, tempName)
		if err != nil {
			return nil, false, err
		}
		successStmts, err := onSuccess(successValue)
		if err != nil {
			return nil, false, err
		}
		list = append(list, successStmts...)
	}
	return list, true, nil
}
