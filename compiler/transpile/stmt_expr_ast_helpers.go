package transpile

import (
	"go/ast"
	"go/token"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) lowerSpecialModuleCallAST(call *checker.ModuleFunctionCall) (ast.Expr, bool, error) {
	if call == nil || call.Call == nil {
		return nil, false, nil
	}
	switch call.Module {
	case "ard/maybe":
		return e.lowerMaybeModuleCallAST(call)
	case "ard/result":
		return e.lowerResultModuleCallAST(call)
	case "ard/list":
		return e.lowerListModuleCallAST(call)
	case "ard/async":
		return e.lowerAsyncModuleCallAST(call)
	default:
		return nil, false, nil
	}
}

func (e *emitter) lowerMaybeModuleCallAST(call *checker.ModuleFunctionCall) (ast.Expr, bool, error) {
	switch call.Call.Name {
	case "some":
		if len(call.Call.Args) != 1 {
			return nil, false, errStructuredLoweringUnsupported
		}
		arg, ok, err := e.lowerExprAST(call.Call.Args[0])
		if err != nil || !ok {
			return nil, ok, err
		}
		resultType, ok2 := call.Call.ReturnType.(*checker.Maybe)
		if !ok2 {
			return nil, false, errStructuredLoweringUnsupported
		}
		inner, err := e.lowerTypeArgExprWithOptions(resultType.Of(), e.typeParams, nil)
		if err != nil {
			return nil, false, err
		}
		return astCall(selectorExpr(ast.NewIdent(helperImportAlias), "Some"), []ast.Expr{inner}, []ast.Expr{arg}), true, nil
	case "none":
		resultType, ok2 := call.Call.ReturnType.(*checker.Maybe)
		if !ok2 {
			return nil, false, errStructuredLoweringUnsupported
		}
		inner, err := e.lowerTypeArgExprWithOptions(resultType.Of(), e.typeParams, nil)
		if err != nil {
			return nil, false, err
		}
		return astCall(selectorExpr(ast.NewIdent(helperImportAlias), "None"), []ast.Expr{inner}, nil), true, nil
	default:
		return nil, false, nil
	}
}

func (e *emitter) lowerResultModuleCallAST(call *checker.ModuleFunctionCall) (ast.Expr, bool, error) {
	switch call.Call.Name {
	case "ok":
		if len(call.Call.Args) != 1 {
			return nil, false, errStructuredLoweringUnsupported
		}
		resultType, ok := call.Call.ReturnType.(*checker.Result)
		if !ok {
			return nil, false, errStructuredLoweringUnsupported
		}
		arg, ok2, err := e.lowerExprAST(call.Call.Args[0])
		if err != nil || !ok2 {
			return nil, ok2, err
		}
		valType, err := e.lowerTypeArgExprWithOptions(resultType.Val(), e.typeParams, nil)
		if err != nil {
			return nil, false, err
		}
		errType, err := e.lowerTypeArgExprWithOptions(resultType.Err(), e.typeParams, nil)
		if err != nil {
			return nil, false, err
		}
		return astCall(selectorExpr(ast.NewIdent(helperImportAlias), "Ok"), []ast.Expr{valType, errType}, []ast.Expr{arg}), true, nil
	case "err":
		if len(call.Call.Args) != 1 {
			return nil, false, errStructuredLoweringUnsupported
		}
		resultType, ok := call.Call.ReturnType.(*checker.Result)
		if !ok {
			return nil, false, errStructuredLoweringUnsupported
		}
		arg, ok2, err := e.lowerExprAST(call.Call.Args[0])
		if err != nil || !ok2 {
			return nil, ok2, err
		}
		valType, err := e.lowerTypeArgExprWithOptions(resultType.Val(), e.typeParams, nil)
		if err != nil {
			return nil, false, err
		}
		errType, err := e.lowerTypeArgExprWithOptions(resultType.Err(), e.typeParams, nil)
		if err != nil {
			return nil, false, err
		}
		return astCall(selectorExpr(ast.NewIdent(helperImportAlias), "Err"), []ast.Expr{valType, errType}, []ast.Expr{arg}), true, nil
	default:
		return nil, false, nil
	}
}

