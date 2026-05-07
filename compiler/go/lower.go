package gotarget

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"unicode"

	"github.com/akonwi/ard/air"
)

type loweredExpr struct {
	stmts []ast.Stmt
	expr  ast.Expr
}

type lowerer struct {
	program        *air.Program
	packageName    string
	tempCounter    int
	currentImports map[string]string
	declaredLocals map[air.LocalID]bool
	runtimeHelpers map[string]bool
}

func lowerProgram(program *air.Program, options Options) (map[string]*ast.File, error) {
	if program == nil {
		return nil, fmt.Errorf("AIR program is nil")
	}
	if err := air.Validate(program); err != nil {
		return nil, err
	}
	l := &lowerer{program: program, packageName: defaultPackageName(options.PackageName), runtimeHelpers: map[string]bool{}}
	files := map[string]*ast.File{}
	rootID, hasRoot := findRootFunction(program)
	modules := make([]air.Module, 0, len(program.Modules))
	if hasRoot {
		rootModuleID := program.Functions[rootID].Module
		for _, module := range program.Modules {
			if module.ID != rootModuleID {
				modules = append(modules, module)
			}
		}
		for _, module := range program.Modules {
			if module.ID == rootModuleID {
				modules = append(modules, module)
				break
			}
		}
	} else {
		modules = append(modules, program.Modules...)
	}
	for _, module := range modules {
		file, err := l.lowerModule(module)
		if err != nil {
			return nil, err
		}
		files[moduleFileName(module)] = file
	}
	return files, nil
}

func (l *lowerer) lowerModule(module air.Module) (*ast.File, error) {
	l.currentImports = map[string]string{}
	decls := []ast.Decl{}
	rootID, hasRoot := findRootFunction(l.program)
	mainModuleID := air.ModuleID(0)
	if hasRoot {
		mainModuleID = l.program.Functions[rootID].Module
	} else if len(l.program.Modules) > 0 {
		mainModuleID = l.program.Modules[len(l.program.Modules)-1].ID
	}
	if module.ID == mainModuleID {
		for _, typ := range l.program.Types {
			typeDecls, err := l.lowerTypeDecls(typ)
			if err != nil {
				return nil, fmt.Errorf("module %s type %s: %w", module.Path, typ.Name, err)
			}
			decls = append(decls, typeDecls...)
		}
	}
	functionIDs := append([]air.FunctionID(nil), module.Functions...)
	sort.Slice(functionIDs, func(i, j int) bool { return functionIDs[i] < functionIDs[j] })
	for _, functionID := range functionIDs {
		fn := l.program.Functions[functionID]
		decl, err := l.lowerFunction(fn)
		if err != nil {
			return nil, fmt.Errorf("module %s function %s: %w", module.Path, fn.Name, err)
		}
		decls = append(decls, decl)
	}
	if !hasRoot && module.ID == mainModuleID {
		decls = append(l.runtimePreludeDecls(), decls...)
		decls = append(decls, &ast.FuncDecl{Name: ast.NewIdent("main"), Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{}})
	} else if hasRoot && module.ID == mainModuleID {
		mainDecl, err := l.lowerMainWrapper(rootID)
		if err != nil {
			return nil, err
		}
		decls = append(decls, mainDecl)
		decls = append(l.runtimePreludeDecls(), decls...)
	}
	if len(l.currentImports) > 0 {
		usedImports := l.usedImports(decls)
		if len(usedImports) > 0 {
			importDecl := &ast.GenDecl{Tok: token.IMPORT}
			aliases := make([]string, 0, len(usedImports))
			for alias := range usedImports {
				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)
			for _, alias := range aliases {
				importDecl.Specs = append(importDecl.Specs, &ast.ImportSpec{
					Name: ast.NewIdent(alias),
					Path: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", usedImports[alias])},
				})
			}
			decls = append([]ast.Decl{importDecl}, decls...)
		}
	}
	return &ast.File{Name: ast.NewIdent(l.packageName), Decls: decls}, nil
}

func (l *lowerer) usedImports(decls []ast.Decl) map[string]string {
	used := map[string]string{}
	for _, decl := range decls {
		ast.Inspect(decl, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			path, ok := l.currentImports[ident.Name]
			if !ok {
				return true
			}
			used[ident.Name] = path
			return true
		})
	}
	return used
}

func (l *lowerer) markRuntimeHelper(name string) {
	l.runtimeHelpers[name] = true
}

func (l *lowerer) runtimePreludeDecls() []ast.Decl {
	parts := []string{"package main\n"}
	if l.runtimeHelpers["fiber"] {
		parts = append(parts, `
	type ardFiberState[T any] struct {
		ch    chan T
		value T
		done  bool
	}

	type ardFiber[T any] struct {
		state *ardFiberState[T]
	}

	func ardSpawnFiber[T any](do func() T) ardFiber[T] {
		state := &ardFiberState[T]{ch: make(chan T, 1)}
		go func() {
			state.ch <- do()
		}()
		return ardFiber[T]{state: state}
	}

	func ardJoinFiber[T any](fiber ardFiber[T]) {
		if !fiber.state.done {
			fiber.state.value = <-fiber.state.ch
			fiber.state.done = true
		}
	}

	func ardGetFiber[T any](fiber ardFiber[T]) T {
		ardJoinFiber(fiber)
		return fiber.state.value
	}
`)
	}
	if l.runtimeHelpers["sorted_int_keys"] {
		l.currentImports["slices"] = "slices"
		parts = append(parts, `
	func ardSortedIntKeys[V any](m map[int]V) []int {
		keys := make([]int, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		return keys
	}
`)
	}
	if l.runtimeHelpers["sorted_string_keys"] {
		l.currentImports["slices"] = "slices"
		parts = append(parts, `
	func ardSortedStringKeys[V any](m map[string]V) []string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		return keys
	}
`)
	}
	if l.runtimeHelpers["sorted_any_keys"] {
		l.currentImports["fmt"] = "fmt"
		l.currentImports["slices"] = "slices"
		parts = append(parts, `
	func ardSortedAnyKeys[V any](m map[any]V) []any {
		keys := make([]any, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a any, b any) int {
			as := fmt.Sprint(a)
			bs := fmt.Sprint(b)
			if as < bs {
				return -1
			}
			if as > bs {
				return 1
			}
			return 0
		})
		return keys
	}
`)
	}
	if l.runtimeHelpers["dynamic_to_any_map"] {
		l.currentImports["stdlibffi"] = "github.com/akonwi/ard/std_lib/ffi"
		parts = append(parts, `
	func ardDynamicToAnyMap(data any) (map[any]any, error) {
		values, err := stdlibffi.DynamicToMap(data)
		if err != nil {
			return nil, err
		}
		out := make(map[any]any, len(values))
		for key, value := range values {
			out[key] = value
		}
		return out, nil
	}
`)
	}
	if l.runtimeHelpers["list_to_any_slice"] {
		parts = append(parts, `
	func ardListToAnySlice[T any](values []T) []any {
		out := make([]any, len(values))
		for i, value := range values {
			out[i] = value
		}
		return out
	}
`)
	}
	src := strings.Join(parts, "\n")
	file, err := parser.ParseFile(token.NewFileSet(), "prelude.go", src, 0)
	if err != nil {
		panic(err)
	}
	return file.Decls
}

func (l *lowerer) lowerTypeDecls(typ air.TypeInfo) ([]ast.Decl, error) {
	switch typ.Kind {
	case air.TypeStruct:
		if l.isStdlibFFIBackedType(typ) {
			decl := &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{
				&ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Assign: token.Pos(1), Type: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", typ.Name)},
			}}
			return []ast.Decl{decl}, nil
		}
		fields := make([]*ast.Field, 0, len(typ.Fields))
		for _, field := range typ.Fields {
			fieldType, err := l.goType(field.Type)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(field.Name)}, Type: fieldType})
		}
		return []ast.Decl{&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}}}}}, nil
	case air.TypeUnion:
		fields := []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("tag")}, Type: ast.NewIdent("uint32")}}
		for _, member := range typ.Members {
			memberType, err := l.goType(member.Type)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(unionMemberFieldName(member))}, Type: memberType})
		}
		return []ast.Decl{&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}}}}}, nil
	case air.TypeEnum:
		typeSpec := &ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Type: ast.NewIdent("int")}
		if l.isStdlibFFIBackedType(typ) {
			typeSpec.Assign = token.Pos(1)
			typeSpec.Type = l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", typ.Name)
		}
		specs := []ast.Spec{typeSpec}
		for _, variant := range typ.Variants {
			specs = append(specs, &ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(enumVariantName(l.program, typ, variant))}, Type: ast.NewIdent(typeName(l.program, typ)), Values: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", variant.Discriminant)}}})
		}
		decls := []ast.Decl{&ast.GenDecl{Tok: token.TYPE, Specs: specs[:1]}}
		if len(specs) > 1 {
			decls = append(decls, &ast.GenDecl{Tok: token.CONST, Specs: specs[1:]})
		}
		return decls, nil
	default:
		return nil, nil
	}
}

func (l *lowerer) lowerMainWrapper(root air.FunctionID) (ast.Decl, error) {
	fn := l.program.Functions[root]
	call := &ast.CallExpr{Fun: ast.NewIdent(functionName(l.program, fn))}
	body := []ast.Stmt{}
	for _, param := range fn.Signature.Params {
		_ = param
		return nil, fmt.Errorf("entry function parameters are not supported yet")
	}
	if l.isVoidType(fn.Signature.Return) {
		body = append(body, &ast.ExprStmt{X: call})
	} else {
		body = append(body, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
	}
	return &ast.FuncDecl{
		Name: ast.NewIdent("main"),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: body},
	}, nil
}

func (l *lowerer) lowerFunction(fn air.Function) (ast.Decl, error) {
	l.declaredLocals = map[air.LocalID]bool{}
	params := []*ast.Field{}
	for _, capture := range fn.Captures {
		captureType, err := l.goType(capture.Type)
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(sanitizeName(capture.Name))},
			Type:  captureType,
		})
		l.declaredLocals[capture.Local] = true
	}
	for _, param := range fn.Signature.Params {
		paramType, err := l.goParamType(param)
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(sanitizeName(param.Name))},
			Type:  paramType,
		})
	}
	for _, local := range fn.Locals {
		if int(local.ID) < len(fn.Signature.Params) {
			l.declaredLocals[local.ID] = true
		}
	}
	returnTypeID := fn.Signature.Return
	if len(fn.Captures) > 0 && l.isVoidType(returnTypeID) && fn.Body.Result != nil && !l.isVoidType(fn.Body.Result.Type) {
		returnTypeID = fn.Body.Result.Type
	}
	body, err := l.lowerBlock(fn, fn.Body, returnTypeID)
	if err != nil {
		return nil, err
	}
	funcType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if !l.isVoidType(returnTypeID) {
		returnType, err := l.goType(returnTypeID)
		if err != nil {
			return nil, err
		}
		funcType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
	}
	return &ast.FuncDecl{
		Name: ast.NewIdent(functionName(l.program, fn)),
		Type: funcType,
		Body: body,
	}, nil
}

func (l *lowerer) lowerBlock(fn air.Function, block air.Block, returnType air.TypeID) (*ast.BlockStmt, error) {
	stmts := []ast.Stmt{}
	for _, stmt := range block.Stmts {
		lowered, err := l.lowerStmt(fn, stmt)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, lowered...)
	}
	if block.Result != nil {
		result, err := l.lowerExprWithExpectedType(fn, *block.Result, returnType)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, result.stmts...)
		if returnType == air.NoType || l.isVoidType(returnType) {
			if l.isVoidType(block.Result.Type) || isVoidExpr(result.expr) {
				if !isVoidExpr(result.expr) {
					stmts = append(stmts, &ast.ExprStmt{X: result.expr})
				}
			} else {
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{result.expr}})
			}
		} else {
			stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{result.expr}})
		}
	}
	return &ast.BlockStmt{List: stmts}, nil
}

func (l *lowerer) lowerStmt(fn air.Function, stmt air.Stmt) ([]ast.Stmt, error) {
	switch stmt.Kind {
	case air.StmtLet:
		if stmt.Value == nil {
			return nil, fmt.Errorf("let statement missing value")
		}
		localType := l.resolvedLocalType(fn, stmt.Local)
		value, err := l.lowerExprWithExpectedType(fn, *stmt.Value, localType)
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, value.stmts...)
		if l.isVoidType(localType) || isVoidExpr(value.expr) {
			if !isVoidExpr(value.expr) {
				out = append(out, &ast.ExprStmt{X: value.expr})
			}
			return out, nil
		}
		name := localName(fn, stmt.Local)
		tok := token.DEFINE
		if l.declaredLocals[stmt.Local] {
			tok = token.ASSIGN
		} else {
			l.declaredLocals[stmt.Local] = true
		}
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(name)},
			Tok: tok,
			Rhs: []ast.Expr{value.expr},
		})
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(name)}})
		return out, nil
	case air.StmtAssign:
		if stmt.Value == nil {
			return nil, fmt.Errorf("assign statement missing value")
		}
		localType := l.resolvedLocalType(fn, stmt.Local)
		value, err := l.lowerExprWithExpectedType(fn, *stmt.Value, localType)
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, value.stmts...)
		if l.isVoidType(localType) || isVoidExpr(value.expr) {
			if !isVoidExpr(value.expr) {
				out = append(out, &ast.ExprStmt{X: value.expr})
			}
			return out, nil
		}
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(localName(fn, stmt.Local))},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{value.expr},
		})
		return out, nil
	case air.StmtSetField:
		if stmt.Target == nil {
			return nil, fmt.Errorf("field set statement missing target")
		}
		if stmt.Value == nil {
			return nil, fmt.Errorf("field set statement missing value")
		}
		target, err := l.lowerExpr(fn, *stmt.Target)
		if err != nil {
			return nil, err
		}
		if !validTypeID(l.program, stmt.Target.Type) {
			return nil, fmt.Errorf("invalid field set target type %d", stmt.Target.Type)
		}
		targetType := l.program.Types[stmt.Target.Type-1]
		if targetType.Kind != air.TypeStruct {
			return nil, fmt.Errorf("field set target must be struct, got kind %d", targetType.Kind)
		}
		if stmt.Field < 0 || stmt.Field >= len(targetType.Fields) {
			return nil, fmt.Errorf("invalid field set index %d", stmt.Field)
		}
		value, err := l.lowerExpr(fn, *stmt.Value)
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, target.stmts...)
		out = append(out, value.stmts...)
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(targetType.Fields[stmt.Field].Name)}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{value.expr},
		})
		return out, nil
	case air.StmtExpr:
		if stmt.Expr == nil {
			return nil, fmt.Errorf("expr statement missing expression")
		}
		expr, err := l.lowerExpr(fn, *stmt.Expr)
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, expr.stmts...)
		if l.isVoidType(stmt.Expr.Type) || isVoidExpr(expr.expr) {
			if !isVoidExpr(expr.expr) {
				out = append(out, &ast.ExprStmt{X: expr.expr})
			}
		} else {
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{expr.expr}})
		}
		return out, nil
	case air.StmtWhile:
		if stmt.Condition == nil {
			return nil, fmt.Errorf("while statement missing condition")
		}
		condition, err := l.lowerExpr(fn, *stmt.Condition)
		if err != nil {
			return nil, err
		}
		if len(condition.stmts) != 0 {
			return nil, fmt.Errorf("while conditions with setup statements are not supported yet")
		}
		body, err := l.lowerBlock(fn, stmt.Body, air.NoType)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{&ast.ForStmt{Cond: condition.expr, Body: body}}, nil
	case air.StmtBreak:
		return []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}, nil
	default:
		return nil, fmt.Errorf("unsupported statement kind %d", stmt.Kind)
	}
}

