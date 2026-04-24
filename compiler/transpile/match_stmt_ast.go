package transpile

import (
	"go/ast"
	"go/token"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) lowerBoolMatchStatementAST(match *checker.BoolMatch) ([]ast.Stmt, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	trueBody, ok, err := e.lowerMatchBranchAST(match.True, checker.Void, nil)
	if err != nil || !ok {
		return nil, ok, err
	}
	falseBody, ok, err := e.lowerMatchBranchAST(match.False, checker.Void, nil)
	if err != nil || !ok {
		return nil, ok, err
	}
	return []ast.Stmt{&ast.IfStmt{Cond: subject, Body: trueBody, Else: falseBody}}, true, nil
}

func (e *emitter) lowerResultMatchStatementAST(match *checker.ResultMatch) ([]ast.Stmt, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	if match.Ok == nil || match.Ok.Body == nil || match.Err == nil || match.Err.Body == nil {
		return nil, false, errStructuredLoweringUnsupported
	}
	resultName := e.nextTemp("Result")
	okBody, ok, err := e.lowerMatchBranchAST(match.Ok.Body, checker.Void, func() ([]ast.Stmt, error) {
		if match.Ok.Pattern == nil || match.Ok.Pattern.Name == "_" {
			return nil, nil
		}
		patternName := e.bindLocal(match.Ok.Pattern.Name)
		bound, err := e.lowerCopiedValueAST(&ast.CallExpr{Fun: selectorExpr(ast.NewIdent(resultName), "UnwrapOk")}, match.OkType)
		if err != nil {
			return nil, err
		}
		prefix := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(patternName)}, Tok: token.DEFINE, Rhs: []ast.Expr{bound}}}
		if !usesNameInStatements(match.Ok.Body.Stmts, match.Ok.Pattern.Name) {
			prefix = append(prefix, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(patternName)}})
		}
		return prefix, nil
	})
	if err != nil || !ok {
		return nil, ok, err
	}
	errBody, ok, err := e.lowerMatchBranchAST(match.Err.Body, checker.Void, func() ([]ast.Stmt, error) {
		if match.Err.Pattern == nil || match.Err.Pattern.Name == "_" {
			return nil, nil
		}
		patternName := e.bindLocal(match.Err.Pattern.Name)
		prefix := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(patternName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: selectorExpr(ast.NewIdent(resultName), "UnwrapErr")}}}}
		if !usesNameInStatements(match.Err.Body.Stmts, match.Err.Pattern.Name) {
			prefix = append(prefix, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(patternName)}})
		}
		return prefix, nil
	})
	if err != nil || !ok {
		return nil, ok, err
	}
	return []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultName)}, Tok: token.DEFINE, Rhs: []ast.Expr{subject}},
		&ast.IfStmt{Cond: &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(resultName), "IsOk")}, Body: okBody, Else: errBody},
	}, true, nil
}
