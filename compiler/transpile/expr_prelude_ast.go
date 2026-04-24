package transpile

import (
	"errors"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/akonwi/ard/checker"
)

func appendPrelude(dst []ast.Stmt, parts ...[]ast.Stmt) []ast.Stmt {
	for _, part := range parts {
		dst = append(dst, part...)
	}
	return dst
}

func specialModuleCallArgTypes(call *checker.ModuleFunctionCall, context checker.Type) []checker.Type {
	if call == nil || call.Call == nil {
		return nil
	}
	switch call.Module {
	case "ard/maybe":
		if call.Call.Name != "some" {
			return nil
		}
		maybeType, ok := call.Call.ReturnType.(*checker.Maybe)
		if context != nil {
			if expectedMaybe, expectedOK := context.(*checker.Maybe); expectedOK {
				maybeType = expectedMaybe
				ok = true
			}
		}
		if !ok || maybeHasUnresolvedTypeVar(maybeType) {
			return nil
		}
		return []checker.Type{maybeType.Of()}
	case "ard/result":
		resultType, ok := call.Call.ReturnType.(*checker.Result)
		if context != nil {
			if expectedResult, expectedOK := context.(*checker.Result); expectedOK {
				resultType = expectedResult
				ok = true
			}
		}
		if !ok || resultHasUnresolvedTypeVar(resultType) {
			return nil
		}
		if call.Call.Name == "ok" {
			return []checker.Type{resultType.Val()}
		}
		if call.Call.Name == "err" {
			return []checker.Type{resultType.Err()}
		}
	}
	return nil
}

func unresolvedContextOverride(exprType checker.Type, context checker.Type) checker.Type {
	if context == nil || exprType == nil {
		return nil
	}
	switch typed := exprType.(type) {
	case *checker.Result:
		if resultHasUnresolvedTypeVar(typed) {
			return context
		}
	case *checker.Maybe:
		if maybeHasUnresolvedTypeVar(typed) {
			return context
		}
	case *checker.TypeVar:
		if typed.Actual() == nil {
			return context
		}
	}
	return nil
}

func (e *emitter) lowerTryExprAST(op *checker.TryOp, returnType checker.Type) ([]ast.Stmt, ast.Expr, bool, error) {
	if op == nil || e.fnReturnType == nil {
		return nil, nil, false, nil
	}
	if returnType != nil && returnType != e.fnReturnType {
		return nil, nil, false, nil
	}
	tempName := e.nextTemp("TryValue")
	typeExpr, err := e.lowerTypeExpr(op.Type())
	if err != nil {
		return nil, nil, false, err
	}
	stmts := []ast.Stmt{&ast.DeclStmt{Decl: astVarDecl(astValueSpec(tempName, typeExpr, nil))}}
	tryStmts, ok, err := e.lowerTryOpAST(op, e.fnReturnType, func(successValue ast.Expr) ([]ast.Stmt, error) {
		return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(tempName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{successValue}}}, nil
	})
	if err != nil || !ok {
		return nil, nil, ok, err
	}
	stmts = append(stmts, tryStmts...)
	return stmts, ast.NewIdent(tempName), true, nil
}

func (e *emitter) lowerBinaryWithPreludeAST(left checker.Expression, op token.Token, right checker.Expression, returnType checker.Type) ([]ast.Stmt, ast.Expr, bool, error) {
	leftPrelude, l, ok, err := e.lowerExprWithPreludeAST(left, returnType)
	if err != nil || !ok {
		return nil, nil, ok, err
	}
	rightPrelude, r, ok, err := e.lowerExprWithPreludeAST(right, returnType)
	if err != nil || !ok {
		return nil, nil, ok, err
	}
	return appendPrelude(nil, leftPrelude, rightPrelude), &ast.BinaryExpr{X: l, Op: op, Y: r}, true, nil
}