func (l *lowerer) lowerExpr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	switch expr.Kind {
	case air.ExprConstVoid:
		return loweredExpr{expr: ast.NewIdent("nil")}, nil
	case air.ExprConstInt:
		return loweredExpr{expr: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", expr.Int)}}, nil
	case air.ExprConstFloat:
		return loweredExpr{expr: &ast.BasicLit{Kind: token.FLOAT, Value: fmt.Sprintf("%v", expr.Float)}}, nil
	case air.ExprConstBool:
		if expr.Bool {
			return loweredExpr{expr: ast.NewIdent("true")}, nil
		}
		return loweredExpr{expr: ast.NewIdent("false")}, nil
	case air.ExprConstStr:
		return loweredExpr{expr: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", expr.Str)}}, nil
	case air.ExprPanic:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("panic missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append([]ast.Stmt{}, target.stmts...)
		stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{target.expr}}})
		return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
	case air.ExprLoadLocal:
		return loweredExpr{expr: ast.NewIdent(localName(fn, expr.Local))}, nil
	case air.ExprUnionWrap:
		return l.lowerUnionWrap(fn, expr)
	case air.ExprMatchUnion:
		return l.lowerMatchUnion(fn, expr)
	case air.ExprTraitUpcast:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("trait upcast missing target")
		}
		return l.lowerExpr(fn, *expr.Target)
	case air.ExprCallTrait:
		return l.lowerTraitCall(fn, expr)
	case air.ExprToStr:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("to_str missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: l.toStringExpr(expr.Target.Type, target.expr)}, nil
	case air.ExprCallExtern:
		return l.lowerExternCall(fn, expr)
	case air.ExprSpawnFiber:
		return l.lowerSpawnFiber(fn, expr)
	case air.ExprFiberGet:
		return l.lowerFiberGet(fn, expr)
	case air.ExprFiberJoin:
		return l.lowerFiberJoin(fn, expr)
	case air.ExprMakeClosure:
		return l.lowerMakeClosure(fn, expr)
	case air.ExprCallClosure:
		return l.lowerCallClosure(fn, expr)
	case air.ExprCopy:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("copy missing target")
		}
		return l.lowerExpr(fn, *expr.Target)
	case air.ExprMakeMaybeSome:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("maybe some missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		typ, err := l.goType(expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		valueExpr := target.expr
		if l.isVoidType(expr.Target.Type) || isVoidExpr(valueExpr) {
			valueExpr = voidValueExpr()
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CompositeLit{Type: typ, Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: valueExpr},
			&ast.KeyValueExpr{Key: ast.NewIdent("Some"), Value: ast.NewIdent("true")},
		}}}, nil
	case air.ExprMakeMaybeNone:
		typ, err := l.goType(expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{expr: &ast.CompositeLit{Type: typ}}, nil
	case air.ExprMakeResultOk:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("result ok missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		typ, err := l.goType(expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		valueExpr := target.expr
		if l.isVoidType(expr.Target.Type) || isVoidExpr(valueExpr) {
			valueExpr = voidValueExpr()
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CompositeLit{Type: typ, Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: valueExpr},
			&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: ast.NewIdent("true")},
		}}}, nil
	case air.ExprMakeResultErr:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("result err missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		typ, err := l.goType(expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CompositeLit{Type: typ, Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: target.expr},
		}}}, nil
	case air.ExprMatchMaybe:
		return l.lowerMatchMaybe(fn, expr)
	case air.ExprTryMaybe:
		return l.lowerTryMaybe(fn, expr)
	case air.ExprMaybeExpect:
		return l.lowerMaybeExpect(fn, expr)
	case air.ExprMaybeIsNone:
		return l.lowerMaybeIsNone(fn, expr)
	case air.ExprMaybeIsSome:
		return l.lowerMaybeIsSome(fn, expr)
	case air.ExprMaybeOr:
		return l.lowerMaybeOr(fn, expr)
	case air.ExprMaybeMap:
		return l.lowerMaybeMap(fn, expr)
	case air.ExprMaybeAndThen:
		return l.lowerMaybeAndThen(fn, expr)
	case air.ExprResultExpect:
		return l.lowerResultExpect(fn, expr)
	case air.ExprResultOr:
		return l.lowerResultOr(fn, expr)
	case air.ExprResultMap:
		return l.lowerResultMap(fn, expr)
	case air.ExprResultMapErr:
		return l.lowerResultMapErr(fn, expr)
	case air.ExprResultAndThen:
		return l.lowerResultAndThen(fn, expr)
	case air.ExprResultIsOk:
		return l.lowerResultIsOk(fn, expr)
	case air.ExprResultIsErr:
		return l.lowerResultIsErr(fn, expr)
	case air.ExprMatchResult:
		return l.lowerMatchResult(fn, expr)
	case air.ExprTryResult:
		return l.lowerTryResult(fn, expr)
	case air.ExprMatchEnum:
		return l.lowerMatchEnum(fn, expr)
	case air.ExprMatchInt:
		return l.lowerMatchInt(fn, expr)
	case air.ExprMakeList:
		return l.lowerMakeList(fn, expr)
	case air.ExprStrContains:
		if expr.Target == nil || len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("str contains expects target and substring")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		substr, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, substr.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "Contains"), Args: []ast.Expr{target.expr, substr.expr}}}, nil
	case air.ExprStrReplace:
		if expr.Target == nil || len(expr.Args) != 2 {
			return loweredExpr{}, fmt.Errorf("str replace expects target, from, to")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		from, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		to, err := l.lowerExpr(fn, expr.Args[1])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, from.stmts...)
		stmts = append(stmts, to.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "Replace"), Args: []ast.Expr{target.expr, from.expr, to.expr, &ast.BasicLit{Kind: token.INT, Value: "1"}}}}, nil
	case air.ExprStrReplaceAll:
		if expr.Target == nil || len(expr.Args) != 2 {
			return loweredExpr{}, fmt.Errorf("str replace_all expects target, from, to")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		from, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		to, err := l.lowerExpr(fn, expr.Args[1])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, from.stmts...)
		stmts = append(stmts, to.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "ReplaceAll"), Args: []ast.Expr{target.expr, from.expr, to.expr}}}, nil
	case air.ExprStrSplit:
		if expr.Target == nil || len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("str split expects target and delimiter")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		delimiter, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, delimiter.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "Split"), Args: []ast.Expr{target.expr, delimiter.expr}}}, nil
	case air.ExprStrStartsWith:
		if expr.Target == nil || len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("str starts_with expects target and prefix")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		prefix, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, prefix.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "HasPrefix"), Args: []ast.Expr{target.expr, prefix.expr}}}, nil
	case air.ExprToDynamic:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("to dynamic missing target")
		}
		return l.lowerExpr(fn, *expr.Target)
	case air.ExprStrTrim:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str trim missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "Trim"), Args: []ast.Expr{target.expr, &ast.BasicLit{Kind: token.STRING, Value: `" "`}}}}, nil
	case air.ExprStrIsEmpty:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str is_empty missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.BinaryExpr{X: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{target.expr}}, Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}}}, nil
	case air.ExprStrSize:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str size missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{target.expr}}}, nil
	case air.ExprStrAt:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str at missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		if len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("str at expects one arg")
		}
		index, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, index.stmts...)
		byteExpr := &ast.IndexExpr{X: target.expr, Index: index.expr}
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("string"), Args: []ast.Expr{byteExpr}}}, nil
	case air.ExprListSize:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("list size missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{target.expr}}}, nil
	case air.ExprListAt:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("list at missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		if len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("list at expects one arg")
		}
		index, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, index.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.IndexExpr{X: target.expr, Index: index.expr}}, nil
	case air.ExprListPush:
		return l.lowerListPush(fn, expr)
	case air.ExprListPrepend:
		return l.lowerListPrepend(fn, expr)
	case air.ExprListSet:
		return l.lowerListSet(fn, expr)
	case air.ExprListSwap:
		return l.lowerListSwap(fn, expr)
	case air.ExprListSort:
		return l.lowerListSort(fn, expr)
	case air.ExprMakeMap:
		return l.lowerMakeMap(fn, expr)
	case air.ExprMapSize:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("map size missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{target.expr}}}, nil
	case air.ExprMapHas:
		return l.lowerMapHas(fn, expr)
	case air.ExprMapGet:
		return l.lowerMapGet(fn, expr)
	case air.ExprMapSet:
		return l.lowerMapSet(fn, expr)
	case air.ExprMapDrop:
		return l.lowerMapDrop(fn, expr)
	case air.ExprMapKeys:
		return l.lowerMapKeys(fn, expr)
	case air.ExprMapKeyAt:
		return l.lowerMapKeyAt(fn, expr)
	case air.ExprMapValueAt:
		return l.lowerMapValueAt(fn, expr)
	case air.ExprEnumVariant:
		if !validTypeID(l.program, expr.Type) {
			return loweredExpr{}, fmt.Errorf("invalid enum type id %d", expr.Type)
		}
		typ := l.program.Types[expr.Type-1]
		if typ.Kind != air.TypeEnum || expr.Variant < 0 || expr.Variant >= len(typ.Variants) {
			return loweredExpr{}, fmt.Errorf("invalid enum variant %d for type %s", expr.Variant, typ.Name)
		}
		return loweredExpr{expr: ast.NewIdent(enumVariantName(l.program, typ, typ.Variants[expr.Variant]))}, nil
	case air.ExprMakeStruct:
		if !validTypeID(l.program, expr.Type) {
			return loweredExpr{}, fmt.Errorf("invalid struct type id %d", expr.Type)
		}
		typ := l.program.Types[expr.Type-1]
		if typ.Kind != air.TypeStruct {
			return loweredExpr{}, fmt.Errorf("make struct with non-struct type %s", typ.Name)
		}
		stmts := []ast.Stmt{}
		elts := make([]ast.Expr, 0, len(expr.Fields))
		for _, field := range expr.Fields {
			value, err := l.lowerExpr(fn, field.Value)
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, value.stmts...)
			elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(l.goFieldName(typ, field.Name)), Value: value.expr})
		}
		return loweredExpr{stmts: stmts, expr: &ast.CompositeLit{Type: ast.NewIdent(typeName(l.program, typ)), Elts: elts}}, nil
	case air.ExprGetField:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("get field missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		if !validTypeID(l.program, expr.Target.Type) {
			return loweredExpr{}, fmt.Errorf("invalid target type id %d", expr.Target.Type)
		}
		targetType := l.program.Types[expr.Target.Type-1]
		if targetType.Kind == air.TypeMaybe {
			if !validTypeID(l.program, targetType.Elem) {
				return loweredExpr{}, fmt.Errorf("invalid maybe elem type id %d", targetType.Elem)
			}
			elemType := l.program.Types[targetType.Elem-1]
			if elemType.Kind != air.TypeStruct || expr.Field < 0 || expr.Field >= len(elemType.Fields) {
				return loweredExpr{}, fmt.Errorf("invalid field index %d", expr.Field)
			}
			field := elemType.Fields[expr.Field]
			targetTemp := l.nextTemp()
			targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
			if err != nil {
				return loweredExpr{}, err
			}
			resultTemp := l.nextTemp()
			resultDecls, err := l.declareTemp(expr.Type, resultTemp)
			if err != nil {
				return loweredExpr{}, err
			}
			targetExpr := ast.NewIdent(targetTemp)
			resultExpr := ast.NewIdent(resultTemp)
			stmts := append([]ast.Stmt{}, target.stmts...)
			stmts = append(stmts, targetDecls...)
			stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
			stmts = append(stmts, resultDecls...)
			fieldExpr := &ast.SelectorExpr{X: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}, Sel: ast.NewIdent(l.goFieldName(elemType, field.Name))}
			assignValue := ast.Expr(fieldExpr)
			if expr.Type != field.Type {
				resultInfo := l.program.Types[expr.Type-1]
				if resultInfo.Kind == air.TypeMaybe && resultInfo.Elem == field.Type {
					assignValue = &ast.CompositeLit{Type: mustTypeExpr(l, expr.Type), Elts: []ast.Expr{
						&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: fieldExpr},
						&ast.KeyValueExpr{Key: ast.NewIdent("Some"), Value: ast.NewIdent("true")},
					}}
				} else {
					return loweredExpr{}, fmt.Errorf("unsupported maybe field projection from %s.%s to type %d", elemType.Name, field.Name, expr.Type)
				}
			}
			stmts = append(stmts, &ast.IfStmt{
				Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Some")},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{assignValue}}}},
			})
			return loweredExpr{stmts: stmts, expr: resultExpr}, nil
		}
		if targetType.Kind != air.TypeStruct || expr.Field < 0 || expr.Field >= len(targetType.Fields) {
			return loweredExpr{}, fmt.Errorf("invalid field index %d", expr.Field)
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(l.goFieldName(targetType, targetType.Fields[expr.Field].Name))}}, nil
	case air.ExprBlock:
		return l.lowerBlockExpr(fn, expr)
	case air.ExprIf:
		return l.lowerIfExpr(fn, expr)
	case air.ExprCall:
		args := make([]ast.Expr, 0, len(expr.Args))
		stmts := []ast.Stmt{}
		if !validFunctionID(l.program, expr.Function) {
			return loweredExpr{}, fmt.Errorf("invalid function id %d", expr.Function)
		}
		target := l.program.Functions[expr.Function]
		for i, arg := range expr.Args {
			loweredArg, err := l.lowerExpr(fn, arg)
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, loweredArg.stmts...)
			argExpr := loweredArg.expr
			if i < len(target.Signature.Params) && target.Signature.Params[i].Mutable && validTypeID(l.program, target.Signature.Params[i].Type) {
				paramType := l.program.Types[target.Signature.Params[i].Type-1]
				if paramType.Kind == air.TypeStruct && !l.localIsPointerParam(fn, arg) {
					argExpr = &ast.UnaryExpr{Op: token.AND, X: argExpr}
				}
			}
			args = append(args, argExpr)
		}
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: ast.NewIdent(functionName(l.program, target)), Args: args}}, nil
	case air.ExprIntAdd, air.ExprIntSub, air.ExprIntMul, air.ExprIntDiv, air.ExprIntMod,
		air.ExprFloatAdd, air.ExprFloatSub, air.ExprFloatMul, air.ExprFloatDiv,
		air.ExprEq, air.ExprNotEq, air.ExprLt, air.ExprLte, air.ExprGt, air.ExprGte,
		air.ExprAnd, air.ExprOr, air.ExprStrConcat:
		left, err := l.lowerExpr(fn, *expr.Left)
		if err != nil {
			return loweredExpr{}, err
		}
		right, err := l.lowerExpr(fn, *expr.Right)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{
			stmts: append(left.stmts, right.stmts...),
			expr:  &ast.BinaryExpr{X: left.expr, Op: l.binaryToken(expr.Kind), Y: right.expr},
		}, nil
	case air.ExprNot:
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.UnaryExpr{Op: token.NOT, X: target.expr}}, nil
	case air.ExprNeg:
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.UnaryExpr{Op: token.SUB, X: target.expr}}, nil
	default:
		return loweredExpr{}, fmt.Errorf("unsupported expression kind %d", expr.Kind)
	}
}

