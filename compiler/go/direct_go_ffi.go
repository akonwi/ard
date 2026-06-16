package gotarget

import (
	"fmt"
	"go/ast"
	"go/parser"
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
		return loweredExpr{stmts: stmts, expr: call}, true, nil
	case 2:
		if len(args) == 0 {
			return loweredExpr{}, true, fmt.Errorf("direct Go method binding %q requires a receiver argument", binding)
		}
		coercedArgs, err := l.coerceDirectGoArgs(ext.Signature, args, signature, 1)
		if err != nil {
			return loweredExpr{}, true, err
		}
		call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: coercedArgs[0], Sel: ast.NewIdent(direct.Symbols[1])}, Args: coercedArgs[1:]}
		return loweredExpr{stmts: stmts, expr: call}, true, nil
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
	return &ast.CallExpr{Fun: conversion, Args: []ast.Expr{arg}}, nil
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
