//go:build ignore

package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/akonwi/ard/parse"
)

type contract struct {
	ExternTypes     []externType
	Aliases         []typeAlias
	Enums           []enumDecl
	Structs         []structDecl
	Functions       []hostFunction
	Skipped         []skippedExtern
	DirectGoImports map[string]string
}

type externType struct {
	Name string
	Type string
}

type typeAlias struct {
	Name string
	Type string
}

type enumDecl struct {
	Name string
}

type structDecl struct {
	Name        string
	TypeParam   string
	Fields      []structField
	Unsupported string
}

type structField struct {
	Name string
	Type string
}

type hostFunction struct {
	Binding string
	Field   string
	Type    string
	Params  []hostParam
	Returns []string
}

type hostParam struct {
	Name string
	Type string
}

type skippedExtern struct {
	Binding string
	Reason  string
}

type generateOptions struct {
	externDir   string
	hostOut     string
	hostPackage string
	goOut       string
	goPackage   string
	goImplDir   string
}

func main() {
	opts := generateOptions{}
	flag.StringVar(&opts.externDir, "extern-dir", "..", "directory containing Ard extern declarations")
	flag.StringVar(&opts.hostOut, "host-out", "ard.gen.go", "generated Go host contract output")
	flag.StringVar(&opts.hostPackage, "host-package", "ffi", "package name for generated Go host contract")
	flag.StringVar(&opts.goOut, "go-out", "", "generated Go-target stdlib lowering output")
	flag.StringVar(&opts.goPackage, "go-package", "gotarget", "package name for generated Go-target lowering")
	flag.StringVar(&opts.goImplDir, "go-impl-dir", ".", "directory containing Go FFI implementations for generated Go-target lowering")
	flag.Parse()

	c, err := loadContract(opts.externDir)
	if err != nil {
		panic(err)
	}
	goImports, err := collectGoImports(opts.goImplDir)
	if err != nil {
		panic(err)
	}
	source, err := render(c, opts.hostPackage, goImports)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(opts.hostOut, source, 0o644); err != nil {
		panic(err)
	}
	if opts.goOut != "" {
		implemented, err := collectExportedGoFuncs(opts.goImplDir)
		if err != nil {
			panic(err)
		}
		goLowering, err := renderGoStdlibLowering(c, opts.goPackage, implemented)
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(opts.goOut, goLowering, 0o644); err != nil {
			panic(err)
		}
	}
}

