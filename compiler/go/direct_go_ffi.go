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
	Alias      string
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
	importPath, alias := parseDirectGoImportHead(parts[0])
	if importPath == "" {
		return directGoExternBinding{}, true, fmt.Errorf("direct Go extern binding %q has invalid import path", binding)
	}
	return directGoExternBinding{ImportPath: importPath, Alias: alias, Symbols: parts[1:]}, true, nil
}

func parseDirectGoImportHead(head string) (string, string) {
	parts := strings.Split(head, " as ")
	if len(parts) == 1 {
		return strings.TrimSpace(head), ""
	}
	if len(parts) != 2 {
		return "", ""
	}
	importPath := strings.TrimSpace(parts[0])
	alias := strings.TrimSpace(parts[1])
	if importPath == "" || alias == "" {
		return "", ""
	}
	return importPath, alias
}

func (binding directGoExternBinding) importAlias() string {
	if binding.Alias != "" {
		return binding.Alias
	}
	return directGoImportAlias(binding.ImportPath)
}

func directGoImportAlias(importPath string) string {
	alias := sanitizeName(path.Base(importPath))
	if alias == "" {
		return "goffi"
	}
	return alias
}

func (l *lowerer) directGoBindingAlias(binding directGoExternBinding) string {
	return l.generatedGoImportAlias(binding.ImportPath, binding.importAlias())
}

func (l *lowerer) generatedGoImportAlias(importPath string, preferred string) string {
	if l.directGoAliases == nil {
		l.directGoAliases = map[string]string{}
	}
	if l.reservedGoIdentifiers == nil {
		l.reservedGoIdentifiers = collectReservedGoIdentifiers(l.program)
	}
	base := sanitizeName(preferred)
	if strings.HasPrefix(preferred, "_tmp_") || !validGeneratedGoImportAlias(base) {
		base = "ardgo"
	}
	key := importPath + "\x00" + preferred
	if alias, ok := l.directGoAliases[key]; ok {
		return alias
	}
	alias := base
	for i := 1; l.reservedGoIdentifiers[alias]; i++ {
		alias = fmt.Sprintf("%s_%d", base, i)
	}
	l.reservedGoIdentifiers[alias] = true
	l.directGoAliases[key] = alias
	return alias
}

func validGeneratedGoImportAlias(alias string) bool {
	return alias != "_" && alias != "init" && !strings.HasPrefix(alias, "_tmp_") && token.IsIdentifier(alias) && token.Lookup(alias) == token.IDENT
}

func collectReservedGoIdentifiers(program *air.Program) map[string]bool {
	reserved := map[string]bool{"main": true}
	for _, name := range predeclaredGoIdentifiers() {
		reserved[name] = true
	}
	for _, name := range runtimePreludeTopLevelNames() {
		reserved[name] = true
	}
	if program == nil {
		return reserved
	}
	for _, typ := range program.Types {
		reserved[typeName(program, typ)] = true
		reserved[fmt.Sprintf("ardJSONEncode_%d", typ.ID)] = true
		reserved[fmt.Sprintf("ardJSONEncodeTop_%d", typ.ID)] = true
		reserved[fmt.Sprintf("ardJSONMarshalTop_%d", typ.ID)] = true
		reserved[fmt.Sprintf("ardJSONDecodeText_%d", typ.ID)] = true
		for _, variant := range typ.Variants {
			reserved[enumVariantName(program, typ, variant)] = true
		}
	}
	for _, global := range program.Globals {
		reserved[globalName(program, global)] = true
	}
	for _, fn := range program.Functions {
		reserved[functionName(program, fn)] = true
		for _, local := range fn.Locals {
			reserved[localName(fn, local.ID)] = true
		}
	}
	return reserved
}

func predeclaredGoIdentifiers() []string {
	return []string{
		"any", "bool", "byte", "comparable", "complex64", "complex128", "error", "float32", "float64",
		"int", "int8", "int16", "int32", "int64", "rune", "string",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"true", "false", "iota", "nil",
		"append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make",
		"max", "min", "new", "panic", "print", "println", "real", "recover", "init",
		// Generated sort comparator parameters. Direct Go selectors can be nested inside those closures.
		"i", "j",
	}
}