func (e *emitter) lowerListModuleCallAST(call *checker.ModuleFunctionCall) (ast.Expr, bool, error) {
	if call == nil || call.Call == nil {
		return nil, false, nil
	}
	switch call.Call.Name {
	case "concat":
		if len(call.Call.Args) != 2 {
			return nil, false, errStructuredLoweringUnsupported
		}
		left, ok, err := e.lowerExprAST(call.Call.Args[0])
		if err != nil || !ok {
			return nil, ok, err
		}
		right, ok, err := e.lowerExprAST(call.Call.Args[1])
		if err != nil || !ok {
			return nil, ok, err
		}
		typeExpr, err := e.lowerTypeExpr(call.Call.ReturnType)
		if err != nil {
			return nil, false, err
		}
		outIdent := ast.NewIdent("out")
		body := []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{outIdent}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{&ast.CallExpr{Fun: typeExpr, Args: []ast.Expr{ast.NewIdent("nil")}}, left}, Ellipsis: token.Pos(1)}}},
			&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{outIdent, right}, Ellipsis: token.Pos(1)}}},
		}
		return &ast.CallExpr{Fun: &ast.FuncLit{Type: &ast.FuncType{Results: funcResults(typeExpr)}, Body: &ast.BlockStmt{List: body}}}, true, nil
	default:
		return nil, false, nil
	}
}

func (e *emitter) lowerMaybeMethodAST(method *checker.MaybeMethod) (ast.Expr, bool, error) {
	subject, ok, err := e.lowerExprAST(method.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	arg := func(i int) (ast.Expr, bool, error) { return e.lowerExprAST(method.Args[i]) }
	switch method.Kind {
	case checker.MaybeExpect:
		m, ok, err := arg(0)
		return &ast.CallExpr{Fun: selectorExpr(subject, "Expect"), Args: []ast.Expr{m}}, ok, err
	case checker.MaybeIsNone:
		return &ast.CallExpr{Fun: selectorExpr(subject, "IsNone")}, true, nil
	case checker.MaybeIsSome:
		return &ast.CallExpr{Fun: selectorExpr(subject, "IsSome")}, true, nil
	case checker.MaybeOr:
		f, ok, err := arg(0)
		return &ast.CallExpr{Fun: selectorExpr(subject, "Or"), Args: []ast.Expr{f}}, ok, err
	case checker.MaybeMap:
		m, ok, err := arg(0)
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "MaybeMap"), Args: []ast.Expr{subject, m}}, ok, err
	case checker.MaybeAndThen:
		m, ok, err := arg(0)
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "MaybeAndThen"), Args: []ast.Expr{subject, m}}, ok, err
	default:
		return nil, false, nil
	}
}

func (e *emitter) lowerResultMethodAST(method *checker.ResultMethod) (ast.Expr, bool, error) {
	subject, ok, err := e.lowerExprAST(method.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	arg := func(i int) (ast.Expr, bool, error) { return e.lowerExprAST(method.Args[i]) }
	switch method.Kind {
	case checker.ResultExpect:
		m, ok, err := arg(0)
		return &ast.CallExpr{Fun: selectorExpr(subject, "Expect"), Args: []ast.Expr{m}}, ok, err
	case checker.ResultOr:
		f, ok, err := arg(0)
		return &ast.CallExpr{Fun: selectorExpr(subject, "Or"), Args: []ast.Expr{f}}, ok, err
	case checker.ResultIsOk:
		return &ast.CallExpr{Fun: selectorExpr(subject, "IsOk")}, true, nil
	case checker.ResultIsErr:
		return &ast.CallExpr{Fun: selectorExpr(subject, "IsErr")}, true, nil
	case checker.ResultMap:
		m, ok, err := arg(0)
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "ResultMap"), Args: []ast.Expr{subject, m}}, ok, err
	case checker.ResultMapErr:
		m, ok, err := arg(0)
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "ResultMapErr"), Args: []ast.Expr{subject, m}}, ok, err
	case checker.ResultAndThen:
		m, ok, err := arg(0)
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "ResultAndThen"), Args: []ast.Expr{subject, m}}, ok, err
	default:
		return nil, false, nil
	}
}

