package go_backend

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/akonwi/ard/checker"
	backendir "github.com/akonwi/ard/go_backend/ir"
	"github.com/akonwi/ard/go_backend/lowering"
)

type backendIREmitter struct {
	packageName         string
	functionNames       map[string]string
	functionReturns     map[string]backendir.Type
	functionTypeParams  map[string]int
	entrypointBlock     *backendir.Block
	localStructNames    map[string]struct{}
	localEnumNames      map[string]struct{}
	localUnionNames     map[string]struct{}
	externTypeNames     map[string]struct{}
	emittedMethods      map[string]struct{}
	scopedCallableNames map[string]struct{}
	typeParamMapping    map[string]string
}

func compileModuleSourceViaBackendIR(module checker.Module, packageName string, entrypoint bool, projectName string) ([]byte, error) {
	if module == nil || module.Program() == nil {
		return nil, fmt.Errorf("module has no program")
	}

	irModule, err := lowering.LowerModuleToBackendIR(module, packageName, entrypoint, projectName)
	if err != nil {
		return nil, err
	}
	return compileLoweredModuleSourceViaBackendIR(irModule, entrypoint)
}

func compileLoweredModuleSourceViaBackendIR(irModule *backendir.Module, entrypoint bool) ([]byte, error) {
	if irModule == nil {
		return nil, fmt.Errorf("nil backend ir module")
	}
	fileIR, err := emitGoFileFromBackendIR(irModule, entrypoint)
	if err != nil {
		return nil, err
	}
	return renderGoFile(optimizeGoFileIR(fileIR))
}

func emitGoFileFromBackendIR(module *backendir.Module, entrypoint bool) (goFileIR, error) {
	if module == nil {
		return goFileIR{}, fmt.Errorf("nil backend ir module")
	}

	emitter := newBackendIREmitter(module)
	fileIR := lowerGoFileIR(module.PackageName, module.Imports)

	for i, decl := range module.Decls {
		astDecls, err := emitter.emitDecls(decl)
		if err != nil {
			return goFileIR{}, fmt.Errorf("decl[%d]: %w", i, err)
		}
		for _, astDecl := range astDecls {
			appendASTDecl(&fileIR, astDecl)
		}
	}

	if entrypoint {
		mainDecl, err := emitter.emitEntrypointMainDecl()
		if err != nil {
			return goFileIR{}, err
		}
		appendASTDecl(&fileIR, mainDecl)
	}

	return fileIR, nil
}

func newBackendIREmitter(module *backendir.Module) *backendIREmitter {
	used := make(map[string]struct{})
	functionNames := make(map[string]string)
	functionReturns := make(map[string]backendir.Type)
	functionTypeParams := make(map[string]int)
	localStructNames := make(map[string]struct{})
	localEnumNames := make(map[string]struct{})
	localUnionNames := make(map[string]struct{})
	externTypeNames := make(map[string]struct{})

	for _, decl := range module.Decls {
		switch d := decl.(type) {
		case *backendir.StructDecl:
			used[goName(d.Name, true)] = struct{}{}
			localStructNames[d.Name] = struct{}{}
		case *backendir.EnumDecl:
			used[goName(d.Name, true)] = struct{}{}
			localEnumNames[d.Name] = struct{}{}
		case *backendir.UnionDecl:
			used[goName(d.Name, true)] = struct{}{}
			localUnionNames[d.Name] = struct{}{}
		case *backendir.ExternTypeDecl:
			used[goName(d.Name, true)] = struct{}{}
			externTypeNames[d.Name] = struct{}{}
		}
	}

	for _, decl := range module.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if !ok {
			continue
		}
		name := goName(fn.Name, !fn.IsPrivate)
		if module.PackageName == "main" && name == "main" {
			name = "ardMain"
		}
		resolved := uniquePackageName(name, used)
		functionNames[fn.Name] = resolved
		functionReturns[fn.Name] = fn.Return
		typeParamOrder, _ := functionTypeParamsFromBackendIR(fn)
		functionTypeParams[fn.Name] = len(typeParamOrder)
	}

	return &backendIREmitter{
		packageName:        module.PackageName,
		functionNames:      functionNames,
		functionReturns:    functionReturns,
		functionTypeParams: functionTypeParams,
		entrypointBlock:    module.Entrypoint,
		localStructNames:   localStructNames,
		localEnumNames:     localEnumNames,
		localUnionNames:    localUnionNames,
		externTypeNames:    externTypeNames,
		emittedMethods:     make(map[string]struct{}),
	}
}

func (e *backendIREmitter) emitDecls(decl backendir.Decl) ([]ast.Decl, error) {
	switch d := decl.(type) {
	case *backendir.StructDecl:
		typeDecl, err := e.emitStructDecl(d)
		if err != nil {
			return nil, err
		}
		decls := []ast.Decl{typeDecl}
		methodDecls, err := e.emitStructMethodDecls(d)
		if err != nil {
			return nil, err
		}
		decls = append(decls, methodDecls...)
		return decls, nil
	case *backendir.EnumDecl:
		decls := []ast.Decl{e.emitEnumDecl(d)}
		methodDecls, err := e.emitEnumMethodDecls(d)
		if err != nil {
			return nil, err
		}
		decls = append(decls, methodDecls...)
		return decls, nil
	case *backendir.UnionDecl:
		return []ast.Decl{e.emitUnionDecl(d)}, nil
	case *backendir.ExternTypeDecl:
		return []ast.Decl{e.emitExternTypeDecl(d)}, nil
	case *backendir.FuncDecl:
		decl, err := e.emitFuncDecl(d)
		if err != nil {
			return nil, err
		}
		return []ast.Decl{decl}, nil
	case *backendir.VarDecl:
		decl, err := e.emitVarDecl(d)
		if err != nil {
			return nil, err
		}
		return []ast.Decl{decl}, nil
	default:
		return nil, fmt.Errorf("unsupported decl %T", decl)
	}
}

func structDeclTypeParamsFromIR(decl *backendir.StructDecl) ([]string, map[string]string, map[string]string) {
	if decl == nil {
		return nil, nil, nil
	}
	order := append([]string(nil), decl.TypeParams...)
	if len(order) == 0 {
		seen := make(map[string]struct{})
		for _, field := range decl.Fields {
			collectBackendIRTypeVars(field.Type, &order, seen)
		}
		for _, method := range decl.Methods {
			collectBackendIRTypeVars(method.Return, &order, seen)
			for _, param := range method.Params {
				collectBackendIRTypeVars(param.Type, &order, seen)
			}
		}
	}
	if len(order) == 0 {
		return nil, nil, nil
	}
	mapping := buildTypeParamMapping(order)
	constraints := make(map[string]string, len(order))
	for _, name := range order {
		constraints[name] = "any"
	}
	return order, mapping, constraints
}

func (e *backendIREmitter) emitStructMethodDecls(decl *backendir.StructDecl) ([]ast.Decl, error) {
	if decl == nil {
		return nil, nil
	}
	order, mapping, _ := structDeclTypeParamsFromIR(decl)
	receiverType := ast.Expr(ast.NewIdent(goName(decl.Name, true)))
	if len(order) > 0 {
		args := make([]ast.Expr, 0, len(order))
		for _, name := range order {
			args = append(args, ast.NewIdent(mapping[name]))
		}
		receiverType = indexExpr(receiverType, args)
	}
	out := make([]ast.Decl, 0, len(decl.Methods))
	for _, method := range decl.Methods {
		if method == nil {
			continue
		}
		methodKey := "struct:" + decl.Name + "." + method.Name
		if _, seen := e.emittedMethods[methodKey]; seen {
			continue
		}
		e.emittedMethods[methodKey] = struct{}{}
		methodDecl, err := e.emitReceiverMethodDecl(decl.Name, receiverType, mapping, method)
		if err != nil {
			return nil, err
		}
		out = append(out, methodDecl)
	}
	return out, nil
}

func (e *backendIREmitter) emitEnumMethodDecls(decl *backendir.EnumDecl) ([]ast.Decl, error) {
	if decl == nil {
		return nil, nil
	}
	receiverType := ast.Expr(ast.NewIdent(goName(decl.Name, true)))
	out := make([]ast.Decl, 0, len(decl.Methods))
	for _, method := range decl.Methods {
		if method == nil {
			continue
		}
		methodKey := "enum:" + decl.Name + "." + method.Name
		if _, seen := e.emittedMethods[methodKey]; seen {
			continue
		}
		e.emittedMethods[methodKey] = struct{}{}
		methodDecl, err := e.emitReceiverMethodDecl(decl.Name, receiverType, nil, method)
		if err != nil {
			return nil, err
		}
		out = append(out, methodDecl)
	}
	return out, nil
}

func (e *backendIREmitter) emitStructDecl(decl *backendir.StructDecl) (ast.Decl, error) {
	typeParamOrder, typeParamMapping, typeParamConstraints := structDeclTypeParamsFromIR(decl)

	fields := make([]*ast.Field, 0, len(decl.Fields))
	for _, field := range decl.Fields {
		typeExpr, err := e.emitTypeWithTypeParams(field.Type, typeParamMapping)
		if err != nil {
			return nil, fmt.Errorf("field %s type: %w", field.Name, err)
		}
		fields = append(fields, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(goName(field.Name, true))},
			Type:  typeExpr,
		})
	}
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name: ast.NewIdent(goName(decl.Name, true)),
		TypeParams: typeParamFieldList(
			typeParamOrder,
			typeParamMapping,
			typeParamConstraints,
		),
		Type: &ast.StructType{Fields: &ast.FieldList{List: fields}},
	}}}, nil
}

func (e *backendIREmitter) emitEnumDecl(decl *backendir.EnumDecl) ast.Decl {
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name: ast.NewIdent(goName(decl.Name, true)),
		Type: &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
			{Names: []*ast.Ident{ast.NewIdent("Tag")}, Type: ast.NewIdent("int")},
		}}},
	}}}
}

func (e *backendIREmitter) emitUnionDecl(decl *backendir.UnionDecl) ast.Decl {
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name: ast.NewIdent(goName(decl.Name, true)),
		Type: &ast.InterfaceType{Methods: &ast.FieldList{}},
	}}}
}

func (e *backendIREmitter) emitExternTypeDecl(decl *backendir.ExternTypeDecl) ast.Decl {
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name: ast.NewIdent(goName(decl.Name, true)),
		Type: &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
			{
				Names: []*ast.Ident{ast.NewIdent("Value")},
				Type:  ast.NewIdent("any"),
			},
		}}},
	}}}
}

const byRefLocalMarker = "*"

func bindLocalRef(name string, byRef bool) string {
	if !byRef || strings.HasPrefix(name, byRefLocalMarker) {
		return name
	}
	return byRefLocalMarker + name
}

func unwrapLocalRef(name string) (string, bool) {
	if strings.HasPrefix(name, byRefLocalMarker) {
		return strings.TrimPrefix(name, byRefLocalMarker), true
	}
	return name, false
}

func (e *backendIREmitter) emitFuncDecl(decl *backendir.FuncDecl) (ast.Decl, error) {
	if decl.IsExtern {
		if !e.canEmitExternDeclNatively(decl) {
			return nil, fmt.Errorf("unsupported extern function declaration: %s", decl.Name)
		}
	} else if !e.canEmitFuncDeclNatively(decl) {
		return nil, fmt.Errorf("unsupported function declaration: %s", decl.Name)
	}
	typeParamOrder, typeParamMapping := functionTypeParamsFromBackendIR(decl)
	typeParamConstraints := functionTypeParamConstraints(decl, typeParamOrder)

	params := make([]*ast.Field, 0, len(decl.Params))
	localNameByOriginal := make(map[string]string)
	seenLocals := make(map[string]struct{})
	for _, param := range decl.Params {
		paramType, err := e.emitTypeWithTypeParams(param.Type, typeParamMapping)
		if err != nil {
			return nil, fmt.Errorf("param %s type: %w", param.Name, err)
		}
		if param.ByRef {
			paramType = &ast.StarExpr{X: paramType}
		}
		localName := uniqueLocalName(goName(param.Name, false), seenLocals)
		localNameByOriginal[param.Name] = bindLocalRef(localName, param.ByRef)
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(localName)}, Type: paramType})
	}

	returnType, err := e.emitTypeWithTypeParams(decl.Return, typeParamMapping)
	if err != nil {
		return nil, fmt.Errorf("return type: %w", err)
	}

	funcType := &ast.FuncType{TypeParams: typeParamFieldList(typeParamOrder, typeParamMapping, typeParamConstraints), Params: &ast.FieldList{List: params}}
	if !lowering.IsVoidIRType(decl.Return) {
		funcType.Results = funcResults(returnType)
	}

	bodyStmts := []ast.Stmt{}
	if decl.IsExtern {
		bodyStmts, err = e.emitExternBody(decl, returnType, localNameByOriginal)
		if err != nil {
			return nil, err
		}
	} else {
		previousTypeParams := e.typeParamMapping
		e.typeParamMapping = typeParamMapping
		bodyStmts, err = e.emitBlock(decl.Body, returnType, localNameByOriginal, seenLocals)
		e.typeParamMapping = previousTypeParams
		if err != nil {
			return nil, err
		}
		if !lowering.IsVoidIRType(decl.Return) && !blockEndsInReturn(bodyStmts) {
			bodyStmts = append(bodyStmts, &ast.ReturnStmt{Results: []ast.Expr{zeroValueExpr(returnType)}})
		}
	}

	name := e.functionNames[decl.Name]
	if name == "" {
		name = goName(decl.Name, !decl.IsPrivate)
	}

	return &ast.FuncDecl{Name: ast.NewIdent(name), Type: funcType, Body: &ast.BlockStmt{List: bodyStmts}}, nil
}

func (e *backendIREmitter) emitReceiverMethodDecl(typeName string, receiverType ast.Expr, typeParams map[string]string, method *backendir.FuncDecl) (ast.Decl, error) {
	if method == nil {
		return nil, fmt.Errorf("nil receiver method")
	}
	if !e.canEmitFuncDeclNatively(method) {
		return nil, fmt.Errorf("unsupported method declaration: %s.%s", typeName, method.Name)
	}

	params := make([]*ast.Field, 0, len(method.Params))
	locals := make(map[string]string, len(method.Params)+1)
	seenLocals := make(map[string]struct{}, len(method.Params)+1)

	receiverName := strings.TrimSpace(method.ReceiverName)
	if receiverName == "" {
		receiverName = "self"
	}
	receiverLocalName := uniqueLocalName(goName(receiverName, false), seenLocals)
	locals[receiverName] = bindLocalRef(receiverLocalName, method.ReceiverMutates)

	for _, param := range method.Params {
		paramType, err := e.emitTypeWithTypeParams(param.Type, typeParams)
		if err != nil {
			return nil, fmt.Errorf("method %s.%s param %s: %w", typeName, method.Name, param.Name, err)
		}
		if param.ByRef {
			paramType = &ast.StarExpr{X: paramType}
		}
		localName := uniqueLocalName(goName(param.Name, false), seenLocals)
		locals[param.Name] = bindLocalRef(localName, param.ByRef)
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(localName)}, Type: paramType})
	}

	returnType, err := e.emitTypeWithTypeParams(method.Return, typeParams)
	if err != nil {
		return nil, fmt.Errorf("method %s.%s return type: %w", typeName, method.Name, err)
	}

	funcType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if !lowering.IsVoidIRType(method.Return) {
		funcType.Results = funcResults(returnType)
	}

	previousTypeParams := e.typeParamMapping
	e.typeParamMapping = typeParams
	bodyStmts, err := e.emitBlock(method.Body, returnType, locals, seenLocals)
	e.typeParamMapping = previousTypeParams
	if err != nil {
		return nil, fmt.Errorf("method %s.%s body: %w", typeName, method.Name, err)
	}
	if !lowering.IsVoidIRType(method.Return) && !blockEndsInReturn(bodyStmts) {
		bodyStmts = append(bodyStmts, &ast.ReturnStmt{Results: []ast.Expr{zeroValueExpr(returnType)}})
	}

	recvType := receiverType
	if method.ReceiverMutates {
		recvType = &ast.StarExpr{X: recvType}
	}

	recvName, _ := unwrapLocalRef(locals[receiverName])
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(recvName)}, Type: recvType}}},
		Name: ast.NewIdent(goName(method.Name, !method.IsPrivate)),
		Type: funcType,
		Body: &ast.BlockStmt{List: bodyStmts},
	}, nil
}

func (e *backendIREmitter) emitVarDecl(decl *backendir.VarDecl) (ast.Decl, error) {
	if !e.canEmitExprNatively(decl.Value) {
		return nil, fmt.Errorf("unsupported package variable: %s", decl.Name)
	}
	return e.emitVarDeclNative(decl)
}

func (e *backendIREmitter) emitVarDeclNative(decl *backendir.VarDecl) (ast.Decl, error) {
	value, err := e.emitExpr(decl.Value, map[string]string{})
	if err != nil {
		return nil, err
	}
	valueSpec := &ast.ValueSpec{
		Names:  []*ast.Ident{ast.NewIdent(goName(decl.Name, true))},
		Values: []ast.Expr{value},
	}
	typeExpr, err := e.emitType(decl.Type)
	if err != nil {
		return nil, err
	}
	if typeExpr != nil {
		valueSpec.Type = typeExpr
	}
	return &ast.GenDecl{
		Tok:   token.VAR,
		Specs: []ast.Spec{valueSpec},
	}, nil
}