func runtimePreludeTopLevelNames() []string {
	return []string{
		"ardFiberState", "ardFiber", "ardSpawnFiber", "ardJoinFiber", "ardGetFiber",
		"ardSortedIntKeys", "ardSortedStringKeys", "ardSortedAnyKeys", "ardListToAnySlice",
		"ardDirectGoCheckSignedIntRange", "ardDirectGoCheckUintIntRange", "ardDirectGoCheckNonNegativeInt",
		"ardDirectGoIntFromSigned", "ardDirectGoIntFromUnsigned",
		"ardDirectGoCheckFloat32Range", "ardDirectGoCheckRune",
		"ardJSONPath", "ardJSONFound", "ardJSONErr", "ardJSONMissing",
		"ardJSONDecodeInt", "ardJSONDecodeFloat", "ardJSONDecodeBool", "ardJSONDecodeString",
		"ardJSONDecodeDynamic", "ardJSONDecodeByteList", "ardJSONDecodeMaybe", "ardJSONDecodeList", "ardJSONDecodeStringMap",
		"ardJSONEncodeInt", "ardJSONEncodeFloat", "ardJSONEncodeBool", "ardJSONEncodeString", "ardJSONEncodeDynamic",
		"ardJSONEncodeMaybe", "ardJSONEncodeList", "ardJSONEncodeMap", "ardJSONEncodeStructuralMap",
	}
}

