package transpile

import (
	"go/ast"
	"go/token"
	"sort"
	"strconv"

	"github.com/akonwi/ard/checker"
)

func panicStmt(message string) ast.Stmt {
	return &ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(message)}}}}
}

func (e *emitter) lowerMatchBranchAST(block *checker.Block, returnType checker.Type, setup func() ([]ast.Stmt, error)) (*ast.BlockStmt, bool, error) {
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
	body, ok, err := e.lowerStatementsBlockAST(block.Stmts, returnType)
	if err != nil || !ok {
		return nil, ok, err
	}
	return &ast.BlockStmt{List: append(prefix, body.List...)}, true, nil
}

func (e *emitter) lowerBoolMatchAST(match *checker.BoolMatch) (ast.Expr, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	returnType := match.Type()
	trueBody, ok, err := e.lowerMatchBranchAST(match.True, returnType, nil)
	if err != nil || !ok {
		return nil, ok, err
	}
	falseBody, ok, err := e.lowerMatchBranchAST(match.False, returnType, nil)
	if err != nil || !ok {
		return nil, ok, err
	}
	out, err := e.inlineFuncCallAST(returnType, []ast.Stmt{&ast.IfStmt{Cond: subject, Body: trueBody, Else: falseBody}})
	return out, err == nil, err
}

func (e *emitter) lowerIntMatchAST(match *checker.IntMatch) (ast.Expr, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	returnType := match.Type()
	switchStmt := &ast.SwitchStmt{}
	for _, value := range sortedIntKeys(match.IntCases) {
		body, ok, err := e.lowerMatchBranchAST(match.IntCases[value], returnType, nil)
		if err != nil || !ok {
			return nil, ok, err
		}
		switchStmt.Body.List = append(switchStmt.Body.List, &ast.CaseClause{List: []ast.Expr{&ast.BinaryExpr{X: subject, Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(value)}}}, Body: body.List})
	}
	for _, intRange := range sortedIntRanges(match.RangeCases) {
		body, ok, err := e.lowerMatchBranchAST(match.RangeCases[intRange], returnType, nil)
		if err != nil || !ok {
			return nil, ok, err
		}
		cond := &ast.BinaryExpr{X: &ast.BinaryExpr{X: subject, Op: token.GEQ, Y: &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(intRange.Start)}}, Op: token.LAND, Y: &ast.BinaryExpr{X: subject, Op: token.LEQ, Y: &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(intRange.End)}}}
		switchStmt.Body.List = append(switchStmt.Body.List, &ast.CaseClause{List: []ast.Expr{cond}, Body: body.List})
	}
	if match.CatchAll != nil {
		body, ok, err := e.lowerMatchBranchAST(match.CatchAll, returnType, nil)
		if err != nil || !ok {
			return nil, ok, err
		}
		switchStmt.Body.List = append(switchStmt.Body.List, &ast.CaseClause{Body: body.List})
	} else if returnType != checker.Void {
		switchStmt.Body.List = append(switchStmt.Body.List, &ast.CaseClause{Body: []ast.Stmt{panicStmt("non-exhaustive int match")}})
	}
	out, err := e.inlineFuncCallAST(returnType, []ast.Stmt{switchStmt})
	return out, err == nil, err
}

func (e *emitter) lowerConditionalMatchAST(match *checker.ConditionalMatch) (ast.Expr, bool, error) {
	returnType := match.Type()
	if len(match.Cases) == 0 {
		body := []ast.Stmt{}
		if match.CatchAll != nil {
			block, ok, err := e.lowerMatchBranchAST(match.CatchAll, returnType, nil)
			if err != nil || !ok {
				return nil, ok, err
			}
			body = append(body, block.List...)
		} else if returnType != checker.Void {
			body = append(body, panicStmt("non-exhaustive conditional match"))
		}
		out, err := e.inlineFuncCallAST(returnType, body)
		return out, err == nil, err
	}
	var root *ast.IfStmt
	var current *ast.IfStmt
	for i, matchCase := range match.Cases {
		cond, ok, err := e.lowerExprAST(matchCase.Condition)
		if err != nil || !ok {
			return nil, ok, err
		}
		body, ok, err := e.lowerMatchBranchAST(matchCase.Body, returnType, nil)
		if err != nil || !ok {
			return nil, ok, err
		}
		stmt := &ast.IfStmt{Cond: cond, Body: body}
		if i == 0 {
			root = stmt
		} else {
			current.Else = stmt
		}
		current = stmt
	}
	if match.CatchAll != nil {
		body, ok, err := e.lowerMatchBranchAST(match.CatchAll, returnType, nil)
		if err != nil || !ok {
			return nil, ok, err
		}
		current.Else = body
	} else if returnType != checker.Void {
		current.Else = &ast.BlockStmt{List: []ast.Stmt{panicStmt("non-exhaustive conditional match")}}
	}
	out, err := e.inlineFuncCallAST(returnType, []ast.Stmt{root})
	return out, err == nil, err
}

