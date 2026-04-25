package go_backend

import (
	"go/ast"
	"go/token"
)

func asyncTypeParamList(names ...string) *ast.FieldList {
	if len(names) == 0 {
		return nil
	}
	fields := make([]*ast.Field, 0, len(names))
	for _, name := range names {
		fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(name)}, Type: ast.NewIdent("any")})
	}
	return &ast.FieldList{List: fields}
}

func asyncFuncType(params []*ast.Field, results ...ast.Expr) *ast.FuncType {
	fnType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if len(results) > 0 && results[0] != nil {
		fnType.Results = funcResults(results[0])
	}
	return fnType
}

func asyncField(name string, typ ast.Expr) *ast.Field {
	return &ast.Field{Names: []*ast.Ident{ast.NewIdent(name)}, Type: typ}
}

func asyncNamedType(name string, args ...ast.Expr) ast.Expr {
	return indexExpr(ast.NewIdent(name), args)
}

func asyncCall(fun ast.Expr, args ...ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: fun, Args: args}
}

func asyncPanic(message string) ast.Stmt {
	return &ast.ExprStmt{X: asyncCall(ast.NewIdent("panic"), &ast.BasicLit{Kind: token.STRING, Value: strconvQuote(message)})}
}

func strconvQuote(value string) string {
	return `"` + value + `"`
}