func (l *lowerer) lowerDirectGoExternCall(ext air.Extern, binding string, args []ast.Expr, stmts []ast.Stmt) (loweredExpr, bool, error) {
	direct, ok, err := parseDirectGoExternBinding(binding)
	if err != nil || !ok {
		return loweredExpr{}, ok, err
	}
	signature, err := l.directGoSignature(ext, direct)
	if err != nil {
		return loweredExpr{}, true, err
	}
	switch len(direct.Symbols) {
	case 1:
		coercedArgs, err := l.coerceDirectGoArgs(ext.Signature, args, signature, direct, 0)
		if err != nil {
			return loweredExpr{}, true, err
		}
		alias := l.directGoBindingAlias(direct)
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
		coercedArgs, err := l.coerceDirectGoArgs(ext.Signature, args, signature, direct, 1)
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

func (l *lowerer) lowerDirectGoPackageValue(binding string, typeID air.TypeID) (loweredExpr, error) {
	direct, ok, err := parseDirectGoExternBinding(binding)
	if err != nil {
		return loweredExpr{}, err
	}
	if !ok {
		return loweredExpr{}, fmt.Errorf("direct Go package value binding %q must use a go: binding", binding)
	}
	if len(direct.Symbols) != 1 || strings.HasPrefix(direct.Symbols[0], "*") {
		return loweredExpr{}, fmt.Errorf("direct Go package value binding %q must be package::Variable", binding)
	}
	alias := l.directGoBindingAlias(direct)
	expr := l.qualified(alias, direct.ImportPath, direct.Symbols[0])
	return loweredExpr{expr: l.directGoPackageValueConversion(typeID, expr)}, nil
}

func (l *lowerer) lowerDirectGoFieldAccess(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("direct Go field access missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if strings.TrimSpace(expr.Str) == "" {
		return loweredExpr{}, fmt.Errorf("direct Go field access missing field name")
	}
	selector := &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(expr.Str)}
	return loweredExpr{stmts: target.stmts, expr: l.directGoPackageValueConversion(expr.Type, selector)}, nil
}

func (l *lowerer) directGoPackageValueConversion(typeID air.TypeID, expr ast.Expr) ast.Expr {
	if !validTypeID(l.program, typeID) {
		return expr
	}
	switch l.program.Types[typeID-1].Kind {
	case air.TypeInt:
		return directGoConversionCall("int", expr)
	case air.TypeFloat:
		return directGoConversionCall("float64", expr)
	case air.TypeBool:
		return directGoConversionCall("bool", expr)
	case air.TypeStr:
		return directGoConversionCall("string", expr)
	case air.TypeByte:
		return directGoConversionCall("byte", expr)
	case air.TypeRune:
		return directGoConversionCall("rune", expr)
	default:
		return expr
	}
}

func (l *lowerer) directGoSignature(ext air.Extern, binding directGoExternBinding) (checker.GoSignature, error) {
	resolver := l.directGoResolverForExtern(ext)
	if resolver == nil {
		return checker.GoSignature{}, nil
	}
	pkg, err := resolver.LoadPackage(binding.ImportPath)
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

func (l *lowerer) directGoResolverForExtern(ext air.Extern) *checker.GoPackagesResolver {
	if l.projectInfo == nil {
		return l.directGoResolver
	}
	modulePath := modulePathForExtern(l.program, ext)
	if _, root, ok := dependencyPackageForModulePath(modulePath, l.projectInfo); ok && root != "" {
		return checker.NewGoPackagesResolver(root)
	}
	return l.directGoResolver
}

func (l *lowerer) adaptDirectGoReturn(returnTypeID air.TypeID, call ast.Expr, results []checker.GoValueType) (loweredExpr, error) {
	if len(results) == 1 {
		if results[0].Kind == checker.GoValueError {
			return l.wrapErrorCall(returnTypeID, call)
		}
		expr, _, err := l.adaptDirectGoReturnValue(returnTypeID, call, results[0])
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
	ardValueType, err := l.goType(resultType.Value)
	if err != nil {
		return loweredExpr{}, err
	}
	goValueTemp := l.nextTemp()
	errTemp := l.nextTemp()
	valueTemp := l.nextTemp()
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(valueTemp)}, Type: ardValueType}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(goValueTemp), ast.NewIdent(errTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
	}
	adaptedValue, _, err := l.adaptDirectGoReturnValue(resultType.Value, ast.NewIdent(goValueTemp), valueResult)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.IfStmt{Cond: &ast.BinaryExpr{X: ast.NewIdent(errTemp), Op: token.EQL, Y: ast.NewIdent("nil")}, Body: &ast.BlockStmt{List: []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{adaptedValue}},
	}}})
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
	valueTemp := l.nextTemp()
	okTemp := l.nextTemp()
	resultTemp := l.nextTemp()
	adaptedValue, _, err := l.adaptDirectGoReturnValue(maybeType.Elem, ast.NewIdent(valueTemp), valueResult)
	if err != nil {
		return loweredExpr{}, err
	}
	someExpr, err := l.maybeSomeExpr(maybeTypeID, adaptedValue)
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
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp), ast.NewIdent(okTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(resultTemp)}, Type: maybeTypeExpr}}}},
		&ast.IfStmt{Cond: ast.NewIdent(okTemp), Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}},
		}}, Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{noneExpr}},
		}}},
	}
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(resultTemp)}, nil
}

func (l *lowerer) adaptDirectGoReturnValue(ardType air.TypeID, value ast.Expr, goType checker.GoValueType) (ast.Expr, bool, error) {
	if checked, ok, err := l.validateDirectGoEnumReturnValue(ardType, value, goType); err != nil || ok {
		return checked, ok, err
	}
	converted, changed, err := l.convertDirectGoScalarReturn(ardType, value, goType)
	if err != nil {
		return nil, false, err
	}
	if l.typeKind(ardType) == air.TypeRune && goType.Kind == checker.GoValueInt && goType.Bits == 32 {
		l.markRuntimeHelper("direct_go_valid_rune")
		return &ast.CallExpr{Fun: ast.NewIdent("ardDirectGoCheckRune"), Args: []ast.Expr{converted}}, true, nil
	}
	return converted, changed, nil
}

func (l *lowerer) validateDirectGoReturnValue(ardType air.TypeID, value ast.Expr, goType checker.GoValueType) (ast.Expr, bool, error) {
	return l.adaptDirectGoReturnValue(ardType, value, goType)
}