func (e *backendIREmitter) canEmitExprNatively(expr backendir.Expr) bool {
	switch v := expr.(type) {
	case nil:
		return false
	case *backendir.IdentExpr:
		return true
	case *backendir.LiteralExpr:
		return isNativeLiteralKind(v.Kind)
	case *backendir.ListLiteralExpr:
		listType, ok := v.Type.(*backendir.ListType)
		if !ok || listType == nil || !e.canEmitTypeNatively(v.Type) {
			return false
		}
		for _, element := range v.Elements {
			if !e.canEmitExprNatively(element) {
				return false
			}
		}
		return true
	case *backendir.MapLiteralExpr:
		mapType, ok := v.Type.(*backendir.MapType)
		if !ok || mapType == nil || !e.canEmitTypeNatively(v.Type) {
			return false
		}
		for _, entry := range v.Entries {
			if entry.Key == nil || entry.Value == nil {
				return false
			}
			if !e.canEmitExprNatively(entry.Key) || !e.canEmitExprNatively(entry.Value) {
				return false
			}
		}
		return true
	case *backendir.StructLiteralExpr:
		if v == nil || v.Type == nil {
			return false
		}
		if _, ok := v.Type.(*backendir.NamedType); !ok || !e.canEmitTypeNatively(v.Type) {
			return false
		}
		if !e.canEmitLocalStructLiteralType(v.Type) {
			return false
		}
		for _, field := range v.Fields {
			if !isSimpleLoweredName(strings.TrimSpace(field.Name)) || field.Value == nil {
				return false
			}
			if !e.canEmitExprNatively(field.Value) {
				return false
			}
		}
		return true
	case *backendir.EnumVariantExpr:
		if v == nil || v.Type == nil {
			return false
		}
		if _, ok := v.Type.(*backendir.NamedType); !ok || !e.canEmitTypeNatively(v.Type) {
			return false
		}
		if !e.canEmitLocalEnumVariantType(v.Type) {
			return false
		}
		return true
	case *backendir.IfExpr:
		if v == nil || v.Cond == nil || v.Then == nil || v.Type == nil {
			return false
		}
		if !e.canEmitExprNatively(v.Cond) || !e.canEmitBlockNatively(v.Then) || !e.canEmitTypeNatively(v.Type) {
			return false
		}
		if !lowering.IsVoidIRType(v.Type) && v.Else == nil {
			return false
		}
		if v.Else != nil && !e.canEmitBlockNatively(v.Else) {
			return false
		}
		return true
	case *backendir.UnionMatchExpr:
		if v == nil || v.Subject == nil || v.Type == nil || len(v.Cases) == 0 {
			return false
		}
		if !e.canEmitExprNatively(v.Subject) || !e.canEmitTypeNatively(v.Type) {
			return false
		}
		if !lowering.IsVoidIRType(v.Type) && v.CatchAll == nil {
			return false
		}
		for _, matchCase := range v.Cases {
			if matchCase.Body == nil || !e.canEmitTypeNatively(matchCase.Type) || !e.canEmitBlockNatively(matchCase.Body) {
				return false
			}
		}
		if v.CatchAll != nil && !e.canEmitBlockNatively(v.CatchAll) {
			return false
		}
		return true
	case *backendir.TryExpr:
		return v != nil && v.Catch != nil && e.canEmitTryExprStmtNatively(v)
	case *backendir.PanicExpr:
		if v == nil || v.Message == nil || v.Type == nil {
			return false
		}
		return e.canEmitExprNatively(v.Message) && e.canEmitTypeNatively(v.Type)
	case *backendir.CopyExpr:
		if v == nil || v.Value == nil || v.Type == nil {
			return false
		}
		if _, ok := v.Type.(*backendir.ListType); !ok {
			return false
		}
		return e.canEmitExprNatively(v.Value) && e.canEmitTypeNatively(v.Type)
	case *backendir.BlockExpr:
		if v == nil || v.Value == nil || v.Type == nil {
			return false
		}
		if !e.canEmitTypeNatively(v.Type) {
			return false
		}
		for _, stmt := range v.Setup {
			if !e.canEmitStmtNatively(stmt) {
				return false
			}
		}
		return e.canEmitExprNatively(v.Value)
	case *backendir.TraitCoerceExpr:
		return v != nil && v.Value != nil && v.Type != nil && e.canEmitExprNatively(v.Value) && e.canEmitTypeNatively(v.Type)
	case *backendir.MaybeSomeExpr:
		if v == nil || v.Value == nil {
			return false
		}
		maybeType, ok := v.Type.(*backendir.MaybeType)
		return ok && maybeType != nil && e.canEmitExprNatively(v.Value) && e.canEmitTypeNatively(maybeType.Of)
	case *backendir.MaybeNoneExpr:
		if v == nil {
			return false
		}
		maybeType, ok := v.Type.(*backendir.MaybeType)
		return ok && maybeType != nil && e.canEmitTypeNatively(maybeType.Of)
	case *backendir.ResultOkExpr:
		if v == nil || v.Value == nil {
			return false
		}
		resultType, ok := v.Type.(*backendir.ResultType)
		return ok && resultType != nil && e.canEmitExprNatively(v.Value) && e.canEmitTypeNatively(resultType.Val) && e.canEmitTypeNatively(resultType.Err)
	case *backendir.ResultErrExpr:
		if v == nil || v.Value == nil {
			return false
		}
		resultType, ok := v.Type.(*backendir.ResultType)
		return ok && resultType != nil && e.canEmitExprNatively(v.Value) && e.canEmitTypeNatively(resultType.Val) && e.canEmitTypeNatively(resultType.Err)
	case *backendir.AddressOfExpr:
		return v != nil && v.Value != nil && isAddressableIRExpr(v.Value)
	case *backendir.FuncLiteralExpr:
		if v == nil || v.Body == nil || !e.canEmitTypeNatively(v.Return) {
			return false
		}
		previousCallables := e.scopedCallableNames
		e.scopedCallableNames = mergeScopedCallableNames(previousCallables, scopedCallableNamesForParams(v.Params))
		defer func() { e.scopedCallableNames = previousCallables }()
		for _, param := range v.Params {
			if !e.canEmitTypeNatively(param.Type) {
				return false
			}
		}
		return e.canEmitBlockNatively(v.Body)
	case *backendir.SelectorExpr:
		return e.canEmitExprNatively(v.Subject)
	case *backendir.CallExpr:
		switch callee := v.Callee.(type) {
		case *backendir.IdentExpr:
			if !e.canEmitNamedCallNatively(callee.Name) {
				return false
			}
			switch callee.Name {
			case "list_push", "list_prepend":
				if len(v.Args) < 1 || !isAddressableIRExpr(v.Args[0]) {
					return false
				}
			}
		case *backendir.SelectorExpr:
			if !e.canEmitSelectorCallNatively(callee) {
				return false
			}
		default:
			return false
		}
		for _, arg := range v.Args {
			if !e.canEmitExprNatively(arg) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func isNativeLiteralKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "str", "int", "float", "bool", "void":
		return true
	default:
		return false
	}
}

func isAddressableIRExpr(expr backendir.Expr) bool {
	switch v := expr.(type) {
	case *backendir.IdentExpr:
		name := strings.TrimSpace(v.Name)
		return name != "" && !strings.Contains(name, "/")
	case *backendir.SelectorExpr:
		return strings.TrimSpace(v.Name) != "" && isSimpleLoweredName(strings.TrimSpace(v.Name)) && isAddressableIRExpr(v.Subject)
	default:
		return false
	}
}

func (e *backendIREmitter) canEmitSelectorCallNatively(selector *backendir.SelectorExpr) bool {
	if selector == nil || strings.TrimSpace(selector.Name) == "" {
		return false
	}
	if !isSimpleLoweredName(selector.Name) {
		return false
	}
	return e.canEmitExprNatively(selector.Subject)
}

func (e *backendIREmitter) canEmitNamedCallNatively(name string) bool {
	if canEmitNativeOpCall(name) {
		return true
	}
	if strings.TrimSpace(name) == "" {
		return false
	}
	if _, ok := e.functionNames[name]; ok {
		return true
	}
	trimmed := strings.TrimSpace(name)
	if _, ok := e.scopedCallableNames[trimmed]; ok {
		return true
	}
	return isSimpleLoweredName(trimmed)
}

func scopedCallableNamesForParams(params []backendir.Param) map[string]struct{} {
	callables := make(map[string]struct{})
	for _, param := range params {
		name := strings.TrimSpace(param.Name)
		if name == "" {
			continue
		}
		if _, ok := param.Type.(*backendir.FuncType); ok {
			callables[name] = struct{}{}
		}
	}
	return callables
}

func mergeScopedCallableNames(base, overlay map[string]struct{}) map[string]struct{} {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	merged := make(map[string]struct{}, len(base)+len(overlay))
	for name := range base {
		merged[name] = struct{}{}
	}
	for name := range overlay {
		merged[name] = struct{}{}
	}
	return merged
}

func canEmitNativeOpCall(name string) bool {
	switch name {
	case "int_add",
		"float_add",
		"str_add",
		"int_sub",
		"float_sub",
		"int_mul",
		"float_mul",
		"int_div",
		"float_div",
		"int_mod",
		"int_gt",
		"float_gt",
		"int_gte",
		"float_gte",
		"int_lt",
		"float_lt",
		"int_lte",
		"float_lte",
		"eq",
		"and",
		"or",
		"not",
		"neg",
		"str_size",
		"str_is_empty",
		"str_contains",
		"str_replace",
		"str_replace_all",
		"str_split",
		"str_starts_with",
		"str_to_str",
		"str_to_dyn",
		"str_trim",
		"int_to_str",
		"int_to_dyn",
		"float_to_str",
		"float_to_int",
		"float_to_dyn",
		"bool_to_str",
		"bool_to_dyn",
		"list_size",
		"list_at",
		"list_push",
		"list_prepend",
		"list_set",
		"list_sort",
		"list_swap",
		"map_size",
		"map_keys",
		"map_has",
		"map_get",
		"map_set",
		"map_drop",
		"maybe_expect",
		"maybe_is_none",
		"maybe_is_some",
		"maybe_or",
		"maybe_map",
		"maybe_and_then",
		"result_expect",
		"result_or",
		"result_is_ok",
		"result_is_err",
		"result_map",
		"result_map_err",
		"result_and_then",
		"fiber_start",
		"fiber_eval",
		"fiber_execution",
		"template":
		return true
	default:
		return false
	}
}

func (e *backendIREmitter) canEmitFuncDeclNatively(decl *backendir.FuncDecl) bool {
	if decl == nil || decl.IsExtern || decl.Body == nil {
		return false
	}
	previousCallables := e.scopedCallableNames
	e.scopedCallableNames = scopedCallableNamesForParams(decl.Params)
	defer func() { e.scopedCallableNames = previousCallables }()
	for _, param := range decl.Params {
		if !e.canEmitTypeNatively(param.Type) {
			return false
		}
	}
	if !e.canEmitTypeNatively(decl.Return) {
		return false
	}
	return e.canEmitBlockNatively(decl.Body)
}

func (e *backendIREmitter) canEmitExternDeclNatively(decl *backendir.FuncDecl) bool {
	if decl == nil || !decl.IsExtern || strings.TrimSpace(decl.ExternBinding) == "" {
		return false
	}
	for _, param := range decl.Params {
		if param.Mutable || param.ByRef || !e.canEmitTypeNatively(param.Type) {
			return false
		}
	}
	return e.canEmitTypeNatively(decl.Return)
}

func (e *backendIREmitter) canEmitBlockNatively(block *backendir.Block) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.Stmts {
		if !e.canEmitStmtNatively(stmt) {
			return false
		}
	}
	return true
}

func (e *backendIREmitter) canEmitStmtNatively(stmt backendir.Stmt) bool {
	switch s := stmt.(type) {
	case *backendir.ReturnStmt:
		if s == nil {
			return false
		}
		if tryExpr, ok := s.Value.(*backendir.TryExpr); ok {
			return e.canEmitTryExprStmtNatively(tryExpr)
		}
		return s.Value == nil || e.canEmitExprNatively(s.Value)
	case *backendir.ExprStmt:
		if s == nil || s.Value == nil {
			return false
		}
		if tryExpr, ok := s.Value.(*backendir.TryExpr); ok {
			return e.canEmitTryExprStmtNatively(tryExpr)
		}
		if ifExpr, ok := s.Value.(*backendir.IfExpr); ok && lowering.IsVoidIRType(ifExpr.Type) {
			return e.canEmitExprNatively(ifExpr)
		}
		if blockExpr, ok := s.Value.(*backendir.BlockExpr); ok && lowering.IsVoidIRType(blockExpr.Type) {
			return e.canEmitExprNatively(blockExpr)
		}
		return e.canEmitExprNatively(s.Value)
	case *backendir.BreakStmt:
		return s != nil
	case *backendir.BindStmt:
		return s != nil && strings.TrimSpace(s.Name) != "" && s.Value != nil && e.canEmitExprNatively(s.Value)
	case *backendir.AssignStmt:
		if s == nil || s.Value == nil {
			return false
		}
		if tryExpr, ok := s.Value.(*backendir.TryExpr); ok {
			return canEmitAssignTargetNatively(s.Target) && e.canEmitTryExprStmtNatively(tryExpr)
		}
		return canEmitAssignTargetNatively(s.Target) && e.canEmitExprNatively(s.Value)
	case *backendir.MemberAssignStmt:
		if s == nil || s.Subject == nil || s.Value == nil {
			return false
		}
		if !isSimpleLoweredName(strings.TrimSpace(s.Field)) {
			return false
		}
		return e.canEmitExprNatively(s.Subject) && e.canEmitExprNatively(s.Value)
	case *backendir.ForIntRangeStmt:
		if s == nil {
			return false
		}
		if !isSimpleLoweredName(strings.TrimSpace(s.Cursor)) {
			return false
		}
		if strings.TrimSpace(s.Index) != "" && !isSimpleLoweredName(strings.TrimSpace(s.Index)) {
			return false
		}
		if s.Start == nil || s.End == nil || s.Body == nil {
			return false
		}
		return e.canEmitExprNatively(s.Start) && e.canEmitExprNatively(s.End) && e.canEmitBlockNatively(s.Body)
	case *backendir.ForLoopStmt:
		if s == nil {
			return false
		}
		if !isSimpleLoweredName(strings.TrimSpace(s.InitName)) {
			return false
		}
		if s.InitValue == nil || s.Cond == nil || s.Update == nil || s.Body == nil {
			return false
		}
		if !e.canEmitExprNatively(s.InitValue) || !e.canEmitExprNatively(s.Cond) || !e.canEmitBlockNatively(s.Body) {
			return false
		}
		return e.canEmitForLoopUpdateNatively(s.Update)
	case *backendir.ForInStrStmt:
		if s == nil {
			return false
		}
		if !isSimpleLoweredName(strings.TrimSpace(s.Cursor)) {
			return false
		}
		if strings.TrimSpace(s.Index) != "" && !isSimpleLoweredName(strings.TrimSpace(s.Index)) {
			return false
		}
		if s.Value == nil || s.Body == nil {
			return false
		}
		return e.canEmitExprNatively(s.Value) && e.canEmitBlockNatively(s.Body)
	case *backendir.ForInListStmt:
		if s == nil {
			return false
		}
		if !isSimpleLoweredName(strings.TrimSpace(s.Cursor)) {
			return false
		}
		if strings.TrimSpace(s.Index) != "" && !isSimpleLoweredName(strings.TrimSpace(s.Index)) {
			return false
		}
		if s.List == nil || s.Body == nil {
			return false
		}
		previousCallables := e.scopedCallableNames
		if _, ok := s.CursorType.(*backendir.FuncType); ok {
			e.scopedCallableNames = mergeScopedCallableNames(previousCallables, map[string]struct{}{strings.TrimSpace(s.Cursor): {}})
		}
		defer func() { e.scopedCallableNames = previousCallables }()
		return e.canEmitExprNatively(s.List) && e.canEmitBlockNatively(s.Body)
	case *backendir.ForInMapStmt:
		if s == nil {
			return false
		}
		if !isSimpleLoweredName(strings.TrimSpace(s.Key)) || !isSimpleLoweredName(strings.TrimSpace(s.Value)) {
			return false
		}
		if s.Map == nil || s.Body == nil {
			return false
		}
		return e.canEmitExprNatively(s.Map) && e.canEmitBlockNatively(s.Body)
	case *backendir.WhileStmt:
		if s == nil || s.Cond == nil || s.Body == nil {
			return false
		}
		return e.canEmitExprNatively(s.Cond) && e.canEmitBlockNatively(s.Body)
	case *backendir.IfStmt:
		if s == nil || s.Cond == nil || s.Then == nil {
			return false
		}
		if !e.canEmitExprNatively(s.Cond) || !e.canEmitBlockNatively(s.Then) {
			return false
		}
		if s.Else != nil && !e.canEmitBlockNatively(s.Else) {
			return false
		}
		return true
	default:
		return false
	}
}

func (e *backendIREmitter) canEmitTryExprStmtNatively(expr *backendir.TryExpr) bool {
	if expr == nil || expr.Subject == nil || expr.Type == nil {
		return false
	}
	kind := strings.TrimSpace(expr.Kind)
	if kind != "result" && kind != "maybe" {
		return false
	}
	catchVar := strings.TrimSpace(expr.CatchVar)
	if catchVar != "" && catchVar != "_" && !isSimpleLoweredName(catchVar) {
		return false
	}
	if !e.canEmitExprNatively(expr.Subject) || !e.canEmitTypeNatively(expr.Type) {
		return false
	}
	if expr.Catch == nil {
		return catchVar == ""
	}
	return e.canEmitBlockNatively(expr.Catch)
}

func canEmitAssignTargetNatively(target string) bool {
	targetName := strings.TrimSpace(target)
	if targetName == "" || strings.Contains(targetName, "<target:") {
		return false
	}
	if targetName == "_" {
		return true
	}
	if strings.Contains(targetName, ".") {
		_, _, ok := splitMemberAssignTarget(targetName)
		return ok
	}
	return isSimpleLoweredName(targetName)
}

