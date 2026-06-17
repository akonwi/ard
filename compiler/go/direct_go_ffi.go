package gotarget

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strings"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
)

type directGoExternBinding struct {
	ImportPath string
	Symbols    []string
}

func parseDirectGoExternBinding(binding string) (directGoExternBinding, bool, error) {
	if !strings.HasPrefix(binding, "go:") {
		return directGoExternBinding{}, false, nil
	}
	trimmed := strings.TrimPrefix(binding, "go:")
	parts := strings.Split(trimmed, "::")
	if len(parts) < 2 {
		return directGoExternBinding{}, true, fmt.Errorf("direct Go extern binding %q must include a package path and symbol", binding)
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return directGoExternBinding{}, true, fmt.Errorf("direct Go extern binding %q contains an empty segment", binding)
		}
	}
	return directGoExternBinding{ImportPath: parts[0], Symbols: parts[1:]}, true, nil
}

func directGoImportAlias(importPath string) string {
	alias := sanitizeName(path.Base(importPath))
	if alias == "" {
		return "goffi"
	}
	return alias
}

func (l *lowerer) lowerDirectGoExternCall(ext air.Extern, binding string, args []ast.Expr, stmts []ast.Stmt) (loweredExpr, bool, error) {
	direct, ok, err := parseDirectGoExternBinding(binding)
	if err != nil || !ok {
		return loweredExpr{}, ok, err
	}
	signature, err := l.directGoSignature(direct)
	if err != nil {
		return loweredExpr{}, true, err
	}
	switch len(direct.Symbols) {
	case 1:
		coercedArgs, err := l.coerceDirectGoArgs(ext.Signature, args, signature, 0)
		if err != nil {
			return loweredExpr{}, true, err
		}
		alias := directGoImportAlias(direct.ImportPath)
		call := &ast.CallExpr{Fun: l.qualified(alias, direct.ImportPath, direct.Symbols[0]), Args: coercedArgs}
		adapted, err := l.adaptDirectGoReturn(ext.Signature.Return, call, signature.Results)
		if err != nil {
			return loweredExpr{}, true, err
		}
		adapted.stmts = append(stmts, adapted.stmts...)
		return adapted, true, nil
	case 2:
		if len(args) == 0 {
			return loweredExpr{}, true, fmt.Errorf("direct Go method binding %q requires a receiver argument", binding)
		}
		coercedArgs, err := l.coerceDirectGoArgs(ext.Signature, args, signature, 1)
		if err != nil {
			return loweredExpr{}, true, err
		}
		call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: coercedArgs[0], Sel: ast.NewIdent(direct.Symbols[1])}, Args: coercedArgs[1:]}
		adapted, err := l.adaptDirectGoReturn(ext.Signature.Return, call, signature.Results)
		if err != nil {
			return loweredExpr{}, true, err
		}
		adapted.stmts = append(stmts, adapted.stmts...)
		return adapted, true, nil
	default:
		return loweredExpr{}, true, fmt.Errorf("direct Go extern binding %q must be package::Function or package::Type::Method", binding)
	}
}

func (l *lowerer) directGoSignature(binding directGoExternBinding) (checker.GoSignature, error) {
	if l.directGoResolver == nil {
		return checker.GoSignature{}, nil
	}
	pkg, err := l.directGoResolver.LoadPackage(binding.ImportPath)
	if err != nil {
		return checker.GoSignature{}, fmt.Errorf("load Go package %q: %w", binding.ImportPath, err)
	}
	switch len(binding.Symbols) {
	case 1:
		fn, ok := pkg.Functions[binding.Symbols[0]]
		if !ok {
			return checker.GoSignature{}, fmt.Errorf("Go package %q has no exported function %q", binding.ImportPath, binding.Symbols[0])
		}
		return fn.Signature, nil
	case 2:
		typ, ok := pkg.Types[binding.Symbols[0]]
		if !ok {
			return checker.GoSignature{}, fmt.Errorf("Go package %q has no exported type %q", binding.ImportPath, binding.Symbols[0])
		}
		method, ok := typ.Methods[binding.Symbols[1]]
		if !ok {
			return checker.GoSignature{}, fmt.Errorf("Go type %q in package %q has no exported method %q", binding.Symbols[0], binding.ImportPath, binding.Symbols[1])
		}
		return method.Signature, nil
	default:
		return checker.GoSignature{}, nil
	}
}