func (e *emitter) lowerSpecialModuleCallWithArgsAST(call *checker.ModuleFunctionCall, args []ast.Expr, expectedType checker.Type) (ast.Expr, bool, error) {
	if call == nil || call.Call == nil {
		return nil, false, nil
	}
	switch call.Module {
	case "ard/maybe":
		switch call.Call.Name {
		case "some":
			if len(args) != 1 {
				return nil, false, errStructuredLoweringUnsupported
			}
			maybeType, ok := call.Call.ReturnType.(*checker.Maybe)
			if expectedType != nil {
				if expectedMaybe, expectedOK := expectedType.(*checker.Maybe); expectedOK {
					maybeType = expectedMaybe
					ok = true
				}
			}
			if !ok {
				return nil, false, errStructuredLoweringUnsupported
			}
			inner, err := e.lowerTypeArgExprWithOptions(maybeType.Of(), e.typeParams, nil)
			if err != nil {
				return nil, false, err
			}
			return astCall(selectorExpr(ast.NewIdent(helperImportAlias), "Some"), []ast.Expr{inner}, args), true, nil
		case "none":
			maybeType, ok := call.Call.ReturnType.(*checker.Maybe)
			if expectedType != nil {
				if expectedMaybe, expectedOK := expectedType.(*checker.Maybe); expectedOK {
					maybeType = expectedMaybe
					ok = true
				}
			}
			if !ok {
				return nil, false, errStructuredLoweringUnsupported
			}
			inner, err := e.lowerTypeArgExprWithOptions(maybeType.Of(), e.typeParams, nil)
			if err != nil {
				return nil, false, err
			}
			return astCall(selectorExpr(ast.NewIdent(helperImportAlias), "None"), []ast.Expr{inner}, nil), true, nil
		}
	case "ard/result":
		switch call.Call.Name {
		case "ok", "err":
			if len(args) != 1 {
				return nil, false, errStructuredLoweringUnsupported
			}
			resultType, ok := call.Call.ReturnType.(*checker.Result)
			if expectedType != nil {
				if expectedResult, expectedOK := expectedType.(*checker.Result); expectedOK {
					resultType = expectedResult
					ok = true
				}
			}
			if !ok {
				return nil, false, errStructuredLoweringUnsupported
			}
			valType, err := e.lowerTypeArgExprWithOptions(resultType.Val(), e.typeParams, nil)
			if err != nil {
				return nil, false, err
			}
			errType, err := e.lowerTypeArgExprWithOptions(resultType.Err(), e.typeParams, nil)
			if err != nil {
				return nil, false, err
			}
			name := "Ok"
			if call.Call.Name == "err" {
				name = "Err"
			}
			return astCall(selectorExpr(ast.NewIdent(helperImportAlias), name), []ast.Expr{valType, errType}, args), true, nil
		}
	case "ard/list":
		if call.Call.Name == "concat" && len(args) == 2 {
			typeExpr, err := e.lowerTypeExpr(call.Call.ReturnType)
			if err != nil {
				return nil, false, err
			}
			outIdent := ast.NewIdent("out")
			body := []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{outIdent}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{&ast.CallExpr{Fun: typeExpr, Args: []ast.Expr{ast.NewIdent("nil")}}, args[0]}, Ellipsis: token.Pos(1)}}},
				&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{outIdent, args[1]}, Ellipsis: token.Pos(1)}}},
			}
			return &ast.CallExpr{Fun: &ast.FuncLit{Type: &ast.FuncType{Results: funcResults(typeExpr)}, Body: &ast.BlockStmt{List: body}}}, true, nil
		}
	}
	return nil, false, nil
}

func (e *emitter) lowerCallArgWithPreludeAST(arg checker.Expression, expectedType checker.Type, flowType checker.Type) ([]ast.Stmt, ast.Expr, bool, error) {
	if expectedType != nil {
		if _, isModuleCall := arg.(*checker.ModuleFunctionCall); isModuleCall {
			lowered, ok, err := e.lowerValueForTypeAST(arg, expectedType)
			if err == nil && ok {
				return nil, lowered, true, nil
			}
			if err != nil && !errors.Is(err, errStructuredLoweringUnsupported) {
				return nil, nil, ok, err
			}
		}
	}
	prelude, lowered, ok, err := e.lowerExprWithPreludeAST(arg, flowType)
	if err != nil || !ok {
		return nil, nil, ok, err
	}
	if expectedType != nil {
		lowered, err = e.wrapTraitValueAST(lowered, expectedType)
		if err != nil {
			return nil, nil, false, err
		}
	}
	return prelude, lowered, true, nil
}