func (e *backendIREmitter) canEmitForLoopUpdateNatively(stmt backendir.Stmt) bool {
	switch update := stmt.(type) {
	case *backendir.AssignStmt:
		if update == nil || update.Value == nil {
			return false
		}
		target := strings.TrimSpace(update.Target)
		if target == "_" || !canEmitAssignTargetNatively(target) {
			return false
		}
		return e.canEmitExprNatively(update.Value)
	case *backendir.MemberAssignStmt:
		if update == nil || update.Subject == nil || update.Value == nil {
			return false
		}
		if !isSimpleLoweredName(strings.TrimSpace(update.Field)) {
			return false
		}
		return e.canEmitExprNatively(update.Subject) && e.canEmitExprNatively(update.Value)
	default:
		return false
	}
}

func splitMemberAssignTarget(target string) (string, string, bool) {
	targetName := strings.TrimSpace(target)
	if strings.Count(targetName, ".") != 1 {
		return "", "", false
	}
	parts := strings.SplitN(targetName, ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	base := strings.TrimSpace(parts[0])
	field := strings.TrimSpace(parts[1])
	if !isSimpleLoweredName(base) || !isSimpleLoweredName(field) {
		return "", "", false
	}
	return base, field, true
}

func isSimpleLoweredName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		// Allow non-ASCII Unicode letters. Go's identifier grammar accepts them
		// and the backend IR uses them deliberately for compiler-synthesized
		// temps (see matchSubjectTempPrefix) so that the temp names cannot
		// collide with any legal user-defined Ard identifier — Ard's lexer
		// only accepts ASCII `[A-Za-z_][A-Za-z_0-9]*`.
		if r >= utf8.RuneSelf && unicode.IsLetter(r) {
			continue
		}
		return false
	}
	return true
}

func (e *backendIREmitter) canEmitTypeNatively(t backendir.Type) bool {
	switch typed := t.(type) {
	case nil:
		return true
	case *backendir.PrimitiveType, *backendir.DynamicType, *backendir.VoidType, *backendir.TypeVarType:
		return true
	case *backendir.TraitType:
		for _, method := range typed.Methods {
			if !e.canEmitTypeNatively(method.Type) {
				return false
			}
		}
		return true
	case *backendir.NamedType:
		for _, arg := range typed.Args {
			if !e.canEmitTypeNatively(arg) {
				return false
			}
		}
		return true
	case *backendir.ListType:
		return e.canEmitTypeNatively(typed.Elem)
	case *backendir.MapType:
		return e.canEmitTypeNatively(typed.Key) && e.canEmitTypeNatively(typed.Value)
	case *backendir.MaybeType:
		return e.canEmitTypeNatively(typed.Of)
	case *backendir.ResultType:
		return e.canEmitTypeNatively(typed.Val) && e.canEmitTypeNatively(typed.Err)
	case *backendir.FuncType:
		for _, param := range typed.Params {
			if !e.canEmitTypeNatively(param) {
				return false
			}
		}
		return e.canEmitTypeNatively(typed.Return)
	default:
		return false
	}
}

func containsDynamicIRType(t backendir.Type) bool {
	switch typed := t.(type) {
	case nil:
		return false
	case *backendir.DynamicType:
		return true
	case *backendir.TraitType:
		for _, method := range typed.Methods {
			if containsDynamicIRType(method.Type) {
				return true
			}
		}
		return false
	case *backendir.NamedType:
		for _, arg := range typed.Args {
			if containsDynamicIRType(arg) {
				return true
			}
		}
		return false
	case *backendir.ListType:
		return containsDynamicIRType(typed.Elem)
	case *backendir.MapType:
		return containsDynamicIRType(typed.Key) || containsDynamicIRType(typed.Value)
	case *backendir.MaybeType:
		return containsDynamicIRType(typed.Of)
	case *backendir.ResultType:
		return containsDynamicIRType(typed.Val) || containsDynamicIRType(typed.Err)
	case *backendir.FuncType:
		for _, param := range typed.Params {
			if containsDynamicIRType(param) {
				return true
			}
		}
		return containsDynamicIRType(typed.Return)
	default:
		return false
	}
}

func containsTypeVarIRType(t backendir.Type) bool {
	switch typed := t.(type) {
	case nil:
		return false
	case *backendir.TypeVarType:
		return true
	case *backendir.TraitType:
		for _, method := range typed.Methods {
			if containsTypeVarIRType(method.Type) {
				return true
			}
		}
		return false
	case *backendir.NamedType:
		for _, arg := range typed.Args {
			if containsTypeVarIRType(arg) {
				return true
			}
		}
		return false
	case *backendir.ListType:
		return containsTypeVarIRType(typed.Elem)
	case *backendir.MapType:
		return containsTypeVarIRType(typed.Key) || containsTypeVarIRType(typed.Value)
	case *backendir.MaybeType:
		return containsTypeVarIRType(typed.Of)
	case *backendir.ResultType:
		return containsTypeVarIRType(typed.Val) || containsTypeVarIRType(typed.Err)
	case *backendir.FuncType:
		for _, param := range typed.Params {
			if containsTypeVarIRType(param) {
				return true
			}
		}
		return containsTypeVarIRType(typed.Return)
	default:
		return false
	}
}

func irNamedTypeName(t backendir.Type) string {
	named, ok := t.(*backendir.NamedType)
	if !ok || named == nil {
		return ""
	}
	return strings.TrimSpace(named.Name)
}

func (e *backendIREmitter) canEmitLocalStructLiteralType(t backendir.Type) bool {
	name := irNamedTypeName(t)
	if name == "" {
		return false
	}
	if named, ok := t.(*backendir.NamedType); ok && strings.TrimSpace(named.Module) != "" {
		return true
	}
	_, ok := e.localStructNames[name]
	return ok
}

func (e *backendIREmitter) canEmitLocalEnumVariantType(t backendir.Type) bool {
	name := irNamedTypeName(t)
	if name == "" {
		return false
	}
	if named, ok := t.(*backendir.NamedType); ok && strings.TrimSpace(named.Module) != "" {
		return true
	}
	_, ok := e.localEnumNames[name]
	return ok
}

func (e *backendIREmitter) externArgNeedsAnySlice(t backendir.Type) bool {
	listType, ok := t.(*backendir.ListType)
	if !ok || listType == nil {
		return false
	}
	named, ok := listType.Elem.(*backendir.NamedType)
	if !ok || named == nil || strings.TrimSpace(named.Module) != "" {
		return false
	}
	_, ok = e.localUnionNames[strings.TrimSpace(named.Name)]
	return ok
}

func (e *backendIREmitter) emitExternBody(decl *backendir.FuncDecl, returnType ast.Expr, locals map[string]string) ([]ast.Stmt, error) {
	if stmts, ok, err := e.emitBuiltinExternBody(decl, locals); ok || err != nil {
		return stmts, err
	}

	args := make([]ast.Expr, 0, len(decl.Params)+1)
	args = append(args, &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(decl.ExternBinding)})
	for _, param := range decl.Params {
		arg := ast.Expr(ast.NewIdent(locals[param.Name]))
		if e.externArgNeedsAnySlice(param.Type) {
			arg = &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "AnySlice"), Args: []ast.Expr{arg}}
		}
		args = append(args, arg)
	}

	call := &ast.CallExpr{
		Fun:  selectorExpr(ast.NewIdent(helperImportAlias), "CallExtern"),
		Args: args,
	}

	resultName := ast.NewIdent("result")
	if lowering.IsVoidIRType(decl.Return) {
		resultName = ast.NewIdent("_")
	}
	stmts := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{resultName, ast.NewIdent("err")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{call},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  ast.NewIdent("err"),
				Op: token.NEQ,
				Y:  ast.NewIdent("nil"),
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{ast.NewIdent("err")}}},
			}},
		},
	}

	if !lowering.IsVoidIRType(decl.Return) {
		coerce := indexExpr(selectorExpr(ast.NewIdent(helperImportAlias), "CoerceExtern"), []ast.Expr{returnType})
		stmts = append(stmts, &ast.ReturnStmt{
			Results: []ast.Expr{
				&ast.CallExpr{Fun: coerce, Args: []ast.Expr{ast.NewIdent("result")}},
			},
		})
	}

	return stmts, nil
}

func (e *backendIREmitter) emitBuiltinExternBody(decl *backendir.FuncDecl, locals map[string]string) ([]ast.Stmt, bool, error) {
	switch decl.ExternBinding {
	case "StrToDynamic", "IntToDynamic", "FloatToDynamic", "BoolToDynamic":
		if len(decl.Params) != 1 {
			return nil, true, fmt.Errorf("extern %s expects 1 arg, got %d", decl.ExternBinding, len(decl.Params))
		}
		return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(locals[decl.Params[0].Name])}}}, true, nil
	case "VoidToDynamic":
		if len(decl.Params) != 0 {
			return nil, true, fmt.Errorf("extern VoidToDynamic expects 0 args, got %d", len(decl.Params))
		}
		return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("nil")}}}, true, nil
	}

	resultType, ok := decl.Return.(*backendir.ResultType)
	if !ok {
		return nil, false, nil
	}

	helper := ast.NewIdent(helperImportAlias)
	arg := func(index int) (ast.Expr, error) {
		if index < 0 || index >= len(decl.Params) {
			return nil, fmt.Errorf("extern %s expects arg %d", decl.ExternBinding, index)
		}
		return ast.NewIdent(locals[decl.Params[index].Name]), nil
	}
	returnCall := func(function string, args ...ast.Expr) []ast.Stmt {
		return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: selectorExpr(helper, function), Args: args}}}}
	}
	returnGenericCall := func(function string, typeArg ast.Expr, args ...ast.Expr) []ast.Stmt {
		return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: indexExpr(selectorExpr(helper, function), []ast.Expr{typeArg}), Args: args}}}}
	}

	switch decl.ExternBinding {
	case "DecodeString", "DecodeInt", "DecodeFloat", "DecodeBool":
		if len(decl.Params) != 1 {
			return nil, true, fmt.Errorf("extern %s expects 1 arg, got %d", decl.ExternBinding, len(decl.Params))
		}
		errType, err := e.emitType(resultType.Err)
		if err != nil {
			return nil, true, err
		}
		data, err := arg(0)
		if err != nil {
			return nil, true, err
		}
		return returnGenericCall(decl.ExternBinding+"Extern", errType, data), true, nil
	case "DecodeIntListErrors":
		if len(decl.Params) != 1 {
			return nil, true, fmt.Errorf("extern %s expects 1 arg, got %d", decl.ExternBinding, len(decl.Params))
		}
		errList, ok := resultType.Err.(*backendir.ListType)
		if !ok {
			return nil, true, fmt.Errorf("extern %s expects list error type", decl.ExternBinding)
		}
		errType, err := e.emitType(errList.Elem)
		if err != nil {
			return nil, true, err
		}
		data, err := arg(0)
		if err != nil {
			return nil, true, err
		}
		return returnGenericCall(decl.ExternBinding+"Extern", errType, data), true, nil
	case "DecodeStringErrors", "DecodeIntErrors":
		if len(decl.Params) != 1 {
			return nil, true, fmt.Errorf("extern %s expects 1 arg, got %d", decl.ExternBinding, len(decl.Params))
		}
		errList, ok := resultType.Err.(*backendir.ListType)
		if !ok {
			return nil, true, fmt.Errorf("extern %s expects list error type", decl.ExternBinding)
		}
		errType, err := e.emitType(errList.Elem)
		if err != nil {
			return nil, true, err
		}
		data, err := arg(0)
		if err != nil {
			return nil, true, err
		}
		return returnGenericCall(decl.ExternBinding+"Extern", errType, data), true, nil
	case "DynamicToList":
		if len(decl.Params) != 1 {
			return nil, true, fmt.Errorf("extern DynamicToList expects 1 arg, got %d", len(decl.Params))
		}
		data, err := arg(0)
		if err != nil {
			return nil, true, err
		}
		return returnCall("DynamicToListExtern", data), true, nil
	case "DynamicToMap", "DynamicToStringMap":
		if len(decl.Params) != 1 {
			return nil, true, fmt.Errorf("extern %s expects 1 arg, got %d", decl.ExternBinding, len(decl.Params))
		}
		data, err := arg(0)
		if err != nil {
			return nil, true, err
		}
		if decl.ExternBinding == "DynamicToStringMap" {
			return returnCall("DynamicToStringMapExtern", data), true, nil
		}
		return returnCall("DynamicToMapExtern", data), true, nil
	case "ExtractField":
		if len(decl.Params) != 2 {
			return nil, true, fmt.Errorf("extern ExtractField expects 2 args, got %d", len(decl.Params))
		}
		data, err := arg(0)
		if err != nil {
			return nil, true, err
		}
		name, err := arg(1)
		if err != nil {
			return nil, true, err
		}
		return returnCall("ExtractFieldExtern", data, name), true, nil
	case "JsonToDynamic":
		if len(decl.Params) != 1 {
			return nil, true, fmt.Errorf("extern JsonToDynamic expects 1 arg, got %d", len(decl.Params))
		}
		data, err := arg(0)
		if err != nil {
			return nil, true, err
		}
		return returnCall("JsonToDynamicExtern", data), true, nil
	}

	return nil, false, nil
}

func (e *backendIREmitter) emitBlock(block *backendir.Block, returnType ast.Expr, locals map[string]string, seenLocals map[string]struct{}) ([]ast.Stmt, error) {
	if block == nil {
		return nil, nil
	}
	out := make([]ast.Stmt, 0, len(block.Stmts))
	for _, stmt := range block.Stmts {
		emitted, err := e.emitStmt(stmt, returnType, locals, seenLocals)
		if err != nil {
			return nil, err
		}
		out = append(out, emitted...)
	}
	return out, nil
}