func (l *lowerer) adaptDirectGoReturn(returnTypeID air.TypeID, call ast.Expr, results []checker.GoValueType) (loweredExpr, error) {
	if len(results) == 1 {
		if results[0].Kind == checker.GoValueError {
			return l.wrapErrorCall(returnTypeID, call)
		}
		expr, _, err := l.validateDirectGoReturnValue(returnTypeID, call, results[0])
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{expr: expr}, nil
	}
	if len(results) == 2 {
		switch results[1].Kind {
		case checker.GoValueError:
			return l.wrapDirectGoValueErrorCall(returnTypeID, call, results[0])
		case checker.GoValueBool:
			if results[1].Named {
				return loweredExpr{}, fmt.Errorf("direct Go maybe adapter requires bool, got named bool %s", results[1].String())
			}
			return l.wrapDirectGoValueBoolMaybeCall(returnTypeID, call, results[0])
		}
	}
	return loweredExpr{expr: call}, nil
}

func (l *lowerer) wrapDirectGoValueErrorCall(resultTypeID air.TypeID, call ast.Expr, valueResult checker.GoValueType) (loweredExpr, error) {
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
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(valueTemp)}, Type: valueType}}}},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(errTemp)}, Type: ast.NewIdent("error")}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp), ast.NewIdent(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
	}
	validatedValue, needsValidation, err := l.validateDirectGoReturnValue(resultType.Value, ast.NewIdent(valueTemp), valueResult)
	if err != nil {
		return loweredExpr{}, err
	}
	if needsValidation {
		stmts = append(stmts, &ast.IfStmt{Cond: &ast.BinaryExpr{X: ast.NewIdent(errTemp), Op: token.EQL, Y: ast.NewIdent("nil")}, Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{validatedValue}},
		}}})
	}
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

func (l *lowerer) wrapDirectGoValueBoolMaybeCall(maybeTypeID air.TypeID, call ast.Expr, valueResult checker.GoValueType) (loweredExpr, error) {
	if !validTypeID(l.program, maybeTypeID) {
		return loweredExpr{}, fmt.Errorf("invalid maybe type id %d", maybeTypeID)
	}
	maybeType := l.program.Types[maybeTypeID-1]
	if maybeType.Kind != air.TypeMaybe {
		return loweredExpr{}, fmt.Errorf("expected maybe type, got kind %d", maybeType.Kind)
	}
	valueType, err := l.goType(maybeType.Elem)
	if err != nil {
		return loweredExpr{}, err
	}
	valueTemp := l.nextTemp()
	okTemp := l.nextTemp()
	resultTemp := l.nextTemp()
	validatedValue, _, err := l.validateDirectGoReturnValue(maybeType.Elem, ast.NewIdent(valueTemp), valueResult)
	if err != nil {
		return loweredExpr{}, err
	}
	someExpr, err := l.maybeSomeExpr(maybeTypeID, validatedValue)
	if err != nil {
		return loweredExpr{}, err
	}
	noneExpr, err := l.maybeNoneExpr(maybeTypeID)
	if err != nil {
		return loweredExpr{}, err
	}
	maybeTypeExpr, err := l.goType(maybeTypeID)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(valueTemp)}, Type: valueType}}}},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(okTemp)}, Type: ast.NewIdent("bool")}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp), ast.NewIdent(okTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(resultTemp)}, Type: maybeTypeExpr}}}},
		&ast.IfStmt{Cond: ast.NewIdent(okTemp), Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}},
		}}, Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{noneExpr}},
		}}},
	}
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(resultTemp)}, nil
}