func (e *emitter) lowerOptionMatchAST(match *checker.OptionMatch) (ast.Expr, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	returnType := match.Type()
	if match.Some == nil || match.Some.Body == nil || match.None == nil {
		return nil, false, errStructuredLoweringUnsupported
	}
	maybeName := e.nextTemp("Maybe")
	someBody, ok, err := e.lowerMatchBranchAST(match.Some.Body, returnType, func() ([]ast.Stmt, error) {
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
	noneBody, ok, err := e.lowerMatchBranchAST(match.None, returnType, nil)
	if err != nil || !ok {
		return nil, ok, err
	}
	out, err := e.inlineFuncCallAST(returnType, []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(maybeName)}, Tok: token.DEFINE, Rhs: []ast.Expr{subject}},
		&ast.IfStmt{Cond: &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(maybeName), "IsSome")}, Body: someBody, Else: noneBody},
	})
	return out, err == nil, err
}

func (e *emitter) lowerResultMatchAST(match *checker.ResultMatch) (ast.Expr, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	if match.Ok == nil || match.Ok.Body == nil || match.Err == nil || match.Err.Body == nil {
		return nil, false, errStructuredLoweringUnsupported
	}
	returnType := match.Type()
	resultName := e.nextTemp("Result")
	okBody, ok, err := e.lowerMatchBranchAST(match.Ok.Body, returnType, func() ([]ast.Stmt, error) {
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
	errBody, ok, err := e.lowerMatchBranchAST(match.Err.Body, returnType, func() ([]ast.Stmt, error) {
		if match.Err.Pattern == nil || match.Err.Pattern.Name == "_" {
			return nil, nil
		}
		patternName := e.bindLocal(match.Err.Pattern.Name)
		bound := &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(resultName), "UnwrapErr")}
		prefix := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(patternName)}, Tok: token.DEFINE, Rhs: []ast.Expr{bound}}}
		if !usesNameInStatements(match.Err.Body.Stmts, match.Err.Pattern.Name) {
			prefix = append(prefix, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(patternName)}})
		}
		return prefix, nil
	})
	if err != nil || !ok {
		return nil, ok, err
	}
	out, err := e.inlineFuncCallAST(returnType, []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultName)}, Tok: token.DEFINE, Rhs: []ast.Expr{subject}},
		&ast.IfStmt{Cond: &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(resultName), "IsOk")}, Body: okBody, Else: errBody},
	})
	return out, err == nil, err
}

func (e *emitter) lowerEnumMatchAST(match *checker.EnumMatch) (ast.Expr, bool, error) {
	subject, ok, err := e.lowerExprAST(match.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	returnType := match.Type()
	switchStmt := &ast.SwitchStmt{Tag: selectorExpr(subject, "Tag")}
	discriminants := make([]int, 0, len(match.DiscriminantToIndex))
	for discriminant := range match.DiscriminantToIndex {
		discriminants = append(discriminants, discriminant)
	}
	sort.Ints(discriminants)
	for _, discriminant := range discriminants {
		idx := match.DiscriminantToIndex[discriminant]
		if idx < 0 || int(idx) >= len(match.Cases) || match.Cases[idx] == nil {
			continue
		}
		body, ok, err := e.lowerMatchBranchAST(match.Cases[idx], returnType, nil)
		if err != nil || !ok {
			return nil, ok, err
		}
		switchStmt.Body.List = append(switchStmt.Body.List, &ast.CaseClause{List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(discriminant)}}, Body: body.List})
	}
	if match.CatchAll != nil {
		body, ok, err := e.lowerMatchBranchAST(match.CatchAll, returnType, nil)
		if err != nil || !ok {
			return nil, ok, err
		}
		switchStmt.Body.List = append(switchStmt.Body.List, &ast.CaseClause{Body: body.List})
	} else if returnType != checker.Void {
		switchStmt.Body.List = append(switchStmt.Body.List, &ast.CaseClause{Body: []ast.Stmt{panicStmt("non-exhaustive enum match")}})
	}
	out, err := e.inlineFuncCallAST(returnType, []ast.Stmt{switchStmt})
	return out, err == nil, err
}