func (e *backendIREmitter) emitStmt(stmt backendir.Stmt, returnType ast.Expr, locals map[string]string, seenLocals map[string]struct{}) ([]ast.Stmt, error) {
	switch s := stmt.(type) {
	case *backendir.ReturnStmt:
		if s.Value == nil {
			return []ast.Stmt{&ast.ReturnStmt{}}, nil
		}
		if tryExpr, ok := s.Value.(*backendir.TryExpr); ok {
			return e.emitTryExprControlStmts(tryExpr, returnType, locals, seenLocals, func(success ast.Expr) ([]ast.Stmt, error) {
				return []ast.Stmt{
					&ast.ReturnStmt{Results: []ast.Expr{success}},
				}, nil
			})
		}
		var (
			value ast.Expr
			err   error
		)
		if panicExpr, ok := s.Value.(*backendir.PanicExpr); ok {
			value, err = e.emitPanicExprWithExpectedType(panicExpr, locals, returnType)
		} else {
			value, err = e.emitExpr(s.Value, locals)
		}
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{value}}}, nil
	case *backendir.ExprStmt:
		if literal, ok := s.Value.(*backendir.LiteralExpr); ok && strings.TrimSpace(literal.Kind) == "void" {
			return nil, nil
		}
		if tryExpr, ok := s.Value.(*backendir.TryExpr); ok {
			return e.emitTryExprControlStmts(tryExpr, returnType, locals, seenLocals, func(success ast.Expr) ([]ast.Stmt, error) {
				if _, ok := success.(*ast.CallExpr); ok {
					return []ast.Stmt{&ast.ExprStmt{X: success}}, nil
				}
				return []ast.Stmt{
					&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{success}},
				}, nil
			})
		}
		if ifExpr, ok := s.Value.(*backendir.IfExpr); ok && lowering.IsVoidIRType(ifExpr.Type) {
			return e.emitIfExprStmt(ifExpr, returnType, locals, seenLocals)
		}
		if unionMatch, ok := s.Value.(*backendir.UnionMatchExpr); ok && lowering.IsVoidIRType(unionMatch.Type) {
			return e.emitUnionMatchStmt(unionMatch, returnType, locals, seenLocals)
		}
		if blockExpr, ok := s.Value.(*backendir.BlockExpr); ok && lowering.IsVoidIRType(blockExpr.Type) {
			return e.emitBlockExprStmt(blockExpr, returnType, locals, seenLocals)
		}
		if call, ok := s.Value.(*backendir.CallExpr); ok {
			if ident, ok := call.Callee.(*backendir.IdentExpr); ok {
				switch ident.Name {
				case "list_push":
					if len(call.Args) != 2 {
						return nil, fmt.Errorf("list_push expects 2 args, got %d", len(call.Args))
					}
					subject, err := e.emitExpr(call.Args[0], locals)
					if err != nil {
						return nil, err
					}
					value, err := e.emitExpr(call.Args[1], locals)
					if err != nil {
						return nil, err
					}
					return []ast.Stmt{&ast.AssignStmt{
						Lhs: []ast.Expr{subject},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{subject, value}}},
					}}, nil
				case "map_set":
					if len(call.Args) != 3 {
						return nil, fmt.Errorf("map_set expects 3 args, got %d", len(call.Args))
					}
					subject, err := e.emitExpr(call.Args[0], locals)
					if err != nil {
						return nil, err
					}
					key, err := e.emitExpr(call.Args[1], locals)
					if err != nil {
						return nil, err
					}
					value, err := e.emitExpr(call.Args[2], locals)
					if err != nil {
						return nil, err
					}
					return []ast.Stmt{&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IndexExpr{X: subject, Index: key}},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{value},
					}}, nil
				}
			}
		}
		value, err := e.emitExpr(s.Value, locals)
		if err != nil {
			return nil, err
		}
		if _, ok := value.(*ast.CallExpr); ok {
			return []ast.Stmt{&ast.ExprStmt{X: value}}, nil
		}
		return []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{value}},
		}, nil
	case *backendir.BreakStmt:
		return []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}, nil
	case *backendir.BindStmt:
		value, err := e.emitExpr(s.Value, locals)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(s.Name)
		if name == "" || name == "_" {
			return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{value}}}, nil
		}
		localName := uniqueLocalName(goName(name, false), seenLocals)
		locals[name] = localName
		return []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(localName)}, Tok: token.DEFINE, Rhs: []ast.Expr{value}},
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(localName)}},
		}, nil
	case *backendir.AssignStmt:
		if tryExpr, ok := s.Value.(*backendir.TryExpr); ok {
			catchBaseLocals := cloneStringMap(locals)
			target, tok, err := e.emitAssignTargetExpr(s.Target, locals, seenLocals)
			if err != nil {
				return nil, err
			}
			return e.emitTryExprControlStmtsWithCatchLocals(tryExpr, returnType, locals, seenLocals, catchBaseLocals, func(success ast.Expr) ([]ast.Stmt, error) {
				return []ast.Stmt{
					&ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: tok, Rhs: []ast.Expr{success}},
				}, nil
			})
		}
		if ifExpr, ok := s.Value.(*backendir.IfExpr); ok && !lowering.IsVoidIRType(ifExpr.Type) {
			return e.emitIfExprAssignStmts(s.Target, ifExpr, returnType, locals, seenLocals)
		}
		if unionMatch, ok := s.Value.(*backendir.UnionMatchExpr); ok {
			return e.emitUnionMatchAssignStmts(s.Target, unionMatch, returnType, locals, seenLocals)
		}
		value, err := e.emitExpr(s.Value, locals)
		if err != nil {
			return nil, err
		}
		target, tok, err := e.emitAssignTargetExpr(s.Target, locals, seenLocals)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: tok, Rhs: []ast.Expr{value}},
		}, nil
	case *backendir.MemberAssignStmt:
		subject, err := e.emitExpr(s.Subject, locals)
		if err != nil {
			return nil, err
		}
		value, err := e.emitExpr(s.Value, locals)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{selectorExpr(subject, goName(strings.TrimSpace(s.Field), true))},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{value},
			},
		}, nil
	case *backendir.ForIntRangeStmt:
		start, err := e.emitExpr(s.Start, locals)
		if err != nil {
			return nil, err
		}
		end, err := e.emitExpr(s.End, locals)
		if err != nil {
			return nil, err
		}

		loopLocals := cloneStringMap(locals)
		loopSeen := cloneSet(seenLocals)
		cursorLocal := uniqueLocalName(goName(strings.TrimSpace(s.Cursor), false), loopSeen)
		loopLocals[s.Cursor] = cursorLocal

		var (
			init ast.Stmt
			post ast.Stmt
		)
		if strings.TrimSpace(s.Index) == "" {
			init = &ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(cursorLocal)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{start},
			}
			post = &ast.IncDecStmt{X: ast.NewIdent(cursorLocal), Tok: token.INC}
		} else {
			indexLocal := uniqueLocalName(goName(strings.TrimSpace(s.Index), false), loopSeen)
			loopLocals[s.Index] = indexLocal
			init = &ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(cursorLocal), ast.NewIdent(indexLocal)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{start, &ast.BasicLit{Kind: token.INT, Value: "0"}},
			}
			post = &ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(cursorLocal), ast.NewIdent(indexLocal)},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{
					&ast.BinaryExpr{X: ast.NewIdent(cursorLocal), Op: token.ADD, Y: &ast.BasicLit{Kind: token.INT, Value: "1"}},
					&ast.BinaryExpr{X: ast.NewIdent(indexLocal), Op: token.ADD, Y: &ast.BasicLit{Kind: token.INT, Value: "1"}},
				},
			}
		}

		bodyStmts, err := e.emitBlock(s.Body, returnType, loopLocals, loopSeen)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{
			&ast.ForStmt{
				Init: init,
				Cond: &ast.BinaryExpr{X: ast.NewIdent(cursorLocal), Op: token.LEQ, Y: end},
				Post: post,
				Body: &ast.BlockStmt{List: bodyStmts},
			},
		}, nil
	case *backendir.ForLoopStmt:
		loopLocals := cloneStringMap(locals)
		loopSeen := cloneSet(seenLocals)
		initName := strings.TrimSpace(s.InitName)
		initLocal := uniqueLocalName(goName(initName, false), loopSeen)
		loopLocals[initName] = initLocal

		initValue, err := e.emitExpr(s.InitValue, loopLocals)
		if err != nil {
			return nil, err
		}
		cond, err := e.emitExpr(s.Cond, loopLocals)
		if err != nil {
			return nil, err
		}
		post, err := e.emitForLoopPostStmt(s.Update, loopLocals)
		if err != nil {
			return nil, err
		}
		bodyStmts, err := e.emitBlock(s.Body, returnType, loopLocals, loopSeen)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{
			&ast.ForStmt{
				Init: &ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(initLocal)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{initValue},
				},
				Cond: cond,
				Post: post,
				Body: &ast.BlockStmt{List: bodyStmts},
			},
		}, nil
	case *backendir.ForInStrStmt:
		valueExpr, err := e.emitExpr(s.Value, locals)
		if err != nil {
			return nil, err
		}
		loopLocals := cloneStringMap(locals)
		loopSeen := cloneSet(seenLocals)
		cursorName := strings.TrimSpace(s.Cursor)
		cursorLocal := uniqueLocalName(goName(cursorName, false), loopSeen)
		loopLocals[cursorName] = cursorLocal

		keyExpr := ast.Expr(ast.NewIdent("_"))
		if indexName := strings.TrimSpace(s.Index); indexName != "" {
			indexLocal := uniqueLocalName(goName(indexName, false), loopSeen)
			loopLocals[indexName] = indexLocal
			keyExpr = ast.NewIdent(indexLocal)
		}

		runeLocal := uniqueLocalName("__ardRune", loopSeen)
		bodyStmts, err := e.emitBlock(s.Body, returnType, loopLocals, loopSeen)
		if err != nil {
			return nil, err
		}
		bodyStmts = append([]ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(cursorLocal)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("string"), Args: []ast.Expr{ast.NewIdent(runeLocal)}}},
			},
		}, bodyStmts...)

		return []ast.Stmt{
			&ast.RangeStmt{
				Key:   keyExpr,
				Value: ast.NewIdent(runeLocal),
				Tok:   token.DEFINE,
				X: &ast.CallExpr{
					Fun:  &ast.ArrayType{Elt: ast.NewIdent("rune")},
					Args: []ast.Expr{valueExpr},
				},
				Body: &ast.BlockStmt{List: bodyStmts},
			},
		}, nil
	case *backendir.ForInListStmt:
		listExpr, err := e.emitExpr(s.List, locals)
		if err != nil {
			return nil, err
		}
		loopLocals := cloneStringMap(locals)
		loopSeen := cloneSet(seenLocals)
		cursorName := strings.TrimSpace(s.Cursor)
		cursorLocal := uniqueLocalName(goName(cursorName, false), loopSeen)
		loopLocals[cursorName] = cursorLocal

		keyExpr := ast.Expr(ast.NewIdent("_"))
		if indexName := strings.TrimSpace(s.Index); indexName != "" {
			indexLocal := uniqueLocalName(goName(indexName, false), loopSeen)
			loopLocals[indexName] = indexLocal
			keyExpr = ast.NewIdent(indexLocal)
		}
		bodyStmts, err := e.emitBlock(s.Body, returnType, loopLocals, loopSeen)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{
			&ast.RangeStmt{
				Key:   keyExpr,
				Value: ast.NewIdent(cursorLocal),
				Tok:   token.DEFINE,
				X:     listExpr,
				Body:  &ast.BlockStmt{List: bodyStmts},
			},
		}, nil
	case *backendir.ForInMapStmt:
		mapExpr, err := e.emitExpr(s.Map, locals)
		if err != nil {
			return nil, err
		}
		mapLocal := uniqueLocalName("ardMap", seenLocals)
		loopLocals := cloneStringMap(locals)
		loopSeen := cloneSet(seenLocals)
		keyName := strings.TrimSpace(s.Key)
		keyLocal := uniqueLocalName(goName(keyName, false), loopSeen)
		loopLocals[keyName] = keyLocal
		valName := strings.TrimSpace(s.Value)
		valLocal := uniqueLocalName(goName(valName, false), loopSeen)
		loopLocals[valName] = valLocal

		bodyStmts, err := e.emitBlock(s.Body, returnType, loopLocals, loopSeen)
		if err != nil {
			return nil, err
		}
		bodyStmts = append([]ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(valLocal)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.IndexExpr{
					X:     ast.NewIdent(mapLocal),
					Index: ast.NewIdent(keyLocal),
				}},
			},
		}, bodyStmts...)

		return []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(mapLocal)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{mapExpr},
			},
			&ast.RangeStmt{
				Key:   ast.NewIdent("_"),
				Value: ast.NewIdent(keyLocal),
				Tok:   token.DEFINE,
				X: &ast.CallExpr{
					Fun: selectorExpr(ast.NewIdent(helperImportAlias), "MapKeys"),
					Args: []ast.Expr{
						ast.NewIdent(mapLocal),
					},
				},
				Body: &ast.BlockStmt{List: bodyStmts},
			},
		}, nil
	case *backendir.WhileStmt:
		cond, err := e.emitExpr(s.Cond, locals)
		if err != nil {
			return nil, err
		}
		loopLocals := cloneStringMap(locals)
		loopSeen := cloneSet(seenLocals)
		bodyStmts, err := e.emitBlock(s.Body, returnType, loopLocals, loopSeen)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{
			&ast.ForStmt{
				Cond: cond,
				Body: &ast.BlockStmt{List: bodyStmts},
			},
		}, nil
	case *backendir.IfStmt:
		cond, err := e.emitExpr(s.Cond, locals)
		if err != nil {
			return nil, err
		}
		thenLocals := cloneStringMap(locals)
		thenSeen := cloneSet(seenLocals)
		thenStmts, err := e.emitBlock(s.Then, returnType, thenLocals, thenSeen)
		if err != nil {
			return nil, err
		}
		ifStmt := &ast.IfStmt{
			Cond: cond,
			Body: &ast.BlockStmt{List: thenStmts},
		}
		if s.Else != nil {
			elseLocals := cloneStringMap(locals)
			elseSeen := cloneSet(seenLocals)
			elseStmts, err := e.emitBlock(s.Else, returnType, elseLocals, elseSeen)
			if err != nil {
				return nil, err
			}
			ifStmt.Else = &ast.BlockStmt{List: elseStmts}
		}
		return []ast.Stmt{ifStmt}, nil
	default:
		return nil, fmt.Errorf("unsupported stmt %T", stmt)
	}
}

func (e *backendIREmitter) emitForLoopPostStmt(stmt backendir.Stmt, locals map[string]string) (ast.Stmt, error) {
	switch update := stmt.(type) {
	case *backendir.AssignStmt:
		value, err := e.emitExpr(update.Value, locals)
		if err != nil {
			return nil, err
		}
		target, err := e.emitForLoopUpdateTargetExpr(update.Target, locals)
		if err != nil {
			return nil, err
		}
		return &ast.AssignStmt{
			Lhs: []ast.Expr{target},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{value},
		}, nil
	case *backendir.MemberAssignStmt:
		subject, err := e.emitExpr(update.Subject, locals)
		if err != nil {
			return nil, err
		}
		value, err := e.emitExpr(update.Value, locals)
		if err != nil {
			return nil, err
		}
		return &ast.AssignStmt{
			Lhs: []ast.Expr{selectorExpr(subject, goName(strings.TrimSpace(update.Field), true))},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{value},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported for-loop update %T", stmt)
	}
}

func (e *backendIREmitter) emitForLoopUpdateTargetExpr(target string, locals map[string]string) (ast.Expr, error) {
	targetName := strings.TrimSpace(target)
	if !canEmitAssignTargetNatively(targetName) || targetName == "_" {
		return nil, fmt.Errorf("unsupported for-loop update target %q", targetName)
	}
	if base, field, ok := splitMemberAssignTarget(targetName); ok {
		baseName := locals[base]
		if strings.TrimSpace(baseName) == "" {
			baseName = goName(base, false)
		}
		binding, _ := unwrapLocalRef(baseName)
		return selectorExpr(ast.NewIdent(binding), goName(field, true)), nil
	}
	if localName := locals[targetName]; strings.TrimSpace(localName) != "" {
		binding, byRef := unwrapLocalRef(localName)
		if byRef {
			return &ast.StarExpr{X: ast.NewIdent(binding)}, nil
		}
		return ast.NewIdent(binding), nil
	}
	return ast.NewIdent(goName(targetName, false)), nil
}

func (e *backendIREmitter) emitExpr(expr backendir.Expr, locals map[string]string) (ast.Expr, error) {
	switch v := expr.(type) {
	case *backendir.IdentExpr:
		name := strings.TrimSpace(v.Name)
		if name == "" {
			return ast.NewIdent("_"), nil
		}
		if local := locals[name]; local != "" {
			binding, byRef := unwrapLocalRef(local)
			if byRef {
				return &ast.StarExpr{X: ast.NewIdent(binding)}, nil
			}
			return ast.NewIdent(binding), nil
		}
		if fn := e.functionNames[name]; fn != "" {
			return ast.NewIdent(fn), nil
		}
		return ast.NewIdent(goName(name, false)), nil
	case *backendir.LiteralExpr:
		return emitLiteralExpr(v), nil
	case *backendir.ListLiteralExpr:
		typeExpr, err := e.emitType(v.Type)
		if err != nil {
			return nil, err
		}
		if typeExpr == nil {
			return nil, fmt.Errorf("list literal type is nil")
		}
		elements := make([]ast.Expr, 0, len(v.Elements))
		for _, element := range v.Elements {
			emitted, err := e.emitExpr(element, locals)
			if err != nil {
				return nil, err
			}
			elements = append(elements, emitted)
		}
		return &ast.CompositeLit{
			Type: typeExpr,
			Elts: elements,
		}, nil
	case *backendir.MapLiteralExpr:
		typeExpr, err := e.emitType(v.Type)
		if err != nil {
			return nil, err
		}
		if typeExpr == nil {
			return nil, fmt.Errorf("map literal type is nil")
		}
		entries := make([]ast.Expr, 0, len(v.Entries))
		for _, entry := range v.Entries {
			key, err := e.emitExpr(entry.Key, locals)
			if err != nil {
				return nil, err
			}
			value, err := e.emitExpr(entry.Value, locals)
			if err != nil {
				return nil, err
			}
			entries = append(entries, &ast.KeyValueExpr{Key: key, Value: value})
		}
		return &ast.CompositeLit{
			Type: typeExpr,
			Elts: entries,
		}, nil
	case *backendir.StructLiteralExpr:
		typeExpr, err := e.emitType(v.Type)
		if err != nil {
			return nil, err
		}
		if typeExpr == nil {
			return nil, fmt.Errorf("struct literal type is nil")
		}
		fields := make([]ast.Expr, 0, len(v.Fields))
		for _, field := range v.Fields {
			value, err := e.emitExpr(field.Value, locals)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &ast.KeyValueExpr{
				Key:   ast.NewIdent(goName(strings.TrimSpace(field.Name), true)),
				Value: value,
			})
		}
		return &ast.CompositeLit{
			Type: typeExpr,
			Elts: fields,
		}, nil
	case *backendir.EnumVariantExpr:
		typeExpr, err := e.emitType(v.Type)
		if err != nil {
			return nil, err
		}
		if typeExpr == nil {
			return nil, fmt.Errorf("enum variant type is nil")
		}
		return &ast.CompositeLit{
			Type: typeExpr,
			Elts: []ast.Expr{
				&ast.KeyValueExpr{
					Key:   ast.NewIdent("Tag"),
					Value: &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(v.Discriminant)},
				},
			},
		}, nil
	case *backendir.IfExpr:
		return e.emitIfExpr(v, locals)
	case *backendir.UnionMatchExpr:
		return e.emitUnionMatchExpr(v, locals)
	case *backendir.TryExpr:
		return e.emitTryExpr(v, locals)
	case *backendir.PanicExpr:
		return e.emitPanicExpr(v, locals)
	case *backendir.BlockExpr:
		return e.emitBlockExpr(v, locals)
	case *backendir.CopyExpr:
		switch literal := v.Value.(type) {
		case *backendir.ListLiteralExpr:
			if len(literal.Elements) == 0 {
				return e.emitExpr(literal, locals)
			}
		case *backendir.MapLiteralExpr:
			if len(literal.Entries) == 0 {
				return e.emitExpr(literal, locals)
			}
		}
		value, err := e.emitExpr(v.Value, locals)
		if err != nil {
			return nil, err
		}
		typeExpr, err := e.emitType(v.Type)
		if err != nil {
			return nil, err
		}
		if typeExpr == nil {
			return nil, fmt.Errorf("copy expression type is nil")
		}
		return &ast.CallExpr{
			Fun: ast.NewIdent("append"),
			Args: []ast.Expr{
				&ast.CallExpr{Fun: typeExpr, Args: []ast.Expr{ast.NewIdent("nil")}},
				value,
			},
			Ellipsis: token.Pos(1),
		}, nil
	case *backendir.SelectorExpr:
		subject, err := e.emitExpr(v.Subject, locals)
		if err != nil {
			return nil, err
		}
		if ident, ok := subject.(*ast.Ident); ok {
			if strings.Contains(ident.Name, "/") {
				ident = ast.NewIdent(packageNameForModulePath(ident.Name))
				subject = ident
			}
		}
		return selectorExpr(subject, goName(v.Name, true)), nil
	case *backendir.CallExpr:
		return e.emitCallExpr(v, locals)
	case *backendir.TraitCoerceExpr:
		return e.emitTraitCoerceExpr(v, locals)
	case *backendir.MaybeSomeExpr:
		return e.emitMaybeSomeExpr(v, locals)
	case *backendir.MaybeNoneExpr:
		return e.emitMaybeNoneExpr(v)
	case *backendir.ResultOkExpr:
		return e.emitResultOkExpr(v, locals)
	case *backendir.ResultErrExpr:
		return e.emitResultErrExpr(v, locals)
	case *backendir.AddressOfExpr:
		return e.emitAddressOfExpr(v, locals)
	case *backendir.FuncLiteralExpr:
		return e.emitFuncLiteralExpr(v, locals)
	default:
		return nil, fmt.Errorf("unsupported expr %T", expr)
	}
}

