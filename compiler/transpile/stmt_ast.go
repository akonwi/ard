package transpile

import (
	"go/ast"
	"go/token"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) lowerStatementsBlockAST(stmts []checker.Statement, returnType checker.Type) (*ast.BlockStmt, bool, error) {
	e.pushScope()
	defer e.popScope()
	lastMeaningful := lastMeaningfulStatementIndex(stmts)
	list := make([]ast.Stmt, 0, len(stmts))
	for i, stmt := range stmts {
		if stmt.Break {
			list = append(list, &ast.BranchStmt{Tok: token.BREAK})
			continue
		}
		isLastExpr := i == lastMeaningful && stmt.Expr != nil
		remaining := stmts[i+1:]
		if stmt.Stmt != nil {
			nodes, ok, err := e.lowerNonProducingAST(stmt.Stmt, remaining, returnType)
			if err != nil || !ok {
				return nil, ok, err
			}
			list = append(list, nodes...)
			continue
		}
		if stmt.Expr == nil {
			continue
		}
		nodes, ok, err := e.lowerExpressionStatementAST(stmt.Expr, returnType, isLastExpr)
		if err != nil || !ok {
			return nil, ok, err
		}
		list = append(list, nodes...)
	}
	return &ast.BlockStmt{List: list}, true, nil
}