func (l *lowerer) lowerBlockExpr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if l.isVoidType(expr.Type) {
		body, err := l.lowerValueBlock(fn, expr.Body, expr.Type, nil)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: body, expr: ast.NewIdent("nil")}, nil
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	body, err := l.lowerValueBlock(fn, expr.Body, expr.Type, ast.NewIdent(temp))
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: append(decls, body...), expr: ast.NewIdent(temp)}, nil
}

func (l *lowerer) lowerIfExpr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Condition == nil {
		return loweredExpr{}, fmt.Errorf("if expression missing condition")
	}
	condition, err := l.lowerExpr(fn, *expr.Condition)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, condition.stmts...)
	var target ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		target = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	thenBody, err := l.lowerValueBlock(fn, expr.Then, expr.Type, target)
	if err != nil {
		return loweredExpr{}, err
	}
	elseBody, err := l.lowerValueBlock(fn, expr.Else, expr.Type, target)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: condition.expr,
		Body: &ast.BlockStmt{List: thenBody},
		Else: &ast.BlockStmt{List: elseBody},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerValueBlock(fn air.Function, block air.Block, resultType air.TypeID, target ast.Expr) ([]ast.Stmt, error) {
	stmts := []ast.Stmt{}
	for _, stmt := range block.Stmts {
		lowered, err := l.lowerStmt(fn, stmt)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, lowered...)
	}
	if block.Result != nil {
		result, err := l.lowerExprWithExpectedType(fn, *block.Result, resultType)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, result.stmts...)
		if l.isVoidType(resultType) {
			if l.isVoidType(block.Result.Type) || isVoidExpr(result.expr) {
				if !isVoidExpr(result.expr) {
					stmts = append(stmts, &ast.ExprStmt{X: result.expr})
				}
			} else {
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{result.expr}})
			}
		} else {
			if l.isVoidType(block.Result.Type) || isVoidExpr(result.expr) {
				return stmts, nil
			}
			if target == nil {
				return nil, fmt.Errorf("non-void block result missing target")
			}
			stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{result.expr}})
		}
	}
	return stmts, nil
}

func (l *lowerer) lowerExprWithExpectedType(fn air.Function, expr air.Expr, expectedType air.TypeID) (loweredExpr, error) {
	if expectedType != air.NoType && expectedType != expr.Type && l.canOverrideExprType(expr, expectedType) {
		cloned := expr
		cloned.Type = expectedType
		return l.lowerExpr(fn, cloned)
	}
	return l.lowerExpr(fn, expr)
}

func (l *lowerer) canOverrideExprType(expr air.Expr, expectedType air.TypeID) bool {
	if !validTypeID(l.program, expr.Type) || !validTypeID(l.program, expectedType) {
		return false
	}
	if inferred := l.inferTypeFromExprShape(&expr); inferred == expectedType {
		return true
	}
	from := l.program.Types[expr.Type-1]
	to := l.program.Types[expectedType-1]
	if from.Kind != to.Kind {
		return false
	}
	switch expr.Kind {
	case air.ExprMakeResultOk, air.ExprMakeResultErr,
		air.ExprMakeMaybeSome, air.ExprMakeMaybeNone,
		air.ExprBlock, air.ExprIf,
		air.ExprMatchEnum, air.ExprMatchInt, air.ExprMatchMaybe, air.ExprMatchResult,
		air.ExprTryResult, air.ExprTryMaybe:
		return from.Kind == air.TypeResult || from.Kind == air.TypeMaybe
	default:
		return false
	}
}

func (l *lowerer) shouldPropagateMaybeNone(expr air.Expr) bool {
	if expr.Target == nil || expr.Type == expr.Target.Type {
		return false
	}
	if len(expr.None.Stmts) != 0 || expr.None.Result == nil {
		return false
	}
	return sameAIRExpr(*expr.None.Result, *expr.Target)
}

func sameAIRExpr(a air.Expr, b air.Expr) bool {
	if a.Kind != b.Kind || a.Type != b.Type || a.Field != b.Field || a.Local != b.Local || a.Function != b.Function || a.Extern != b.Extern {
		return false
	}
	if a.Int != b.Int || a.Float != b.Float || a.Bool != b.Bool || a.Str != b.Str {
		return false
	}
	if (a.Target == nil) != (b.Target == nil) || len(a.Args) != len(b.Args) {
		return false
	}
	if a.Target != nil && !sameAIRExpr(*a.Target, *b.Target) {
		return false
	}
	for i := range a.Args {
		if !sameAIRExpr(a.Args[i], b.Args[i]) {
			return false
		}
	}
	return true
}

func (l *lowerer) declareTemp(typeID air.TypeID, name string) ([]ast.Stmt, error) {
	if l.isVoidType(typeID) {
		return nil, nil
	}
	typ, err := l.goType(typeID)
	if err != nil {
		return nil, err
	}
	return []ast.Stmt{&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(name)}, Type: typ}}}}}, nil
}

func (l *lowerer) nextTemp() string {
	name := fmt.Sprintf("_tmp_%d", l.tempCounter)
	l.tempCounter++
	return name
}

func (l *lowerer) binaryToken(kind air.ExprKind) token.Token {
	switch kind {
	case air.ExprIntAdd, air.ExprFloatAdd, air.ExprStrConcat:
		return token.ADD
	case air.ExprIntSub, air.ExprFloatSub:
		return token.SUB
	case air.ExprIntMul, air.ExprFloatMul:
		return token.MUL
	case air.ExprIntDiv, air.ExprFloatDiv:
		return token.QUO
	case air.ExprIntMod:
		return token.REM
	case air.ExprEq:
		return token.EQL
	case air.ExprNotEq:
		return token.NEQ
	case air.ExprLt:
		return token.LSS
	case air.ExprLte:
		return token.LEQ
	case air.ExprGt:
		return token.GTR
	case air.ExprGte:
		return token.GEQ
	case air.ExprAnd:
		return token.LAND
	case air.ExprOr:
		return token.LOR
	default:
		return token.ILLEGAL
	}
}

func voidTypeExpr() ast.Expr {
	return &ast.StructType{Fields: &ast.FieldList{}}
}

func voidValueExpr() ast.Expr {
	return &ast.CompositeLit{Type: voidTypeExpr()}
}

func (l *lowerer) goParamType(param air.Param) (ast.Expr, error) {
	typ, err := l.goType(param.Type)
	if err != nil {
		return nil, err
	}
	if !param.Mutable || !validTypeID(l.program, param.Type) {
		return typ, nil
	}
	info := l.program.Types[param.Type-1]
	if info.Kind == air.TypeStruct {
		return &ast.StarExpr{X: typ}, nil
	}
	return typ, nil
}

func (l *lowerer) modulePathForType(typeID air.TypeID) string {
	for _, module := range l.program.Modules {
		for _, moduleTypeID := range module.Types {
			if moduleTypeID == typeID {
				return module.Path
			}
		}
	}
	return ""
}

func (l *lowerer) isStdlibFFIBackedType(info air.TypeInfo) bool {
	if info.ID == 0 {
		return false
	}
	if info.Kind != air.TypeStruct && info.Kind != air.TypeEnum {
		return false
	}
	path := l.modulePathForType(info.ID)
	return strings.HasPrefix(path, "ard/") && info.Name != ""
}

func (l *lowerer) isHTTPHandlerFunctionType(info air.TypeInfo) bool {
	if info.Kind != air.TypeFunction || len(info.Params) != 2 || !l.isVoidType(info.Return) {
		return false
	}
	if !validTypeID(l.program, info.Params[0]) || !validTypeID(l.program, info.Params[1]) {
		return false
	}
	left := l.program.Types[info.Params[0]-1]
	right := l.program.Types[info.Params[1]-1]
	return left.Kind == air.TypeStruct && right.Kind == air.TypeStruct && left.Name == "Request" && right.Name == "Response"
}

func (l *lowerer) goType(typeID air.TypeID) (ast.Expr, error) {
	if !validTypeID(l.program, typeID) {
		return nil, fmt.Errorf("invalid type id %d", typeID)
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeVoid:
		return voidTypeExpr(), nil
	case air.TypeInt:
		return ast.NewIdent("int"), nil
	case air.TypeFloat:
		return ast.NewIdent("float64"), nil
	case air.TypeBool:
		return ast.NewIdent("bool"), nil
	case air.TypeStr:
		return ast.NewIdent("string"), nil
	case air.TypeMaybe:
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.IndexExpr{X: l.qualified("runtime", "github.com/akonwi/ard/runtime", "Maybe"), Index: elem}, nil
	case air.TypeFiber:
		l.markRuntimeHelper("fiber")
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.IndexExpr{X: ast.NewIdent("ardFiber"), Index: elem}, nil
	case air.TypeFunction:
		params := make([]*ast.Field, 0, len(info.Params))
		for i, paramTypeID := range info.Params {
			paramType, err := l.goType(paramTypeID)
			if err != nil {
				return nil, err
			}
			if l.isHTTPHandlerFunctionType(info) && i == 1 {
				paramType = &ast.StarExpr{X: paramType}
			}
			params = append(params, &ast.Field{Type: paramType})
		}
		fnType := &ast.FuncType{Params: &ast.FieldList{List: params}}
		if !l.isVoidType(info.Return) {
			returnType, err := l.goType(info.Return)
			if err != nil {
				return nil, err
			}
			fnType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
		}
		return fnType, nil
	case air.TypeResult:
		l.markRuntimeHelper("result")
		value, err := l.goType(info.Value)
		if err != nil {
			return nil, err
		}
		errType, err := l.goType(info.Error)
		if err != nil {
			return nil, err
		}
		return &ast.IndexListExpr{X: l.qualified("runtime", "github.com/akonwi/ard/runtime", "Result"), Indices: []ast.Expr{value, errType}}, nil
	case air.TypeList:
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.ArrayType{Elt: elem}, nil
	case air.TypeMap:
		key, err := l.goType(info.Key)
		if err != nil {
			return nil, err
		}
		value, err := l.goType(info.Value)
		if err != nil {
			return nil, err
		}
		return &ast.MapType{Key: key, Value: value}, nil
	case air.TypeStruct, air.TypeEnum:
		if l.isStdlibFFIBackedType(info) {
			return l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", info.Name), nil
		}
		return ast.NewIdent(typeName(l.program, info)), nil
	case air.TypeUnion:
		return ast.NewIdent(typeName(l.program, info)), nil
	case air.TypeDynamic, air.TypeExtern, air.TypeTraitObject:
		return ast.NewIdent("any"), nil
	default:
		return nil, fmt.Errorf("unsupported Go type kind %d", info.Kind)
	}
}

func (l *lowerer) isVoidType(typeID air.TypeID) bool {
	return validTypeID(l.program, typeID) && l.program.Types[typeID-1].Kind == air.TypeVoid
}

func (l *lowerer) resolvedLocalType(fn air.Function, local air.LocalID) air.TypeID {
	if int(local) < 0 || int(local) >= len(fn.Locals) {
		return air.NoType
	}
	typeID := fn.Locals[local].Type
	if !l.isWeakContextType(typeID) && !l.isVoidType(typeID) {
		return typeID
	}
	if inferred := l.inferLocalTypeFromBlock(fn, local, fn.Body); inferred != air.NoType {
		return inferred
	}
	if initExpr := l.findLocalInitializerExpr(fn.Body, local); initExpr != nil {
		if validTypeID(l.program, initExpr.Type) && !l.isVoidType(initExpr.Type) && !l.isWeakContextType(initExpr.Type) {
			return initExpr.Type
		}
		if inferred := l.resolveExpectedTypeFromExpr(typeID, initExpr); inferred != air.NoType {
			return inferred
		}
	}
	if fn.Body.Result != nil && fn.Body.Result.Kind == air.ExprLoadLocal && fn.Body.Result.Local == local && !l.isWeakContextType(fn.Signature.Return) {
		return fn.Signature.Return
	}
	return typeID
}