func (e *backendIREmitter) emitFuncLiteralExpr(expr *backendir.FuncLiteralExpr, locals map[string]string) (ast.Expr, error) {
	if expr == nil {
		return ast.NewIdent("nil"), nil
	}
	params := make([]*ast.Field, 0, len(expr.Params))
	bodyLocals := cloneStringMap(locals)
	bodySeen := seenLocalNames(bodyLocals)
	for _, param := range expr.Params {
		paramType, err := e.emitType(param.Type)
		if err != nil {
			return nil, fmt.Errorf("function literal param %s type: %w", param.Name, err)
		}
		if param.ByRef {
			paramType = &ast.StarExpr{X: paramType}
		}
		localName := uniqueLocalName(goName(param.Name, false), bodySeen)
		bodyLocals[param.Name] = bindLocalRef(localName, param.ByRef)
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(localName)}, Type: paramType})
	}

	returnType, err := e.emitType(expr.Return)
	if err != nil {
		return nil, fmt.Errorf("function literal return type: %w", err)
	}
	funcType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if !lowering.IsVoidIRType(expr.Return) {
		funcType.Results = funcResults(returnType)
	}

	bodyStmts, err := e.emitBlock(expr.Body, returnType, bodyLocals, bodySeen)
	if err != nil {
		return nil, err
	}
	if !lowering.IsVoidIRType(expr.Return) && !blockEndsInReturn(bodyStmts) {
		bodyStmts = append(bodyStmts, &ast.ReturnStmt{Results: []ast.Expr{zeroValueExpr(returnType)}})
	}
	return &ast.FuncLit{Type: funcType, Body: &ast.BlockStmt{List: bodyStmts}}, nil
}

func (e *backendIREmitter) emitAssignTargetExpr(target string, locals map[string]string, seenLocals map[string]struct{}) (ast.Expr, token.Token, error) {
	targetName := strings.TrimSpace(target)
	if !canEmitAssignTargetNatively(targetName) {
		return nil, token.ILLEGAL, fmt.Errorf("unsupported assign target %q", targetName)
	}
	if targetName == "_" {
		return ast.NewIdent("_"), token.ASSIGN, nil
	}
	if base, field, ok := splitMemberAssignTarget(targetName); ok {
		baseName := locals[base]
		if strings.TrimSpace(baseName) == "" {
			baseName = goName(base, false)
		}
		binding, _ := unwrapLocalRef(baseName)
		return selectorExpr(ast.NewIdent(binding), goName(field, true)), token.ASSIGN, nil
	}
	localName := locals[targetName]
	if localName == "" {
		localName = uniqueLocalName(goName(targetName, false), seenLocals)
		locals[targetName] = localName
		return ast.NewIdent(localName), token.DEFINE, nil
	}
	binding, byRef := unwrapLocalRef(localName)
	if byRef {
		return &ast.StarExpr{X: ast.NewIdent(binding)}, token.ASSIGN, nil
	}
	return ast.NewIdent(binding), token.ASSIGN, nil
}

func (e *backendIREmitter) emitIfExpr(expr *backendir.IfExpr, locals map[string]string) (ast.Expr, error) {
	cond, err := e.emitExpr(expr.Cond, locals)
	if err != nil {
		return nil, err
	}
	returnTypeExpr, err := e.emitType(expr.Type)
	if err != nil {
		return nil, err
	}
	isVoidResult := lowering.IsVoidIRType(expr.Type)
	if !isVoidResult && returnTypeExpr == nil {
		return nil, fmt.Errorf("if expression return type is nil")
	}

	thenLocals := cloneStringMap(locals)
	thenSeen := seenLocalNames(thenLocals)
	thenStmts, err := e.emitBlock(expr.Then, returnTypeExpr, thenLocals, thenSeen)
	if err != nil {
		return nil, err
	}
	if !isVoidResult && !blockEndsInReturn(thenStmts) {
		thenStmts = append(thenStmts, &ast.ReturnStmt{Results: []ast.Expr{zeroValueExpr(returnTypeExpr)}})
	}

	ifStmt := &ast.IfStmt{
		Cond: cond,
		Body: &ast.BlockStmt{List: thenStmts},
	}
	if expr.Else != nil {
		elseLocals := cloneStringMap(locals)
		elseSeen := seenLocalNames(elseLocals)
		elseStmts, err := e.emitBlock(expr.Else, returnTypeExpr, elseLocals, elseSeen)
		if err != nil {
			return nil, err
		}
		if !isVoidResult && !blockEndsInReturn(elseStmts) {
			elseStmts = append(elseStmts, &ast.ReturnStmt{Results: []ast.Expr{zeroValueExpr(returnTypeExpr)}})
		}
		ifStmt.Else = &ast.BlockStmt{List: elseStmts}
	}

	body := []ast.Stmt{ifStmt}
	if !isVoidResult && expr.Else == nil {
		body = append(body, &ast.ReturnStmt{Results: []ast.Expr{zeroValueExpr(returnTypeExpr)}})
	}

	funcType := &ast.FuncType{}
	if !isVoidResult {
		funcType.Results = funcResults(returnTypeExpr)
	}
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: funcType,
			Body: &ast.BlockStmt{List: body},
		},
	}, nil
}

func (e *backendIREmitter) emitIfExprStmt(expr *backendir.IfExpr, returnType ast.Expr, locals map[string]string, seenLocals map[string]struct{}) ([]ast.Stmt, error) {
	if expr == nil || expr.Cond == nil || expr.Then == nil {
		return nil, fmt.Errorf("invalid if expression statement")
	}
	cond, err := e.emitExpr(expr.Cond, locals)
	if err != nil {
		return nil, err
	}
	thenLocals := cloneStringMap(locals)
	thenSeen := cloneSet(seenLocals)
	thenStmts, err := e.emitBlock(expr.Then, returnType, thenLocals, thenSeen)
	if err != nil {
		return nil, err
	}
	ifStmt := &ast.IfStmt{Cond: cond, Body: &ast.BlockStmt{List: thenStmts}}
	if expr.Else != nil {
		elseLocals := cloneStringMap(locals)
		elseSeen := cloneSet(seenLocals)
		elseStmts, err := e.emitBlock(expr.Else, returnType, elseLocals, elseSeen)
		if err != nil {
			return nil, err
		}
		ifStmt.Else = &ast.BlockStmt{List: elseStmts}
	}
	return []ast.Stmt{ifStmt}, nil
}

func (e *backendIREmitter) emitIfExprAssignStmts(targetName string, expr *backendir.IfExpr, returnType ast.Expr, locals map[string]string, seenLocals map[string]struct{}) ([]ast.Stmt, error) {
	if expr == nil || expr.Cond == nil || expr.Then == nil || expr.Type == nil {
		return nil, fmt.Errorf("invalid if expression assignment")
	}
	target, tok, err := e.emitAssignTargetExpr(targetName, locals, seenLocals)
	if err != nil {
		return nil, err
	}
	typeExpr, err := e.emitType(expr.Type)
	if err != nil {
		return nil, err
	}
	out := make([]ast.Stmt, 0, 2)
	if tok == token.DEFINE && targetName != "_" {
		ident, ok := target.(*ast.Ident)
		if !ok {
			return nil, fmt.Errorf("if assignment target must be identifier")
		}
		out = append(out, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ident}, Type: typeExpr}}}})
	} else if tok == token.DEFINE {
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.DEFINE, Rhs: []ast.Expr{zeroValueExpr(typeExpr)}})
	}
	cond, err := e.emitExpr(expr.Cond, locals)
	if err != nil {
		return nil, err
	}
	thenLocals := cloneStringMap(locals)
	thenSeen := cloneSet(seenLocals)
	thenStmts, err := e.emitBlockAssigningReturns(expr.Then, expr.Type, target, returnType, thenLocals, thenSeen)
	if err != nil {
		return nil, err
	}
	ifStmt := &ast.IfStmt{Cond: cond, Body: &ast.BlockStmt{List: thenStmts}}
	if expr.Else != nil {
		elseLocals := cloneStringMap(locals)
		elseSeen := cloneSet(seenLocals)
		elseStmts, err := e.emitBlockAssigningReturns(expr.Else, expr.Type, target, returnType, elseLocals, elseSeen)
		if err != nil {
			return nil, err
		}
		ifStmt.Else = &ast.BlockStmt{List: elseStmts}
	} else {
		ifStmt.Else = &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{zeroValueExpr(typeExpr)}}}}
	}
	out = append(out, ifStmt)
	return out, nil
}

func (e *backendIREmitter) emitBlockExprStmt(expr *backendir.BlockExpr, returnType ast.Expr, locals map[string]string, seenLocals map[string]struct{}) ([]ast.Stmt, error) {
	if expr == nil || expr.Value == nil {
		return nil, fmt.Errorf("invalid block expression statement")
	}
	stmts := make([]ast.Stmt, 0, len(expr.Setup)+1)
	for _, setup := range expr.Setup {
		emitted, err := e.emitStmt(setup, returnType, locals, seenLocals)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, emitted...)
	}
	emittedValue, err := e.emitStmt(&backendir.ExprStmt{Value: expr.Value}, returnType, locals, seenLocals)
	if err != nil {
		return nil, err
	}
	stmts = append(stmts, emittedValue...)
	return stmts, nil
}

func (e *backendIREmitter) emitBlockExpr(expr *backendir.BlockExpr, locals map[string]string) (ast.Expr, error) {
	if expr == nil || expr.Value == nil || expr.Type == nil {
		return nil, fmt.Errorf("invalid block expression")
	}
	returnTypeExpr, err := e.emitType(expr.Type)
	if err != nil {
		return nil, err
	}
	isVoidResult := lowering.IsVoidIRType(expr.Type)
	if !isVoidResult && returnTypeExpr == nil {
		return nil, fmt.Errorf("block expression return type is nil")
	}

	bodyLocals := cloneStringMap(locals)
	bodySeen := seenLocalNames(bodyLocals)
	bodyStmts := make([]ast.Stmt, 0, len(expr.Setup)+1)
	for _, stmt := range expr.Setup {
		emitted, err := e.emitStmt(stmt, returnTypeExpr, bodyLocals, bodySeen)
		if err != nil {
			return nil, err
		}
		bodyStmts = append(bodyStmts, emitted...)
	}
	valueExpr, err := e.emitExpr(expr.Value, bodyLocals)
	if err != nil {
		return nil, err
	}
	if isVoidResult {
		if call, ok := valueExpr.(*ast.CallExpr); ok {
			bodyStmts = append(bodyStmts, &ast.ExprStmt{X: call})
		} else {
			bodyStmts = append(bodyStmts, &ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent("_")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{valueExpr},
			})
		}
	} else {
		bodyStmts = append(bodyStmts, &ast.ReturnStmt{Results: []ast.Expr{valueExpr}})
	}

	funcType := &ast.FuncType{}
	if !isVoidResult {
		funcType.Results = funcResults(returnTypeExpr)
	}
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: funcType,
			Body: &ast.BlockStmt{List: bodyStmts},
		},
	}, nil
}

func (e *backendIREmitter) emitUnionMatchStmt(expr *backendir.UnionMatchExpr, returnType ast.Expr, locals map[string]string, seenLocals map[string]struct{}) ([]ast.Stmt, error) {
	if expr == nil || expr.Subject == nil {
		return nil, fmt.Errorf("invalid union match statement")
	}
	subject, err := e.emitExpr(expr.Subject, locals)
	if err != nil {
		return nil, err
	}
	seen := cloneSet(seenLocals)
	matchValueName := uniqueLocalName("unionValue", seen)
	clauses := make([]ast.Stmt, 0, len(expr.Cases)+1)
	for _, matchCase := range orderedUnionMatchCases(expr.Cases) {
		caseType, err := e.emitType(matchCase.Type)
		if err != nil {
			return nil, err
		}
		caseLocals := cloneStringMap(locals)
		caseSeen := seenLocalNames(caseLocals)
		prefix := []ast.Stmt{}
		if pattern := strings.TrimSpace(matchCase.Pattern); pattern != "" && pattern != "_" {
			localPattern := uniqueLocalName(goName(pattern, false), caseSeen)
			caseLocals[pattern] = localPattern
			prefix = append(prefix,
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(localPattern)}, Tok: token.DEFINE, Rhs: []ast.Expr{ast.NewIdent(matchValueName)}},
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(localPattern)}},
			)
		}
		body, err := e.emitBlock(matchCase.Body, returnType, caseLocals, caseSeen)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, &ast.CaseClause{List: []ast.Expr{caseType}, Body: append(prefix, body...)})
	}
	if expr.CatchAll != nil {
		catchLocals := cloneStringMap(locals)
		catchSeen := seenLocalNames(catchLocals)
		catchBody, err := e.emitBlock(expr.CatchAll, returnType, catchLocals, catchSeen)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, &ast.CaseClause{Body: catchBody})
	}
	return []ast.Stmt{&ast.TypeSwitchStmt{
		Assign: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(matchValueName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: &ast.CallExpr{Fun: ast.NewIdent("any"), Args: []ast.Expr{subject}}}}},
		Body:   &ast.BlockStmt{List: clauses},
	}}, nil
}

func (e *backendIREmitter) emitUnionMatchAssignStmts(targetName string, expr *backendir.UnionMatchExpr, returnType ast.Expr, locals map[string]string, seenLocals map[string]struct{}) ([]ast.Stmt, error) {
	if expr == nil || expr.Subject == nil || expr.Type == nil {
		return nil, fmt.Errorf("invalid union match assignment")
	}
	target, tok, err := e.emitAssignTargetExpr(targetName, locals, seenLocals)
	if err != nil {
		return nil, err
	}
	typeExpr, err := e.emitType(expr.Type)
	if err != nil {
		return nil, err
	}
	out := make([]ast.Stmt, 0, 2)
	if tok == token.DEFINE && targetName != "_" {
		out = append(out, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{target.(*ast.Ident)}, Type: typeExpr}}}})
	} else if tok == token.DEFINE {
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.DEFINE, Rhs: []ast.Expr{zeroValueExpr(typeExpr)}})
	}

	subject, err := e.emitExpr(expr.Subject, locals)
	if err != nil {
		return nil, err
	}
	seen := cloneSet(seenLocals)
	matchValueName := uniqueLocalName("unionValue", seen)
	clauses := make([]ast.Stmt, 0, len(expr.Cases)+1)
	for _, matchCase := range orderedUnionMatchCases(expr.Cases) {
		caseType, err := e.emitType(matchCase.Type)
		if err != nil {
			return nil, err
		}
		caseLocals := cloneStringMap(locals)
		caseSeen := seenLocalNames(caseLocals)
		prefix := []ast.Stmt{}
		if pattern := strings.TrimSpace(matchCase.Pattern); pattern != "" && pattern != "_" {
			localPattern := uniqueLocalName(goName(pattern, false), caseSeen)
			caseLocals[pattern] = localPattern
			prefix = append(prefix,
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(localPattern)}, Tok: token.DEFINE, Rhs: []ast.Expr{ast.NewIdent(matchValueName)}},
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(localPattern)}},
			)
		}
		body, err := e.emitBlockAssigningReturns(matchCase.Body, expr.Type, target, returnType, caseLocals, caseSeen)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, &ast.CaseClause{List: []ast.Expr{caseType}, Body: append(prefix, body...)})
	}
	if expr.CatchAll != nil {
		catchLocals := cloneStringMap(locals)
		catchSeen := seenLocalNames(catchLocals)
		catchBody, err := e.emitBlockAssigningReturns(expr.CatchAll, expr.Type, target, returnType, catchLocals, catchSeen)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, &ast.CaseClause{Body: catchBody})
	}
	out = append(out, &ast.TypeSwitchStmt{
		Assign: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(matchValueName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: &ast.CallExpr{Fun: ast.NewIdent("any"), Args: []ast.Expr{subject}}}}},
		Body:   &ast.BlockStmt{List: clauses},
	})
	return out, nil
}

func (e *backendIREmitter) emitBlockAssigningReturns(block *backendir.Block, assignType backendir.Type, target ast.Expr, returnType ast.Expr, locals map[string]string, seenLocals map[string]struct{}) ([]ast.Stmt, error) {
	if block == nil {
		return nil, nil
	}
	out := make([]ast.Stmt, 0, len(block.Stmts))
	for _, stmt := range block.Stmts {
		ret, ok := stmt.(*backendir.ReturnStmt)
		if !ok || shouldReturnFromOuter(ret.Value, assignType) {
			emitted, err := e.emitStmt(stmt, returnType, locals, seenLocals)
			if err != nil {
				return nil, err
			}
			out = append(out, emitted...)
			continue
		}
		if tryExpr, ok := ret.Value.(*backendir.TryExpr); ok {
			emitted, err := e.emitTryExprControlStmts(tryExpr, returnType, locals, seenLocals, func(success ast.Expr) ([]ast.Stmt, error) {
				return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{success}}}, nil
			})
			if err != nil {
				return nil, err
			}
			out = append(out, emitted...)
			continue
		}
		value, err := e.emitExpr(ret.Value, locals)
		if err != nil {
			return nil, err
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{value}})
	}
	return out, nil
}

func shouldReturnFromOuter(value backendir.Expr, assignType backendir.Type) bool {
	switch v := value.(type) {
	case *backendir.ResultOkExpr:
		return !sameBackendIRType(v.Type, assignType)
	case *backendir.ResultErrExpr:
		return !sameBackendIRType(v.Type, assignType)
	default:
		return false
	}
}

func sameBackendIRType(a backendir.Type, b backendir.Type) bool {
	return backendIRTypeKey(a) == backendIRTypeKey(b)
}

