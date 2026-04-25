package go_backend

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) lowerVariableDefFromMatchAST(name string, def *checker.VariableDef, remaining []checker.Statement) ([]ast.Stmt, bool, error) {
	if def == nil {
		return nil, false, nil
	}
	target := ast.NewIdent(name)
	var body []ast.Stmt
	var ok bool
	var err error
	switch match := def.Value.(type) {
	case *checker.BoolMatch:
		body, ok, err = e.lowerBoolMatchAssignAST(match, target, def.Type())
	case *checker.OptionMatch:
		body, ok, err = e.lowerOptionMatchAssignAST(match, target, def.Type())
	case *checker.ResultMatch:
		body, ok, err = e.lowerResultMatchAssignAST(match, target, def.Type())
	case *checker.UnionMatch:
		body, ok, err = e.lowerUnionMatchAssignAST(match, target, def.Type())
	default:
		return nil, false, nil
	}
	if err != nil || !ok {
		return nil, ok, err
	}
	typeExpr, err := e.lowerTypeExpr(def.Type())
	if err != nil {
		return nil, false, err
	}
	stmts := append([]ast.Stmt{&ast.DeclStmt{Decl: astVarDecl(astValueSpec(name, typeExpr, nil))}}, body...)
	if !usesNameInStatements(remaining, def.Name) {
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{target}})
	}
	return stmts, true, nil
}

func (e *emitter) lowerStatementsIntoTargetAST(stmts []checker.Statement, target ast.Expr, targetType checker.Type) ([]ast.Stmt, bool, error) {
	lastMeaningful := lastMeaningfulStatementIndex(stmts)
	if lastMeaningful < 0 {
		return nil, false, nil
	}
	list := make([]ast.Stmt, 0, len(stmts))
	for i, stmt := range stmts {
		if stmt.Break {
			list = append(list, &ast.BranchStmt{Tok: token.BREAK})
			continue
		}
		isLastExpr := i == lastMeaningful && stmt.Expr != nil
		remaining := stmts[i+1:]
		if stmt.Stmt != nil {
			nodes, ok, err := e.lowerNonProducingIntoTargetAST(stmt.Stmt, remaining, targetType)
			if err != nil || !ok {
				return nil, ok, err
			}
			list = append(list, nodes...)
			continue
		}
		if stmt.Expr == nil {
			continue
		}
		if isLastExpr {
			nodes, ok, err := e.lowerExpressionIntoTargetAST(stmt.Expr, target, targetType)
			if err != nil || !ok {
				return nil, ok, err
			}
			list = append(list, nodes...)
			continue
		}
		nodes, ok, err := e.lowerExpressionStatementAST(stmt.Expr, checker.Void, false)
		if err != nil || !ok {
			return nil, ok, err
		}
		list = append(list, nodes...)
	}
	return list, true, nil
}

func (e *emitter) lowerNonProducingIntoTargetAST(stmt checker.NonProducing, remaining []checker.Statement, targetType checker.Type) ([]ast.Stmt, bool, error) {
	flowType := e.fnReturnType
	if flowType == nil {
		flowType = targetType
	}
	switch s := stmt.(type) {
	case *checker.VariableDef:
		name := e.bindLocal(s.Name)
		if tryOp, ok := s.Value.(*checker.TryOp); ok {
			stmts, ok, err := e.lowerTryOpAST(tryOp, flowType, func(successValue ast.Expr) ([]ast.Stmt, error) {
				lhs := ast.NewIdent(name)
				if typeNeedsExplicitVarAnnotation(s.Type()) {
					typeExpr, err := e.lowerTypeExpr(s.Type())
					if err != nil {
						return nil, err
					}
					return []ast.Stmt{&ast.DeclStmt{Decl: astVarDecl(astValueSpec(name, typeExpr, successValue))}}, nil
				}
				return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.DEFINE, Rhs: []ast.Expr{successValue}}}, nil
			})
			if err != nil || !ok {
				return nil, ok, err
			}
			if !usesNameInStatements(remaining, s.Name) {
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(name)}})
			}
			return stmts, true, nil
		}
	}
	return e.lowerNonProducingAST(stmt, remaining, checker.Void)
}