func (e *emitter) lowerNonProducingAST(stmt checker.NonProducing, remaining []checker.Statement, returnType checker.Type) ([]ast.Stmt, bool, error) {
	switch s := stmt.(type) {
	case *checker.VariableDef:
		name := e.bindLocal(s.Name)
		if tryOp, ok := s.Value.(*checker.TryOp); ok {
			stmts, ok, err := e.lowerTryOpAST(tryOp, returnType, func(successValue ast.Expr) ([]ast.Stmt, error) {
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
		value, ok, err := e.lowerExprAST(s.Value)
		if err != nil || !ok {
			return nil, ok, err
		}
		lhs := ast.NewIdent(name)
		if typeNeedsExplicitVarAnnotation(s.Type()) {
			typeExpr, err := e.lowerTypeExpr(s.Type())
			if err != nil {
				return nil, false, err
			}
			stmts := []ast.Stmt{&ast.DeclStmt{Decl: astVarDecl(astValueSpec(name, typeExpr, value))}}
			if !usesNameInStatements(remaining, s.Name) {
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{lhs}})
			}
			return stmts, true, nil
		}
		stmts := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.DEFINE, Rhs: []ast.Expr{value}}}
		if !usesNameInStatements(remaining, s.Name) {
			stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{lhs}})
		}
		return stmts, true, nil
	case *checker.Reassignment:
		target, ok, err := e.lowerAssignmentTargetAST(s.Target)
		if err != nil || !ok {
			return nil, ok, err
		}
		if tryOp, ok := s.Value.(*checker.TryOp); ok {
			return e.lowerTryOpAST(tryOp, returnType, func(successValue ast.Expr) ([]ast.Stmt, error) {
				return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{successValue}}}, nil
			})
		}
		value, ok, err := e.lowerExprAST(s.Value)
		if err != nil || !ok {
			return nil, ok, err
		}
		return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{value}}}, true, nil
	case *checker.WhileLoop:
		cond, ok, err := e.lowerExprAST(s.Condition)
		if err != nil || !ok {
			return nil, ok, err
		}
		body, ok, err := e.lowerStatementsBlockAST(s.Body.Stmts, checker.Void)
		if err != nil || !ok {
			return nil, ok, err
		}
		return []ast.Stmt{&ast.ForStmt{Cond: cond, Body: body}}, true, nil
	case *checker.ForLoop:
		if s.Init == nil || s.Update == nil {
			return nil, false, nil
		}
		e.pushScope()
		defer e.popScope()
		initName := e.bindLocal(s.Init.Name)
		initValue, ok, err := e.lowerExprAST(s.Init.Value)
		if err != nil || !ok {
			return nil, ok, err
		}
		cond, ok, err := e.lowerExprAST(s.Condition)
		if err != nil || !ok {
			return nil, ok, err
		}
		updateTarget, ok, err := e.lowerAssignmentTargetAST(s.Update.Target)
		if err != nil || !ok {
			return nil, ok, err
		}
		updateValue, ok, err := e.lowerExprAST(s.Update.Value)
		if err != nil || !ok {
			return nil, ok, err
		}
		body, ok, err := e.lowerStatementsBlockAST(s.Body.Stmts, checker.Void)
		if err != nil || !ok {
			return nil, ok, err
		}
		return []ast.Stmt{&ast.ForStmt{Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(initName)}, Tok: token.DEFINE, Rhs: []ast.Expr{initValue}}, Cond: cond, Post: &ast.AssignStmt{Lhs: []ast.Expr{updateTarget}, Tok: token.ASSIGN, Rhs: []ast.Expr{updateValue}}, Body: body}}, true, nil
	case *checker.ForIntRange:
		start, ok, err := e.lowerExprAST(s.Start)
		if err != nil || !ok {
			return nil, ok, err
		}
		end, ok, err := e.lowerExprAST(s.End)
		if err != nil || !ok {
			return nil, ok, err
		}
		e.pushScope()
		defer e.popScope()
		cursor := e.bindLocal(s.Cursor)
		var init ast.Stmt
		var post ast.Stmt
		if s.Index == "" {
			init = &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(cursor)}, Tok: token.DEFINE, Rhs: []ast.Expr{start}}
			post = &ast.IncDecStmt{X: ast.NewIdent(cursor), Tok: token.INC}
		} else {
			index := e.bindLocal(s.Index)
			init = &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(cursor), ast.NewIdent(index)}, Tok: token.DEFINE, Rhs: []ast.Expr{start, &ast.BasicLit{Kind: token.INT, Value: "0"}}}
			post = &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(cursor), ast.NewIdent(index)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.BinaryExpr{X: ast.NewIdent(cursor), Op: token.ADD, Y: &ast.BasicLit{Kind: token.INT, Value: "1"}}, &ast.BinaryExpr{X: ast.NewIdent(index), Op: token.ADD, Y: &ast.BasicLit{Kind: token.INT, Value: "1"}}}}
		}
		body, ok, err := e.lowerStatementsBlockAST(s.Body.Stmts, checker.Void)
		if err != nil || !ok {
			return nil, ok, err
		}
		return []ast.Stmt{&ast.ForStmt{Init: init, Cond: &ast.BinaryExpr{X: ast.NewIdent(cursor), Op: token.LEQ, Y: end}, Post: post, Body: body}}, true, nil
	case *checker.ForInStr:
		value, ok, err := e.lowerExprAST(s.Value)
		if err != nil || !ok {
			return nil, ok, err
		}
		e.pushScope()
		defer e.popScope()
		cursor := e.bindLocal(s.Cursor)
		key := ast.Expr(ast.NewIdent("_"))
		if s.Index != "" {
			key = ast.NewIdent(e.bindLocal(s.Index))
		}
		body, ok, err := e.lowerStatementsBlockAST(s.Body.Stmts, checker.Void)
		if err != nil || !ok {
			return nil, ok, err
		}
		body.List = append([]ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(cursor)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("string"), Args: []ast.Expr{ast.NewIdent("__ardRune")}}}}}, body.List...)
		return []ast.Stmt{&ast.RangeStmt{Key: key, Value: ast.NewIdent("__ardRune"), Tok: token.DEFINE, X: &ast.CallExpr{Fun: &ast.ArrayType{Elt: ast.NewIdent("rune")}, Args: []ast.Expr{value}}, Body: body}}, true, nil
	case *checker.ForInList:
		listExpr, ok, err := e.lowerExprAST(s.List)
		if err != nil || !ok {
			return nil, ok, err
		}
		e.pushScope()
		defer e.popScope()
		cursor := ast.NewIdent(e.bindLocal(s.Cursor))
		key := ast.Expr(ast.NewIdent("_"))
		if s.Index != "" {
			key = ast.NewIdent(e.bindLocal(s.Index))
		}
		body, ok, err := e.lowerStatementsBlockAST(s.Body.Stmts, checker.Void)
		if err != nil || !ok {
			return nil, ok, err
		}
		return []ast.Stmt{&ast.RangeStmt{Key: key, Value: cursor, Tok: token.DEFINE, X: listExpr, Body: body}}, true, nil
	case *checker.ForInMap:
		mapExpr, ok, err := e.lowerExprAST(s.Map)
		if err != nil || !ok {
			return nil, ok, err
		}
		e.pushScope()
		defer e.popScope()
		mapName := e.nextTemp("Map")
		key := e.bindLocal(s.Key)
		val := e.bindLocal(s.Val)
		body, ok, err := e.lowerStatementsBlockAST(s.Body.Stmts, checker.Void)
		if err != nil || !ok {
			return nil, ok, err
		}
		body.List = append([]ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(val)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.IndexExpr{X: ast.NewIdent(mapName), Index: ast.NewIdent(key)}}}}, body.List...)
		return []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(mapName)}, Tok: token.DEFINE, Rhs: []ast.Expr{mapExpr}},
			&ast.RangeStmt{Key: ast.NewIdent("_"), Value: ast.NewIdent(key), Tok: token.DEFINE, X: &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "MapKeys"), Args: []ast.Expr{ast.NewIdent(mapName)}}, Body: body},
		}, true, nil
	default:
		return nil, false, nil
	}
}