func (l *lowerer) findLocalInitializerExpr(block air.Block, local air.LocalID) *air.Expr {
	for _, stmt := range block.Stmts {
		switch stmt.Kind {
		case air.StmtLet:
			if stmt.Local == local {
				return stmt.Value
			}
		case air.StmtWhile:
			if expr := l.findLocalInitializerExpr(stmt.Body, local); expr != nil {
				return expr
			}
		}
		for _, expr := range []*air.Expr{stmt.Value, stmt.Expr, stmt.Target, stmt.Condition} {
			if nested := l.findLocalInitializerExprInExpr(expr, local); nested != nil {
				return nested
			}
		}
	}
	if nested := l.findLocalInitializerExprInExpr(block.Result, local); nested != nil {
		return nested
	}
	return nil
}

func (l *lowerer) findLocalInitializerExprInExpr(expr *air.Expr, local air.LocalID) *air.Expr {
	if expr == nil {
		return nil
	}
	for _, block := range []air.Block{expr.Body, expr.Then, expr.Else, expr.CatchAll, expr.Some, expr.None, expr.Ok, expr.Err, expr.Catch} {
		if nested := l.findLocalInitializerExpr(block, local); nested != nil {
			return nested
		}
	}
	for _, c := range expr.EnumCases {
		if nested := l.findLocalInitializerExpr(c.Body, local); nested != nil {
			return nested
		}
	}
	for _, c := range expr.IntCases {
		if nested := l.findLocalInitializerExpr(c.Body, local); nested != nil {
			return nested
		}
	}
	for _, c := range expr.RangeCases {
		if nested := l.findLocalInitializerExpr(c.Body, local); nested != nil {
			return nested
		}
	}
	for _, c := range expr.UnionCases {
		if nested := l.findLocalInitializerExpr(c.Body, local); nested != nil {
			return nested
		}
	}
	for _, child := range []*air.Expr{expr.Target, expr.Left, expr.Right} {
		if nested := l.findLocalInitializerExprInExpr(child, local); nested != nil {
			return nested
		}
	}
	for i := range expr.Args {
		if nested := l.findLocalInitializerExprInExpr(&expr.Args[i], local); nested != nil {
			return nested
		}
	}
	return nil
}

func (l *lowerer) inferLocalTypeFromBlock(fn air.Function, local air.LocalID, block air.Block) air.TypeID {
	for _, stmt := range block.Stmts {
		switch stmt.Kind {
		case air.StmtLet, air.StmtAssign:
			if stmt.Local == local && stmt.Value != nil && !l.isWeakContextType(stmt.Value.Type) {
				return stmt.Value.Type
			}
		case air.StmtWhile:
			if inferred := l.inferLocalTypeFromBlock(fn, local, stmt.Body); inferred != air.NoType {
				return inferred
			}
		}
		if stmt.Value != nil {
			if inferred := l.inferLocalTypeFromExpr(fn, local, *stmt.Value); inferred != air.NoType {
				return inferred
			}
		}
		if stmt.Expr != nil {
			if inferred := l.inferLocalTypeFromExpr(fn, local, *stmt.Expr); inferred != air.NoType {
				return inferred
			}
		}
		if stmt.Target != nil {
			if inferred := l.inferLocalTypeFromExpr(fn, local, *stmt.Target); inferred != air.NoType {
				return inferred
			}
		}
		if stmt.Condition != nil {
			if inferred := l.inferLocalTypeFromExpr(fn, local, *stmt.Condition); inferred != air.NoType {
				return inferred
			}
		}
	}
	if block.Result != nil {
		if inferred := l.inferLocalTypeFromExpr(fn, local, *block.Result); inferred != air.NoType {
			return inferred
		}
	}
	return air.NoType
}

func (l *lowerer) resolveExpectedTypeFromExpr(fallback air.TypeID, expr *air.Expr) air.TypeID {
	if expr == nil || !validTypeID(l.program, fallback) {
		return air.NoType
	}
	fallbackInfo := l.program.Types[fallback-1]
	if inferred := l.inferTypeFromExprShape(expr); validTypeID(l.program, inferred) {
		inferredInfo := l.program.Types[inferred-1]
		if inferredInfo.Kind == fallbackInfo.Kind || l.isVoidType(fallback) {
			return inferred
		}
	}
	if validTypeID(l.program, expr.Type) {
		exprInfo := l.program.Types[expr.Type-1]
		if exprInfo.Kind == fallbackInfo.Kind && !l.isWeakContextType(expr.Type) {
			return expr.Type
		}
	}
	for _, block := range []air.Block{expr.Body, expr.Then, expr.Else, expr.CatchAll, expr.Some, expr.None, expr.Ok, expr.Err, expr.Catch} {
		if block.Result != nil {
			if inferred := l.resolveExpectedTypeFromExpr(fallback, block.Result); inferred != air.NoType {
				return inferred
			}
		}
	}
	for _, c := range expr.EnumCases {
		if c.Body.Result != nil {
			if inferred := l.resolveExpectedTypeFromExpr(fallback, c.Body.Result); inferred != air.NoType {
				return inferred
			}
		}
	}
	for _, c := range expr.IntCases {
		if c.Body.Result != nil {
			if inferred := l.resolveExpectedTypeFromExpr(fallback, c.Body.Result); inferred != air.NoType {
				return inferred
			}
		}
	}
	for _, c := range expr.RangeCases {
		if c.Body.Result != nil {
			if inferred := l.resolveExpectedTypeFromExpr(fallback, c.Body.Result); inferred != air.NoType {
				return inferred
			}
		}
	}
	for _, c := range expr.UnionCases {
		if c.Body.Result != nil {
			if inferred := l.resolveExpectedTypeFromExpr(fallback, c.Body.Result); inferred != air.NoType {
				return inferred
			}
		}
	}
	if expr.Target != nil {
		if inferred := l.resolveExpectedTypeFromExpr(fallback, expr.Target); inferred != air.NoType {
			return inferred
		}
	}
	if expr.Left != nil {
		if inferred := l.resolveExpectedTypeFromExpr(fallback, expr.Left); inferred != air.NoType {
			return inferred
		}
	}
	if expr.Right != nil {
		if inferred := l.resolveExpectedTypeFromExpr(fallback, expr.Right); inferred != air.NoType {
			return inferred
		}
	}
	for i := range expr.Args {
		if inferred := l.resolveExpectedTypeFromExpr(fallback, &expr.Args[i]); inferred != air.NoType {
			return inferred
		}
	}
	return air.NoType
}

func (l *lowerer) inferTypeFromExprShape(expr *air.Expr) air.TypeID {
	if expr == nil {
		return air.NoType
	}
	switch expr.Kind {
	case air.ExprMakeMaybeSome:
		if expr.Target != nil {
			return l.findMaybeTypeByElem(expr.Target.Type)
		}
	case air.ExprTryMaybe:
		if expr.Target != nil && validTypeID(l.program, expr.Target.Type) {
			targetType := l.program.Types[expr.Target.Type-1]
			if targetType.Kind == air.TypeMaybe {
				return targetType.Elem
			}
		}
	case air.ExprTryResult:
		if expr.Target != nil && validTypeID(l.program, expr.Target.Type) {
			targetType := l.program.Types[expr.Target.Type-1]
			if targetType.Kind == air.TypeResult {
				return targetType.Value
			}
		}
	}
	return air.NoType
}

func (l *lowerer) findMaybeTypeByElem(elem air.TypeID) air.TypeID {
	for _, info := range l.program.Types {
		if info.Kind == air.TypeMaybe && info.Elem == elem {
			return info.ID
		}
	}
	if !validTypeID(l.program, elem) {
		return air.NoType
	}
	id := air.TypeID(len(l.program.Types) + 1)
	l.program.Types = append(l.program.Types, air.TypeInfo{ID: id, Kind: air.TypeMaybe, Name: fmt.Sprintf("Maybe<%d>", elem), Elem: elem})
	return id
}

func (l *lowerer) inferLocalTypeFromExpr(fn air.Function, local air.LocalID, expr air.Expr) air.TypeID {
	if expr.Target != nil {
		if inferred := l.inferLocalTypeFromExpr(fn, local, *expr.Target); inferred != air.NoType {
			return inferred
		}
	}
	if expr.Left != nil {
		if inferred := l.inferLocalTypeFromExpr(fn, local, *expr.Left); inferred != air.NoType {
			return inferred
		}
	}
	if expr.Right != nil {
		if inferred := l.inferLocalTypeFromExpr(fn, local, *expr.Right); inferred != air.NoType {
			return inferred
		}
	}
	for _, arg := range expr.Args {
		if inferred := l.inferLocalTypeFromExpr(fn, local, arg); inferred != air.NoType {
			return inferred
		}
	}
	for _, block := range []air.Block{expr.Body, expr.Then, expr.Else, expr.CatchAll, expr.Some, expr.None, expr.Ok, expr.Err, expr.Catch} {
		if inferred := l.inferLocalTypeFromBlock(fn, local, block); inferred != air.NoType {
			return inferred
		}
	}
	for _, c := range expr.EnumCases {
		if inferred := l.inferLocalTypeFromBlock(fn, local, c.Body); inferred != air.NoType {
			return inferred
		}
	}
	for _, c := range expr.IntCases {
		if inferred := l.inferLocalTypeFromBlock(fn, local, c.Body); inferred != air.NoType {
			return inferred
		}
	}
	for _, c := range expr.RangeCases {
		if inferred := l.inferLocalTypeFromBlock(fn, local, c.Body); inferred != air.NoType {
			return inferred
		}
	}
	for _, c := range expr.UnionCases {
		if inferred := l.inferLocalTypeFromBlock(fn, local, c.Body); inferred != air.NoType {
			return inferred
		}
	}
	return air.NoType
}

func (l *lowerer) isWeakContextType(typeID air.TypeID) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeMaybe:
		return !validTypeID(l.program, info.Elem) || l.program.Types[info.Elem-1].Kind == air.TypeVoid
	case air.TypeResult:
		return !validTypeID(l.program, info.Value) || !validTypeID(l.program, info.Error) || l.program.Types[info.Value-1].Kind == air.TypeVoid || l.program.Types[info.Error-1].Kind == air.TypeVoid
	default:
		return false
	}
}

func (l *lowerer) resolvedExprType(fn air.Function, expr air.Expr) air.TypeID {
	if expr.Kind == air.ExprLoadLocal {
		if resolved := l.resolvedLocalType(fn, expr.Local); resolved != air.NoType {
			return resolved
		}
	}
	if inferred := l.inferTypeFromExprShape(&expr); inferred != air.NoType {
		return inferred
	}
	return expr.Type
}

func (l *lowerer) localIsPointerParam(fn air.Function, expr air.Expr) bool {
	if expr.Kind != air.ExprLoadLocal {
		return false
	}
	local := int(expr.Local)
	if local < 0 || local >= len(fn.Signature.Params) || !validTypeID(l.program, expr.Type) {
		return false
	}
	param := fn.Signature.Params[local]
	if !param.Mutable || !validTypeID(l.program, param.Type) {
		return false
	}
	return l.program.Types[param.Type-1].Kind == air.TypeStruct
}

func (l *lowerer) qualified(alias string, importPath string, name string) ast.Expr {
	l.currentImports[alias] = importPath
	return &ast.SelectorExpr{X: ast.NewIdent(alias), Sel: ast.NewIdent(name)}
}

func (l *lowerer) toStringExpr(typeID air.TypeID, expr ast.Expr) ast.Expr {
	if validTypeID(l.program, typeID) && l.program.Types[typeID-1].Kind == air.TypeFloat {
		return &ast.CallExpr{Fun: l.qualified("strconv", "strconv", "FormatFloat"), Args: []ast.Expr{expr, &ast.BasicLit{Kind: token.CHAR, Value: "'f'"}, &ast.BasicLit{Kind: token.INT, Value: "2"}, &ast.BasicLit{Kind: token.INT, Value: "64"}}}
	}
	return &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{expr}}
}

func (l *lowerer) lowerUnionWrap(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("union wrap missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if !validTypeID(l.program, expr.Type) {
		return loweredExpr{}, fmt.Errorf("invalid union type id %d", expr.Type)
	}
	unionType := l.program.Types[expr.Type-1]
	if unionType.Kind != air.TypeUnion {
		return loweredExpr{}, fmt.Errorf("union wrap with non-union type %s", unionType.Name)
	}
	fieldName := ""
	for _, member := range unionType.Members {
		if member.Tag == expr.Tag {
			fieldName = unionMemberFieldName(member)
			break
		}
	}
	if fieldName == "" {
		return loweredExpr{}, fmt.Errorf("invalid union tag %d for %s", expr.Tag, unionType.Name)
	}
	fieldValue := target.expr
	for _, member := range unionType.Members {
		if member.Tag == expr.Tag && validTypeID(l.program, member.Type) && l.program.Types[member.Type-1].Kind == air.TypeVoid {
			fieldValue = voidValueExpr()
			break
		}
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.CompositeLit{Type: ast.NewIdent(typeName(l.program, unionType)), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("tag"), Value: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", expr.Tag)}},
		&ast.KeyValueExpr{Key: ast.NewIdent(fieldName), Value: fieldValue},
	}}}, nil
}