func (e *emitter) lowerListMethodAST(method *checker.ListMethod) (ast.Expr, bool, error) {
	subject, ok, err := e.lowerExprAST(method.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	arg := func(i int) (ast.Expr, bool, error) {
		if i >= len(method.Args) {
			return nil, false, errStructuredLoweringUnsupported
		}
		return e.lowerExprAST(method.Args[i])
	}
	switch method.Kind {
	case checker.ListSize:
		return &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{subject}}, true, nil
	case checker.ListAt:
		idx, ok, err := arg(0)
		if err != nil || !ok {
			return nil, ok, err
		}
		return &ast.IndexExpr{X: subject, Index: idx}, true, nil
	case checker.ListPush, checker.ListPrepend, checker.ListSet, checker.ListSort, checker.ListSwap:
		target, ok, err := e.lowerAssignmentTargetAST(method.Subject)
		if err != nil || !ok {
			return nil, ok, err
		}
		subjectType, ok := method.Subject.Type().(*checker.List)
		if !ok {
			return nil, false, errStructuredLoweringUnsupported
		}
		switch method.Kind {
		case checker.ListPush:
			value, ok, err := arg(0)
			if err != nil || !ok {
				return nil, ok, err
			}
			out, err := e.inlineFuncCallAST(method.Type(), []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{target},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{target, value}}},
				},
				&ast.ReturnStmt{Results: []ast.Expr{target}},
			})
			return out, err == nil, err
		case checker.ListPrepend:
			value, ok, err := arg(0)
			if err != nil || !ok {
				return nil, ok, err
			}
			typeExpr, err := e.lowerTypeExpr(subjectType)
			if err != nil {
				return nil, false, err
			}
			out, err := e.inlineFuncCallAST(method.Type(), []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{target},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{&ast.CallExpr{
						Fun:      ast.NewIdent("append"),
						Args:     []ast.Expr{&ast.CompositeLit{Type: typeExpr, Elts: []ast.Expr{value}}, target},
						Ellipsis: token.Pos(1),
					}},
				},
				&ast.ReturnStmt{Results: []ast.Expr{target}},
			})
			return out, err == nil, err
		case checker.ListSet:
			index, ok, err := arg(0)
			if err != nil || !ok {
				return nil, ok, err
			}
			value, ok, err := arg(1)
			if err != nil || !ok {
				return nil, ok, err
			}
			out, err := e.inlineFuncCallAST(method.Type(), []ast.Stmt{
				&ast.IfStmt{
					Cond: &ast.BinaryExpr{
						X:  &ast.BinaryExpr{X: index, Op: token.GEQ, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}},
						Op: token.LAND,
						Y:  &ast.BinaryExpr{X: index, Op: token.LSS, Y: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{target}}},
					},
					Body: &ast.BlockStmt{List: []ast.Stmt{
						&ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: target, Index: index}}, Tok: token.ASSIGN, Rhs: []ast.Expr{value}},
						&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("true")}},
					}},
					Else: &ast.BlockStmt{List: []ast.Stmt{
						&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("false")}},
					}},
				},
			})
			return out, err == nil, err
		case checker.ListSort:
			cmp, ok, err := arg(0)
			if err != nil || !ok {
				return nil, ok, err
			}
			out, err := e.inlineFuncCallAST(method.Type(), []ast.Stmt{
				&ast.ExprStmt{X: &ast.CallExpr{Fun: selectorExpr(ast.NewIdent("sort"), "SliceStable"), Args: []ast.Expr{
					target,
					&ast.FuncLit{
						Type: &ast.FuncType{
							Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("i")}, Type: ast.NewIdent("int")}, {Names: []*ast.Ident{ast.NewIdent("j")}, Type: ast.NewIdent("int")}}},
							Results: funcResults(ast.NewIdent("bool")),
						},
						Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: cmp, Args: []ast.Expr{&ast.IndexExpr{X: target, Index: ast.NewIdent("i")}, &ast.IndexExpr{X: target, Index: ast.NewIdent("j")}}}}}}},
					},
				}}},
			})
			return out, err == nil, err
		case checker.ListSwap:
			left, ok, err := arg(0)
			if err != nil || !ok {
				return nil, ok, err
			}
			right, ok, err := arg(1)
			if err != nil || !ok {
				return nil, ok, err
			}
			out, err := e.inlineFuncCallAST(method.Type(), []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: target, Index: left}, &ast.IndexExpr{X: target, Index: right}}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.IndexExpr{X: target, Index: right}, &ast.IndexExpr{X: target, Index: left}}},
			})
			return out, err == nil, err
		}
	}
	return nil, false, nil
}