func (e *emitter) lowerExpressionIntoTargetAST(expr checker.Expression, target ast.Expr, targetType checker.Type) ([]ast.Stmt, bool, error) {
	value, ok, err := e.lowerValueForTypeAST(expr, targetType)
	if err == nil && ok {
		return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{value}}}, true, nil
	}
	flowType := e.fnReturnType
	if flowType == nil {
		flowType = targetType
	}
	prelude, lowered, preludeOK, preludeErr := e.lowerExprWithPreludeAST(expr, flowType)
	if preludeErr != nil || !preludeOK {
		return nil, ok, err
	}
	lowered, preludeErr = e.wrapTraitValueAST(lowered, targetType)
	if preludeErr != nil {
		return nil, false, preludeErr
	}
	return append(prelude, &ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{lowered}}), true, nil
}

func (e *emitter) lowerMatchBranchIntoTargetAST(block *checker.Block, target ast.Expr, targetType checker.Type, setup func() ([]ast.Stmt, error)) (*ast.BlockStmt, bool, error) {
	if block == nil {
		return &ast.BlockStmt{}, true, nil
	}
	e.pushScope()
	defer e.popScope()
	prefix := []ast.Stmt{}
	if setup != nil {
		var err error
		prefix, err = setup()
		if err != nil {
			return nil, false, err
		}
	}
	body, ok, err := e.lowerStatementsIntoTargetAST(block.Stmts, target, targetType)
	if err != nil || !ok {
		return nil, ok, err
	}
	return &ast.BlockStmt{List: append(prefix, body...)}, true, nil
}

func (e *emitter) lowerBoolMatchAssignAST(match *checker.BoolMatch, target ast.Expr, targetType checker.Type) ([]ast.Stmt, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	trueBody, ok, err := e.lowerMatchBranchIntoTargetAST(match.True, target, targetType, nil)
	if err != nil || !ok {
		return nil, ok, err
	}
	falseBody, ok, err := e.lowerMatchBranchIntoTargetAST(match.False, target, targetType, nil)
	if err != nil || !ok {
		return nil, ok, err
	}
	return []ast.Stmt{&ast.IfStmt{Cond: subject, Body: trueBody, Else: falseBody}}, true, nil
}

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

func (e *emitter) lowerOptionMatchAssignAST(match *checker.OptionMatch, target ast.Expr, targetType checker.Type) ([]ast.Stmt, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	if match.Some == nil || match.Some.Body == nil || match.None == nil {
		return nil, false, errStructuredLoweringUnsupported
	}
	maybeName := e.nextTemp("Maybe")
	someBody, ok, err := e.lowerMatchBranchIntoTargetAST(match.Some.Body, target, targetType, func() ([]ast.Stmt, error) {
		if match.Some.Pattern == nil || match.Some.Pattern.Name == "_" {
			return nil, nil
		}
		patternName := e.bindLocal(match.Some.Pattern.Name)
		bound := &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(maybeName), "Expect"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("unreachable none in maybe match")}}}
		prefix := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(patternName)}, Tok: token.DEFINE, Rhs: []ast.Expr{bound}}}
		if !usesNameInStatements(match.Some.Body.Stmts, match.Some.Pattern.Name) {
			prefix = append(prefix, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(patternName)}})
		}
		return prefix, nil
	})
	if err != nil || !ok {
		return nil, ok, err
	}
	noneBody, ok, err := e.lowerMatchBranchIntoTargetAST(match.None, target, targetType, nil)
	if err != nil || !ok {
		return nil, ok, err
	}
	return []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(maybeName)}, Tok: token.DEFINE, Rhs: []ast.Expr{subject}},
		&ast.IfStmt{Cond: &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(maybeName), "IsSome")}, Body: someBody, Else: noneBody},
	}, true, nil
}