func (l *lowerer) lowerMatchUnion(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("union match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if !validTypeID(l.program, expr.Target.Type) {
		return loweredExpr{}, fmt.Errorf("invalid union target type %d", expr.Target.Type)
	}
	unionType := l.program.Types[expr.Target.Type-1]
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	cases := make([]ast.Stmt, 0, len(expr.UnionCases)+1)
	for _, unionCase := range expr.UnionCases {
		fieldName := ""
		for _, member := range unionType.Members {
			if member.Tag == unionCase.Tag {
				fieldName = unionMemberFieldName(member)
				break
			}
		}
		if fieldName == "" {
			return loweredExpr{}, fmt.Errorf("invalid union case tag %d", unionCase.Tag)
		}
		localName := localName(fn, unionCase.Local)
		l.declaredLocals[unionCase.Local] = true
		bind := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(localName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(fieldName)}}}
		body, err := l.lowerValueBlock(fn, unionCase.Body, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		body = append([]ast.Stmt{bind, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(localName)}}}, body...)
		cases = append(cases, &ast.CaseClause{List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", unionCase.Tag)}}, Body: body})
	}
	if len(expr.CatchAll.Stmts) > 0 || expr.CatchAll.Result != nil {
		body, err := l.lowerValueBlock(fn, expr.CatchAll, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{Body: body})
	}
	stmts = append(stmts, &ast.SwitchStmt{Tag: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("tag")}, Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMatchInt(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("int match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	resultTypeID := expr.Type
	if l.isWeakContextType(resultTypeID) || l.isVoidType(resultTypeID) {
		for _, intCase := range expr.IntCases {
			if intCase.Body.Result != nil && intCase.Body.Result.Kind == air.ExprMakeMaybeSome && intCase.Body.Result.Target != nil {
				if inferred := l.findMaybeTypeByElem(intCase.Body.Result.Target.Type); inferred != air.NoType {
					resultTypeID = inferred
					break
				}
			}
		}
		if resultTypeID == expr.Type {
			for _, rangeCase := range expr.RangeCases {
				if rangeCase.Body.Result != nil && rangeCase.Body.Result.Kind == air.ExprMakeMaybeSome && rangeCase.Body.Result.Target != nil {
					if inferred := l.findMaybeTypeByElem(rangeCase.Body.Result.Target.Type); inferred != air.NoType {
						resultTypeID = inferred
						break
					}
				}
			}
		}
		if resultTypeID == expr.Type {
			if inferred := l.resolveExpectedTypeFromExpr(resultTypeID, &expr); inferred != air.NoType {
				resultTypeID = inferred
			}
		}
	}
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(resultTypeID) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(resultTypeID, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	cases := make([]ast.Stmt, 0, len(expr.IntCases)+len(expr.RangeCases)+1)
	for _, intCase := range expr.IntCases {
		body, err := l.lowerValueBlock(fn, intCase.Body, resultTypeID, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{List: []ast.Expr{&ast.BinaryExpr{X: target.expr, Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", intCase.Value)}}}, Body: body})
	}
	for _, rangeCase := range expr.RangeCases {
		body, err := l.lowerValueBlock(fn, rangeCase.Body, resultTypeID, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cond := &ast.BinaryExpr{X: &ast.BinaryExpr{X: target.expr, Op: token.GEQ, Y: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", rangeCase.Start)}}, Op: token.LAND, Y: &ast.BinaryExpr{X: target.expr, Op: token.LEQ, Y: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", rangeCase.End)}}}
		cases = append(cases, &ast.CaseClause{List: []ast.Expr{cond}, Body: body})
	}
	if len(expr.CatchAll.Stmts) > 0 || expr.CatchAll.Result != nil {
		body, err := l.lowerValueBlock(fn, expr.CatchAll, resultTypeID, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{Body: body})
	}
	stmts = append(stmts, &ast.SwitchStmt{Tag: ast.NewIdent("true"), Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMatchEnum(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("enum match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	cases := make([]ast.Stmt, 0, len(expr.EnumCases)+1)
	for _, enumCase := range expr.EnumCases {
		body, err := l.lowerValueBlock(fn, enumCase.Body, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", enumCase.Discriminant)}},
			Body: body,
		})
	}
	if len(expr.CatchAll.Stmts) > 0 || expr.CatchAll.Result != nil {
		body, err := l.lowerValueBlock(fn, expr.CatchAll, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{Body: body})
	}
	stmts = append(stmts, &ast.SwitchStmt{Tag: target.expr, Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMaybeExpect(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("maybe expect missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("maybe expect expects one argument")
	}
	message, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Target.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	stmts := append(target.stmts, message.stmts...)
	stmts = append(stmts, resultDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	if l.isVoidType(expr.Type) {
		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.SelectorExpr{X: resultExpr, Sel: ast.NewIdent("Some")},
			Body: &ast.BlockStmt{},
			Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{message.expr}}}}},
		})
		return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, decls...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: resultExpr, Sel: ast.NewIdent("Some")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: resultExpr, Sel: ast.NewIdent("Value")}}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{message.expr}}}}},
	})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(temp)}, nil
}

func (l *lowerer) lowerMaybeIsNone(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("maybe is_none missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.UnaryExpr{Op: token.NOT, X: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Some")}}}, nil
}

func (l *lowerer) lowerMaybeIsSome(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("maybe is_some missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Some")}}, nil
}

func (l *lowerer) lowerMaybeOr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("maybe or expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	defaultValue, err := l.lowerExprWithExpectedType(fn, expr.Args[0], expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	stmts := append(target.stmts, defaultValue.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Some")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{defaultValue.expr}}}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerResultOr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("result or expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	defaultValue, err := l.lowerExprWithExpectedType(fn, expr.Args[0], expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	stmts := append(target.stmts, defaultValue.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{defaultValue.expr}}}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMaybeMap(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("maybe map expects target and callback")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	callback, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}
	var valueExpr ast.Expr = call
	if l.isVoidType(expr.Type) || isVoidExpr(call) {
		valueExpr = voidValueExpr()
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Some")},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: valueExpr},
					&ast.KeyValueExpr{Key: ast.NewIdent("Some"), Value: ast.NewIdent("true")},
				}}},
			},
		}},
		Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType}}},
		}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMaybeAndThen(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("maybe and_then expects target and callback")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	callback, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Some")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType}}}}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerResultIsOk(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("result is_ok missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Ok")}}, nil
}

func (l *lowerer) lowerResultIsErr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("result is_err missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.UnaryExpr{Op: token.NOT, X: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Ok")}}}, nil
}

func (l *lowerer) lowerResultMap(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("result map expects target and callback")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	callback, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}
	var valueExpr ast.Expr = call
	if l.isVoidType(expr.Type) || isVoidExpr(call) {
		valueExpr = voidValueExpr()
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: valueExpr},
					&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: ast.NewIdent("true")},
				}}},
			},
		}},
		Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}},
				}}},
			},
		}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerResultMapErr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("result map_err expects target and callback")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	callback, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}}}
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}},
					&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: ast.NewIdent("true")},
				}}},
			},
		}},
		Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: call},
				}}},
			},
		}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerResultAndThen(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("result and_then expects target and callback")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	callback, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
		}},
		Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}},
				}}},
			},
		}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMatchResult(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("result match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	okName := localName(fn, expr.OkLocal)
	errName := localName(fn, expr.ErrLocal)
	l.declaredLocals[expr.OkLocal] = true
	l.declaredLocals[expr.ErrLocal] = true
	okBind := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(okName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}
	errBind := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}}}
	okBody, err := l.lowerValueBlock(fn, expr.Ok, expr.Type, assignTarget)
	if err != nil {
		return loweredExpr{}, err
	}
	okBody = append([]ast.Stmt{okBind, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(okName)}}}, okBody...)
	errBody, err := l.lowerValueBlock(fn, expr.Err, expr.Type, assignTarget)
	if err != nil {
		return loweredExpr{}, err
	}
	errBody = append([]ast.Stmt{errBind, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(errName)}}}, errBody...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: okBody},
		Else: &ast.BlockStmt{List: errBody},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerResultExpect(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("result expect missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("result expect expects one argument")
	}
	message, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Target.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	panicMsg := &ast.BinaryExpr{X: message.expr, Op: token.ADD, Y: &ast.BinaryExpr{X: &ast.BasicLit{Kind: token.STRING, Value: `": "`}, Op: token.ADD, Y: &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{&ast.SelectorExpr{X: resultExpr, Sel: ast.NewIdent("Err")}}}}}
	stmts := append(target.stmts, message.stmts...)
	stmts = append(stmts, resultDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	if l.isVoidType(expr.Type) {
		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.SelectorExpr{X: resultExpr, Sel: ast.NewIdent("Ok")},
			Body: &ast.BlockStmt{},
			Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{panicMsg}}}}},
		})
		return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, decls...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: resultExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: resultExpr, Sel: ast.NewIdent("Value")}}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{panicMsg}}}}},
	})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(temp)}, nil
}

func (l *lowerer) lowerTryResult(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("try result missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	var resultExpr ast.Expr = ast.NewIdent("nil")
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		resultExpr = ast.NewIdent(temp)
		assignTarget = resultExpr
	}
	okBody := []ast.Stmt{}
	if assignTarget != nil {
		okBody = append(okBody, &ast.AssignStmt{Lhs: []ast.Expr{assignTarget}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}})
		if expr.HasCatch {
			okBody = append(okBody, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{assignTarget}})
		}
	} else {
		okBody = append(okBody, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}})
	}
	var elseBody []ast.Stmt
	if expr.HasCatch {
		catchTargetName := l.nextTemp()
		catchDecls, err := l.declareTemp(fn.Signature.Return, catchTargetName)
		if err != nil {
			return loweredExpr{}, err
		}
		catchTarget := ast.NewIdent(catchTargetName)
		errName := localName(fn, expr.CatchLocal)
		l.declaredLocals[expr.CatchLocal] = true
		errBind := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}}}
		catchBody, err := l.lowerValueBlock(fn, expr.Catch, fn.Signature.Return, catchTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		elseBody = append(catchDecls, errBind, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(errName)}})
		elseBody = append(elseBody, catchBody...)
		if !l.isVoidType(fn.Signature.Return) {
			elseBody = append(elseBody, &ast.ReturnStmt{Results: []ast.Expr{catchTarget}})
		} else {
			elseBody = append(elseBody, &ast.ReturnStmt{})
		}
	} else {
		returnExpr := ast.Expr(targetExpr)
		if fn.Signature.Return != expr.Target.Type {
			returnType, err := l.goType(fn.Signature.Return)
			if err != nil {
				return loweredExpr{}, err
			}
			returnExpr = &ast.CompositeLit{Type: returnType, Elts: []ast.Expr{
				&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}},
			}}
		}
		elseBody = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{returnExpr}}}
	}
	stmts = append(stmts, &ast.IfStmt{Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")}, Body: &ast.BlockStmt{List: okBody}, Else: &ast.BlockStmt{List: elseBody}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerTryMaybe(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("try maybe missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTypeID := l.resolvedExprType(fn, *expr.Target)
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(targetTypeID, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	resultTypeID := expr.Type
	if l.isVoidType(resultTypeID) {
		if inferred := l.inferTypeFromExprShape(&expr); inferred != air.NoType {
			resultTypeID = inferred
		}
	}
	var resultExpr ast.Expr = ast.NewIdent("nil")
	var assignTarget ast.Expr
	if !l.isVoidType(resultTypeID) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(resultTypeID, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		resultExpr = ast.NewIdent(temp)
		assignTarget = resultExpr
	}
	someBody := []ast.Stmt{}
	if assignTarget != nil {
		someBody = append(someBody, &ast.AssignStmt{Lhs: []ast.Expr{assignTarget}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}})
		if expr.HasCatch {
			someBody = append(someBody, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{assignTarget}})
		}
	} else {
		someBody = append(someBody, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}})
	}
	var noneBody []ast.Stmt
	if expr.HasCatch {
		catchTargetName := l.nextTemp()
		catchDecls, err := l.declareTemp(fn.Signature.Return, catchTargetName)
		if err != nil {
			return loweredExpr{}, err
		}
		catchTarget := ast.NewIdent(catchTargetName)
		catchBody, err := l.lowerValueBlock(fn, expr.Catch, fn.Signature.Return, catchTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		noneBody = append(catchDecls, catchBody...)
		if !l.isVoidType(fn.Signature.Return) {
			noneBody = append(noneBody, &ast.ReturnStmt{Results: []ast.Expr{catchTarget}})
		} else {
			noneBody = append(noneBody, &ast.ReturnStmt{})
		}
	} else {
		returnExpr := ast.Expr(targetExpr)
		if fn.Signature.Return != targetTypeID {
			returnType, err := l.goType(fn.Signature.Return)
			if err != nil {
				return loweredExpr{}, err
			}
			returnExpr = &ast.CompositeLit{Type: returnType}
		}
		noneBody = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{returnExpr}}}
	}
	stmts = append(stmts, &ast.IfStmt{Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Some")}, Body: &ast.BlockStmt{List: someBody}, Else: &ast.BlockStmt{List: noneBody}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMatchMaybe(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("maybe match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTypeID := l.resolvedExprType(fn, *expr.Target)
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(targetTypeID, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	someName := localName(fn, expr.SomeLocal)
	l.declaredLocals[expr.SomeLocal] = true
	someDecl := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(someName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}
	someBody, err := l.lowerValueBlock(fn, expr.Some, expr.Type, assignTarget)
	if err != nil {
		return loweredExpr{}, err
	}
	someBody = append([]ast.Stmt{someDecl, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(someName)}}}, someBody...)
	var noneBody []ast.Stmt
	if l.shouldPropagateMaybeNone(expr) {
		noneBody = nil
	} else {
		noneBody, err = l.lowerValueBlock(fn, expr.None, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Some")},
		Body: &ast.BlockStmt{List: someBody},
		Else: &ast.BlockStmt{List: noneBody},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMakeList(fn air.Function, expr air.Expr) (loweredExpr, error) {
	typ, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	elts := make([]ast.Expr, 0, len(expr.Args))
	stmts := []ast.Stmt{}
	for _, arg := range expr.Args {
		loweredArg, err := l.lowerExpr(fn, arg)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, loweredArg.stmts...)
		elts = append(elts, loweredArg.expr)
	}
	return loweredExpr{stmts: stmts, expr: &ast.CompositeLit{Type: typ, Elts: elts}}, nil
}

func (l *lowerer) lowerSpawnFiber(fn air.Function, expr air.Expr) (loweredExpr, error) {
	l.markRuntimeHelper("fiber")
	var targetExpr ast.Expr
	stmts := []ast.Stmt{}
	if expr.Target != nil {
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, target.stmts...)
		targetExpr = target.expr
		if validTypeID(l.program, expr.Type) {
			fiberType := l.program.Types[expr.Type-1]
			if validTypeID(l.program, fiberType.Elem) && l.program.Types[fiberType.Elem-1].Kind == air.TypeVoid {
				targetExpr = &ast.FuncLit{
					Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: voidTypeExpr()}}}},
					Body: &ast.BlockStmt{List: []ast.Stmt{
						&ast.ExprStmt{X: &ast.CallExpr{Fun: target.expr}},
						&ast.ReturnStmt{Results: []ast.Expr{voidValueExpr()}},
					}},
				}
			}
		}
	} else {
		if !validFunctionID(l.program, expr.Function) {
			return loweredExpr{}, fmt.Errorf("invalid fiber function %d", expr.Function)
		}
		targetFn := l.program.Functions[expr.Function]
		if l.isVoidType(targetFn.Signature.Return) {
			targetExpr = &ast.FuncLit{
				Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: voidTypeExpr()}}}},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent(functionName(l.program, targetFn))}},
					&ast.ReturnStmt{Results: []ast.Expr{voidValueExpr()}},
				}},
			}
		} else {
			targetExpr = &ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: mustTypeExpr(l, targetFn.Signature.Return)}}}}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent(functionName(l.program, targetFn))}}}}}}
		}
	}
	return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: &ast.IndexExpr{X: ast.NewIdent("ardSpawnFiber"), Index: mustTypeExpr(l, l.program.Types[expr.Type-1].Elem)}, Args: []ast.Expr{targetExpr}}}, nil
}