func (e *emitter) lowerExprWithPreludeAST(expr checker.Expression, returnType checker.Type) ([]ast.Stmt, ast.Expr, bool, error) {
	switch v := expr.(type) {
	case *checker.TryOp:
		return e.lowerTryExprAST(v, returnType)
	case *checker.TemplateStr:
		prelude := []ast.Stmt{}
		if len(v.Chunks) == 0 {
			return nil, &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("")}, true, nil
		}
		var out ast.Expr
		for i, chunk := range v.Chunks {
			chunkPrelude, lowered, ok, err := e.lowerExprWithPreludeAST(chunk, returnType)
			if err != nil || !ok {
				return nil, nil, ok, err
			}
			prelude = append(prelude, chunkPrelude...)
			if i == 0 {
				out = lowered
			} else {
				out = &ast.BinaryExpr{X: out, Op: token.ADD, Y: lowered}
			}
		}
		return prelude, out, true, nil
	case *checker.IntAddition:
		return e.lowerBinaryWithPreludeAST(v.Left, token.ADD, v.Right, returnType)
	case *checker.IntSubtraction:
		return e.lowerBinaryWithPreludeAST(v.Left, token.SUB, v.Right, returnType)
	case *checker.IntMultiplication:
		return e.lowerBinaryWithPreludeAST(v.Left, token.MUL, v.Right, returnType)
	case *checker.IntDivision:
		return e.lowerBinaryWithPreludeAST(v.Left, token.QUO, v.Right, returnType)
	case *checker.IntModulo:
		return e.lowerBinaryWithPreludeAST(v.Left, token.REM, v.Right, returnType)
	case *checker.FloatAddition:
		return e.lowerBinaryWithPreludeAST(v.Left, token.ADD, v.Right, returnType)
	case *checker.FloatSubtraction:
		return e.lowerBinaryWithPreludeAST(v.Left, token.SUB, v.Right, returnType)
	case *checker.FloatMultiplication:
		return e.lowerBinaryWithPreludeAST(v.Left, token.MUL, v.Right, returnType)
	case *checker.FloatDivision:
		return e.lowerBinaryWithPreludeAST(v.Left, token.QUO, v.Right, returnType)
	case *checker.StrAddition:
		return e.lowerBinaryWithPreludeAST(v.Left, token.ADD, v.Right, returnType)
	case *checker.IntGreater:
		return e.lowerBinaryWithPreludeAST(v.Left, token.GTR, v.Right, returnType)
	case *checker.IntGreaterEqual:
		return e.lowerBinaryWithPreludeAST(v.Left, token.GEQ, v.Right, returnType)
	case *checker.IntLess:
		return e.lowerBinaryWithPreludeAST(v.Left, token.LSS, v.Right, returnType)
	case *checker.IntLessEqual:
		return e.lowerBinaryWithPreludeAST(v.Left, token.LEQ, v.Right, returnType)
	case *checker.FloatGreater:
		return e.lowerBinaryWithPreludeAST(v.Left, token.GTR, v.Right, returnType)
	case *checker.FloatGreaterEqual:
		return e.lowerBinaryWithPreludeAST(v.Left, token.GEQ, v.Right, returnType)
	case *checker.FloatLess:
		return e.lowerBinaryWithPreludeAST(v.Left, token.LSS, v.Right, returnType)
	case *checker.FloatLessEqual:
		return e.lowerBinaryWithPreludeAST(v.Left, token.LEQ, v.Right, returnType)
	case *checker.Equality:
		return e.lowerBinaryWithPreludeAST(v.Left, token.EQL, v.Right, returnType)
	case *checker.And:
		return e.lowerBinaryWithPreludeAST(v.Left, token.LAND, v.Right, returnType)
	case *checker.Or:
		return e.lowerBinaryWithPreludeAST(v.Left, token.LOR, v.Right, returnType)
	case *checker.ListLiteral:
		prelude := []ast.Stmt{}
		elts := make([]ast.Expr, 0, len(v.Elements))
		for _, element := range v.Elements {
			elementPrelude, lowered, ok, err := e.lowerExprWithPreludeAST(element, returnType)
			if err != nil || !ok {
				return nil, nil, ok, err
			}
			prelude = append(prelude, elementPrelude...)
			elts = append(elts, lowered)
		}
		typeExpr, err := e.lowerTypeExpr(v.ListType)
		if err != nil {
			return nil, nil, false, err
		}
		return prelude, &ast.CompositeLit{Type: typeExpr, Elts: elts}, true, nil
	case *checker.MapLiteral:
		prelude := []ast.Stmt{}
		elts := make([]ast.Expr, 0, len(v.Keys))
		for i := range v.Keys {
			keyPrelude, keyExpr, ok, err := e.lowerExprWithPreludeAST(v.Keys[i], returnType)
			if err != nil || !ok {
				return nil, nil, ok, err
			}
			valPrelude, valExpr, ok, err := e.lowerExprWithPreludeAST(v.Values[i], returnType)
			if err != nil || !ok {
				return nil, nil, ok, err
			}
			prelude = appendPrelude(prelude, keyPrelude, valPrelude)
			elts = append(elts, &ast.KeyValueExpr{Key: keyExpr, Value: valExpr})
		}
		typeExpr, err := e.lowerTypeExpr(v.Type())
		if err != nil {
			return nil, nil, false, err
		}
		return prelude, &ast.CompositeLit{Type: typeExpr, Elts: elts}, true, nil
	case *checker.StructInstance:
		prelude := []ast.Stmt{}
		elts := make([]ast.Expr, 0, len(v.Fields))
		for _, fieldName := range sortedStringKeys(v.Fields) {
			fieldPrelude, lowered, ok, err := e.lowerExprWithPreludeAST(v.Fields[fieldName], returnType)
			if err != nil || !ok {
				return nil, nil, ok, err
			}
			prelude = append(prelude, fieldPrelude...)
			elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(goName(fieldName, true)), Value: lowered})
		}
		typeExpr, err := e.lowerTypeExpr(v.StructType)
		if err != nil {
			return nil, nil, false, err
		}
		return prelude, &ast.CompositeLit{Type: typeExpr, Elts: elts}, true, nil
	case *checker.ModuleStructInstance:
		prelude := []ast.Stmt{}
		elts := make([]ast.Expr, 0, len(v.Property.Fields))
		for _, fieldName := range sortedStringKeys(v.Property.Fields) {
			fieldPrelude, lowered, ok, err := e.lowerExprWithPreludeAST(v.Property.Fields[fieldName], returnType)
			if err != nil || !ok {
				return nil, nil, ok, err
			}
			prelude = append(prelude, fieldPrelude...)
			elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(goName(fieldName, true)), Value: lowered})
		}
		typeExpr, err := e.lowerTypeExpr(v.StructType)
		if err != nil {
			return nil, nil, false, err
		}
		return prelude, &ast.CompositeLit{Type: typeExpr, Elts: elts}, true, nil
	case *checker.BoolMatch:
		lowered, ok, err := e.lowerBoolMatchWithExpectedAST(v, unresolvedContextOverride(v.Type(), returnType))
		return nil, lowered, ok, err
	case *checker.IntMatch:
		lowered, ok, err := e.lowerIntMatchWithExpectedAST(v, unresolvedContextOverride(v.Type(), returnType))
		return nil, lowered, ok, err
	case *checker.ConditionalMatch:
		lowered, ok, err := e.lowerConditionalMatchWithExpectedAST(v, unresolvedContextOverride(v.Type(), returnType))
		return nil, lowered, ok, err
	case *checker.OptionMatch:
		lowered, ok, err := e.lowerOptionMatchWithExpectedAST(v, unresolvedContextOverride(v.Type(), returnType))
		return nil, lowered, ok, err
	case *checker.ResultMatch:
		lowered, ok, err := e.lowerResultMatchWithExpectedAST(v, unresolvedContextOverride(v.Type(), returnType))
		return nil, lowered, ok, err
	case *checker.EnumMatch:
		lowered, ok, err := e.lowerEnumMatchWithExpectedAST(v, unresolvedContextOverride(v.Type(), returnType))
		return nil, lowered, ok, err
	case *checker.UnionMatch:
		lowered, ok, err := e.lowerUnionMatchWithExpectedAST(v, unresolvedContextOverride(v.Type(), returnType))
		return nil, lowered, ok, err
	case *checker.FunctionCall:
		prelude := []ast.Stmt{}
		args := make([]ast.Expr, 0, len(v.Args))
		var params []checker.Parameter
		if def := v.Definition(); def != nil {
			params = def.Parameters
		}
		for i, arg := range v.Args {
			if i < len(params) && params[i].Mutable {
				lowered, ok, err := e.lowerMutableCallArgAST(arg, params[i])
				if err != nil || !ok {
					return nil, nil, ok, err
				}
				args = append(args, lowered)
				continue
			}
			var expectedType checker.Type
			if i < len(params) {
				expectedType = params[i].Type
			}
			argPrelude, lowered, ok, err := e.lowerCallArgWithPreludeAST(arg, expectedType, e.fnReturnType)
			if err != nil || !ok {
				return nil, nil, ok, err
			}
			prelude = append(prelude, argPrelude...)
			args = append(args, lowered)
		}
		typeArgs, err := e.lowerFunctionCallTypeArgsAST(e.originalFunctionDef(v), v.Definition())
		if err != nil {
			return nil, nil, false, err
		}
		name := e.functionNames[v.Name]
		if name == "" {
			name = goName(v.Name, false)
		}
		return prelude, astCall(ast.NewIdent(name), typeArgs, args), true, nil
	case *checker.ModuleFunctionCall:
		prelude := []ast.Stmt{}
		args := make([]ast.Expr, 0, len(v.Call.Args))
		var params []checker.Parameter
		if def := v.Call.Definition(); def != nil {
			params = def.Parameters
		}
		if len(params) == 0 {
			if original := e.originalModuleFunctionDef(v.Module, v.Call); original != nil {
				params = original.Parameters
			}
		}
		specialArgTypes := specialModuleCallArgTypes(v, unresolvedContextOverride(v.Type(), returnType))
		if len(specialArgTypes) == 0 {
			specialArgTypes = specialModuleCallArgTypes(v, v.Type())
		}
		for i, arg := range v.Call.Args {
			if i < len(params) && params[i].Mutable {
				lowered, ok, err := e.lowerMutableCallArgAST(arg, params[i])
				if err != nil || !ok {
					return nil, nil, ok, err
				}
				args = append(args, lowered)
				continue
			}
			var expectedType checker.Type
			if i < len(params) {
				expectedType = params[i].Type
			}
			if expectedType == nil && i < len(specialArgTypes) {
				expectedType = specialArgTypes[i]
			}
			argPrelude, lowered, ok, err := e.lowerCallArgWithPreludeAST(arg, expectedType, e.fnReturnType)
			if err != nil || !ok {
				return nil, nil, ok, err
			}
			prelude = append(prelude, argPrelude...)
			args = append(args, lowered)
		}
		expectedOverride := unresolvedContextOverride(v.Type(), returnType)
		if expectedOverride != nil {
			if special, ok, err := e.lowerSpecialModuleCallWithArgsAST(v, args, expectedOverride); ok {
				return prelude, special, ok, nil
			} else if err != nil && !errors.Is(err, errStructuredLoweringUnsupported) {
				return nil, nil, false, err
			}
		}
		if special, ok, err := e.lowerSpecialModuleCallWithArgsAST(v, args, expr.Type()); ok || err != nil {
			return prelude, special, ok, err
		}
		typeArgs, err := e.lowerFunctionCallTypeArgsAST(e.originalModuleFunctionDef(v.Module, v.Call), v.Call.Definition())
		if err != nil {
			return nil, nil, false, err
		}
		alias := packageNameForModulePath(v.Module)
		name := goName(e.resolvedModuleFunctionName(v.Module, v.Call), true)
		return prelude, astCall(selectorExpr(ast.NewIdent(alias), name), typeArgs, args), true, nil
	default:
		lowered, ok, err := e.lowerExprAST(expr)
		return nil, lowered, ok, err
	}
}