func (e *emitter) lowerExpressionStatementAST(expr checker.Expression, returnType checker.Type, isLast bool) ([]ast.Stmt, bool, error) {
	if tryOp, ok := expr.(*checker.TryOp); ok {
		var onSuccess func(ast.Expr) ([]ast.Stmt, error)
		if isLast && returnType != nil && returnType != checker.Void {
			onSuccess = func(successValue ast.Expr) ([]ast.Stmt, error) {
				return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{successValue}}}, nil
			}
		}
		return e.lowerTryOpAST(tryOp, returnType, onSuccess)
	}
	if panicExpr, ok := expr.(*checker.Panic); ok {
		msg, ok2, err := e.lowerExprAST(panicExpr.Message)
		if err != nil || !ok2 {
			return nil, ok2, err
		}
		return []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{msg}}}}, true, nil
	}
	if ifExpr, ok := expr.(*checker.If); ok {
		stmt, ok, err := e.lowerIfStatementAST(ifExpr, returnType, isLast)
		if err != nil || !ok {
			return nil, ok, err
		}
		return []ast.Stmt{stmt}, true, nil
	}
	value, ok, err := e.lowerExprAST(expr)
	if err != nil || !ok {
		return nil, ok, err
	}
	if isLast && returnType != nil && returnType != checker.Void {
		return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{value}}}, true, nil
	}
	if _, ok := value.(*ast.CallExpr); ok {
		return []ast.Stmt{&ast.ExprStmt{X: value}}, true, nil
	}
	if expr.Type() == checker.Void {
		return []ast.Stmt{&ast.ExprStmt{X: value}}, true, nil
	}
	return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{value}}}, true, nil
}

func (e *emitter) lowerIfStatementAST(expr *checker.If, returnType checker.Type, isLast bool) (ast.Stmt, bool, error) {
	cond, ok, err := e.lowerExprAST(expr.Condition)
	if err != nil || !ok {
		return nil, ok, err
	}
	thenBody, ok, err := e.lowerStatementsBlockAST(expr.Body.Stmts, returnType)
	if err != nil || !ok {
		return nil, ok, err
	}
	stmt := &ast.IfStmt{Cond: cond, Body: thenBody}
	if expr.ElseIf != nil {
		elseStmt, ok, err := e.lowerIfStatementAST(withElseFallback(expr.ElseIf, expr.Else), returnType, isLast)
		if err != nil || !ok {
			return nil, ok, err
		}
		stmt.Else = elseStmt
		return stmt, true, nil
	}
	if expr.Else != nil {
		elseBody, ok, err := e.lowerStatementsBlockAST(expr.Else.Stmts, returnType)
		if err != nil || !ok {
			return nil, ok, err
		}
		stmt.Else = elseBody
	} else if isLast && returnType != nil && returnType != checker.Void {
		return nil, false, nil
	}
	return stmt, true, nil
}