func lowerAsyncModuleFileIR(packageName string) (goFileIR, error) {
	fileIR := lowerGoFileIR(packageName, map[string]string{
		helperImportPath: helperImportAlias,
		"sync":           "sync",
	})

	appendASTDecl(&fileIR, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name:       ast.NewIdent("fiberState"),
		TypeParams: asyncTypeParamList("T"),
		Type: &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
			asyncField("wg", selectorExpr(ast.NewIdent("sync"), "WaitGroup")),
			asyncField("result", ast.NewIdent("T")),
		}}},
	}}})

	appendASTDecl(&fileIR, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name:       ast.NewIdent("Fiber"),
		TypeParams: asyncTypeParamList("T"),
		Type: &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
			asyncField("Result", ast.NewIdent("T")),
			asyncField("Wg", ast.NewIdent("any")),
		}}},
	}}})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("self")}, Type: asyncNamedType("Fiber", ast.NewIdent("T"))}}},
		Name: ast.NewIdent("fiberHandle"),
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: funcResults(ast.NewIdent("any"))},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{Results: []ast.Expr{selectorExpr(ast.NewIdent("self"), "Wg")}},
		}},
	})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("self")}, Type: asyncNamedType("Fiber", ast.NewIdent("T"))}}},
		Name: ast.NewIdent("Get"),
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: funcResults(ast.NewIdent("T"))},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ExprStmt{X: asyncCall(selectorExpr(ast.NewIdent("self"), "Join"))},
			&ast.ReturnStmt{Results: []ast.Expr{astCall(ast.NewIdent("fiberGet"), []ast.Expr{ast.NewIdent("T")}, []ast.Expr{selectorExpr(ast.NewIdent("self"), "Wg")})}},
		}},
	})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("self")}, Type: asyncNamedType("Fiber", ast.NewIdent("T"))}}},
		Name: ast.NewIdent("Join"),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ExprStmt{X: asyncCall(ast.NewIdent("fiberWait"), selectorExpr(ast.NewIdent("self"), "Wg"))},
		}},
	})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Name: ast.NewIdent("Sleep"),
		Type: asyncFuncType([]*ast.Field{asyncField("ms", ast.NewIdent("int"))}),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("err")}, Tok: token.DEFINE, Rhs: []ast.Expr{asyncCall(selectorExpr(ast.NewIdent(helperImportAlias), "CallExtern"), &ast.BasicLit{Kind: token.STRING, Value: strconvQuote("Sleep")}, ast.NewIdent("ms"))}},
			&ast.IfStmt{Cond: &ast.BinaryExpr{X: ast.NewIdent("err"), Op: token.NEQ, Y: ast.NewIdent("nil")}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: asyncCall(ast.NewIdent("panic"), ast.NewIdent("err"))}}}},
		}},
	})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Name: ast.NewIdent("Start"),
		Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{asyncField("do", asyncFuncType(nil))}}, Results: funcResults(asyncNamedType("Fiber", &ast.StructType{Fields: &ast.FieldList{}}))},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("state")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: asyncNamedType("fiberState", &ast.StructType{Fields: &ast.FieldList{}})}}}},
			&ast.ExprStmt{X: asyncCall(selectorExpr(selectorExpr(ast.NewIdent("state"), "wg"), "Add"), &ast.BasicLit{Kind: token.INT, Value: "1"})},
			&ast.GoStmt{Call: &ast.CallExpr{Fun: &ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.DeferStmt{Call: &ast.CallExpr{Fun: selectorExpr(selectorExpr(ast.NewIdent("state"), "wg"), "Done")}},
				&ast.ExprStmt{X: asyncCall(ast.NewIdent("do"))},
			}}}}},
			&ast.ReturnStmt{Results: []ast.Expr{&ast.CompositeLit{Type: asyncNamedType("Fiber", &ast.StructType{Fields: &ast.FieldList{}}), Elts: []ast.Expr{&ast.KeyValueExpr{Key: ast.NewIdent("Wg"), Value: ast.NewIdent("state")}}}}},
		}},
	})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Name: ast.NewIdent("Eval"),
		Type: &ast.FuncType{TypeParams: asyncTypeParamList("T"), Params: &ast.FieldList{List: []*ast.Field{asyncField("do", asyncFuncType(nil, ast.NewIdent("T")))}}, Results: funcResults(asyncNamedType("Fiber", ast.NewIdent("T")))},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("state")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: asyncNamedType("fiberState", ast.NewIdent("T"))}}}},
			&ast.ExprStmt{X: asyncCall(selectorExpr(selectorExpr(ast.NewIdent("state"), "wg"), "Add"), &ast.BasicLit{Kind: token.INT, Value: "1"})},
			&ast.GoStmt{Call: &ast.CallExpr{Fun: &ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.DeferStmt{Call: &ast.CallExpr{Fun: selectorExpr(selectorExpr(ast.NewIdent("state"), "wg"), "Done")}},
				&ast.AssignStmt{Lhs: []ast.Expr{selectorExpr(ast.NewIdent("state"), "result")}, Tok: token.ASSIGN, Rhs: []ast.Expr{asyncCall(ast.NewIdent("do"))}},
			}}}}},
			&ast.ReturnStmt{Results: []ast.Expr{&ast.CompositeLit{Type: asyncNamedType("Fiber", ast.NewIdent("T")), Elts: []ast.Expr{&ast.KeyValueExpr{Key: ast.NewIdent("Wg"), Value: ast.NewIdent("state")}}}}},
		}},
	})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Name: ast.NewIdent("Join"),
		Type: &ast.FuncType{TypeParams: asyncTypeParamList("T"), Params: &ast.FieldList{List: []*ast.Field{asyncField("fibers", &ast.ArrayType{Elt: asyncNamedType("Fiber", ast.NewIdent("T"))})}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.RangeStmt{Key: ast.NewIdent("_"), Value: ast.NewIdent("fiber"), Tok: token.DEFINE, X: ast.NewIdent("fibers"), Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ExprStmt{X: asyncCall(ast.NewIdent("fiberWait"), selectorExpr(ast.NewIdent("fiber"), "Wg"))},
			}}},
		}},
	})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Name: ast.NewIdent("JoinAny"),
		Type: asyncFuncType([]*ast.Field{asyncField("fibers", &ast.ArrayType{Elt: ast.NewIdent("any")})}),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.RangeStmt{Key: ast.NewIdent("_"), Value: ast.NewIdent("fiber"), Tok: token.DEFINE, X: ast.NewIdent("fibers"), Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("handleProvider"), ast.NewIdent("ok")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: ast.NewIdent("fiber"), Type: &ast.InterfaceType{Methods: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("fiberHandle")}, Type: &ast.FuncType{Params: &ast.FieldList{}, Results: funcResults(ast.NewIdent("any"))}}}}}}}},
				&ast.IfStmt{Cond: &ast.UnaryExpr{Op: token.NOT, X: ast.NewIdent("ok")}, Body: &ast.BlockStmt{List: []ast.Stmt{asyncPanic("unexpected async fiber")}}},
				&ast.ExprStmt{X: asyncCall(ast.NewIdent("fiberWait"), asyncCall(selectorExpr(ast.NewIdent("handleProvider"), "fiberHandle")))},
			}}},
		}},
	})

	appendASTDecl(&fileIR, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name: ast.NewIdent("fiberWaiter"),
		Type: &ast.InterfaceType{Methods: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("wait")}, Type: &ast.FuncType{Params: &ast.FieldList{}}}}}},
	}}})

	appendASTDecl(&fileIR, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name:       ast.NewIdent("fiberGetter"),
		TypeParams: asyncTypeParamList("T"),
		Type:       &ast.InterfaceType{Methods: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("get")}, Type: &ast.FuncType{Params: &ast.FieldList{}, Results: funcResults(ast.NewIdent("T"))}}}}},
	}}})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("state")}, Type: &ast.StarExpr{X: asyncNamedType("fiberState", ast.NewIdent("T"))}}}},
		Name: ast.NewIdent("wait"),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ExprStmt{X: asyncCall(selectorExpr(selectorExpr(ast.NewIdent("state"), "wg"), "Wait"))},
		}},
	})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("state")}, Type: &ast.StarExpr{X: asyncNamedType("fiberState", ast.NewIdent("T"))}}}},
		Name: ast.NewIdent("get"),
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: funcResults(ast.NewIdent("T"))},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ExprStmt{X: asyncCall(selectorExpr(selectorExpr(ast.NewIdent("state"), "wg"), "Wait"))},
			&ast.ReturnStmt{Results: []ast.Expr{selectorExpr(ast.NewIdent("state"), "result")}},
		}},
	})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Name: ast.NewIdent("fiberWait"),
		Type: asyncFuncType([]*ast.Field{asyncField("handle", ast.NewIdent("any"))}),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.IfStmt{Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("waiter"), ast.NewIdent("ok")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: ast.NewIdent("handle"), Type: ast.NewIdent("fiberWaiter")}}}, Cond: ast.NewIdent("ok"), Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ExprStmt{X: asyncCall(selectorExpr(ast.NewIdent("waiter"), "wait"))},
				&ast.ReturnStmt{},
			}}},
			asyncPanic("unexpected async fiber handle"),
		}},
	})

	appendASTDecl(&fileIR, &ast.FuncDecl{
		Name: ast.NewIdent("fiberGet"),
		Type: &ast.FuncType{TypeParams: asyncTypeParamList("T"), Params: &ast.FieldList{List: []*ast.Field{asyncField("handle", ast.NewIdent("any"))}}, Results: funcResults(ast.NewIdent("T"))},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.IfStmt{Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("getter"), ast.NewIdent("ok")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: ast.NewIdent("handle"), Type: asyncNamedType("fiberGetter", ast.NewIdent("T"))}}}, Cond: ast.NewIdent("ok"), Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ReturnStmt{Results: []ast.Expr{asyncCall(selectorExpr(ast.NewIdent("getter"), "get"))}},
			}}},
			asyncPanic("unexpected async fiber handle"),
		}},
	})

	return fileIR, nil
}