func loadContract(stdlibDir string) (contract, error) {
	files, err := filepath.Glob(filepath.Join(stdlibDir, "*.ard"))
	if err != nil {
		return contract{}, err
	}
	slices.Sort(files)

	c := contract{DirectGoImports: map[string]string{}}
	aliases := map[string]string{}
	definedTypes := map[string]struct{}{}
	bindings := map[string]hostFunction{}

	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			return contract{}, err
		}
		result := parse.Parse(source, file)
		if len(result.Errors) > 0 {
			return contract{}, fmt.Errorf("%s: %s", file, result.Errors[0].Message)
		}

		for _, imp := range result.Program.Imports {
			if imp.Kind != parse.ImportKindGo {
				continue
			}
			alias := imp.Name
			if alias == "" {
				alias = filepath.Base(imp.Path)
			}
			if existing := c.DirectGoImports[alias]; existing != "" && existing != imp.Path {
				return contract{}, fmt.Errorf("direct Go import alias %q is used for both %q and %q", alias, existing, imp.Path)
			}
			c.DirectGoImports[alias] = imp.Path
		}

		for _, stmt := range result.Program.Statements {
			switch node := stmt.(type) {
			case *parse.ExternTypeDeclaration:
				name := goExportedName(node.Name)
				goType := goExternTypeBinding(node)
				if goType != "" {
					aliases[node.Name] = goType
					aliases[name] = goType
				}
				definedTypes[name] = struct{}{}
				c.ExternTypes = append(c.ExternTypes, externType{Name: name, Type: goType})
			case *parse.TypeDeclaration:
				name := goExportedName(node.Name.Name)
				goType, ok := aliasGoType(node, c.DirectGoImports)
				if !ok {
					goType = "any"
				}
				aliases[node.Name.Name] = goType
				aliases[name] = goType
				definedTypes[name] = struct{}{}
				c.Aliases = append(c.Aliases, typeAlias{Name: name, Type: goType})
			case *parse.EnumDefinition:
				name := goExportedName(node.Name)
				definedTypes[name] = struct{}{}
				c.Enums = append(c.Enums, enumDecl{Name: name})
			case *parse.StructDefinition:
				decl := lowerStruct(node, aliases, definedTypes, c.DirectGoImports)
				definedTypes[decl.Name] = struct{}{}
				c.Structs = append(c.Structs, decl)
			}
		}
	}

	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			return contract{}, err
		}
		result := parse.Parse(source, file)
		if len(result.Errors) > 0 {
			return contract{}, fmt.Errorf("%s: %s", file, result.Errors[0].Message)
		}

		for _, stmt := range result.Program.Statements {
			node, ok := stmt.(*parse.ExternalFunction)
			if !ok {
				continue
			}
			binding := goBinding(node)
			if binding == "" || directGoBinding(binding) {
				continue
			}
			fn, err := lowerHostFunction(binding, node, aliases, definedTypes, c.DirectGoImports)
			if err != nil {
				c.Skipped = append(c.Skipped, skippedExtern{Binding: binding, Reason: err.Error()})
				continue
			}
			if existing, ok := bindings[binding]; ok {
				if existing.Type != fn.Type {
					return contract{}, fmt.Errorf("binding %s has conflicting signatures: %s vs %s", binding, existing.Type, fn.Type)
				}
				return contract{}, fmt.Errorf("duplicate extern binding %s", binding)
			}
			bindings[binding] = fn
			c.Functions = append(c.Functions, fn)
		}
	}

	slices.SortFunc(c.ExternTypes, func(a, b externType) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(c.Aliases, func(a, b typeAlias) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(c.Enums, func(a, b enumDecl) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(c.Structs, func(a, b structDecl) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(c.Functions, func(a, b hostFunction) int { return strings.Compare(a.Binding, b.Binding) })
	slices.SortFunc(c.Skipped, func(a, b skippedExtern) int { return strings.Compare(a.Binding, b.Binding) })

	return c, nil
}

func goBinding(fn *parse.ExternalFunction) string {
	if fn.ExternalBinding != "" {
		return fn.ExternalBinding
	}
	return fn.ExternalBindings["go"]
}

func directGoBinding(binding string) bool {
	return strings.Contains(binding, "::")
}

func goExternTypeBinding(typ *parse.ExternTypeDeclaration) string {
	if typ.ExternalBinding != "" {
		return typ.ExternalBinding
	}
	return typ.ExternalBindings["go"]
}

func aliasGoType(alias *parse.TypeDeclaration, directGoImports map[string]string) (string, bool) {
	if len(alias.Type) != 1 {
		return "any", true
	}
	if goType, ok := directGoAliasGoType(alias.Type[0], directGoImports); ok {
		return goType, true
	}
	if fn, ok := alias.Type[0].(*parse.FunctionType); ok {
		goType, err := functionTypeGoType(fn, nil, nil, directGoImports)
		return goType, err == nil
	}
	return "any", true
}

func directGoAliasGoType(typ parse.DeclaredType, directGoImports map[string]string) (string, bool) {
	switch t := typ.(type) {
	case *parse.MutableType:
		inner, ok := directGoAliasGoType(t.Inner, directGoImports)
		if !ok {
			return "", false
		}
		return "*" + inner, true
	case *parse.CustomType:
		goType, ok := directGoCustomTypeGoType(t, directGoImports)
		if !ok {
			return "", false
		}
		if t.IsNullable() {
			return "Maybe[" + goType + "]", true
		}
		return goType, true
	default:
		return "", false
	}
}

func directGoCustomTypeGoType(t *parse.CustomType, directGoImports map[string]string) (string, bool) {
	parts := strings.Split(t.Name, "::")
	if len(parts) != 2 || len(t.TypeArgs) > 0 {
		return "", false
	}
	alias := parts[0]
	importPath := directGoImports[alias]
	if importPath == "" {
		return "", false
	}
	if importPath == ffiSelfImportPath {
		// A reference to the ffi package itself (e.g. ffi::Runner) is emitted
		// unqualified, since ard.gen.go lives in package ffi.
		return parts[1], true
	}
	return alias + "." + parts[1], true
}

// ffiSelfImportPath is the import path of the std_lib/ffi package that hosts the
// generated ard.gen.go. References to it are emitted unqualified and never
// self-imported.
const ffiSelfImportPath = "github.com/akonwi/ard/std_lib/ffi"

func lowerStruct(node *parse.StructDefinition, aliases map[string]string, definedTypes map[string]struct{}, directGoImports map[string]string) structDecl {
	decl := structDecl{Name: goExportedName(node.Name.Name)}
	generics := map[string]struct{}{}
	for _, param := range node.TypeParams {
		generics[param] = struct{}{}
	}
	for _, field := range node.Fields {
		collectGenerics(field.Type, generics)
	}
	if len(generics) > 0 {
		if len(generics) > 1 {
			decl.Unsupported = "multiple generic struct parameters are not generated yet"
			return decl
		}
		for name := range generics {
			decl.TypeParam = name
		}
	}
	for _, field := range node.Fields {
		goType, err := typeGoType(field.Type, aliases, definedTypes, directGoImports)
		if err != nil {
			decl.Unsupported = err.Error()
			return decl
		}
		decl.Fields = append(decl.Fields, structField{
			Name: goExportedName(field.Name.Name),
			Type: goType,
		})
	}
	return decl
}

func lowerHostFunction(binding string, fn *parse.ExternalFunction, aliases map[string]string, definedTypes map[string]struct{}, directGoImports map[string]string) (hostFunction, error) {
	for _, param := range fn.Parameters {
		if hasGeneric(param.Type) {
			return hostFunction{}, fmt.Errorf("generic parameters are not generated yet")
		}
	}
	if hasGeneric(fn.ReturnType) {
		return hostFunction{}, fmt.Errorf("generic returns are not generated yet")
	}

	params := make([]string, 0, len(fn.Parameters))
	hostParams := make([]hostParam, 0, len(fn.Parameters))
	for _, param := range fn.Parameters {
		goType, err := typeGoType(param.Type, aliases, definedTypes, directGoImports)
		if err != nil {
			return hostFunction{}, fmt.Errorf("parameter %s: %w", param.Name, err)
		}
		params = append(params, fmt.Sprintf("%s %s", goIdentifier(param.Name), goType))
		hostParams = append(hostParams, hostParam{Name: goIdentifier(param.Name), Type: goType})
	}

	returns, err := returnGoTypes(fn.ReturnType, aliases, definedTypes, directGoImports)
	if err != nil {
		return hostFunction{}, err
	}

	signature := fmt.Sprintf("func(%s)", strings.Join(params, ", "))
	if len(returns) == 1 {
		signature += " " + returns[0]
	} else if len(returns) > 1 {
		signature += " (" + strings.Join(returns, ", ") + ")"
	}

	return hostFunction{
		Binding: binding,
		Field:   goExportedName(binding),
		Type:    signature,
		Params:  hostParams,
		Returns: returns,
	}, nil
}

func collectGoImports(dir string) (map[string]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	imports := map[string]string{}
	for _, path := range matches {
		if strings.HasSuffix(path, ".gen.go") || strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "generate.go" {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		for _, spec := range file.Imports {
			if spec.Path == nil {
				continue
			}
			importPath := strings.Trim(spec.Path.Value, "\"")
			if importPath == "" || importPath == "C" {
				continue
			}
			alias := ""
			if spec.Name != nil {
				if spec.Name.Name == "." || spec.Name.Name == "_" {
					continue
				}
				alias = spec.Name.Name
			} else {
				alias = filepath.Base(importPath)
			}
			imports[alias] = importPath
		}
	}
	return imports, nil
}

func returnGoTypes(typ parse.DeclaredType, aliases map[string]string, definedTypes map[string]struct{}, directGoImports map[string]string) ([]string, error) {
	if result, ok := typ.(*parse.ResultType); ok {
		if isVoidType(result.Val) {
			if !isStringType(result.Err) {
				errType, err := typeGoType(result.Err, aliases, definedTypes, directGoImports)
				if err != nil {
					return nil, err
				}
				return []string{fmt.Sprintf("Result[Void, %s]", errType)}, nil
			}
			return []string{"error"}, nil
		}
		valueType, err := typeGoType(result.Val, aliases, definedTypes, directGoImports)
		if err != nil {
			return nil, err
		}
		if !isStringType(result.Err) {
			errType, err := typeGoType(result.Err, aliases, definedTypes, directGoImports)
			if err != nil {
				return nil, err
			}
			return []string{fmt.Sprintf("Result[%s, %s]", valueType, errType)}, nil
		}
		return []string{valueType, "error"}, nil
	}
	if isVoidType(typ) {
		return nil, nil
	}
	goType, err := typeGoType(typ, aliases, definedTypes, directGoImports)
	if err != nil {
		return nil, err
	}
	return []string{goType}, nil
}

func typeGoType(typ parse.DeclaredType, aliases map[string]string, definedTypes map[string]struct{}, directGoImports map[string]string) (string, error) {
	nullable := typ.IsNullable()
	var goType string
	switch t := typ.(type) {
	case *parse.IntType:
		goType = "int"
	case *parse.FloatType:
		goType = "float64"
	case *parse.BooleanType:
		goType = "bool"
	case *parse.StringType:
		goType = "string"
	case *parse.VoidType:
		goType = "Void"
	case *parse.MutableType:
		inner, err := typeGoType(t.Inner, aliases, definedTypes, directGoImports)
		if err != nil {
			return "", err
		}
		goType = "*" + inner
	case *parse.List:
		elem, err := typeGoType(t.Element, aliases, definedTypes, directGoImports)
		if err != nil {
			return "", err
		}
		goType = "[]" + elem
	case *parse.Map:
		key, err := typeGoType(t.Key, aliases, definedTypes, directGoImports)
		if err != nil {
			return "", err
		}
		value, err := typeGoType(t.Value, aliases, definedTypes, directGoImports)
		if err != nil {
			return "", err
		}
		goType = fmt.Sprintf("map[%s]%s", key, value)
	case *parse.CustomType:
		name := goExportedName(t.Name)
		switch t.Name {
		case "Dynamic", "Encodable":
			goType = "any"
		case "Byte":
			goType = "byte"
		case "Rune":
			goType = "rune"
		default:
			if directGoType, ok := directGoCustomTypeGoType(t, directGoImports); ok {
				goType = directGoType
			} else if alias, ok := aliases[t.Name]; ok {
				goType = alias
			} else if _, ok := definedTypes[name]; ok {
				goType = name
			} else {
				goType = name
			}
			if len(t.TypeArgs) > 0 {
				args := make([]string, 0, len(t.TypeArgs))
				for _, arg := range t.TypeArgs {
					argType, err := typeGoType(arg, aliases, definedTypes, directGoImports)
					if err != nil {
						return "", err
					}
					args = append(args, argType)
				}
				goType += "[" + strings.Join(args, ", ") + "]"
			}
		}
	case *parse.GenericType:
		goType = t.Name
	case *parse.FunctionType:
		var err error
		goType, err = functionTypeGoType(t, aliases, definedTypes, directGoImports)
		if err != nil {
			return "", err
		}
	case *parse.ResultType:
		return "", fmt.Errorf("nested Result types are not generated yet")
	default:
		return "", fmt.Errorf("unsupported type %T", typ)
	}
	if nullable {
		return "Maybe[" + goType + "]", nil
	}
	return goType, nil
}

func functionTypeGoType(fn *parse.FunctionType, aliases map[string]string, definedTypes map[string]struct{}, directGoImports map[string]string) (string, error) {
	args := make([]string, 0, len(fn.Params)+1)
	for i, param := range fn.Params {
		paramType, err := typeGoType(param, aliases, definedTypes, directGoImports)
		if err != nil {
			return "", err
		}
		if i < len(fn.ParamMutability) && fn.ParamMutability[i] {
			paramType = "*" + paramType
		}
		args = append(args, paramType)
	}
	returnTypes := []string{}
	if fn.Return != nil {
		if hasGeneric(fn.Return) {
			return "", fmt.Errorf("generic function returns are not generated yet")
		}
		var err error
		returnTypes, err = functionReturnGoTypes(fn.Return, aliases, definedTypes, directGoImports)
		if err != nil {
			return "", err
		}
	}
	params := strings.Join(args, ", ")
	if len(fn.Params) == 0 {
		params = ""
	}
	if len(returnTypes) == 0 {
		return fmt.Sprintf("func(%s)", params), nil
	}
	return fmt.Sprintf("func(%s) (%s)", params, strings.Join(returnTypes, ", ")), nil
}

func functionReturnGoTypes(typ parse.DeclaredType, aliases map[string]string, definedTypes map[string]struct{}, directGoImports map[string]string) ([]string, error) {
	if result, ok := typ.(*parse.ResultType); ok {
		if !isStringType(result.Err) {
			returnTypes, err := returnGoTypes(typ, aliases, definedTypes, directGoImports)
			if err != nil {
				return nil, err
			}
			return returnTypes, nil
		}
		valueType := "Void"
		if !isVoidType(result.Val) {
			var err error
			valueType, err = typeGoType(result.Val, aliases, definedTypes, directGoImports)
			if err != nil {
				return nil, err
			}
		}
		return []string{valueType, "error"}, nil
	}
	if isVoidType(typ) {
		return nil, nil
	}
	returnType, err := typeGoType(typ, aliases, definedTypes, directGoImports)
	if err != nil {
		return nil, err
	}
	return []string{returnType}, nil
}

func contractTypeImports(c contract, availableImports map[string]string) (map[string]string, error) {
	imports := map[string]string{}
	collect := func(label string, typeExpr string) error {
		if strings.TrimSpace(typeExpr) == "" {
			return nil
		}
		found, err := typeExpressionImports(label, typeExpr, availableImports, c.DirectGoImports)
		if err != nil {
			return err
		}
		for alias, path := range found {
			imports[alias] = path
		}
		return nil
	}
	for _, typ := range c.ExternTypes {
		if err := collect("extern type binding "+typ.Name, typ.Type); err != nil {
			return nil, err
		}
	}
	for _, alias := range c.Aliases {
		if err := collect("type alias "+alias.Name, alias.Type); err != nil {
			return nil, err
		}
	}
	for _, strct := range c.Structs {
		for _, field := range strct.Fields {
			if err := collect("struct field "+strct.Name+"."+field.Name, field.Type); err != nil {
				return nil, err
			}
		}
	}
	for _, fn := range c.Functions {
		if err := collect("host function "+fn.Binding, fn.Type); err != nil {
			return nil, err
		}
	}
	return imports, nil
}

func typeExpressionImports(label string, typeExpr string, availableImports map[string]string, directGoImports map[string]string) (map[string]string, error) {
	imports := map[string]string{}
	expr, err := parser.ParseExpr(typeExpr)
	if err != nil {
		return nil, fmt.Errorf("parse %s = %q: %w", label, typeExpr, err)
	}
	missingAlias := ""
	ast.Inspect(expr, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if path := directGoImports[ident.Name]; path != "" {
			imports[ident.Name] = path
			return true
		}
		if path := availableImports[ident.Name]; path != "" {
			imports[ident.Name] = path
			return true
		}
		missingAlias = ident.Name
		return false
	})
	if missingAlias != "" {
		return nil, fmt.Errorf("%s = %q references Go package alias %q, but no matching import was found", label, typeExpr, missingAlias)
	}
	return imports, nil
}

func importsForContract(c contract, availableImports map[string]string) (map[string]string, error) {
	imports := map[string]string{"ardruntime": "github.com/akonwi/ard/runtime"}
	contractImports, err := contractTypeImports(c, availableImports)
	if err != nil {
		return nil, err
	}
	for alias, path := range contractImports {
		if path == ffiSelfImportPath {
			continue
		}
		imports[alias] = path
	}
	return imports, nil
}

func renderImports(out *strings.Builder, imports map[string]string) {
	aliases := make([]string, 0, len(imports))
	for alias := range imports {
		aliases = append(aliases, alias)
	}
	slices.Sort(aliases)
	if len(aliases) == 1 && aliases[0] == "ardruntime" {
		out.WriteString("import ardruntime \"github.com/akonwi/ard/runtime\"\n\n")
		return
	}
	out.WriteString("import (\n")
	for _, alias := range aliases {
		path := imports[alias]
		if alias == filepath.Base(path) {
			fmt.Fprintf(out, "\t%q\n", path)
			continue
		}
		fmt.Fprintf(out, "\t%s %q\n", alias, path)
	}
	out.WriteString(")\n\n")
}

func render(c contract, packageName string, availableImports map[string]string) ([]byte, error) {
	var out strings.Builder
	out.WriteString("// Code generated by ard; DO NOT EDIT.\n\n")
	fmt.Fprintf(&out, "package %s\n\n", packageName)
	imports, err := importsForContract(c, availableImports)
	if err != nil {
		return nil, err
	}
	renderImports(&out, imports)
	out.WriteString("type Void = struct{}\n\n")
	out.WriteString("type Maybe[T any] = ardruntime.Maybe[T]\n\n")
	out.WriteString("func Some[T any](value T) Maybe[T] {\n\treturn ardruntime.Some(value)\n}\n\n")
	out.WriteString("func None[T any]() Maybe[T] {\n\treturn ardruntime.None[T]()\n}\n\n")
	out.WriteString("type Result[T, E any] = ardruntime.Result[T, E]\n\n")
	out.WriteString("func Ok[T, E any](value T) Result[T, E] {\n\treturn ardruntime.Ok[T, E](value)\n}\n\n")
	out.WriteString("func Err[T, E any](err E) Result[T, E] {\n\treturn ardruntime.Err[T](err)\n}\n\n")
	for _, typ := range c.ExternTypes {
		if strings.TrimSpace(typ.Type) != "" {
			continue
		}
		fmt.Fprintf(&out, "type %s = any\n\n", typ.Name)
	}
	for _, alias := range c.Aliases {
		fmt.Fprintf(&out, "type %s = %s\n\n", alias.Name, alias.Type)
	}
	for _, enum := range c.Enums {
		fmt.Fprintf(&out, "type %s int\n\n", enum.Name)
	}
	for _, strct := range c.Structs {
		if strct.Unsupported != "" {
			fmt.Fprintf(&out, "// Skipped struct %s: %s.\n\n", strct.Name, strct.Unsupported)
			continue
		}
		if strct.TypeParam != "" {
			fmt.Fprintf(&out, "type %s[%s any] struct {\n", strct.Name, strct.TypeParam)
		} else {
			fmt.Fprintf(&out, "type %s struct {\n", strct.Name)
		}
		for _, field := range strct.Fields {
			fmt.Fprintf(&out, "\t%s %s\n", field.Name, field.Type)
		}
		out.WriteString("}\n\n")
	}

	for _, skipped := range c.Skipped {
		fmt.Fprintf(&out, "// Skipped extern %s: %s.\n", skipped.Binding, skipped.Reason)
	}

	return format.Source([]byte(out.String()))
}

func collectExportedGoFuncs(dir string) (map[string]bool, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, path := range matches {
		if strings.HasSuffix(path, ".gen.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !fn.Name.IsExported() {
				continue
			}
			out[fn.Name.Name] = true
		}
	}
	return out, nil
}

func renderGoStdlibLowering(c contract, packageName string, implemented map[string]bool) ([]byte, error) {
	var out strings.Builder
	out.WriteString("// Code generated by ard; DO NOT EDIT.\n\n")
	fmt.Fprintf(&out, "package %s\n\n", packageName)
	out.WriteString("import (\n")
	out.WriteString("\t\"go/ast\"\n\n")
	out.WriteString("\t\"github.com/akonwi/ard/air\"\n")
	out.WriteString(")\n\n")
	out.WriteString("type generatedStdlibExternKind uint8\n\n")
	out.WriteString("const (\n")
	out.WriteString("\tgeneratedStdlibExternCall generatedStdlibExternKind = iota\n")
	out.WriteString("\tgeneratedStdlibExternJSONEncode\n")
	out.WriteString("\tgeneratedStdlibExternJSONParse\n")
	out.WriteString(")\n\n")
	out.WriteString("type generatedStdlibExternReturn uint8\n\n")
	out.WriteString("const (\n")
	out.WriteString("\tgeneratedStdlibReturnDirect generatedStdlibExternReturn = iota\n")
	out.WriteString("\tgeneratedStdlibReturnError\n")
	out.WriteString("\tgeneratedStdlibReturnValueError\n")
	out.WriteString("\tgeneratedStdlibReturnResult\n")
	out.WriteString(")\n\n")
	out.WriteString("type generatedStdlibExternParamAdapter uint8\n\n")
	out.WriteString("const (\n")
	out.WriteString("\tgeneratedStdlibParamDirect generatedStdlibExternParamAdapter = iota\n")
	out.WriteString("\tgeneratedStdlibParamAny\n")
	out.WriteString("\tgeneratedStdlibParamAnySlice\n")
	out.WriteString(")\n\n")
	out.WriteString("type generatedStdlibExternLowering struct {\n")
	out.WriteString("\tkind     generatedStdlibExternKind\n")
	out.WriteString("\tfunction string\n")
	out.WriteString("\treturns  generatedStdlibExternReturn\n")
	out.WriteString("\tparams   []generatedStdlibExternParamAdapter\n")
	out.WriteString("}\n\n")
	specialBindings := map[string]bool{}
	for _, skipped := range c.Skipped {
		switch skipped.Binding {
		case "JsonEncode", "JsonParse":
			specialBindings[skipped.Binding] = true
		}
	}
	out.WriteString("var generatedStdlibExternLowerings = map[string]generatedStdlibExternLowering{\n")
	for _, fn := range c.Functions {
		if specialBindings[fn.Binding] || !implemented[fn.Field] {
			continue
		}
		fmt.Fprintf(&out, "\t%q: {function: %q, returns: %s, params: %s},\n", fn.Binding, fn.Field, goStdlibReturnKind(fn), goStdlibParamAdapters(fn))
	}
	for _, skipped := range c.Skipped {
		switch skipped.Binding {
		case "JsonEncode":
			fmt.Fprintf(&out, "\t%q: {kind: generatedStdlibExternJSONEncode},\n", skipped.Binding)
		case "JsonParse":
			fmt.Fprintf(&out, "\t%q: {kind: generatedStdlibExternJSONParse},\n", skipped.Binding)
		}
	}
	out.WriteString("}\n\n")
	out.WriteString("func (l *lowerer) lowerGeneratedStdlibExtern(binding string, signature air.Signature, args []ast.Expr, stmts []ast.Stmt, returnTypeID air.TypeID) (loweredExpr, bool, error) {\n")
	out.WriteString("\tlowering, ok := generatedStdlibExternLowerings[binding]\n")
	out.WriteString("\tif !ok {\n")
	out.WriteString("\t\treturn loweredExpr{}, false, nil\n")
	out.WriteString("\t}\n")
	out.WriteString("\tswitch lowering.kind {\n")
	out.WriteString("\tcase generatedStdlibExternJSONEncode:\n")
	out.WriteString("\t\twrapped, err := l.lowerJSONEncodeStdlibExtern(signature, args, stmts, returnTypeID)\n")
	out.WriteString("\t\treturn wrapped, true, err\n")
	out.WriteString("\tcase generatedStdlibExternJSONParse:\n")
	out.WriteString("\t\twrapped, err := l.lowerJSONParseStdlibExtern(args, stmts, returnTypeID)\n")
	out.WriteString("\t\treturn wrapped, true, err\n")
	out.WriteString("\t}\n")
	out.WriteString("\tadaptedArgs := append([]ast.Expr(nil), args...)\n")
	out.WriteString("\tfor i, adapter := range lowering.params {\n")
	out.WriteString("\t\tif i >= len(adaptedArgs) || i >= len(signature.Params) {\n")
	out.WriteString("\t\t\tbreak\n")
	out.WriteString("\t\t}\n")
	out.WriteString("\t\tswitch adapter {\n")
	out.WriteString("\t\tcase generatedStdlibParamAny:\n")
	out.WriteString("\t\t\targ, err := l.lowerUnionArgToAny(adaptedArgs[i], signature.Params[i].Type)\n")
	out.WriteString("\t\t\tif err != nil {\n\t\t\t\treturn loweredExpr{}, true, err\n\t\t\t}\n")
	out.WriteString("\t\t\tstmts = append(stmts, arg.stmts...)\n")
	out.WriteString("\t\t\tadaptedArgs[i] = arg.expr\n")
	out.WriteString("\t\tcase generatedStdlibParamAnySlice:\n")
	out.WriteString("\t\t\targ, err := l.lowerUnionSliceArgToAny(adaptedArgs[i], signature.Params[i].Type)\n")
	out.WriteString("\t\t\tif err != nil {\n\t\t\t\treturn loweredExpr{}, true, err\n\t\t\t}\n")
	out.WriteString("\t\t\tstmts = append(stmts, arg.stmts...)\n")
	out.WriteString("\t\t\tadaptedArgs[i] = arg.expr\n")
	out.WriteString("\t\t}\n")
	out.WriteString("\t}\n")
	out.WriteString("\tcall := &ast.CallExpr{Fun: l.qualified(\"stdlibffi\", \"github.com/akonwi/ard/std_lib/ffi\", lowering.function), Args: adaptedArgs}\n")
	out.WriteString("\tswitch lowering.returns {\n")
	out.WriteString("\tcase generatedStdlibReturnError:\n")
	out.WriteString("\t\twrapped, err := l.wrapErrorCall(returnTypeID, call)\n")
	out.WriteString("\t\tif err != nil {\n\t\t\treturn loweredExpr{}, true, err\n\t\t}\n")
	out.WriteString("\t\twrapped.stmts = append(stmts, wrapped.stmts...)\n")
	out.WriteString("\t\treturn wrapped, true, nil\n")
	out.WriteString("\tcase generatedStdlibReturnValueError:\n")
	out.WriteString("\t\twrapped, err := l.wrapValueErrorCall(returnTypeID, call)\n")
	out.WriteString("\t\tif err != nil {\n\t\t\treturn loweredExpr{}, true, err\n\t\t}\n")
	out.WriteString("\t\twrapped.stmts = append(stmts, wrapped.stmts...)\n")
	out.WriteString("\t\treturn wrapped, true, nil\n")
	out.WriteString("\tcase generatedStdlibReturnResult:\n")
	out.WriteString("\t\twrapped, err := l.wrapStdlibResultCall(returnTypeID, call)\n")
	out.WriteString("\t\tif err != nil {\n\t\t\treturn loweredExpr{}, true, err\n\t\t}\n")
	out.WriteString("\t\twrapped.stmts = append(stmts, wrapped.stmts...)\n")
	out.WriteString("\t\treturn wrapped, true, nil\n")
	out.WriteString("\tdefault:\n")
	out.WriteString("\t\treturn loweredExpr{stmts: stmts, expr: call}, true, nil\n")
	out.WriteString("\t}\n")
	out.WriteString("}\n")
	return format.Source([]byte(out.String()))
}

func goStdlibParamAdapters(fn hostFunction) string {
	if len(fn.Params) == 0 {
		return "nil"
	}
	adapters := make([]string, len(fn.Params))
	for i, param := range fn.Params {
		switch param.Type {
		case "any":
			adapters[i] = "generatedStdlibParamAny"
		case "[]any":
			adapters[i] = "generatedStdlibParamAnySlice"
		default:
			adapters[i] = "generatedStdlibParamDirect"
		}
	}
	return "[]generatedStdlibExternParamAdapter{" + strings.Join(adapters, ", ") + "}"
}

func goStdlibReturnKind(fn hostFunction) string {
	switch len(fn.Returns) {
	case 0:
		return "generatedStdlibReturnDirect"
	case 1:
		ret := fn.Returns[0]
		switch {
		case ret == "error":
			return "generatedStdlibReturnError"
		case strings.HasPrefix(ret, "Result["):
			return "generatedStdlibReturnResult"
		default:
			return "generatedStdlibReturnDirect"
		}
	case 2:
		return "generatedStdlibReturnValueError"
	default:
		return "generatedStdlibReturnDirect"
	}
}

func collectGenerics(typ parse.DeclaredType, out map[string]struct{}) {
	switch t := typ.(type) {
	case *parse.GenericType:
		out[t.Name] = struct{}{}
	case *parse.MutableType:
		collectGenerics(t.Inner, out)
	case *parse.List:
		collectGenerics(t.Element, out)
	case *parse.Map:
		collectGenerics(t.Key, out)
		collectGenerics(t.Value, out)
	case *parse.ResultType:
		collectGenerics(t.Val, out)
		collectGenerics(t.Err, out)
	case *parse.CustomType:
		for _, arg := range t.TypeArgs {
			collectGenerics(arg, out)
		}
	case *parse.FunctionType:
		for _, param := range t.Params {
			collectGenerics(param, out)
		}
		if t.Return != nil {
			collectGenerics(t.Return, out)
		}
	}
}

func hasGeneric(typ parse.DeclaredType) bool {
	generics := map[string]struct{}{}
	collectGenerics(typ, generics)
	return len(generics) > 0
}

func isStringType(typ parse.DeclaredType) bool {
	_, ok := typ.(*parse.StringType)
	return ok
}

func isVoidType(typ parse.DeclaredType) bool {
	_, ok := typ.(*parse.VoidType)
	return ok
}

func goIdentifier(name string) string {
	if token.IsIdentifier(name) && !goKeyword(name) {
		return name
	}
	return goExportedName(name)
}

func goExportedName(name string) string {
	if !strings.ContainsAny(name, "_- :") && token.IsIdentifier(name) {
		return goUpperFirst(name)
	}
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == ':'
	})
	if len(parts) == 0 {
		return "Value"
	}
	for i := range parts {
		if containsUpper(parts[i]) {
			parts[i] = goUpperFirst(parts[i])
		} else {
			parts[i] = goUpperFirst(strings.ToLower(parts[i]))
		}
	}
	result := strings.Join(parts, "")
	if result == "" {
		return "Value"
	}
	if goKeyword(result) {
		return result + "_"
	}
	return result
}

func goUpperFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func containsUpper(value string) bool {
	for _, r := range value {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func goKeyword(value string) bool {
	switch value {
	case "break", "default", "func", "interface", "select",
		"case", "defer", "go", "map", "struct",
		"chan", "else", "goto", "package", "switch",
		"const", "fallthrough", "if", "range", "type",
		"continue", "for", "import", "return", "var":
		return true
	default:
		return false
	}
}