func (l *lowerer) convertDirectGoScalarReturn(ardType air.TypeID, value ast.Expr, goType checker.GoValueType) (ast.Expr, bool, error) {
	switch l.typeKind(ardType) {
	case air.TypeBool:
		if goType.Kind == checker.GoValueBool && goType.Named {
			return directGoConversionCall("bool", value), true, nil
		}
	case air.TypeStr:
		if goType.Kind == checker.GoValueString && goType.Named {
			return directGoConversionCall("string", value), true, nil
		}
	case air.TypeInt:
		switch goType.Kind {
		case checker.GoValueInt:
			if goType.Bits == 0 {
				if goType.Named {
					return directGoConversionCall("int", value), true, nil
				}
				return value, false, nil
			}
			l.markRuntimeHelper("direct_go_signed_to_int")
			return &ast.CallExpr{Fun: ast.NewIdent("ardDirectGoIntFromSigned"), Args: []ast.Expr{directGoConversionCall("int64", value), stringLit(goType.String())}}, true, nil
		case checker.GoValueUint:
			l.markRuntimeHelper("direct_go_unsigned_to_int")
			return &ast.CallExpr{Fun: ast.NewIdent("ardDirectGoIntFromUnsigned"), Args: []ast.Expr{directGoConversionCall("uint64", value), stringLit(goType.String())}}, true, nil
		}
	case air.TypeByte:
		if goType.Kind == checker.GoValueUint && goType.Bits == 8 && (goType.Named || (goType.Expr != "uint8" && goType.Expr != "byte")) {
			return directGoConversionCall("byte", value), true, nil
		}
	case air.TypeRune:
		if goType.Kind == checker.GoValueInt && goType.Bits == 32 && goType.Named {
			return directGoConversionCall("rune", value), true, nil
		}
	case air.TypeFloat:
		if goType.Kind == checker.GoValueFloat && (goType.Named || goType.Bits != 64) {
			return directGoConversionCall("float64", value), true, nil
		}
	}
	return value, false, nil
}

func directGoConversionCall(typeName string, value ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: ast.NewIdent(typeName), Args: []ast.Expr{value}}
}

func (l *lowerer) validateDirectGoEnumReturnValue(ardType air.TypeID, value ast.Expr, goType checker.GoValueType) (ast.Expr, bool, error) {
	if !validTypeID(l.program, ardType) || !goType.Named {
		return value, false, nil
	}
	typ := l.program.Types[ardType-1]
	if typ.Kind != air.TypeEnum || typ.EnumOpen || strings.TrimSpace(typ.ExternBinding) == "" {
		return value, false, nil
	}
	direct, ok, err := parseDirectGoExternBinding(typ.ExternBinding)
	if err != nil || !ok {
		return value, ok, err
	}
	if len(direct.Symbols) != 1 || direct.ImportPath != goType.ImportPath || direct.Symbols[0] != goType.Name {
		return value, false, nil
	}
	checked, err := l.directGoEnumValidationCall(ardType, typ, value)
	return checked, true, err
}

func (l *lowerer) directGoEnumValidationCall(typeID air.TypeID, typ air.TypeInfo, value ast.Expr) (ast.Expr, error) {
	goType, err := l.goType(typeID)
	if err != nil {
		return nil, err
	}
	valueIdent := ast.NewIdent("value")
	cases := make([]ast.Stmt, 0, len(typ.Variants)+1)
	seen := map[int]bool{}
	for _, variant := range typ.Variants {
		if seen[variant.Discriminant] {
			continue
		}
		seen[variant.Discriminant] = true
		cases = append(cases, &ast.CaseClause{List: []ast.Expr{ast.NewIdent(enumVariantName(l.program, typ, variant))}, Body: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{valueIdent}}}})
	}
	message := fmt.Sprintf("Ard direct Go FFI: Go returned invalid %s", typ.Name)
	cases = append(cases, &ast.CaseClause{Body: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{stringLit(message)}}}}})
	fun := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{valueIdent}, Type: goType}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: goType}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.SwitchStmt{Tag: valueIdent, Body: &ast.BlockStmt{List: cases}}}},
	}
	return &ast.CallExpr{Fun: fun, Args: []ast.Expr{value}}, nil
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

