package gotarget

import (
	"fmt"
	"go/ast"
	"path"
	"strings"
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

func (l *lowerer) lowerDirectGoExternCall(binding string, args []ast.Expr, stmts []ast.Stmt) (loweredExpr, bool, error) {
	direct, ok, err := parseDirectGoExternBinding(binding)
	if err != nil || !ok {
		return loweredExpr{}, ok, err
	}
	switch len(direct.Symbols) {
	case 1:
		alias := directGoImportAlias(direct.ImportPath)
		call := &ast.CallExpr{Fun: l.qualified(alias, direct.ImportPath, direct.Symbols[0]), Args: args}
		return loweredExpr{stmts: stmts, expr: call}, true, nil
	case 2:
		if len(args) == 0 {
			return loweredExpr{}, true, fmt.Errorf("direct Go method binding %q requires a receiver argument", binding)
		}
		call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: args[0], Sel: ast.NewIdent(direct.Symbols[1])}, Args: args[1:]}
		return loweredExpr{stmts: stmts, expr: call}, true, nil
	default:
		return loweredExpr{}, true, fmt.Errorf("direct Go extern binding %q must be package::Function or package::Type::Method", binding)
	}
}