func (l *lowerer) lowerFiberGet(fn air.Function, expr air.Expr) (loweredExpr, error) {
	l.markRuntimeHelper("fiber")
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("fiber get missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: &ast.IndexExpr{X: ast.NewIdent("ardGetFiber"), Index: mustTypeExpr(l, expr.Type)}, Args: []ast.Expr{target.expr}}}, nil
}

func (l *lowerer) lowerFiberJoin(fn air.Function, expr air.Expr) (loweredExpr, error) {
	l.markRuntimeHelper("fiber")
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("fiber join missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	fiberType := l.program.Types[expr.Target.Type-1]
	return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: &ast.IndexExpr{X: ast.NewIdent("ardJoinFiber"), Index: mustTypeExpr(l, fiberType.Elem)}, Args: []ast.Expr{target.expr}}}, nil
}

func (l *lowerer) lowerMakeClosure(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if !validFunctionID(l.program, expr.Function) {
		return loweredExpr{}, fmt.Errorf("invalid closure function %d", expr.Function)
	}
	closureFn := l.program.Functions[expr.Function]
	closureType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	funcType, _ := closureType.(*ast.FuncType)
	callArgs := make([]ast.Expr, 0, len(expr.CaptureLocals)+len(closureFn.Signature.Params))
	stmts := []ast.Stmt{}
	for _, local := range expr.CaptureLocals {
		callArgs = append(callArgs, ast.NewIdent(localName(fn, local)))
	}
	params := []*ast.Field{}
	for i, param := range closureFn.Signature.Params {
		paramType, err := l.goParamType(param)
		if err != nil {
			return loweredExpr{}, err
		}
		name := sanitizeName(param.Name)
		if name == "" {
			name = fmt.Sprintf("arg_%d", i)
		}
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(name)}, Type: paramType})
		callArgs = append(callArgs, ast.NewIdent(name))
	}
	bodyStmts := []ast.Stmt{}
	call := &ast.CallExpr{Fun: ast.NewIdent(functionName(l.program, closureFn)), Args: callArgs}
	if funcType == nil {
		funcType = &ast.FuncType{Params: &ast.FieldList{List: params}}
	} else {
		funcType = &ast.FuncType{Params: &ast.FieldList{List: params}, Results: funcType.Results}
	}
	if (funcType.Results == nil || len(funcType.Results.List) == 0) && closureFn.Body.Result != nil && !l.isVoidType(closureFn.Body.Result.Type) {
		returnType, err := l.goType(closureFn.Body.Result.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		funcType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
	}
	if funcType.Results == nil || len(funcType.Results.List) == 0 {
		bodyStmts = append(bodyStmts, &ast.ExprStmt{X: call})
	} else {
		bodyStmts = append(bodyStmts, &ast.ReturnStmt{Results: []ast.Expr{call}})
	}
	funcLit := &ast.FuncLit{Type: funcType, Body: &ast.BlockStmt{List: bodyStmts}}
	return loweredExpr{stmts: stmts, expr: funcLit}, nil
}

func (l *lowerer) lowerCallClosure(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("call closure missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	args := make([]ast.Expr, 0, len(expr.Args))
	stmts := append([]ast.Stmt{}, target.stmts...)
	for _, arg := range expr.Args {
		loweredArg, err := l.lowerExpr(fn, arg)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, loweredArg.stmts...)
		args = append(args, loweredArg.expr)
	}
	return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: target.expr, Args: args}}, nil
}

func (l *lowerer) lowerListSet(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 2 {
		return loweredExpr{}, fmt.Errorf("list set expects target and two args")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	index, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	value, err := l.lowerExpr(fn, expr.Args[1])
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(target.stmts, index.stmts...)
	stmts = append(stmts, value.stmts...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: target.expr, Index: index.expr}}, Tok: token.ASSIGN, Rhs: []ast.Expr{value.expr}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent("true")}, nil
}

func (l *lowerer) lowerListSwap(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 2 {
		return loweredExpr{}, fmt.Errorf("list swap expects target and two indexes")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	left, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	right, err := l.lowerExpr(fn, expr.Args[1])
	if err != nil {
		return loweredExpr{}, err
	}
	leftName := l.nextTemp()
	rightName := l.nextTemp()
	stmts := append(target.stmts, left.stmts...)
	stmts = append(stmts, right.stmts...)
	stmts = append(stmts,
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(leftName)}, Tok: token.DEFINE, Rhs: []ast.Expr{left.expr}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(rightName)}, Tok: token.DEFINE, Rhs: []ast.Expr{right.expr}},
		&ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: target.expr, Index: ast.NewIdent(leftName)}, &ast.IndexExpr{X: target.expr, Index: ast.NewIdent(rightName)}}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.IndexExpr{X: target.expr, Index: ast.NewIdent(rightName)}, &ast.IndexExpr{X: target.expr, Index: ast.NewIdent(leftName)}}},
	)
	return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
}

func (l *lowerer) lowerListPrepend(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("list prepend expects target and value")
	}
	if expr.Target.Kind != air.ExprLoadLocal {
		return loweredExpr{}, fmt.Errorf("list prepend currently requires local target")
	}
	value, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	if !validTypeID(l.program, expr.Target.Type) {
		return loweredExpr{}, fmt.Errorf("invalid list prepend target type")
	}
	listInfo := l.program.Types[expr.Target.Type-1]
	if listInfo.Kind != air.TypeList {
		return loweredExpr{}, fmt.Errorf("list prepend target type kind %d", listInfo.Kind)
	}
	elemType, err := l.goType(listInfo.Elem)
	if err != nil {
		return loweredExpr{}, err
	}
	name := localName(fn, expr.Target.Local)
	assign := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{&ast.CompositeLit{Type: &ast.ArrayType{Elt: elemType}, Elts: []ast.Expr{value.expr}}, ast.NewIdent(name)}, Ellipsis: 2}}}
	stmts := append([]ast.Stmt{}, value.stmts...)
	stmts = append(stmts, assign)
	return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{ast.NewIdent(name)}}}, nil
}

func (l *lowerer) lowerListSort(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("list sort expects target and comparator")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	cmp, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	l.currentImports["sort"] = "sort"
	lessFunc := &ast.FuncLit{
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent("i")}, Type: ast.NewIdent("int")},
				{Names: []*ast.Ident{ast.NewIdent("j")}, Type: ast.NewIdent("int")},
			}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("bool")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: cmp.expr, Args: []ast.Expr{
				&ast.IndexExpr{X: target.expr, Index: ast.NewIdent("i")},
				&ast.IndexExpr{X: target.expr, Index: ast.NewIdent("j")},
			}}}},
		}},
	}
	stmts := append(target.stmts, cmp.stmts...)
	stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("sort"), Sel: ast.NewIdent("SliceStable")}, Args: []ast.Expr{target.expr, lessFunc}}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
}

func (l *lowerer) lowerListPush(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("list push missing target")
	}
	if expr.Target.Kind != air.ExprLoadLocal {
		return loweredExpr{}, fmt.Errorf("list push currently requires local target")
	}
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("list push expects one arg")
	}
	value, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	name := localName(fn, expr.Target.Local)
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(name)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{ast.NewIdent(name), value.expr}}},
	}
	stmts := append([]ast.Stmt{}, value.stmts...)
	stmts = append(stmts, assign)
	return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{ast.NewIdent(name)}}}, nil
}

func (l *lowerer) lowerMakeMap(fn air.Function, expr air.Expr) (loweredExpr, error) {
	typ, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	elts := make([]ast.Expr, 0, len(expr.Entries))
	stmts := []ast.Stmt{}
	for _, entry := range expr.Entries {
		key, err := l.lowerExpr(fn, entry.Key)
		if err != nil {
			return loweredExpr{}, err
		}
		value, err := l.lowerExpr(fn, entry.Value)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, key.stmts...)
		stmts = append(stmts, value.stmts...)
		elts = append(elts, &ast.KeyValueExpr{Key: key.expr, Value: value.expr})
	}
	return loweredExpr{stmts: stmts, expr: &ast.CompositeLit{Type: typ, Elts: elts}}, nil
}

func (l *lowerer) lowerMapHas(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("map has expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	key, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	okName := l.nextTemp()
	stmts := append(target.stmts, key.stmts...)
	stmts = append(stmts, decls...)
	lookup := &ast.IndexExpr{X: target.expr, Index: key.expr}
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent(okName)}, Tok: token.DEFINE, Rhs: []ast.Expr{lookup}})
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(okName)}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(temp)}, nil
}

func (l *lowerer) lowerMapGet(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("map get expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	key, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	valueTemp := l.nextTemp()
	okName := l.nextTemp()
	lookup := &ast.IndexExpr{X: target.expr, Index: key.expr}
	stmts := append(target.stmts, key.stmts...)
	stmts = append(stmts, decls...)
	stmts = append(stmts, &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp), ast.NewIdent(okName)}, Tok: token.DEFINE, Rhs: []ast.Expr{lookup}},
		Cond: ast.NewIdent(okName),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CompositeLit{Type: mustTypeExpr(l, expr.Type), Elts: []ast.Expr{
				&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: ast.NewIdent(valueTemp)},
				&ast.KeyValueExpr{Key: ast.NewIdent("Some"), Value: ast.NewIdent("true")},
			}}}},
		}},
	})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(temp)}, nil
}

func (l *lowerer) lowerMapSet(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 2 {
		return loweredExpr{}, fmt.Errorf("map set expects target and two args")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	key, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	value, err := l.lowerExpr(fn, expr.Args[1])
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(target.stmts, key.stmts...)
	stmts = append(stmts, value.stmts...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: target.expr, Index: key.expr}}, Tok: token.ASSIGN, Rhs: []ast.Expr{value.expr}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent("true")}, nil
}

func (l *lowerer) lowerMapDrop(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("map drop expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	key, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(target.stmts, key.stmts...)
	stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("delete"), Args: []ast.Expr{target.expr, key.expr}}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
}

func (l *lowerer) lowerMapKeys(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("map keys missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	helper, err := l.mapKeyHelper(expr.Target.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: ast.NewIdent(helper), Args: []ast.Expr{target.expr}}}, nil
}

func (l *lowerer) lowerMapKeyAt(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("map key_at expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	index, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	helper, err := l.mapKeyHelper(expr.Target.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	keys := &ast.CallExpr{Fun: ast.NewIdent(helper), Args: []ast.Expr{target.expr}}
	stmts := append(target.stmts, index.stmts...)
	return loweredExpr{stmts: stmts, expr: &ast.IndexExpr{X: keys, Index: index.expr}}, nil
}

func (l *lowerer) lowerMapValueAt(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("map value_at expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	index, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	helper, err := l.mapKeyHelper(expr.Target.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	keyExpr := &ast.IndexExpr{X: &ast.CallExpr{Fun: ast.NewIdent(helper), Args: []ast.Expr{target.expr}}, Index: index.expr}
	stmts := append(target.stmts, index.stmts...)
	return loweredExpr{stmts: stmts, expr: &ast.IndexExpr{X: target.expr, Index: keyExpr}}, nil
}

func (l *lowerer) mapKeyHelper(typeID air.TypeID) (string, error) {
	if !validTypeID(l.program, typeID) {
		return "", fmt.Errorf("invalid map type %d", typeID)
	}
	info := l.program.Types[typeID-1]
	if info.Kind != air.TypeMap {
		return "", fmt.Errorf("type %s is not a map", info.Name)
	}
	keyType := l.program.Types[info.Key-1]
	switch keyType.Kind {
	case air.TypeInt:
		l.markRuntimeHelper("sorted_int_keys")
		return "ardSortedIntKeys", nil
	case air.TypeStr:
		l.markRuntimeHelper("sorted_string_keys")
		return "ardSortedStringKeys", nil
	case air.TypeDynamic, air.TypeExtern:
		l.markRuntimeHelper("sorted_any_keys")
		return "ardSortedAnyKeys", nil
	default:
		return "", fmt.Errorf("unsupported map key type %s for ordered iteration", keyType.Name)
	}
}

func mustTypeExpr(l *lowerer, typeID air.TypeID) ast.Expr {
	typ, err := l.goType(typeID)
	if err != nil {
		panic(err)
	}
	return typ
}

func (l *lowerer) lowerTraitCall(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("trait call missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if expr.Trait < 0 || int(expr.Trait) >= len(l.program.Traits) {
		return loweredExpr{}, fmt.Errorf("invalid trait id %d", expr.Trait)
	}
	trait := l.program.Traits[expr.Trait]
	if expr.Method < 0 || expr.Method >= len(trait.Methods) {
		return loweredExpr{}, fmt.Errorf("invalid trait method %d for %s", expr.Method, trait.Name)
	}
	method := trait.Methods[expr.Method]
	switch {
	case trait.Name == "ToString" && method.Name == "to_str":
		if validTypeID(l.program, expr.Target.Type) && l.program.Types[expr.Target.Type-1].Kind == air.TypeTraitObject {
			return l.lowerTraitObjectToString(target, expr.Trait, expr.Method)
		}
		return loweredExpr{stmts: target.stmts, expr: l.toStringExpr(expr.Target.Type, target.expr)}, nil
	default:
		return loweredExpr{}, fmt.Errorf("unsupported trait call %s.%s", trait.Name, method.Name)
	}
}

func (l *lowerer) lowerTraitObjectToString(target loweredExpr, traitID air.TraitID, methodIndex int) (loweredExpr, error) {
	resultTemp := l.nextTemp()
	stmts := append([]ast.Stmt{}, target.stmts...)
	stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(resultTemp)}, Type: ast.NewIdent("string")}}}})
	cases := []ast.Stmt{}
	for _, impl := range l.program.Impls {
		if impl.Trait != traitID || methodIndex >= len(impl.Methods) || !validTypeID(l.program, impl.ForType) {
			continue
		}
		methodFn := l.program.Functions[impl.Methods[methodIndex]]
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{mustTypeExpr(l, impl.ForType)},
			Body: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent(functionName(l.program, methodFn)), Args: []ast.Expr{ast.NewIdent("typed")}}}}},
		})
	}
	if len(cases) == 0 {
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{target.expr}}}, nil
	}
	cases = append(cases, &ast.CaseClause{Body: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{target.expr}}}}}})
	stmts = append(stmts, &ast.TypeSwitchStmt{Assign: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("typed")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: target.expr}}}, Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(resultTemp)}, nil
}