func (e *emitter) lowerMapMethodAST(method *checker.MapMethod) (ast.Expr, bool, error) {
	subject, ok, err := e.lowerExprAST(method.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	arg := func(i int) (ast.Expr, bool, error) {
		if i >= len(method.Args) {
			return nil, false, errStructuredLoweringUnsupported
		}
		return e.lowerExprAST(method.Args[i])
	}
	switch method.Kind {
	case checker.MapSize:
		return &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{subject}}, true, nil
	case checker.MapKeys:
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "MapKeys"), Args: []ast.Expr{subject}}, true, nil
	case checker.MapHas:
		key, ok, err := arg(0)
		if err != nil || !ok {
			return nil, ok, err
		}
		body := []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("ok")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.IndexExpr{X: subject, Index: key}}},
			&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("ok")}},
		}
		out, err := e.inlineFuncCallAST(method.Type(), body)
		return out, err == nil, err
	case checker.MapGet:
		key, ok, err := arg(0)
		if err != nil || !ok {
			return nil, ok, err
		}
		maybeType, ok2 := method.Type().(*checker.Maybe)
		if !ok2 {
			return nil, false, errStructuredLoweringUnsupported
		}
		resultType, err := e.lowerTypeExpr(method.Type())
		if err != nil {
			return nil, false, err
		}
		innerType, err := e.lowerTypeArgExprWithOptions(maybeType.Of(), e.typeParams, nil)
		if err != nil {
			return nil, false, err
		}
		body := []ast.Stmt{
			&ast.IfStmt{Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("value"), ast.NewIdent("ok")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.IndexExpr{X: subject, Index: key}}}, Cond: ast.NewIdent("ok"), Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{astCall(selectorExpr(ast.NewIdent(helperImportAlias), "Some"), []ast.Expr{innerType}, []ast.Expr{ast.NewIdent("value")})}}}}, Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{astCall(selectorExpr(ast.NewIdent(helperImportAlias), "None"), []ast.Expr{innerType}, nil)}}}}},
		}
		return &ast.CallExpr{Fun: &ast.FuncLit{Type: &ast.FuncType{Results: funcResults(resultType)}, Body: &ast.BlockStmt{List: body}}}, true, nil
	case checker.MapSet, checker.MapDrop:
		target, ok, err := e.lowerAssignmentTargetAST(method.Subject)
		if err != nil || !ok {
			return nil, ok, err
		}
		switch method.Kind {
		case checker.MapSet:
			key, ok, err := arg(0)
			if err != nil || !ok {
				return nil, ok, err
			}
			value, ok, err := arg(1)
			if err != nil || !ok {
				return nil, ok, err
			}
			out, err := e.inlineFuncCallAST(method.Type(), []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: target, Index: key}}, Tok: token.ASSIGN, Rhs: []ast.Expr{value}}, &ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("true")}}})
			return out, err == nil, err
		case checker.MapDrop:
			key, ok, err := arg(0)
			if err != nil || !ok {
				return nil, ok, err
			}
			out, err := e.inlineFuncCallAST(method.Type(), []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("delete"), Args: []ast.Expr{target, key}}}})
			return out, err == nil, err
		}
	}
	return nil, false, nil
}

func (e *emitter) lowerFunctionLiteralAST(def *checker.FunctionDef) (ast.Expr, bool, error) {
	returnType := effectiveFunctionReturnType(def)
	inner := &emitter{
		module:          e.module,
		packageName:     e.packageName,
		projectName:     e.projectName,
		entrypoint:      e.entrypoint,
		imports:         e.imports,
		functionNames:   e.functionNames,
		emittedTypes:    e.emittedTypes,
		tempCounter:     e.tempCounter,
		fnReturnType:    returnType,
		localScopes:     cloneLocalScopes(e.localScopes),
		pointerScopes:   clonePointerScopes(e.pointerScopes),
		localNameCounts: cloneLocalNameCounts(e.localNameCounts),
		typeParams:      cloneTypeParams(e.typeParams),
	}
	inner.pushScope()
	params, err := inner.lowerBoundFunctionParamFields(def.Parameters)
	if err != nil {
		return nil, false, err
	}
	body, err := inner.lowerFunctionBodyBlock(def.Body.Stmts, returnType)
	if err != nil {
		return nil, false, err
	}
	e.tempCounter = inner.tempCounter
	funcType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if returnType != checker.Void {
		resultType, err := inner.lowerTypeExpr(returnType)
		if err != nil {
			return nil, false, err
		}
		funcType.Results = funcResults(resultType)
	}
	return &ast.FuncLit{Type: funcType, Body: body}, true, nil
}