func (e *emitter) lowerOptionMatchStatementAST(match *checker.OptionMatch) ([]ast.Stmt, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	if match.Some == nil || match.Some.Body == nil || match.None == nil {
		return nil, false, errStructuredLoweringUnsupported
	}
	maybeName := e.nextTemp("Maybe")
	someBody, ok, err := e.lowerMatchBranchAST(match.Some.Body, checker.Void, func() ([]ast.Stmt, error) {
		if match.Some.Pattern == nil || match.Some.Pattern.Name == "_" {
			return nil, nil
		}
		patternName := e.bindLocal(match.Some.Pattern.Name)
		bound := &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(maybeName), "Expect"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("unreachable none in maybe match")}}}
		prefix := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(patternName)}, Tok: token.DEFINE, Rhs: []ast.Expr{bound}}}
		if !usesNameInStatements(match.Some.Body.Stmts, match.Some.Pattern.Name) {
			prefix = append(prefix, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(patternName)}})
		}
		return prefix, nil
	})
	if err != nil || !ok {
		return nil, ok, err
	}
	noneBody, ok, err := e.lowerMatchBranchAST(match.None, checker.Void, nil)
	if err != nil || !ok {
		return nil, ok, err
	}
	return []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(maybeName)}, Tok: token.DEFINE, Rhs: []ast.Expr{subject}},
		&ast.IfStmt{Cond: &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(maybeName), "IsSome")}, Body: someBody, Else: noneBody},
	}, true, nil
}

func (e *emitter) lowerResultMatchAssignAST(match *checker.ResultMatch, target ast.Expr, targetType checker.Type) ([]ast.Stmt, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	if match.Ok == nil || match.Ok.Body == nil || match.Err == nil || match.Err.Body == nil {
		return nil, false, errStructuredLoweringUnsupported
	}
	resultName := e.nextTemp("Result")
	okBody, ok, err := e.lowerMatchBranchIntoTargetAST(match.Ok.Body, target, targetType, func() ([]ast.Stmt, error) {
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
	errBody, ok, err := e.lowerMatchBranchIntoTargetAST(match.Err.Body, target, targetType, func() ([]ast.Stmt, error) {
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

func (e *emitter) lowerUnionMatchAssignAST(match *checker.UnionMatch, target ast.Expr, targetType checker.Type) ([]ast.Stmt, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	subjectName := e.nextTemp("Union")
	typeSwitch := &ast.TypeSwitchStmt{Assign: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(subjectName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: &ast.CallExpr{Fun: ast.NewIdent("any"), Args: []ast.Expr{subject}}}}}, Body: &ast.BlockStmt{}}
	for _, caseName := range sortedStringKeys(match.TypeCases) {
		matchCase := match.TypeCases[caseName]
		if matchCase == nil || matchCase.Body == nil {
			continue
		}
		caseType := checker.Type(nil)
		for t := range match.TypeCasesByType {
			if t.String() == caseName {
				caseType = t
				break
			}
		}
		if caseType == nil {
			return nil, false, errStructuredLoweringUnsupported
		}
		typeExpr, err := e.lowerTypeArgExprWithOptions(caseType, e.typeParams, nil)
		if err != nil {
			return nil, false, err
		}
		body, ok, err := e.lowerMatchBranchIntoTargetAST(matchCase.Body, target, targetType, func() ([]ast.Stmt, error) {
			if matchCase.Pattern == nil || matchCase.Pattern.Name == "_" {
				return nil, nil
			}
			boundName := e.bindLocal(matchCase.Pattern.Name)
			prefix := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(boundName)}, Tok: token.DEFINE, Rhs: []ast.Expr{ast.NewIdent(subjectName)}}}
			if !usesNameInStatements(matchCase.Body.Stmts, matchCase.Pattern.Name) {
				prefix = append(prefix, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(boundName)}})
			}
			return prefix, nil
		})
		if err != nil || !ok {
			return nil, ok, err
		}
		typeSwitch.Body.List = append(typeSwitch.Body.List, &ast.CaseClause{List: []ast.Expr{typeExpr}, Body: body.List})
	}
	if match.CatchAll != nil {
		body, ok, err := e.lowerMatchBranchIntoTargetAST(match.CatchAll, target, targetType, nil)
		if err != nil || !ok {
			return nil, ok, err
		}
		typeSwitch.Body.List = append(typeSwitch.Body.List, &ast.CaseClause{Body: body.List})
	}
	return []ast.Stmt{typeSwitch}, true, nil
}