func exportedFieldName(name string) string {
	if name == "" {
		return ""
	}
	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func (l *lowerer) goFieldName(typ air.TypeInfo, fieldName string) string {
	if l.isStdlibFFIBackedType(typ) {
		return exportedFieldName(fieldName)
	}
	return fieldName
}

func (l *lowerer) wrapStdlibMaybeCall(maybeTypeID air.TypeID, call ast.Expr) (loweredExpr, error) {
	maybeType, err := l.goType(maybeTypeID)
	if err != nil {
		return loweredExpr{}, err
	}
	info := l.program.Types[maybeTypeID-1]
	elemType, err := l.goType(info.Elem)
	if err != nil {
		return loweredExpr{}, err
	}
	temp := l.nextTemp()
	stdlibMaybeType := &ast.IndexExpr{X: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Maybe"), Index: elemType}
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(temp)}, Type: stdlibMaybeType}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
	}
	return loweredExpr{stmts: stmts, expr: &ast.CompositeLit{Type: maybeType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: &ast.SelectorExpr{X: ast.NewIdent(temp), Sel: ast.NewIdent("Value")}},
		&ast.KeyValueExpr{Key: ast.NewIdent("Some"), Value: &ast.SelectorExpr{X: ast.NewIdent(temp), Sel: ast.NewIdent("Some")}},
	}}}, nil
}

func (l *lowerer) stdlibMaybeExpr(typeID air.TypeID, expr ast.Expr) (ast.Expr, error) {
	if !validTypeID(l.program, typeID) {
		return nil, fmt.Errorf("invalid maybe type id %d", typeID)
	}
	info := l.program.Types[typeID-1]
	if info.Kind != air.TypeMaybe {
		return nil, fmt.Errorf("expected maybe type, got kind %d", info.Kind)
	}
	elemType, err := l.goType(info.Elem)
	if err != nil {
		return nil, err
	}
	stdlibMaybeType := &ast.IndexExpr{X: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Maybe"), Index: elemType}
	return &ast.CompositeLit{Type: stdlibMaybeType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: &ast.SelectorExpr{X: expr, Sel: ast.NewIdent("Value")}},
		&ast.KeyValueExpr{Key: ast.NewIdent("Some"), Value: &ast.SelectorExpr{X: expr, Sel: ast.NewIdent("Some")}},
	}}, nil
}

func (l *lowerer) convertStdlibError(typeID air.TypeID, expr ast.Expr) (ast.Expr, error) {
	if !validTypeID(l.program, typeID) {
		return nil, fmt.Errorf("invalid error type id %d", typeID)
	}
	info := l.program.Types[typeID-1]
	if info.Kind == air.TypeStr {
		return &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{expr}}, nil
	}
	if info.Kind != air.TypeStruct {
		return nil, fmt.Errorf("unsupported stdlib error target kind %d", info.Kind)
	}
	elts := make([]ast.Expr, 0, len(info.Fields))
	for _, field := range info.Fields {
		elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(field.Name), Value: &ast.SelectorExpr{X: expr, Sel: ast.NewIdent(exportedFieldName(field.Name))}})
	}
	return &ast.CompositeLit{Type: ast.NewIdent(typeName(l.program, info)), Elts: elts}, nil
}

func (l *lowerer) wrapValueErrorCall(resultTypeID air.TypeID, call ast.Expr) (loweredExpr, error) {
	if !validTypeID(l.program, resultTypeID) {
		return loweredExpr{}, fmt.Errorf("invalid result type id %d", resultTypeID)
	}
	resultType := l.program.Types[resultTypeID-1]
	if resultType.Kind != air.TypeResult {
		return loweredExpr{}, fmt.Errorf("expected result type, got kind %d", resultType.Kind)
	}
	valueType, err := l.goType(resultType.Value)
	if err != nil {
		return loweredExpr{}, err
	}
	valueTemp := l.nextTemp()
	errTemp := l.nextTemp()
	decls := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(valueTemp)}, Type: valueType}}}},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(errTemp)}, Type: ast.NewIdent("error")}}}},
	}
	stmts := append([]ast.Stmt{}, decls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp), ast.NewIdent(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
	errExpr, err := l.convertStdlibError(resultType.Error, ast.NewIdent(errTemp))
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := &ast.CompositeLit{Type: mustTypeExpr(l, resultTypeID), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: ast.NewIdent(valueTemp)},
		&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: errExpr},
		&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: &ast.BinaryExpr{X: ast.NewIdent(errTemp), Op: token.EQL, Y: ast.NewIdent("nil")}},
	}}
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) wrapErrorCall(resultTypeID air.TypeID, call ast.Expr) (loweredExpr, error) {
	if !validTypeID(l.program, resultTypeID) {
		return loweredExpr{}, fmt.Errorf("invalid result type id %d", resultTypeID)
	}
	resultType := l.program.Types[resultTypeID-1]
	if resultType.Kind != air.TypeResult {
		return loweredExpr{}, fmt.Errorf("expected result type, got kind %d", resultType.Kind)
	}
	if !validTypeID(l.program, resultType.Value) || l.program.Types[resultType.Value-1].Kind != air.TypeVoid {
		return loweredExpr{}, fmt.Errorf("expected void result value, got type %d", resultType.Value)
	}
	errTemp := l.nextTemp()
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(errTemp)}, Type: ast.NewIdent("error")}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
	}
	errExpr, err := l.convertStdlibError(resultType.Error, ast.NewIdent(errTemp))
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := &ast.CompositeLit{Type: mustTypeExpr(l, resultTypeID), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: voidValueExpr()},
		&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: errExpr},
		&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: &ast.BinaryExpr{X: ast.NewIdent(errTemp), Op: token.EQL, Y: ast.NewIdent("nil")}},
	}}
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerUnionArgToAny(expr ast.Expr, typeID air.TypeID) (loweredExpr, error) {
	if !validTypeID(l.program, typeID) {
		return loweredExpr{}, fmt.Errorf("invalid union type id %d", typeID)
	}
	info := l.program.Types[typeID-1]
	if info.Kind != air.TypeUnion {
		return loweredExpr{expr: expr}, nil
	}
	temp := l.nextTemp()
	wrappedExpr := expr
	if _, ok := expr.(*ast.CompositeLit); ok {
		wrappedExpr = &ast.ParenExpr{X: expr}
	}
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(temp)}, Type: ast.NewIdent("any")}}}},
	}
	cases := make([]ast.Stmt, 0, len(info.Members))
	for _, member := range info.Members {
		fieldName := unionMemberFieldName(member)
		valueExpr := ast.Expr(&ast.SelectorExpr{X: wrappedExpr, Sel: ast.NewIdent(fieldName)})
		if validTypeID(l.program, member.Type) && l.program.Types[member.Type-1].Kind == air.TypeVoid {
			valueExpr = ast.NewIdent("nil")
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", member.Tag)}},
			Body: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}}},
		})
	}
	stmts = append(stmts, &ast.SwitchStmt{Tag: &ast.SelectorExpr{X: wrappedExpr, Sel: ast.NewIdent("tag")}, Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(temp)}, nil
}

func (l *lowerer) lowerUnionSliceArgToAny(expr ast.Expr, typeID air.TypeID) (loweredExpr, error) {
	if !validTypeID(l.program, typeID) {
		return loweredExpr{}, fmt.Errorf("invalid list type id %d", typeID)
	}
	listInfo := l.program.Types[typeID-1]
	if listInfo.Kind != air.TypeList || !validTypeID(l.program, listInfo.Elem) {
		return loweredExpr{expr: expr}, nil
	}
	elemInfo := l.program.Types[listInfo.Elem-1]
	if elemInfo.Kind != air.TypeUnion {
		l.markRuntimeHelper("list_to_any_slice")
		return loweredExpr{expr: &ast.CallExpr{Fun: ast.NewIdent("ardListToAnySlice"), Args: []ast.Expr{expr}}}, nil
	}
	valueTemp := l.nextTemp()
	indexTemp := l.nextTemp()
	outTemp := l.nextTemp()
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(outTemp)}, Type: &ast.ArrayType{Elt: ast.NewIdent("any")}}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(outTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("make"), Args: []ast.Expr{&ast.ArrayType{Elt: ast.NewIdent("any")}, &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{expr}}}}}},
		&ast.RangeStmt{Key: ast.NewIdent(indexTemp), Value: ast.NewIdent(valueTemp), Tok: token.DEFINE, X: expr, Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.SwitchStmt{Tag: &ast.SelectorExpr{X: ast.NewIdent(valueTemp), Sel: ast.NewIdent("tag")}, Body: &ast.BlockStmt{List: unionSliceCaseClauses(l.program, elemInfo, outTemp, indexTemp, valueTemp)}},
		}}},
	}
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(outTemp)}, nil
}

func unionSliceCaseClauses(program *air.Program, unionInfo air.TypeInfo, outTemp string, indexTemp string, valueTemp string) []ast.Stmt {
	cases := make([]ast.Stmt, 0, len(unionInfo.Members))
	for _, member := range unionInfo.Members {
		valueExpr := ast.Expr(&ast.SelectorExpr{X: ast.NewIdent(valueTemp), Sel: ast.NewIdent(unionMemberFieldName(member))})
		if validTypeID(program, member.Type) && program.Types[member.Type-1].Kind == air.TypeVoid {
			valueExpr = ast.NewIdent("nil")
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", member.Tag)}},
			Body: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: ast.NewIdent(outTemp), Index: ast.NewIdent(indexTemp)}}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}}},
		})
	}
	return cases
}