func (l *lowerer) validateDirectGoReturnValue(ardType air.TypeID, value ast.Expr, goType checker.GoValueType) (ast.Expr, bool, error) {
	if l.typeKind(ardType) == air.TypeRune && goType.Kind == checker.GoValueInt && goType.Bits == 32 {
		l.markRuntimeHelper("direct_go_valid_rune")
		return &ast.CallExpr{Fun: ast.NewIdent("ardDirectGoCheckRune"), Args: []ast.Expr{value}}, true, nil
	}
	return value, false, nil
}

func (l *lowerer) wrapValueBoolMaybeCall(maybeTypeID air.TypeID, call ast.Expr) (loweredExpr, error) {
	if !validTypeID(l.program, maybeTypeID) {
		return loweredExpr{}, fmt.Errorf("invalid maybe type id %d", maybeTypeID)
	}
	maybeType := l.program.Types[maybeTypeID-1]
	if maybeType.Kind != air.TypeMaybe {
		return loweredExpr{}, fmt.Errorf("expected maybe type, got kind %d", maybeType.Kind)
	}
	valueType, err := l.goType(maybeType.Elem)
	if err != nil {
		return loweredExpr{}, err
	}
	valueTemp := l.nextTemp()
	okTemp := l.nextTemp()
	resultTemp := l.nextTemp()
	someExpr, err := l.maybeSomeExpr(maybeTypeID, ast.NewIdent(valueTemp))
	if err != nil {
		return loweredExpr{}, err
	}
	noneExpr, err := l.maybeNoneExpr(maybeTypeID)
	if err != nil {
		return loweredExpr{}, err
	}
	maybeTypeExpr, err := l.goType(maybeTypeID)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(valueTemp)}, Type: valueType}}}},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(okTemp)}, Type: ast.NewIdent("bool")}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp), ast.NewIdent(okTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(resultTemp)}, Type: maybeTypeExpr}}}},
		&ast.IfStmt{Cond: ast.NewIdent(okTemp), Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}},
		}}, Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{noneExpr}},
		}}},
	}
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(resultTemp)}, nil
}

func (l *lowerer) coerceDirectGoArgs(signature air.Signature, args []ast.Expr, goSignature checker.GoSignature, argOffset int) ([]ast.Expr, error) {
	if len(goSignature.Params) == 0 {
		return args, nil
	}
	coerced := append([]ast.Expr(nil), args...)
	for i := argOffset; i < len(coerced); i++ {
		goParam := i - argOffset
		if i >= len(signature.Params) || goParam >= len(goSignature.Params) {
			break
		}
		arg, err := l.coerceDirectGoArg(signature.Params[i].Type, coerced[i], goSignature.Params[goParam])
		if err != nil {
			return nil, err
		}
		coerced[i] = arg
	}
	return coerced, nil
}

func (l *lowerer) coerceDirectGoArg(ardType air.TypeID, arg ast.Expr, goType checker.GoValueType) (ast.Expr, error) {
	if !l.directGoScalarNeedsConversion(ardType, goType) {
		return arg, nil
	}
	conversion, err := l.directGoTypeExpr(goType)
	if err != nil {
		return nil, err
	}
	checkedArg := l.checkedDirectGoArg(ardType, arg, goType)
	return &ast.CallExpr{Fun: conversion, Args: []ast.Expr{checkedArg}}, nil
}