func backendIRTypeKey(t backendir.Type) string {
	switch typed := t.(type) {
	case nil:
		return ""
	case *backendir.PrimitiveType:
		return "prim:" + typed.Name
	case *backendir.DynamicType:
		return "dynamic"
	case *backendir.VoidType:
		return "void"
	case *backendir.TypeVarType:
		return "var:" + typed.Name
	case *backendir.NamedType:
		parts := make([]string, 0, len(typed.Args))
		for _, arg := range typed.Args {
			parts = append(parts, backendIRTypeKey(arg))
		}
		return "named:" + typed.Module + ":" + typed.Name + "[" + strings.Join(parts, ",") + "]"
	case *backendir.ListType:
		return "list:" + backendIRTypeKey(typed.Elem)
	case *backendir.MapType:
		return "map:" + backendIRTypeKey(typed.Key) + ":" + backendIRTypeKey(typed.Value)
	case *backendir.MaybeType:
		return "maybe:" + backendIRTypeKey(typed.Of)
	case *backendir.ResultType:
		return "result:" + backendIRTypeKey(typed.Val) + ":" + backendIRTypeKey(typed.Err)
	case *backendir.FuncType:
		parts := make([]string, 0, len(typed.Params))
		for i, param := range typed.Params {
			key := backendIRTypeKey(param)
			if i < len(typed.ParamByRef) && typed.ParamByRef[i] {
				key = "*" + key
			}
			parts = append(parts, key)
		}
		return "fn:(" + strings.Join(parts, ",") + ")->" + backendIRTypeKey(typed.Return)
	default:
		return fmt.Sprintf("%T", t)
	}
}

func orderedUnionMatchCases(cases []backendir.UnionMatchCase) []backendir.UnionMatchCase {
	if len(cases) < 2 {
		return cases
	}
	out := make([]backendir.UnionMatchCase, 0, len(cases))
	dynamicCases := make([]backendir.UnionMatchCase, 0, 1)
	for _, matchCase := range cases {
		if _, ok := matchCase.Type.(*backendir.DynamicType); ok {
			dynamicCases = append(dynamicCases, matchCase)
			continue
		}
		out = append(out, matchCase)
	}
	return append(out, dynamicCases...)
}

func (e *backendIREmitter) emitUnionMatchExpr(expr *backendir.UnionMatchExpr, locals map[string]string) (ast.Expr, error) {
	subject, err := e.emitExpr(expr.Subject, locals)
	if err != nil {
		return nil, err
	}
	returnTypeExpr, err := e.emitType(expr.Type)
	if err != nil {
		return nil, err
	}
	isVoidResult := lowering.IsVoidIRType(expr.Type)
	if !isVoidResult && returnTypeExpr == nil {
		return nil, fmt.Errorf("union match return type is nil")
	}
	if !isVoidResult && expr.CatchAll == nil {
		return nil, fmt.Errorf("union match expression missing catch-all for value type")
	}
	if len(expr.Cases) == 0 {
		return nil, fmt.Errorf("union match expression has no cases")
	}

	seen := seenLocalNames(locals)
	matchValueName := uniqueLocalName("unionValue", seen)
	clauses := make([]ast.Stmt, 0, len(expr.Cases)+1)
	for _, matchCase := range orderedUnionMatchCases(expr.Cases) {
		caseType, err := e.emitType(matchCase.Type)
		if err != nil {
			return nil, err
		}
		caseLocals := cloneStringMap(locals)
		caseSeen := seenLocalNames(caseLocals)
		prefix := []ast.Stmt{}
		if pattern := strings.TrimSpace(matchCase.Pattern); pattern != "" && pattern != "_" {
			localPattern := uniqueLocalName(goName(pattern, false), caseSeen)
			caseLocals[pattern] = localPattern
			prefix = append(prefix,
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(localPattern)}, Tok: token.DEFINE, Rhs: []ast.Expr{ast.NewIdent(matchValueName)}},
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(localPattern)}},
			)
		}
		bodyStmts, err := e.emitBlock(matchCase.Body, returnTypeExpr, caseLocals, caseSeen)
		if err != nil {
			return nil, err
		}
		if !isVoidResult && !blockEndsInReturn(bodyStmts) {
			bodyStmts = append(bodyStmts, &ast.ReturnStmt{Results: []ast.Expr{zeroValueExpr(returnTypeExpr)}})
		}
		clauses = append(clauses, &ast.CaseClause{
			List: []ast.Expr{caseType},
			Body: append(prefix, bodyStmts...),
		})
	}
	if expr.CatchAll != nil {
		catchLocals := cloneStringMap(locals)
		catchSeen := seenLocalNames(catchLocals)
		catchStmts, err := e.emitBlock(expr.CatchAll, returnTypeExpr, catchLocals, catchSeen)
		if err != nil {
			return nil, err
		}
		if !isVoidResult && !blockEndsInReturn(catchStmts) {
			catchStmts = append(catchStmts, &ast.ReturnStmt{Results: []ast.Expr{zeroValueExpr(returnTypeExpr)}})
		}
		clauses = append(clauses, &ast.CaseClause{Body: catchStmts})
	}

	typeSwitch := &ast.TypeSwitchStmt{
		Assign: &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(matchValueName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{
				&ast.TypeAssertExpr{
					X: &ast.CallExpr{
						Fun:  ast.NewIdent("any"),
						Args: []ast.Expr{subject},
					},
				},
			},
		},
		Body: &ast.BlockStmt{List: clauses},
	}

	body := []ast.Stmt{typeSwitch}
	funcType := &ast.FuncType{}
	if !isVoidResult {
		funcType.Results = funcResults(returnTypeExpr)
	}
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: funcType,
			Body: &ast.BlockStmt{List: body},
		},
	}, nil
}

func (e *backendIREmitter) emitTryExprControlStmts(
	expr *backendir.TryExpr,
	returnType ast.Expr,
	locals map[string]string,
	seenLocals map[string]struct{},
	onSuccess func(ast.Expr) ([]ast.Stmt, error),
) ([]ast.Stmt, error) {
	return e.emitTryExprControlStmtsWithCatchLocals(expr, returnType, locals, seenLocals, nil, onSuccess)
}

func (e *backendIREmitter) emitTryExprControlStmtsWithCatchLocals(
	expr *backendir.TryExpr,
	returnType ast.Expr,
	locals map[string]string,
	seenLocals map[string]struct{},
	catchBaseLocals map[string]string,
	onSuccess func(ast.Expr) ([]ast.Stmt, error),
) ([]ast.Stmt, error) {
	if expr == nil || expr.Subject == nil || expr.Type == nil {
		return nil, fmt.Errorf("invalid try expression")
	}
	if onSuccess == nil {
		return nil, fmt.Errorf("try expression success handler is nil")
	}
	subject, err := e.emitExpr(expr.Subject, locals)
	if err != nil {
		return nil, err
	}
	tryValueName := uniqueLocalName("__ardTryValue", seenLocals)

	kind := strings.TrimSpace(expr.Kind)
	var (
		cond         ast.Expr
		success      ast.Expr
		catchBinding ast.Expr
	)
	switch kind {
	case "result":
		cond = &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tryValueName), "IsErr")}
		success = &ast.CallExpr{
			Fun: selectorExpr(ast.NewIdent(tryValueName), "ExpectRef"),
			Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("unreachable err in try success path")},
			},
		}
		catchBinding = &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tryValueName), "UnwrapErr")}
	case "maybe":
		cond = &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tryValueName), "IsNone")}
		success = &ast.CallExpr{
			Fun: selectorExpr(ast.NewIdent(tryValueName), "Expect"),
			Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("unreachable none in try success path")},
			},
		}
	default:
		return nil, fmt.Errorf("unsupported try expression kind %q", kind)
	}

	failureBody := []ast.Stmt{}
	if expr.Catch != nil {
		catchLocals := cloneStringMap(locals)
		if catchBaseLocals != nil {
			catchLocals = cloneStringMap(catchBaseLocals)
		}
		catchSeen := cloneSet(seenLocals)
		catchPrefix := []ast.Stmt{}
		catchVar := strings.TrimSpace(expr.CatchVar)
		if kind == "result" && catchBinding != nil && catchVar != "" && catchVar != "_" {
			catchLocal := uniqueLocalName(goName(catchVar, false), catchSeen)
			catchLocals[catchVar] = catchLocal
			catchPrefix = append(catchPrefix,
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(catchLocal)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{catchBinding},
				},
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent("_")},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{ast.NewIdent(catchLocal)},
				},
			)
		}

		catchReturnType := returnType
		if catchReturnType == nil {
			catchReturnType, err = e.emitType(expr.Type)
			if err != nil {
				return nil, err
			}
		}
		catchSeenFromLocals := seenLocalNames(catchLocals)
		for name := range catchSeen {
			catchSeenFromLocals[name] = struct{}{}
		}
		catchStmts, err := e.emitBlock(expr.Catch, catchReturnType, catchLocals, catchSeenFromLocals)
		if err != nil {
			return nil, err
		}
		failureBody = append(failureBody, catchPrefix...)
		failureBody = append(failureBody, catchStmts...)
	} else {
		failureBody, err = emitTryExprDefaultFailureStmts(kind, tryValueName, returnType)
		if err != nil {
			return nil, err
		}
	}

	out := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tryValueName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{subject},
		},
		&ast.IfStmt{
			Cond: cond,
			Body: &ast.BlockStmt{List: failureBody},
		},
	}
	successStmts, err := onSuccess(success)
	if err != nil {
		return nil, err
	}
	return append(out, successStmts...), nil
}

func emitTryExprDefaultFailureStmts(kind string, tryValueName string, returnType ast.Expr) ([]ast.Stmt, error) {
	if returnType == nil {
		return nil, fmt.Errorf("try expression without catch requires function return type")
	}
	switch strings.TrimSpace(kind) {
	case "result":
		valType, errType, ok := resultTypeArgsFromExpr(returnType)
		if !ok {
			return nil, fmt.Errorf("try result without catch requires result function return type")
		}
		return []ast.Stmt{
			&ast.ReturnStmt{
				Results: []ast.Expr{
					astCall(
						selectorExpr(ast.NewIdent(helperImportAlias), "Err"),
						[]ast.Expr{valType, errType},
						[]ast.Expr{
							&ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tryValueName), "UnwrapErr")},
						},
					),
				},
			},
		}, nil
	case "maybe":
		innerType, ok := maybeTypeArgFromExpr(returnType)
		if !ok {
			return nil, fmt.Errorf("try maybe without catch requires maybe function return type")
		}
		return []ast.Stmt{
			&ast.ReturnStmt{
				Results: []ast.Expr{
					astCall(
						selectorExpr(ast.NewIdent(helperImportAlias), "None"),
						[]ast.Expr{innerType},
						nil,
					),
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported try expression kind %q", strings.TrimSpace(kind))
	}
}

func resultTypeArgsFromExpr(expr ast.Expr) (ast.Expr, ast.Expr, bool) {
	args, ok := genericTypeArgsFromExpr(expr, "Result")
	if !ok || len(args) != 2 {
		return nil, nil, false
	}
	return args[0], args[1], true
}

func maybeTypeArgFromExpr(expr ast.Expr) (ast.Expr, bool) {
	args, ok := genericTypeArgsFromExpr(expr, "Maybe")
	if !ok || len(args) != 1 {
		return nil, false
	}
	return args[0], true
}

func genericTypeArgsFromExpr(expr ast.Expr, selectorName string) ([]ast.Expr, bool) {
	switch typed := expr.(type) {
	case *ast.IndexExpr:
		name, ok := selectorNameFromExpr(typed.X)
		if !ok || name != selectorName {
			return nil, false
		}
		return []ast.Expr{typed.Index}, true
	case *ast.IndexListExpr:
		name, ok := selectorNameFromExpr(typed.X)
		if !ok || name != selectorName {
			return nil, false
		}
		return typed.Indices, true
	default:
		return nil, false
	}
}

func selectorNameFromExpr(expr ast.Expr) (string, bool) {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil {
		return "", false
	}
	return strings.TrimSpace(selector.Sel.Name), true
}

func (e *backendIREmitter) emitTryExpr(expr *backendir.TryExpr, locals map[string]string) (ast.Expr, error) {
	if expr == nil {
		return nil, fmt.Errorf("try expression is nil")
	}
	if expr.Catch == nil {
		return nil, fmt.Errorf("try expression without catch cannot be emitted as pure expression")
	}
	subject, err := e.emitExpr(expr.Subject, locals)
	if err != nil {
		return nil, err
	}
	returnTypeExpr, err := e.emitType(expr.Type)
	if err != nil {
		return nil, err
	}
	isVoidResult := lowering.IsVoidIRType(expr.Type)
	if !isVoidResult && returnTypeExpr == nil {
		return nil, fmt.Errorf("try expression return type is nil")
	}

	seen := seenLocalNames(locals)
	tryValueName := uniqueLocalName("__ardTryValue", seen)
	body := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tryValueName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{subject},
		},
	}

	catchLocals := cloneStringMap(locals)
	catchSeen := seenLocalNames(catchLocals)
	catchSeen[tryValueName] = struct{}{}
	catchPrefix := []ast.Stmt{}
	var cond ast.Expr
	var success ast.Expr
	switch strings.TrimSpace(expr.Kind) {
	case "result":
		cond = &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tryValueName), "IsErr")}
		success = &ast.CallExpr{
			Fun: selectorExpr(ast.NewIdent(tryValueName), "ExpectRef"),
			Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("unreachable err in try success path")},
			},
		}
		catchVar := strings.TrimSpace(expr.CatchVar)
		if catchVar != "" && catchVar != "_" {
			localName := uniqueLocalName(goName(catchVar, false), catchSeen)
			catchLocals[catchVar] = localName
			catchPrefix = append(catchPrefix,
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(localName)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{&ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tryValueName), "UnwrapErr")}},
				},
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent("_")},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{ast.NewIdent(localName)},
				},
			)
		}
	case "maybe":
		cond = &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(tryValueName), "IsNone")}
		success = &ast.CallExpr{
			Fun: selectorExpr(ast.NewIdent(tryValueName), "Expect"),
			Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("unreachable none in try success path")},
			},
		}
	default:
		return nil, fmt.Errorf("unsupported try expression kind %q", strings.TrimSpace(expr.Kind))
	}

	catchStmts, err := e.emitBlock(expr.Catch, returnTypeExpr, catchLocals, catchSeen)
	if err != nil {
		return nil, err
	}
	catchBody := append(catchPrefix, catchStmts...)
	if !isVoidResult && !blockEndsInReturn(catchBody) {
		catchBody = append(catchBody, &ast.ReturnStmt{Results: []ast.Expr{zeroValueExpr(returnTypeExpr)}})
	}

	body = append(body, &ast.IfStmt{
		Cond: cond,
		Body: &ast.BlockStmt{List: catchBody},
	})
	if isVoidResult {
		if success != nil {
			body = append(body, &ast.ExprStmt{X: success})
		}
	} else {
		body = append(body, &ast.ReturnStmt{Results: []ast.Expr{success}})
	}

	funcType := &ast.FuncType{}
	if !isVoidResult {
		funcType.Results = funcResults(returnTypeExpr)
	}
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: funcType,
			Body: &ast.BlockStmt{List: body},
		},
	}, nil
}

func (e *backendIREmitter) emitPanicExpr(expr *backendir.PanicExpr, locals map[string]string) (ast.Expr, error) {
	return e.emitPanicExprWithExpectedType(expr, locals, nil)
}

func (e *backendIREmitter) emitPanicExprWithExpectedType(expr *backendir.PanicExpr, locals map[string]string, expectedType ast.Expr) (ast.Expr, error) {
	if expr == nil || expr.Message == nil || expr.Type == nil {
		return nil, fmt.Errorf("invalid panic expression")
	}
	message, err := e.emitExpr(expr.Message, locals)
	if err != nil {
		return nil, err
	}
	if lowering.IsVoidIRType(expr.Type) && expectedType == nil {
		return &ast.CallExpr{
			Fun:  ast.NewIdent("panic"),
			Args: []ast.Expr{message},
		}, nil
	}
	returnTypeExpr := expectedType
	if returnTypeExpr == nil {
		returnTypeExpr, err = e.emitType(expr.Type)
		if err != nil {
			return nil, err
		}
	}
	if returnTypeExpr == nil {
		return nil, fmt.Errorf("panic expression return type is nil")
	}
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{Results: funcResults(returnTypeExpr)},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ExprStmt{X: &ast.CallExpr{
					Fun:  ast.NewIdent("panic"),
					Args: []ast.Expr{message},
				}},
			}},
		},
	}, nil
}

func emitLiteralExpr(lit *backendir.LiteralExpr) ast.Expr {
	if lit == nil {
		return ast.NewIdent("nil")
	}
	switch lit.Kind {
	case "str":
		return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit.Value)}
	case "int":
		return &ast.BasicLit{Kind: token.INT, Value: lit.Value}
	case "float":
		return &ast.BasicLit{Kind: token.FLOAT, Value: lit.Value}
	case "bool":
		if lit.Value == "true" {
			return ast.NewIdent("true")
		}
		return ast.NewIdent("false")
	case "void":
		return ast.NewIdent("nil")
	default:
		return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit.Kind + ":" + lit.Value)}
	}
}

func (e *backendIREmitter) emitTraitCoerceExpr(expr *backendir.TraitCoerceExpr, locals map[string]string) (ast.Expr, error) {
	if expr == nil || expr.Value == nil || expr.Type == nil {
		return nil, fmt.Errorf("invalid trait coercion")
	}
	value, err := e.emitExpr(expr.Value, locals)
	if err != nil {
		return nil, err
	}
	traitType, ok := expr.Type.(*backendir.TraitType)
	if !ok || traitType == nil {
		return value, nil
	}
	switch strings.TrimSpace(traitType.Name) {
	case "ToString":
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "AsToString"), Args: []ast.Expr{value}}, nil
	case "Encodable":
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "AsEncodable"), Args: []ast.Expr{value}}, nil
	default:
		return value, nil
	}
}