func (l *lowerer) lowerHTTPServeExtern(args []ast.Expr, handlerMapType air.TypeID, resultTypeID air.TypeID) (loweredExpr, error) {
	if len(args) != 2 {
		return loweredExpr{}, fmt.Errorf("HTTP_Serve expects 2 args")
	}
	if !validTypeID(l.program, handlerMapType) {
		return loweredExpr{}, fmt.Errorf("invalid handler map type %d", handlerMapType)
	}
	mapInfo := l.program.Types[handlerMapType-1]
	if mapInfo.Kind != air.TypeMap || !validTypeID(l.program, mapInfo.Value) {
		return loweredExpr{}, fmt.Errorf("HTTP_Serve handlers must be a map of functions")
	}
	fnInfo := l.program.Types[mapInfo.Value-1]
	if fnInfo.Kind != air.TypeFunction || len(fnInfo.Params) != 2 {
		return loweredExpr{}, fmt.Errorf("HTTP_Serve handler signature mismatch")
	}
	reqType := l.program.Types[fnInfo.Params[0]-1]
	resType := l.program.Types[fnInfo.Params[1]-1]

	callbackType := &ast.FuncType{
		Params: &ast.FieldList{List: []*ast.Field{
			{Type: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Request")},
			{Type: &ast.StarExpr{X: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Response")}},
		}},
		Results: &ast.FieldList{List: []*ast.Field{{Type: voidTypeExpr()}, {Type: ast.NewIdent("error")}}},
	}
	adapterType := &ast.MapType{Key: ast.NewIdent("string"), Value: callbackType}
	adapterName := l.nextTemp()
	pathName := l.nextTemp()
	handlerName := l.nextTemp()
	reqName := l.nextTemp()
	resName := l.nextTemp()
	ardReqName := l.nextTemp()
	ardResName := l.nextTemp()
	errName := l.nextTemp()

	requestMethodType := ast.NewIdent(typeName(l.program, l.program.Types[reqType.Fields[2].Type-1]))
	requestType := ast.NewIdent(typeName(l.program, reqType))
	responseType := ast.NewIdent(typeName(l.program, resType))

	bodyMaybe := &ast.CompositeLit{Type: mustTypeExpr(l, reqType.Fields[0].Type), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: &ast.SelectorExpr{X: &ast.SelectorExpr{X: ast.NewIdent(reqName), Sel: ast.NewIdent("Body")}, Sel: ast.NewIdent("Value")}},
		&ast.KeyValueExpr{Key: ast.NewIdent("Some"), Value: &ast.SelectorExpr{X: &ast.SelectorExpr{X: ast.NewIdent(reqName), Sel: ast.NewIdent("Body")}, Sel: ast.NewIdent("Some")}},
	}}
	rawMaybe := &ast.CompositeLit{Type: mustTypeExpr(l, reqType.Fields[3].Type), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: &ast.SelectorExpr{X: &ast.SelectorExpr{X: ast.NewIdent(reqName), Sel: ast.NewIdent("Raw")}, Sel: ast.NewIdent("Value")}},
		&ast.KeyValueExpr{Key: ast.NewIdent("Some"), Value: &ast.SelectorExpr{X: &ast.SelectorExpr{X: ast.NewIdent(reqName), Sel: ast.NewIdent("Raw")}, Sel: ast.NewIdent("Some")}},
	}}
	timeoutMaybe := &ast.CompositeLit{Type: mustTypeExpr(l, reqType.Fields[4].Type), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: &ast.SelectorExpr{X: &ast.SelectorExpr{X: ast.NewIdent(reqName), Sel: ast.NewIdent("Timeout")}, Sel: ast.NewIdent("Value")}},
		&ast.KeyValueExpr{Key: ast.NewIdent("Some"), Value: &ast.SelectorExpr{X: &ast.SelectorExpr{X: ast.NewIdent(reqName), Sel: ast.NewIdent("Timeout")}, Sel: ast.NewIdent("Some")}},
	}}
	ardReqLit := &ast.CompositeLit{Type: requestType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(reqType.Fields[0].Name), Value: bodyMaybe},
		&ast.KeyValueExpr{Key: ast.NewIdent(reqType.Fields[1].Name), Value: &ast.SelectorExpr{X: ast.NewIdent(reqName), Sel: ast.NewIdent("Headers")}},
		&ast.KeyValueExpr{Key: ast.NewIdent(reqType.Fields[2].Name), Value: &ast.CallExpr{Fun: requestMethodType, Args: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(reqName), Sel: ast.NewIdent("Method")}}}},
		&ast.KeyValueExpr{Key: ast.NewIdent(reqType.Fields[3].Name), Value: rawMaybe},
		&ast.KeyValueExpr{Key: ast.NewIdent(reqType.Fields[4].Name), Value: timeoutMaybe},
		&ast.KeyValueExpr{Key: ast.NewIdent(reqType.Fields[5].Name), Value: &ast.SelectorExpr{X: ast.NewIdent(reqName), Sel: ast.NewIdent("Url")}},
	}}
	ardResLit := &ast.CompositeLit{Type: responseType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(resType.Fields[0].Name), Value: &ast.SelectorExpr{X: ast.NewIdent(resName), Sel: ast.NewIdent("Body")}},
		&ast.KeyValueExpr{Key: ast.NewIdent(resType.Fields[1].Name), Value: &ast.SelectorExpr{X: ast.NewIdent(resName), Sel: ast.NewIdent("Headers")}},
		&ast.KeyValueExpr{Key: ast.NewIdent(resType.Fields[2].Name), Value: &ast.SelectorExpr{X: ast.NewIdent(resName), Sel: ast.NewIdent("Status")}},
	}}
	stdlibResLit := &ast.CompositeLit{Type: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Response"), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Body"), Value: &ast.SelectorExpr{X: ast.NewIdent(ardResName), Sel: ast.NewIdent(resType.Fields[0].Name)}},
		&ast.KeyValueExpr{Key: ast.NewIdent("Headers"), Value: &ast.SelectorExpr{X: ast.NewIdent(ardResName), Sel: ast.NewIdent(resType.Fields[1].Name)}},
		&ast.KeyValueExpr{Key: ast.NewIdent("Status"), Value: &ast.SelectorExpr{X: ast.NewIdent(ardResName), Sel: ast.NewIdent(resType.Fields[2].Name)}},
	}}
	wrapperFunc := &ast.FuncLit{
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(reqName)}, Type: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Request")},
				{Names: []*ast.Ident{ast.NewIdent(resName)}, Type: &ast.StarExpr{X: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Response")}},
			}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: voidTypeExpr()}, {Type: ast.NewIdent("error")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(ardReqName)}, Tok: token.DEFINE, Rhs: []ast.Expr{ardReqLit}},
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(ardResName)}, Tok: token.DEFINE, Rhs: []ast.Expr{ardResLit}},
			&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent(handlerName), Args: []ast.Expr{ast.NewIdent(ardReqName), &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(ardResName)}}}},
			&ast.AssignStmt{Lhs: []ast.Expr{&ast.StarExpr{X: ast.NewIdent(resName)}}, Tok: token.ASSIGN, Rhs: []ast.Expr{stdlibResLit}},
			&ast.ReturnStmt{Results: []ast.Expr{voidValueExpr(), ast.NewIdent("nil")}},
		}},
	}

	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(adapterName)}, Type: adapterType}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(adapterName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CompositeLit{Type: adapterType}}},
		&ast.RangeStmt{
			Key:   ast.NewIdent(pathName),
			Value: ast.NewIdent(handlerName),
			Tok:   token.DEFINE,
			X:     args[1],
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: ast.NewIdent(adapterName), Index: ast.NewIdent(pathName)}}, Tok: token.ASSIGN, Rhs: []ast.Expr{wrapperFunc}},
			}},
		},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(errName)}, Type: ast.NewIdent("error")}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "HTTPServe"), Args: []ast.Expr{args[0], ast.NewIdent(adapterName)}}}},
	}
	resultType, err := l.goType(resultTypeID)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: stmts, expr: &ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: voidValueExpr()},
		&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{ast.NewIdent(errName)}}},
		&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: &ast.BinaryExpr{X: ast.NewIdent(errName), Op: token.EQL, Y: ast.NewIdent("nil")}},
	}}}, nil
}

func (l *lowerer) wrapStdlibResultCall(resultTypeID air.TypeID, call ast.Expr) (loweredExpr, error) {
	if !validTypeID(l.program, resultTypeID) {
		return loweredExpr{}, fmt.Errorf("invalid result type id %d", resultTypeID)
	}
	resultType := l.program.Types[resultTypeID-1]
	if resultType.Kind != air.TypeResult {
		return loweredExpr{}, fmt.Errorf("expected result type, got kind %d", resultType.Kind)
	}
	valueType, err := l.goType(resultType.Value)
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	stdlibResultType := &ast.IndexListExpr{X: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Result"), Indices: []ast.Expr{valueType, l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Error")}}
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(resultTemp)}, Type: stdlibResultType}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
	}
	errExpr, err := l.convertStdlibError(resultType.Error, &ast.SelectorExpr{X: ast.NewIdent(resultTemp), Sel: ast.NewIdent("Err")})
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := &ast.CompositeLit{Type: mustTypeExpr(l, resultTypeID), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: &ast.SelectorExpr{X: ast.NewIdent(resultTemp), Sel: ast.NewIdent("Value")}},
		&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: errExpr},
		&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: &ast.SelectorExpr{X: ast.NewIdent(resultTemp), Sel: ast.NewIdent("Ok")}},
	}}
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerExternCall(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Extern < 0 || int(expr.Extern) >= len(l.program.Externs) {
		return loweredExpr{}, fmt.Errorf("invalid extern id %d", expr.Extern)
	}
	ext := l.program.Externs[expr.Extern]
	args := make([]ast.Expr, 0, len(expr.Args))
	stmts := []ast.Stmt{}
	for _, arg := range expr.Args {
		loweredArg, err := l.lowerExpr(fn, arg)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, loweredArg.stmts...)
		args = append(args, loweredArg.expr)
	}
	binding := ext.Name
	if goBinding, ok := ext.Bindings["go"]; ok && goBinding != "" {
		binding = goBinding
	}
	switch binding {
	case "Print":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Print"), Args: args}}, nil
	case "FloatFromInt":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FloatFromInt"), Args: args}}, nil
	case "ReadLine":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "ReadLine"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_Exists":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSExists"), Args: args}}, nil
	case "FS_IsFile":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSIsFile"), Args: args}}, nil
	case "FS_IsDir":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSIsDir"), Args: args}}, nil
	case "FS_CreateFile":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSCreateFile"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_WriteFile":
		wrapped, err := l.wrapErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSWriteFile"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_AppendFile":
		wrapped, err := l.wrapErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSAppendFile"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_ReadFile":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSReadFile"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_DeleteFile":
		wrapped, err := l.wrapErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSDeleteFile"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_Copy":
		wrapped, err := l.wrapErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSCopy"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_Rename":
		wrapped, err := l.wrapErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSRename"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_Cwd":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSCwd"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_Abs":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSAbs"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_CreateDir":
		wrapped, err := l.wrapErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSCreateDir"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_DeleteDir":
		wrapped, err := l.wrapErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSDeleteDir"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "FS_ListDir":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FSListDir"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "IntFromStr":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "IntFromStr"), Args: args}}, nil
	case "Sleep":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Sleep"), Args: args}}, nil
	case "FloatFromStr":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FloatFromStr"), Args: args}}, nil
	case "FloatFloor":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "FloatFloor"), Args: args}}, nil
	case "EnvGet":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "EnvGet"), Args: args}}, nil
	case "OsArgs":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "OsArgs"), Args: args}}, nil
	case "Base64Encode":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Base64Encode"), Args: args}}, nil
	case "SqlCreateConnection":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "SqlCreateConnection"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "SqlClose":
		wrapped, err := l.wrapErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "SqlClose"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "SqlExecute":
		sqlArgs := append([]ast.Expr{}, args...)
		if len(sqlArgs) != 3 {
			return loweredExpr{}, fmt.Errorf("SqlExecute expects 3 args")
		}
		connArg, err := l.lowerUnionArgToAny(sqlArgs[0], expr.Args[0].Type)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, connArg.stmts...)
		sqlArgs[0] = connArg.expr
		valueArgs, err := l.lowerUnionSliceArgToAny(sqlArgs[2], expr.Args[2].Type)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, valueArgs.stmts...)
		sqlArgs[2] = valueArgs.expr
		wrapped, err := l.wrapErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "SqlExecute"), Args: sqlArgs})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "SqlQuery":
		sqlArgs := append([]ast.Expr{}, args...)
		if len(sqlArgs) != 3 {
			return loweredExpr{}, fmt.Errorf("SqlQuery expects 3 args")
		}
		connArg, err := l.lowerUnionArgToAny(sqlArgs[0], expr.Args[0].Type)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, connArg.stmts...)
		sqlArgs[0] = connArg.expr
		valueArgs, err := l.lowerUnionSliceArgToAny(sqlArgs[2], expr.Args[2].Type)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, valueArgs.stmts...)
		sqlArgs[2] = valueArgs.expr
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "SqlQuery"), Args: sqlArgs})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "SqlBeginTx":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "SqlBeginTx"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "SqlCommit":
		wrapped, err := l.wrapErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "SqlCommit"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "SqlRollback":
		wrapped, err := l.wrapErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "SqlRollback"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "SqlExtractParams":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "SqlExtractParams"), Args: args}}, nil
	case "CryptoMd5":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "CryptoMd5"), Args: args}}, nil
	case "CryptoSha256":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "CryptoSha256"), Args: args}}, nil
	case "CryptoSha512":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "CryptoSha512"), Args: args}}, nil
	case "CryptoUUID":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "CryptoUUID"), Args: args}}, nil
	case "CryptoHashPassword":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "CryptoHashPassword"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "CryptoVerifyPassword":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "CryptoVerifyPassword"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "CryptoScryptHash":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "CryptoScryptHash"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "CryptoScryptVerify":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "CryptoScryptVerify"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "Base64Decode":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Base64Decode"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "Base64EncodeURL":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Base64EncodeURL"), Args: args}}, nil
	case "Base64DecodeURL":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Base64DecodeURL"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "HexEncode":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "HexEncode"), Args: args}}, nil
	case "HexDecode":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "HexDecode"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "StrToDynamic", "IntToDynamic", "FloatToDynamic", "BoolToDynamic":
		return loweredExpr{stmts: stmts, expr: args[0]}, nil
	case "ListToDynamic":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "ListToDynamic"), Args: args}}, nil
	case "MapToDynamic":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "MapToDynamic"), Args: args}}, nil
	case "JsonEncode":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "JsonEncode"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "VoidToDynamic":
		return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
	case "JsonToDynamic":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "JsonToDynamic"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "IsNil":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "IsNil"), Args: args}}, nil
	case "ExtractField":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "ExtractField"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "DecodeString":
		wrapped, err := l.wrapStdlibResultCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "DecodeString"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "DecodeInt":
		wrapped, err := l.wrapStdlibResultCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "DecodeInt"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "DecodeFloat":
		wrapped, err := l.wrapStdlibResultCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "DecodeFloat"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "DecodeBool":
		wrapped, err := l.wrapStdlibResultCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "DecodeBool"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "DynamicToList":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "DynamicToList"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "DynamicToMap":
		l.markRuntimeHelper("dynamic_to_any_map")
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: ast.NewIdent("ardDynamicToAnyMap"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "HTTP_Do":
		if len(args) != 5 {
			return loweredExpr{}, fmt.Errorf("HTTP_Do expects 5 args")
		}
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "HTTPDo"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "HTTP_ResponseStatus":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "HTTPResponseStatus"), Args: args}}, nil
	case "HTTP_ResponseHeaders":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "HTTPResponseHeaders"), Args: args}}, nil
	case "HTTP_ResponseBody":
		wrapped, err := l.wrapValueErrorCall(expr.Type, &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "HTTPResponseBody"), Args: args})
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	case "HTTP_ResponseClose":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "HTTPResponseClose"), Args: args}}, nil
	case "GetReqPath":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "GetReqPath"), Args: args}}, nil
	case "GetPathValue":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "GetPathValue"), Args: args}}, nil
	case "GetQueryParam":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "GetQueryParam"), Args: args}}, nil
	case "HTTP_Serve":
		wrapped, err := l.lowerHTTPServeExtern(args, expr.Args[1].Type, expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	default:
		return loweredExpr{}, fmt.Errorf("unsupported go extern binding %q", binding)
	}
}

func isVoidExpr(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

func validFunctionID(program *air.Program, id air.FunctionID) bool {
	return id >= 0 && int(id) < len(program.Functions)
}

func validTypeID(program *air.Program, id air.TypeID) bool {
	return id > 0 && int(id) <= len(program.Types)
}