func (l *lowerer) coerceDirectGoArgs(signature air.Signature, args []ast.Expr, goSignature checker.GoSignature, binding directGoExternBinding, argOffset int) ([]ast.Expr, error) {
	if len(goSignature.Params) == 0 {
		return args, nil
	}
	coerced := append([]ast.Expr(nil), args...)
	for i := argOffset; i < len(coerced); i++ {
		goParam := i - argOffset
		if i >= len(signature.Params) || goParam >= len(goSignature.Params) {
			break
		}
		arg, err := l.coerceDirectGoArg(signature.Params[i].Type, coerced[i], goSignature.Params[goParam], binding)
		if err != nil {
			return nil, err
		}
		coerced[i] = arg
	}
	return coerced, nil
}

func (l *lowerer) coerceDirectGoArg(ardType air.TypeID, arg ast.Expr, goType checker.GoValueType, binding directGoExternBinding) (ast.Expr, error) {
	if !l.directGoScalarNeedsConversion(ardType, goType) {
		return arg, nil
	}
	conversion, err := l.directGoTypeExpr(goType, binding)
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

func (l *lowerer) directGoEnumConstantExpr(typeBinding string, constantName string) (ast.Expr, bool, error) {
	direct, ok, err := parseDirectGoExternBinding(typeBinding)
	if err != nil || !ok {
		return nil, ok, err
	}
	if len(direct.Symbols) != 1 || strings.HasPrefix(direct.Symbols[0], "*") {
		return nil, true, fmt.Errorf("direct Go enum type binding %q must be package::Type", typeBinding)
	}
	return l.qualified(l.directGoBindingAlias(direct), direct.ImportPath, constantName), true, nil
}

func (l *lowerer) directGoExternTypeExpr(binding string) (ast.Expr, bool, error) {
	direct, ok, err := parseDirectGoExternBinding(binding)
	if err != nil || !ok {
		return nil, ok, err
	}
	if len(direct.Symbols) != 1 {
		return nil, true, fmt.Errorf("direct Go extern type binding %q must be package::Type", binding)
	}
	alias := l.directGoBindingAlias(direct)
	symbol := direct.Symbols[0]
	if strings.HasPrefix(symbol, "*") {
		typeName := strings.TrimPrefix(symbol, "*")
		if strings.TrimSpace(typeName) == "" {
			return nil, true, fmt.Errorf("direct Go extern type binding %q has empty pointer type", binding)
		}
		return &ast.StarExpr{X: l.qualified(alias, direct.ImportPath, typeName)}, true, nil
	}
	return l.qualified(alias, direct.ImportPath, symbol), true, nil
}

func rewriteQualifiedGoTypeExpr(expr string, pkg string, alias string) string {
	if pkg == "" || alias == "" || pkg == alias {
		return expr
	}
	return strings.ReplaceAll(expr, pkg+".", alias+".")
}

func (l *lowerer) directGoTypeExpr(goType checker.GoValueType, binding directGoExternBinding) (ast.Expr, error) {
	typeExpr := goType.Expr
	if strings.TrimSpace(typeExpr) == "" {
		typeExpr = goType.String()
	}
	if goType.ImportPath != "" && goType.Package != "" {
		preferred := goType.Package
		if binding.ImportPath == goType.ImportPath && binding.Alias != "" {
			preferred = binding.importAlias()
		}
		alias := l.generatedGoImportAlias(goType.ImportPath, preferred)
		typeExpr = rewriteQualifiedGoTypeExpr(typeExpr, goType.Package, alias)
		l.registerImport(alias, goType.ImportPath)
	}
	expr, err := parser.ParseExpr(typeExpr)
	if err != nil {
		return nil, fmt.Errorf("parse Go scalar conversion type %q: %w", goType.Expr, err)
	}
	return expr, nil
}