func (e *backendIREmitter) emitMaybeSomeExpr(expr *backendir.MaybeSomeExpr, locals map[string]string) (ast.Expr, error) {
	if expr == nil || expr.Value == nil {
		return nil, fmt.Errorf("invalid maybe some expression")
	}
	maybeType, ok := expr.Type.(*backendir.MaybeType)
	if !ok || maybeType == nil {
		return nil, fmt.Errorf("maybe some expression type is %T", expr.Type)
	}
	innerType, err := e.emitType(maybeType.Of)
	if err != nil {
		return nil, err
	}
	value, err := e.emitExpr(expr.Value, locals)
	if err != nil {
		return nil, err
	}
	return astCall(selectorExpr(ast.NewIdent(helperImportAlias), "Some"), []ast.Expr{innerType}, []ast.Expr{value}), nil
}

func (e *backendIREmitter) emitMaybeNoneExpr(expr *backendir.MaybeNoneExpr) (ast.Expr, error) {
	if expr == nil {
		return nil, fmt.Errorf("invalid maybe none expression")
	}
	maybeType, ok := expr.Type.(*backendir.MaybeType)
	if !ok || maybeType == nil {
		return nil, fmt.Errorf("maybe none expression type is %T", expr.Type)
	}
	innerType, err := e.emitType(maybeType.Of)
	if err != nil {
		return nil, err
	}
	return astCall(selectorExpr(ast.NewIdent(helperImportAlias), "None"), []ast.Expr{innerType}, nil), nil
}

func (e *backendIREmitter) emitResultOkExpr(expr *backendir.ResultOkExpr, locals map[string]string) (ast.Expr, error) {
	if expr == nil || expr.Value == nil {
		return nil, fmt.Errorf("invalid result ok expression")
	}
	resultType, ok := expr.Type.(*backendir.ResultType)
	if !ok || resultType == nil {
		return nil, fmt.Errorf("result ok expression type is %T", expr.Type)
	}
	valType, err := e.emitType(resultType.Val)
	if err != nil {
		return nil, err
	}
	errType, err := e.emitType(resultType.Err)
	if err != nil {
		return nil, err
	}
	value, err := e.emitExpr(expr.Value, locals)
	if err != nil {
		return nil, err
	}
	return astCall(selectorExpr(ast.NewIdent(helperImportAlias), "Ok"), []ast.Expr{valType, errType}, []ast.Expr{value}), nil
}

func (e *backendIREmitter) emitResultErrExpr(expr *backendir.ResultErrExpr, locals map[string]string) (ast.Expr, error) {
	if expr == nil || expr.Value == nil {
		return nil, fmt.Errorf("invalid result err expression")
	}
	resultType, ok := expr.Type.(*backendir.ResultType)
	if !ok || resultType == nil {
		return nil, fmt.Errorf("result err expression type is %T", expr.Type)
	}
	valType, err := e.emitType(resultType.Val)
	if err != nil {
		return nil, err
	}
	errType, err := e.emitType(resultType.Err)
	if err != nil {
		return nil, err
	}
	value, err := e.emitExpr(expr.Value, locals)
	if err != nil {
		return nil, err
	}
	return astCall(selectorExpr(ast.NewIdent(helperImportAlias), "Err"), []ast.Expr{valType, errType}, []ast.Expr{value}), nil
}

func (e *backendIREmitter) emitAddressOfExpr(expr *backendir.AddressOfExpr, locals map[string]string) (ast.Expr, error) {
	if expr == nil || expr.Value == nil {
		return nil, fmt.Errorf("invalid address-of expression")
	}
	if ident, ok := expr.Value.(*backendir.IdentExpr); ok {
		if local := locals[strings.TrimSpace(ident.Name)]; local != "" {
			binding, byRef := unwrapLocalRef(local)
			if byRef {
				return ast.NewIdent(binding), nil
			}
			return &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(binding)}, nil
		}
	}
	value, err := e.emitExpr(expr.Value, locals)
	if err != nil {
		return nil, err
	}
	if !isAddressableASTExpr(value) {
		return nil, fmt.Errorf("address-of expression value is not assignable")
	}
	return &ast.UnaryExpr{Op: token.AND, X: value}, nil
}

func (e *backendIREmitter) emitCallExpr(call *backendir.CallExpr, locals map[string]string) (ast.Expr, error) {
	if ident, ok := call.Callee.(*backendir.IdentExpr); ok {
		switch ident.Name {
		case "int_add", "float_add", "str_add":
			return emitBinaryCall(call, token.ADD, e, locals)
		case "int_sub", "float_sub":
			return emitBinaryCall(call, token.SUB, e, locals)
		case "int_mul", "float_mul":
			return emitBinaryCall(call, token.MUL, e, locals)
		case "int_div", "float_div":
			return emitBinaryCall(call, token.QUO, e, locals)
		case "int_mod":
			return emitBinaryCall(call, token.REM, e, locals)
		case "int_gt", "float_gt":
			return emitBinaryCall(call, token.GTR, e, locals)
		case "int_gte", "float_gte":
			return emitBinaryCall(call, token.GEQ, e, locals)
		case "int_lt", "float_lt":
			return emitBinaryCall(call, token.LSS, e, locals)
		case "int_lte", "float_lte":
			return emitBinaryCall(call, token.LEQ, e, locals)
		case "eq":
			return emitBinaryCall(call, token.EQL, e, locals)
		case "and":
			return emitBinaryCall(call, token.LAND, e, locals)
		case "or":
			return emitBinaryCall(call, token.LOR, e, locals)
		case "not":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("not expects 1 arg, got %d", len(call.Args))
			}
			value, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			return &ast.UnaryExpr{Op: token.NOT, X: value}, nil
		case "neg":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("neg expects 1 arg, got %d", len(call.Args))
			}
			value, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			return &ast.UnaryExpr{Op: token.SUB, X: value}, nil
		case "str_size":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("str_size expects 1 arg, got %d", len(call.Args))
			}
			subject, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{subject}}, nil
		case "str_is_empty":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("str_is_empty expects 1 arg, got %d", len(call.Args))
			}
			subject, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			return &ast.BinaryExpr{
				X:  &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{subject}},
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
			}, nil
		case "str_contains":
			return emitCallToSelector(call, e, locals, "strings", "Contains", 2)
		case "str_replace":
			if len(call.Args) != 3 {
				return nil, fmt.Errorf("str_replace expects 3 args, got %d", len(call.Args))
			}
			args, err := e.emitArgs(call.Args, locals)
			if err != nil {
				return nil, err
			}
			args = append(args, &ast.BasicLit{Kind: token.INT, Value: "1"})
			return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent("strings"), "Replace"), Args: args}, nil
		case "str_replace_all":
			return emitCallToSelector(call, e, locals, "strings", "ReplaceAll", 3)
		case "str_split":
			return emitCallToSelector(call, e, locals, "strings", "Split", 2)
		case "str_starts_with":
			return emitCallToSelector(call, e, locals, "strings", "HasPrefix", 2)
		case "str_to_str":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("str_to_str expects 1 arg, got %d", len(call.Args))
			}
			return e.emitExpr(call.Args[0], locals)
		case "str_to_dyn":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("str_to_dyn expects 1 arg, got %d", len(call.Args))
			}
			return e.emitExpr(call.Args[0], locals)
		case "str_trim":
			return emitCallToSelector(call, e, locals, "strings", "TrimSpace", 1)
		case "int_to_str":
			return emitCallToSelector(call, e, locals, "strconv", "Itoa", 1)
		case "int_to_dyn":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("int_to_dyn expects 1 arg, got %d", len(call.Args))
			}
			return e.emitExpr(call.Args[0], locals)
		case "float_to_str":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("float_to_str expects 1 arg, got %d", len(call.Args))
			}
			subject, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{
				Fun: selectorExpr(ast.NewIdent("strconv"), "FormatFloat"),
				Args: []ast.Expr{
					subject,
					&ast.BasicLit{Kind: token.CHAR, Value: "'f'"},
					&ast.BasicLit{Kind: token.INT, Value: "2"},
					&ast.BasicLit{Kind: token.INT, Value: "64"},
				},
			}, nil
		case "float_to_int":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("float_to_int expects 1 arg, got %d", len(call.Args))
			}
			subject, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{Results: funcResults(ast.NewIdent("int"))},
					Body: &ast.BlockStmt{List: []ast.Stmt{
						&ast.AssignStmt{
							Lhs: []ast.Expr{ast.NewIdent("value")},
							Tok: token.DEFINE,
							Rhs: []ast.Expr{
								&ast.CallExpr{Fun: ast.NewIdent("float64"), Args: []ast.Expr{subject}},
							},
						},
						&ast.ReturnStmt{
							Results: []ast.Expr{
								&ast.CallExpr{Fun: ast.NewIdent("int"), Args: []ast.Expr{ast.NewIdent("value")}},
							},
						},
					}},
				},
			}, nil
		case "float_to_dyn":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("float_to_dyn expects 1 arg, got %d", len(call.Args))
			}
			return e.emitExpr(call.Args[0], locals)
		case "bool_to_str":
			return emitCallToSelector(call, e, locals, "strconv", "FormatBool", 1)
		case "bool_to_dyn":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("bool_to_dyn expects 1 arg, got %d", len(call.Args))
			}
			return e.emitExpr(call.Args[0], locals)
		case "list_size":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("list_size expects 1 arg, got %d", len(call.Args))
			}
			subject, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{subject}}, nil
		case "list_at":
			if len(call.Args) != 2 {
				return nil, fmt.Errorf("list_at expects 2 args, got %d", len(call.Args))
			}
			subject, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			index, err := e.emitExpr(call.Args[1], locals)
			if err != nil {
				return nil, err
			}
			return &ast.IndexExpr{X: subject, Index: index}, nil
		case "list_push":
			return emitCallToSelectorWithAddressedFirstArg(call, e, locals, helperImportAlias, "ListPush", 2)
		case "list_prepend":
			return emitCallToSelectorWithAddressedFirstArg(call, e, locals, helperImportAlias, "ListPrepend", 2)
		case "list_set":
			return emitCallToSelector(call, e, locals, helperImportAlias, "ListSet", 3)
		case "list_sort":
			if len(call.Args) != 2 {
				return nil, fmt.Errorf("list_sort expects 2 args, got %d", len(call.Args))
			}
			subject, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			comparator, err := e.emitExpr(call.Args[1], locals)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{
				Fun: selectorExpr(ast.NewIdent("sort"), "SliceStable"),
				Args: []ast.Expr{
					subject,
					&ast.FuncLit{
						Type: &ast.FuncType{
							Params: &ast.FieldList{List: []*ast.Field{
								{Names: []*ast.Ident{ast.NewIdent("i")}, Type: ast.NewIdent("int")},
								{Names: []*ast.Ident{ast.NewIdent("j")}, Type: ast.NewIdent("int")},
							}},
							Results: funcResults(ast.NewIdent("bool")),
						},
						Body: &ast.BlockStmt{List: []ast.Stmt{
							&ast.ReturnStmt{Results: []ast.Expr{
								&ast.CallExpr{
									Fun: comparator,
									Args: []ast.Expr{
										&ast.IndexExpr{X: subject, Index: ast.NewIdent("i")},
										&ast.IndexExpr{X: subject, Index: ast.NewIdent("j")},
									},
								},
							}},
						}},
					},
				},
			}, nil
		case "list_swap":
			return emitCallToSelector(call, e, locals, helperImportAlias, "ListSwap", 3)
		case "map_size":
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("map_size expects 1 arg, got %d", len(call.Args))
			}
			subject, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{subject}}, nil
		case "map_keys":
			return emitCallToSelector(call, e, locals, helperImportAlias, "MapKeys", 1)
		case "map_has":
			if len(call.Args) != 2 {
				return nil, fmt.Errorf("map_has expects 2 args, got %d", len(call.Args))
			}
			subject, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			key, err := e.emitExpr(call.Args[1], locals)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{Results: funcResults(ast.NewIdent("bool"))},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.AssignStmt{
								Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("ok")},
								Tok: token.DEFINE,
								Rhs: []ast.Expr{
									&ast.IndexExpr{X: subject, Index: key},
								},
							},
							&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("ok")}},
						},
					},
				},
			}, nil
		case "map_get":
			return emitCallToSelector(call, e, locals, helperImportAlias, "MapGet", 2)
		case "map_set":
			return emitCallToSelector(call, e, locals, helperImportAlias, "MapSet", 3)
		case "map_drop":
			return emitCallToSelector(call, e, locals, helperImportAlias, "MapDrop", 2)
		case "maybe_expect":
			return emitCallToMethod(call, e, locals, "Expect", 2)
		case "maybe_is_none":
			return emitCallToMethod(call, e, locals, "IsNone", 1)
		case "maybe_is_some":
			return emitCallToMethod(call, e, locals, "IsSome", 1)
		case "maybe_or":
			return emitCallToMethod(call, e, locals, "Or", 2)
		case "maybe_map":
			return emitCallToSelector(call, e, locals, helperImportAlias, "MaybeMap", 2)
		case "maybe_and_then":
			return emitCallToSelector(call, e, locals, helperImportAlias, "MaybeAndThen", 2)
		case "result_expect":
			if len(call.Args) != 2 {
				return nil, fmt.Errorf("result_expect expects 2 args, got %d", len(call.Args))
			}
			subject, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			message, err := e.emitExpr(call.Args[1], locals)
			if err != nil {
				return nil, err
			}
			if isAddressableASTExpr(subject) {
				return &ast.CallExpr{Fun: selectorExpr(subject, "ExpectRef"), Args: []ast.Expr{message}}, nil
			}
			return &ast.CallExpr{Fun: selectorExpr(subject, "Expect"), Args: []ast.Expr{message}}, nil
		case "result_or":
			return emitCallToMethod(call, e, locals, "Or", 2)
		case "result_is_ok":
			return emitCallToMethod(call, e, locals, "IsOk", 1)
		case "result_is_err":
			return emitCallToMethod(call, e, locals, "IsErr", 1)
		case "result_map":
			return emitCallToSelector(call, e, locals, helperImportAlias, "ResultMap", 2)
		case "result_map_err":
			return emitCallToSelector(call, e, locals, helperImportAlias, "ResultMapErr", 2)
		case "result_and_then":
			return emitCallToSelector(call, e, locals, helperImportAlias, "ResultAndThen", 2)
		case "fiber_start":
			return emitCallToSelector(call, e, locals, packageNameForModulePath("ard/async"), goName("start", true), 1)
		case "fiber_eval":
			return emitCallToSelector(call, e, locals, packageNameForModulePath("ard/async"), goName("eval", true), 1)
		case "fiber_execution":
			if len(call.Args) != 2 {
				return nil, fmt.Errorf("fiber_execution expects 2 args, got %d", len(call.Args))
			}
			modulePath, ok := literalExprStringValue(call.Args[0])
			if !ok {
				return nil, fmt.Errorf("fiber_execution module path must be string literal")
			}
			mainName, ok := literalExprStringValue(call.Args[1])
			if !ok {
				return nil, fmt.Errorf("fiber_execution function name must be string literal")
			}
			moduleAlias := packageNameForModulePath(modulePath)
			return &ast.CallExpr{
				Fun: selectorExpr(ast.NewIdent(packageNameForModulePath("ard/async")), goName("start", true)),
				Args: []ast.Expr{
					selectorExpr(ast.NewIdent(moduleAlias), goName(mainName, true)),
				},
			}, nil
		case "template":
			if len(call.Args) == 0 {
				return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("")}, nil
			}
			first, err := e.emitExpr(call.Args[0], locals)
			if err != nil {
				return nil, err
			}
			out := first
			for _, chunk := range call.Args[1:] {
				next, err := e.emitExpr(chunk, locals)
				if err != nil {
					return nil, err
				}
				out = &ast.BinaryExpr{X: out, Op: token.ADD, Y: next}
			}
			return out, nil
		}
	}

	callee, err := e.emitExpr(call.Callee, locals)
	if err != nil {
		return nil, err
	}
	args, err := e.emitArgs(call.Args, locals)
	if err != nil {
		return nil, err
	}
	if len(call.TypeArgs) > 0 && e.shouldEmitCallTypeArgs(call) {
		typeArgs := make([]ast.Expr, 0, len(call.TypeArgs))
		for _, arg := range call.TypeArgs {
			typeArg, err := e.emitType(arg)
			if err != nil {
				return nil, err
			}
			if typeArg == nil {
				typeArg = ast.NewIdent("any")
			}
			typeArgs = append(typeArgs, typeArg)
		}
		callee = indexExpr(callee, typeArgs)
	}
	return &ast.CallExpr{Fun: callee, Args: args}, nil
}

func (e *backendIREmitter) shouldEmitCallTypeArgs(call *backendir.CallExpr) bool {
	if call == nil || len(call.TypeArgs) == 0 {
		return false
	}
	if ident, ok := call.Callee.(*backendir.IdentExpr); ok {
		return e.functionTypeParams[strings.TrimSpace(ident.Name)] > 0
	}
	if _, ok := call.Callee.(*backendir.SelectorExpr); ok {
		return true
	}
	return false
}

func emitBinaryCall(call *backendir.CallExpr, op token.Token, e *backendIREmitter, locals map[string]string) (ast.Expr, error) {
	if len(call.Args) != 2 {
		return nil, fmt.Errorf("binary op expects 2 args, got %d", len(call.Args))
	}
	left, err := e.emitExpr(call.Args[0], locals)
	if err != nil {
		return nil, err
	}
	right, err := e.emitExpr(call.Args[1], locals)
	if err != nil {
		return nil, err
	}
	return &ast.BinaryExpr{X: left, Op: op, Y: right}, nil
}