func (l *lowerer) checkedDirectGoArg(ardType air.TypeID, arg ast.Expr, goType checker.GoValueType) ast.Expr {
	switch l.typeKind(ardType) {
	case air.TypeInt:
		switch goType.Kind {
		case checker.GoValueInt:
			if min, max, ok := directGoSignedRange(goType.Bits); ok {
				l.markRuntimeHelper("direct_go_signed_int_range")
				return &ast.CallExpr{Fun: ast.NewIdent("ardDirectGoCheckSignedIntRange"), Args: []ast.Expr{arg, intLit(min), intLit(max), stringLit(goType.String())}}
			}
		case checker.GoValueUint:
			if max, ok := directGoUintRange(goType.Bits); ok {
				l.markRuntimeHelper("direct_go_uint_int_range")
				return &ast.CallExpr{Fun: ast.NewIdent("ardDirectGoCheckUintIntRange"), Args: []ast.Expr{arg, uintLit(max), stringLit(goType.String())}}
			}
			l.markRuntimeHelper("direct_go_nonnegative_int")
			return &ast.CallExpr{Fun: ast.NewIdent("ardDirectGoCheckNonNegativeInt"), Args: []ast.Expr{arg, stringLit(goType.String())}}
		}
	case air.TypeFloat:
		if goType.Kind == checker.GoValueFloat && goType.Bits == 32 {
			l.markRuntimeHelper("direct_go_float32_range")
			return &ast.CallExpr{Fun: ast.NewIdent("ardDirectGoCheckFloat32Range"), Args: []ast.Expr{arg, stringLit(goType.String())}}
		}
	}
	return arg
}

func directGoSignedRange(bits int) (string, string, bool) {
	switch bits {
	case 8:
		return "-128", "127", true
	case 16:
		return "-32768", "32767", true
	case 32:
		return "-2147483648", "2147483647", true
	default:
		return "", "", false
	}
}

func directGoUintRange(bits int) (string, bool) {
	switch bits {
	case 8:
		return "255", true
	case 16:
		return "65535", true
	case 32:
		return "4294967295", true
	default:
		return "", false
	}
}

func intLit(value string) ast.Expr {
	return &ast.BasicLit{Kind: token.INT, Value: value}
}

func uintLit(value string) ast.Expr {
	return &ast.BasicLit{Kind: token.INT, Value: value}
}

func stringLit(value string) ast.Expr {
	return &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", value)}
}

func (l *lowerer) directGoScalarNeedsConversion(ardType air.TypeID, goType checker.GoValueType) bool {
	if goType.Kind == checker.GoValueInvalid || goType.Kind == checker.GoValueOther || goType.Kind == checker.GoValueError {
		return false
	}
	if goType.Named {
		return true
	}
	switch l.typeKind(ardType) {
	case air.TypeBool:
		return goType.Kind != checker.GoValueBool
	case air.TypeStr:
		return goType.Kind != checker.GoValueString
	case air.TypeInt:
		return goType.Kind != checker.GoValueInt || goType.Bits != 0
	case air.TypeByte:
		return goType.Kind != checker.GoValueUint || goType.Bits != 8 || goType.Expr != "uint8"
	case air.TypeRune:
		return goType.Kind != checker.GoValueInt || goType.Bits != 32 || goType.Expr != "int32"
	case air.TypeFloat:
		return goType.Kind != checker.GoValueFloat || goType.Bits != 64
	default:
		return false
	}
}

func (l *lowerer) directGoExternTypeExpr(binding string) (ast.Expr, bool, error) {
	direct, ok, err := parseDirectGoExternBinding(binding)
	if err != nil || !ok {
		return nil, ok, err
	}
	if len(direct.Symbols) != 1 {
		return nil, true, fmt.Errorf("direct Go extern type binding %q must be package::Type", binding)
	}
	alias := directGoImportAlias(direct.ImportPath)
	return l.qualified(alias, direct.ImportPath, direct.Symbols[0]), true, nil
}

func (l *lowerer) directGoTypeExpr(goType checker.GoValueType) (ast.Expr, error) {
	if goType.ImportPath != "" && goType.Package != "" {
		l.currentImports[goType.Package] = goType.ImportPath
	}
	expr, err := parser.ParseExpr(goType.Expr)
	if err != nil {
		return nil, fmt.Errorf("parse Go scalar conversion type %q: %w", goType.Expr, err)
	}
	return expr, nil
}