func (e *backendIREmitter) emitArgs(args []backendir.Expr, locals map[string]string) ([]ast.Expr, error) {
	out := make([]ast.Expr, 0, len(args))
	for _, arg := range args {
		emitted, err := e.emitExpr(arg, locals)
		if err != nil {
			return nil, err
		}
		out = append(out, emitted)
	}
	return out, nil
}

func emitCallToSelector(call *backendir.CallExpr, e *backendIREmitter, locals map[string]string, pkg string, name string, arity int) (ast.Expr, error) {
	if len(call.Args) != arity {
		return nil, fmt.Errorf("%s expects %d args, got %d", strings.ToLower(name), arity, len(call.Args))
	}
	args, err := e.emitArgs(call.Args, locals)
	if err != nil {
		return nil, err
	}
	fun, err := e.emitSelectorFuncWithTypeArgs(pkg, name, call.TypeArgs)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{
		Fun:  fun,
		Args: args,
	}, nil
}

func (e *backendIREmitter) emitSelectorFuncWithTypeArgs(pkg string, name string, typeArgs []backendir.Type) (ast.Expr, error) {
	base := ast.Expr(selectorExpr(ast.NewIdent(pkg), name))
	if len(typeArgs) == 0 {
		return base, nil
	}
	args := make([]ast.Expr, 0, len(typeArgs))
	for _, typeArg := range typeArgs {
		emitted, err := e.emitType(typeArg)
		if err != nil {
			return nil, err
		}
		if emitted == nil {
			emitted = ast.NewIdent("any")
		}
		args = append(args, emitted)
	}
	return indexExpr(base, args), nil
}

func emitCallToSelectorWithAddressedFirstArg(call *backendir.CallExpr, e *backendIREmitter, locals map[string]string, pkg string, name string, arity int) (ast.Expr, error) {
	if len(call.Args) != arity {
		return nil, fmt.Errorf("%s expects %d args, got %d", strings.ToLower(name), arity, len(call.Args))
	}
	if arity < 1 {
		return nil, fmt.Errorf("%s expects at least 1 arg", strings.ToLower(name))
	}
	var first ast.Expr
	if ident, ok := call.Args[0].(*backendir.IdentExpr); ok {
		if local := locals[strings.TrimSpace(ident.Name)]; local != "" {
			binding, byRef := unwrapLocalRef(local)
			if byRef {
				first = ast.NewIdent(binding)
			}
		}
	}
	if first == nil {
		emitted, err := e.emitExpr(call.Args[0], locals)
		if err != nil {
			return nil, err
		}
		if !isAddressableASTExpr(emitted) {
			return nil, fmt.Errorf("%s first arg is not assignable", strings.ToLower(name))
		}
		first = &ast.UnaryExpr{Op: token.AND, X: emitted}
	}
	rest, err := e.emitArgs(call.Args[1:], locals)
	if err != nil {
		return nil, err
	}
	args := make([]ast.Expr, 0, len(rest)+1)
	args = append(args, first)
	args = append(args, rest...)
	fun, err := e.emitSelectorFuncWithTypeArgs(pkg, name, call.TypeArgs)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{
		Fun:  fun,
		Args: args,
	}, nil
}

func isAddressableASTExpr(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.Ident:
		return v != nil && strings.TrimSpace(v.Name) != ""
	case *ast.SelectorExpr:
		return v != nil && isAddressableASTExpr(v.X)
	default:
		return false
	}
}

func literalExprStringValue(expr backendir.Expr) (string, bool) {
	literal, ok := expr.(*backendir.LiteralExpr)
	if !ok || literal == nil {
		return "", false
	}
	if strings.TrimSpace(literal.Kind) != "str" {
		return "", false
	}
	return literal.Value, true
}

func emitCallToMethod(call *backendir.CallExpr, e *backendIREmitter, locals map[string]string, methodName string, arity int) (ast.Expr, error) {
	if len(call.Args) != arity {
		return nil, fmt.Errorf("%s expects %d args, got %d", strings.ToLower(methodName), arity, len(call.Args))
	}
	subject, err := e.emitExpr(call.Args[0], locals)
	if err != nil {
		return nil, err
	}
	args, err := e.emitArgs(call.Args[1:], locals)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{
		Fun:  selectorExpr(subject, methodName),
		Args: args,
	}, nil
}

func functionTypeParamsFromBackendIR(decl *backendir.FuncDecl) ([]string, map[string]string) {
	if decl == nil {
		return nil, nil
	}
	order := make([]string, 0, 2)
	seen := make(map[string]struct{})
	for _, param := range decl.Params {
		collectBackendIRTypeVars(param.Type, &order, seen)
	}
	collectBackendIRTypeVars(decl.Return, &order, seen)
	if len(order) == 0 {
		return nil, nil
	}
	return order, buildTypeParamMapping(order)
}

func functionTypeParamConstraints(decl *backendir.FuncDecl, order []string) map[string]string {
	constraints := make(map[string]string, len(order))
	for _, name := range order {
		constraints[name] = "any"
	}
	if decl == nil || len(order) == 0 {
		return constraints
	}
	comparableVars := make(map[string]struct{})
	for _, param := range decl.Params {
		collectComparableBackendIRTypeVars(param.Type, comparableVars)
	}
	collectComparableBackendIRTypeVars(decl.Return, comparableVars)
	for name := range comparableVars {
		if _, ok := constraints[name]; ok {
			constraints[name] = "comparable"
		}
	}
	return constraints
}

func collectComparableBackendIRTypeVars(t backendir.Type, out map[string]struct{}) {
	switch typed := t.(type) {
	case nil:
		return
	case *backendir.MapType:
		collectBackendIRTypeVarsIntoSet(typed.Key, out)
		collectComparableBackendIRTypeVars(typed.Value, out)
	case *backendir.ListType:
		collectComparableBackendIRTypeVars(typed.Elem, out)
	case *backendir.MaybeType:
		collectComparableBackendIRTypeVars(typed.Of, out)
	case *backendir.ResultType:
		collectComparableBackendIRTypeVars(typed.Val, out)
		collectComparableBackendIRTypeVars(typed.Err, out)
	case *backendir.FuncType:
		for _, param := range typed.Params {
			collectComparableBackendIRTypeVars(param, out)
		}
		collectComparableBackendIRTypeVars(typed.Return, out)
	case *backendir.NamedType:
		for _, arg := range typed.Args {
			collectComparableBackendIRTypeVars(arg, out)
		}
	}
}

func collectBackendIRTypeVarsIntoSet(t backendir.Type, out map[string]struct{}) {
	switch typed := t.(type) {
	case *backendir.TypeVarType:
		name := strings.TrimSpace(typed.Name)
		if name != "" {
			out[name] = struct{}{}
		}
	case *backendir.NamedType:
		for _, arg := range typed.Args {
			collectBackendIRTypeVarsIntoSet(arg, out)
		}
	case *backendir.ListType:
		collectBackendIRTypeVarsIntoSet(typed.Elem, out)
	case *backendir.MapType:
		collectBackendIRTypeVarsIntoSet(typed.Key, out)
		collectBackendIRTypeVarsIntoSet(typed.Value, out)
	case *backendir.MaybeType:
		collectBackendIRTypeVarsIntoSet(typed.Of, out)
	case *backendir.ResultType:
		collectBackendIRTypeVarsIntoSet(typed.Val, out)
		collectBackendIRTypeVarsIntoSet(typed.Err, out)
	case *backendir.FuncType:
		for _, param := range typed.Params {
			collectBackendIRTypeVarsIntoSet(param, out)
		}
		collectBackendIRTypeVarsIntoSet(typed.Return, out)
	}
}

func buildTypeParamMapping(order []string) map[string]string {
	if len(order) == 0 {
		return nil
	}
	used := make(map[string]struct{}, len(order))
	mapping := make(map[string]string, len(order))
	for _, name := range order {
		emitted := goName(name, true)
		if emitted == "Any" {
			emitted = "T"
		}
		if _, exists := used[emitted]; !exists {
			used[emitted] = struct{}{}
			mapping[name] = emitted
			continue
		}
		for i := 2; ; i++ {
			candidate := fmt.Sprintf("%s%d", emitted, i)
			if _, exists := used[candidate]; exists {
				continue
			}
			used[candidate] = struct{}{}
			mapping[name] = candidate
			break
		}
	}
	return mapping
}

func collectBackendIRTypeVars(t backendir.Type, out *[]string, seen map[string]struct{}) {
	switch typed := t.(type) {
	case *backendir.TypeVarType:
		name := strings.TrimSpace(typed.Name)
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		*out = append(*out, name)
	case *backendir.NamedType:
		for _, arg := range typed.Args {
			collectBackendIRTypeVars(arg, out, seen)
		}
	case *backendir.ListType:
		collectBackendIRTypeVars(typed.Elem, out, seen)
	case *backendir.MapType:
		collectBackendIRTypeVars(typed.Key, out, seen)
		collectBackendIRTypeVars(typed.Value, out, seen)
	case *backendir.MaybeType:
		collectBackendIRTypeVars(typed.Of, out, seen)
	case *backendir.ResultType:
		collectBackendIRTypeVars(typed.Val, out, seen)
		collectBackendIRTypeVars(typed.Err, out, seen)
	case *backendir.FuncType:
		for _, param := range typed.Params {
			collectBackendIRTypeVars(param, out, seen)
		}
		collectBackendIRTypeVars(typed.Return, out, seen)
	case *backendir.TraitType:
		for _, method := range typed.Methods {
			collectBackendIRTypeVars(method.Type, out, seen)
		}
	}
}

func (e *backendIREmitter) emitType(t backendir.Type) (ast.Expr, error) {
	return e.emitTypeWithTypeParams(t, e.typeParamMapping)
}

func (e *backendIREmitter) emitTypeWithTypeParams(t backendir.Type, typeParams map[string]string) (ast.Expr, error) {
	switch typed := t.(type) {
	case nil:
		return ast.NewIdent("any"), nil
	case *backendir.PrimitiveType:
		switch typed.Name {
		case "Int":
			return ast.NewIdent("int"), nil
		case "Float":
			return ast.NewIdent("float64"), nil
		case "Str":
			return ast.NewIdent("string"), nil
		case "Bool":
			return ast.NewIdent("bool"), nil
		default:
			return ast.NewIdent("any"), nil
		}
	case *backendir.DynamicType:
		return ast.NewIdent("any"), nil
	case *backendir.VoidType:
		return nil, nil
	case *backendir.TypeVarType:
		if typeParams != nil {
			if resolved := strings.TrimSpace(typeParams[typed.Name]); resolved != "" {
				return ast.NewIdent(resolved), nil
			}
		}
		return ast.NewIdent("any"), nil
	case *backendir.TraitType:
		switch strings.TrimSpace(typed.Name) {
		case "ToString":
			return selectorExpr(ast.NewIdent(helperImportAlias), "ToString"), nil
		case "Encodable":
			return selectorExpr(ast.NewIdent(helperImportAlias), "Encodable"), nil
		}
		fields := make([]*ast.Field, 0, len(typed.Methods))
		for _, method := range typed.Methods {
			methodType, err := e.emitTypeWithTypeParams(method.Type, typeParams)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &ast.Field{
				Names: []*ast.Ident{ast.NewIdent(goName(method.Name, true))},
				Type:  methodType,
			})
		}
		return &ast.InterfaceType{Methods: &ast.FieldList{List: fields}}, nil
	case *backendir.NamedType:
		if strings.TrimSpace(typed.Name) == "" {
			return ast.NewIdent("any"), nil
		}
		name := typed.Name
		if _, isExternType := e.externTypeNames[name]; isExternType && strings.TrimSpace(typed.Module) == "" {
			return ast.NewIdent("any"), nil
		}
		if strings.Contains(name, "/") {
			return ast.NewIdent("any"), nil
		}
		base := ast.Expr(ast.NewIdent(goName(name, true)))
		if module := strings.TrimSpace(typed.Module); module != "" {
			base = selectorExpr(ast.NewIdent(packageNameForModulePath(module)), goName(name, true))
		}
		if len(typed.Args) == 0 {
			return base, nil
		}
		args := make([]ast.Expr, 0, len(typed.Args))
		for _, arg := range typed.Args {
			typArg, err := e.emitTypeWithTypeParams(arg, typeParams)
			if err != nil {
				return nil, err
			}
			if typArg == nil {
				typArg = ast.NewIdent("any")
			}
			args = append(args, typArg)
		}
		return indexExpr(base, args), nil
	case *backendir.ListType:
		elem, err := e.emitTypeWithTypeParams(typed.Elem, typeParams)
		if err != nil {
			return nil, err
		}
		if elem == nil {
			elem = ast.NewIdent("any")
		}
		return &ast.ArrayType{Elt: elem}, nil
	case *backendir.MapType:
		key, err := e.emitTypeWithTypeParams(typed.Key, typeParams)
		if err != nil {
			return nil, err
		}
		value, err := e.emitTypeWithTypeParams(typed.Value, typeParams)
		if err != nil {
			return nil, err
		}
		if key == nil {
			key = ast.NewIdent("any")
		}
		if value == nil {
			value = ast.NewIdent("any")
		}
		return &ast.MapType{Key: key, Value: value}, nil
	case *backendir.MaybeType:
		innerType, err := e.emitTypeWithTypeParams(typed.Of, typeParams)
		if err != nil {
			return nil, err
		}
		if innerType == nil {
			innerType = &ast.StructType{Fields: &ast.FieldList{}}
		}
		return indexExpr(selectorExpr(ast.NewIdent(helperImportAlias), "Maybe"), []ast.Expr{innerType}), nil
	case *backendir.ResultType:
		valType, err := e.emitTypeWithTypeParams(typed.Val, typeParams)
		if err != nil {
			return nil, err
		}
		errType, err := e.emitTypeWithTypeParams(typed.Err, typeParams)
		if err != nil {
			return nil, err
		}
		if valType == nil {
			valType = &ast.StructType{Fields: &ast.FieldList{}}
		}
		if errType == nil {
			errType = &ast.StructType{Fields: &ast.FieldList{}}
		}
		return indexExpr(selectorExpr(ast.NewIdent(helperImportAlias), "Result"), []ast.Expr{valType, errType}), nil
	case *backendir.FuncType:
		params := make([]*ast.Field, 0, len(typed.Params))
		for i, param := range typed.Params {
			paramType, err := e.emitTypeWithTypeParams(param, typeParams)
			if err != nil {
				return nil, err
			}
			if paramType == nil {
				paramType = ast.NewIdent("any")
			}
			if i < len(typed.ParamByRef) && typed.ParamByRef[i] {
				paramType = &ast.StarExpr{X: paramType}
			}
			params = append(params, &ast.Field{Type: paramType})
		}
		var results *ast.FieldList
		if !lowering.IsVoidIRType(typed.Return) {
			returnType, err := e.emitTypeWithTypeParams(typed.Return, typeParams)
			if err != nil {
				return nil, err
			}
			results = funcResults(returnType)
		}
		return &ast.FuncType{
			Params:  &ast.FieldList{List: params},
			Results: results,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported IR type %T", t)
	}
}

func (e *backendIREmitter) emitEntrypointMainDecl() (ast.Decl, error) {
	callRegister := &ast.ExprStmt{
		X: &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "RegisterBuiltinExterns")},
	}

	mainName := e.functionNames["main"]
	body := []ast.Stmt{callRegister}
	if mainName != "" {
		if lowering.IsVoidIRType(e.functionReturns["main"]) {
			body = append(body, &ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent(mainName)}})
		} else {
			body = append(body, &ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent("_")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent(mainName)}},
			})
		}
	} else if e.entrypointBlock != nil {
		entrypointStmts, err := e.emitBlock(e.entrypointBlock, nil, map[string]string{}, map[string]struct{}{})
		if err != nil {
			return nil, fmt.Errorf("entrypoint: %w", err)
		}
		body = append(body, entrypointStmts...)
	}

	return &ast.FuncDecl{
		Name: ast.NewIdent("main"),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: body},
	}, nil
}

func blockEndsInReturn(stmts []ast.Stmt) bool {
	if len(stmts) == 0 {
		return false
	}
	switch last := stmts[len(stmts)-1].(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.IfStmt:
		thenBlock := last.Body
		elseBlock, okElse := last.Else.(*ast.BlockStmt)
		if !okElse {
			return false
		}
		return blockEndsInReturn(thenBlock.List) && blockEndsInReturn(elseBlock.List)
	default:
		return false
	}
}

func zeroValueExpr(typ ast.Expr) ast.Expr {
	switch t := typ.(type) {
	case *ast.Ident:
		switch t.Name {
		case "int", "float64":
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case "string":
			return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("")}
		case "bool":
			return ast.NewIdent("false")
		case "any":
			return ast.NewIdent("nil")
		}
	case *ast.ArrayType, *ast.MapType, *ast.InterfaceType, *ast.FuncType, *ast.StarExpr:
		return ast.NewIdent("nil")
	}
	return &ast.StarExpr{X: &ast.CallExpr{Fun: ast.NewIdent("new"), Args: []ast.Expr{typ}}}
}

func uniqueLocalName(base string, used map[string]struct{}) string {
	if base == "" {
		base = "v"
	}
	if _, ok := used[base]; !ok {
		used[base] = struct{}{}
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", base, i)
		if _, ok := used[candidate]; ok {
			continue
		}
		used[candidate] = struct{}{}
		return candidate
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func seenLocalNames(locals map[string]string) map[string]struct{} {
	seen := make(map[string]struct{}, len(locals))
	for _, name := range locals {
		if strings.TrimSpace(name) == "" {
			continue
		}
		seen[name] = struct{}{}
	}
	return seen
}

func cloneSet(input map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(input))
	for key := range input {
		out[key] = struct{}{}
	}
	return out
}
