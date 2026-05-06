//go:build ignore

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/akonwi/ard/parse"
)

type contract struct {
	ExternTypes []externType
	Aliases     []typeAlias
	Enums       []enumDecl
	Structs     []structDecl
	Functions   []hostFunction
	Skipped     []skippedExtern
}

type externType struct {
	Name string
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
	externDir         string
	hostOut           string
	hostPackage       string
	vmNextOut         string
	vmNextAdapterFunc string
}

func main() {
	opts := generateOptions{}
	flag.StringVar(&opts.externDir, "extern-dir", "..", "directory containing Ard extern declarations")
	flag.StringVar(&opts.hostOut, "host-out", "ard.gen.go", "generated Go host contract output")
	flag.StringVar(&opts.hostPackage, "host-package", "ffi", "package name for generated Go host contract")
	flag.StringVar(&opts.vmNextOut, "vm-next-out", "vm_next_adapters.gen.go", "generated vm_next adapter output")
	flag.StringVar(&opts.vmNextAdapterFunc, "vm-next-adapter-func", "VMNextAdapter", "generated vm_next adapter lookup function")
	flag.Parse()

	c, err := loadContract(opts.externDir)
	if err != nil {
		panic(err)
	}
	source, err := render(c, opts.hostPackage)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(opts.hostOut, source, 0o644); err != nil {
		panic(err)
	}
	vmAdapters, err := renderVMNextAdapters(c, opts)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(opts.vmNextOut, vmAdapters, 0o644); err != nil {
		panic(err)
	}
}

func loadContract(stdlibDir string) (contract, error) {
	files, err := filepath.Glob(filepath.Join(stdlibDir, "*.ard"))
	if err != nil {
		return contract{}, err
	}
	slices.Sort(files)

	c := contract{}
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

		for _, stmt := range result.Program.Statements {
			switch node := stmt.(type) {
			case *parse.ExternTypeDeclaration:
				name := goExportedName(node.Name)
				definedTypes[name] = struct{}{}
				c.ExternTypes = append(c.ExternTypes, externType{Name: name})
			case *parse.TypeDeclaration:
				name := goExportedName(node.Name.Name)
				goType, ok := aliasGoType(node)
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
				decl := lowerStruct(node, aliases, definedTypes)
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
			if binding == "" {
				continue
			}
			fn, err := lowerHostFunction(binding, node, aliases, definedTypes)
			if err != nil {
				c.Skipped = append(c.Skipped, skippedExtern{Binding: binding, Reason: err.Error()})
				continue
			}
			if existing, ok := bindings[binding]; ok {
				if existing.Type != fn.Type {
					return contract{}, fmt.Errorf("binding %s has conflicting signatures: %s vs %s", binding, existing.Type, fn.Type)
				}
				continue
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

func aliasGoType(alias *parse.TypeDeclaration) (string, bool) {
	if len(alias.Type) != 1 {
		return "any", true
	}
	if fn, ok := alias.Type[0].(*parse.FunctionType); ok {
		goType, err := functionTypeGoType(fn, nil, nil)
		return goType, err == nil
	}
	return "any", true
}

func lowerStruct(node *parse.StructDefinition, aliases map[string]string, definedTypes map[string]struct{}) structDecl {
	decl := structDecl{Name: goExportedName(node.Name.Name)}
	generics := map[string]struct{}{}
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
		goType, err := typeGoType(field.Type, aliases, definedTypes)
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

func lowerHostFunction(binding string, fn *parse.ExternalFunction, aliases map[string]string, definedTypes map[string]struct{}) (hostFunction, error) {
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
		goType, err := typeGoType(param.Type, aliases, definedTypes)
		if err != nil {
			return hostFunction{}, fmt.Errorf("parameter %s: %w", param.Name, err)
		}
		params = append(params, fmt.Sprintf("%s %s", goIdentifier(param.Name), goType))
		hostParams = append(hostParams, hostParam{Name: goIdentifier(param.Name), Type: goType})
	}

	returns, err := returnGoTypes(fn.ReturnType, aliases, definedTypes)
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

func returnGoTypes(typ parse.DeclaredType, aliases map[string]string, definedTypes map[string]struct{}) ([]string, error) {
	if result, ok := typ.(*parse.ResultType); ok {
		if isVoidType(result.Val) {
			if !isStringType(result.Err) {
				errType, err := typeGoType(result.Err, aliases, definedTypes)
				if err != nil {
					return nil, err
				}
				return []string{fmt.Sprintf("Result[struct{}, %s]", errType)}, nil
			}
			return []string{"error"}, nil
		}
		valueType, err := typeGoType(result.Val, aliases, definedTypes)
		if err != nil {
			return nil, err
		}
		if !isStringType(result.Err) {
			errType, err := typeGoType(result.Err, aliases, definedTypes)
			if err != nil {
				return nil, err
			}
			return []string{fmt.Sprintf("Result[%s, %s]", valueType, errType)}, nil
		}
		return []string{valueType, "error"}, nil
	}
	goType, err := typeGoType(typ, aliases, definedTypes)
	if err != nil {
		return nil, err
	}
	if goType == "struct{}" {
		return nil, nil
	}
	return []string{goType}, nil
}

func typeGoType(typ parse.DeclaredType, aliases map[string]string, definedTypes map[string]struct{}) (string, error) {
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
		goType = "struct{}"
	case *parse.List:
		elem, err := typeGoType(t.Element, aliases, definedTypes)
		if err != nil {
			return "", err
		}
		goType = "[]" + elem
	case *parse.Map:
		key, err := typeGoType(t.Key, aliases, definedTypes)
		if err != nil {
			return "", err
		}
		if customKey, ok := t.Key.(*parse.CustomType); ok && customKey.Name == "Dynamic" {
			key = "string"
		}
		value, err := typeGoType(t.Value, aliases, definedTypes)
		if err != nil {
			return "", err
		}
		goType = fmt.Sprintf("map[%s]%s", key, value)
	case *parse.CustomType:
		name := goExportedName(t.Name)
		switch t.Name {
		case "Dynamic", "Encodable":
			goType = "any"
		default:
			if alias, ok := aliases[t.Name]; ok {
				goType = alias
			} else if _, ok := definedTypes[name]; ok {
				goType = name
			} else {
				goType = name
			}
			if len(t.TypeArgs) > 0 {
				args := make([]string, 0, len(t.TypeArgs))
				for _, arg := range t.TypeArgs {
					argType, err := typeGoType(arg, aliases, definedTypes)
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
		goType, err = functionTypeGoType(t, aliases, definedTypes)
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

func functionTypeGoType(fn *parse.FunctionType, aliases map[string]string, definedTypes map[string]struct{}) (string, error) {
	args := make([]string, 0, len(fn.Params)+1)
	for i, param := range fn.Params {
		paramType, err := typeGoType(param, aliases, definedTypes)
		if err != nil {
			return "", err
		}
		if i < len(fn.ParamMutability) && fn.ParamMutability[i] {
			paramType = "*" + paramType
		}
		args = append(args, paramType)
	}
	returnType := "struct{}"
	if fn.Return != nil {
		var err error
		returnType, err = typeGoType(fn.Return, aliases, definedTypes)
		if err != nil {
			return "", err
		}
	}
	params := strings.Join(args, ", ")
	if len(fn.Params) == 0 {
		params = ""
	}
	return fmt.Sprintf("func(%s) (%s, error)", params, returnType), nil
}

func render(c contract, packageName string) ([]byte, error) {
	var out strings.Builder
	out.WriteString("// Code generated by ard; DO NOT EDIT.\n\n")
	fmt.Fprintf(&out, "package %s\n\n", packageName)
	out.WriteString("import ardruntime \"github.com/akonwi/ard/runtime\"\n\n")
	out.WriteString("type Maybe[T any] = ardruntime.Maybe[T]\n\n")
	out.WriteString("func Some[T any](value T) Maybe[T] {\n\treturn ardruntime.Some(value)\n}\n\n")
	out.WriteString("func None[T any]() Maybe[T] {\n\treturn ardruntime.None[T]()\n}\n\n")
	out.WriteString("type Result[T, E any] = ardruntime.Result[T, E]\n\n")
	out.WriteString("func Ok[T, E any](value T) Result[T, E] {\n\treturn ardruntime.Ok[T, E](value)\n}\n\n")
	out.WriteString("func Err[T, E any](err E) Result[T, E] {\n\treturn ardruntime.Err[T](err)\n}\n\n")
	for _, typ := range c.ExternTypes {
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

	out.WriteString("type Host struct {\n")
	for _, fn := range c.Functions {
		fmt.Fprintf(&out, "\t%s %s\n", fn.Field, fn.Type)
	}
	out.WriteString("}\n\n")

	out.WriteString("func (h Host) Functions() map[string]any {\n")
	out.WriteString("\tfunctions := map[string]any{}\n")
	for _, fn := range c.Functions {
		fmt.Fprintf(&out, "\tif h.%s != nil {\n\t\tfunctions[%q] = h.%s\n\t}\n", fn.Field, fn.Binding, fn.Field)
	}
	out.WriteString("\treturn functions\n")
	out.WriteString("}\n\n")

	for _, skipped := range c.Skipped {
		fmt.Fprintf(&out, "// Skipped extern %s: %s.\n", skipped.Binding, skipped.Reason)
	}

	return format.Source([]byte(out.String()))
}

func renderVMNextAdapters(c contract, opts generateOptions) ([]byte, error) {
	file := &ast.File{Name: ast.NewIdent(opts.hostPackage)}
	file.Decls = append(file.Decls, &ast.GenDecl{
		Tok: token.IMPORT,
		Specs: []ast.Spec{
			&ast.ImportSpec{Path: stringLit("fmt")},
			&ast.ImportSpec{Path: stringLit("reflect")},
			&ast.ImportSpec{Path: stringLit("github.com/akonwi/ard/air")},
			&ast.ImportSpec{Name: ast.NewIdent("vmnextffi"), Path: stringLit("github.com/akonwi/ard/vm_next/ffi")},
		},
	})
	file.Decls = append(file.Decls, vmNextAdapterSupportDecls()...)

	cases := make([]ast.Stmt, 0, len(c.Functions))
	for _, fn := range c.Functions {
		clause, err := vmNextAdapterCase(c, opts, fn)
		if err != nil {
			return nil, err
		}
		cases = append(cases, clause)
	}
	body := []ast.Stmt{
		&ast.SwitchStmt{
			Tag:  ast.NewIdent("binding"),
			Body: &ast.BlockStmt{List: cases},
		},
		returnStmt(ast.NewIdent("nil"), ast.NewIdent("false")),
	}
	file.Decls = append(file.Decls, &ast.FuncDecl{
		Name: ast.NewIdent(opts.vmNextAdapterFunc),
		Type: &ast.FuncType{
			Params: fields(
				field("binding", ast.NewIdent("string")),
				field("fn", ast.NewIdent("any")),
			),
			Results: fields(
				field("", selector("vmnextffi", "ExternAdapter")),
				field("", ast.NewIdent("bool")),
			),
		},
		Body: &ast.BlockStmt{List: body},
	})

	var out bytes.Buffer
	if err := printer.Fprint(&out, token.NewFileSet(), file); err != nil {
		return nil, err
	}
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return nil, err
	}
	return append([]byte("// Code generated by ard; DO NOT EDIT.\n\n"), formatted...), nil
}

func vmNextAdapterSupportDecls() []ast.Decl {
	return []ast.Decl{
		&ast.FuncDecl{
			Name: ast.NewIdent("init"),
			Type: &ast.FuncType{Params: fields()},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ExprStmt{X: callExpr(selector("vmnextffi", "RegisterAdapterLookup"), ast.NewIdent("VMNextAdapter"))},
			}},
		},
		&ast.FuncDecl{
			Name: ast.NewIdent("generatedHostCast"),
			Type: &ast.FuncType{TypeParams: fields(field("T", ast.NewIdent("any"))), Params: fields(field("value", ast.NewIdent("any"))), Results: fields(field("", ast.NewIdent("T")), field("", ast.NewIdent("bool")))},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent("zero")}, Type: ast.NewIdent("T")}}}},
				&ast.IfStmt{Cond: &ast.BinaryExpr{X: ast.NewIdent("value"), Op: token.EQL, Y: ast.NewIdent("nil")}, Body: &ast.BlockStmt{List: []ast.Stmt{returnStmt(ast.NewIdent("zero"), ast.NewIdent("true"))}}},
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("out"), ast.NewIdent("ok")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: ast.NewIdent("value"), Type: ast.NewIdent("T")}}},
				returnStmt(ast.NewIdent("out"), ast.NewIdent("ok")),
			}},
		},
	}
}

func vmNextAdapterCase(c contract, opts generateOptions, fn hostFunction) (ast.Stmt, error) {
	sig, err := parseGoExpr(vmNextFuncType(fn))
	if err != nil {
		return nil, fmt.Errorf("%s adapter signature: %w", fn.Binding, err)
	}
	body := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent("typed"), ast.NewIdent("ok")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.TypeAssertExpr{X: ast.NewIdent("fn"), Type: sig}},
		},
		&ast.IfStmt{
			Cond: &ast.UnaryExpr{Op: token.NOT, X: ast.NewIdent("ok")},
			Body: &ast.BlockStmt{List: []ast.Stmt{returnStmt(ast.NewIdent("nil"), ast.NewIdent("false"))}},
		},
		returnStmt(vmNextAdapterFuncLit(c, opts, fn), ast.NewIdent("true")),
	}
	return &ast.CaseClause{
		List: []ast.Expr{stringLit(fn.Binding)},
		Body: body,
	}, nil
}

func vmNextAdapterArgStmts(argName string, index int, goType string) []ast.Stmt {
	directMethod := map[string]string{
		"int":     "HostArgInt",
		"float64": "HostArgFloat64",
		"bool":    "HostArgBool",
		"string":  "HostArgString",
		"any":     "HostArgAny",
	}[goType]
	if directMethod != "" {
		return []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(argName), ast.NewIdent("err")},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{callExpr(selector("bridge", directMethod), ast.NewIdent("args"), intLit(index))},
			},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent("err"), Op: token.NEQ, Y: ast.NewIdent("nil")},
				Body: &ast.BlockStmt{List: []ast.Stmt{returnStmt(ast.NewIdent("nil"), callExpr(selector("fmt", "Errorf"), stringLit("extern %s arg "+strconv.Itoa(index)+": %w"), ast.NewIdent("binding"), ast.NewIdent("err")))}},
			},
		}
	}

	argType := mustParseGoExpr(qualifyVMNextType(goType))
	argRawName := argName + "Raw"
	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(argRawName), ast.NewIdent("err")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{callExpr(selector("bridge", "HostArg"), ast.NewIdent("args"), intLit(index), callExpr(&ast.IndexExpr{X: selector("reflect", "TypeFor"), Index: argType}))},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: ast.NewIdent("err"), Op: token.NEQ, Y: ast.NewIdent("nil")},
			Body: &ast.BlockStmt{List: []ast.Stmt{returnStmt(ast.NewIdent("nil"), callExpr(selector("fmt", "Errorf"), stringLit("extern %s arg "+strconv.Itoa(index)+": %w"), ast.NewIdent("binding"), ast.NewIdent("err")))}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(argName), ast.NewIdent("ok")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{callExpr(&ast.IndexExpr{X: ast.NewIdent("generatedHostCast"), Index: argType}, ast.NewIdent(argRawName))},
		},
		&ast.IfStmt{
			Cond: &ast.UnaryExpr{Op: token.NOT, X: ast.NewIdent("ok")},
			Body: &ast.BlockStmt{List: []ast.Stmt{returnStmt(ast.NewIdent("nil"), callExpr(selector("fmt", "Errorf"), stringLit("extern %s arg "+strconv.Itoa(index)+": cannot use generated host arg %T"), ast.NewIdent("binding"), ast.NewIdent(argRawName)))}},
		},
	}
}

func vmNextAdapterFuncLit(c contract, opts generateOptions, fn hostFunction) ast.Expr {
	stmts := make([]ast.Stmt, 0, len(fn.Params)*2+3)
	for i, param := range fn.Params {
		argName := fmt.Sprintf("arg%d", i)
		stmts = append(stmts, vmNextAdapterArgStmts(argName, i, param.Type)...)
	}

	callArgs := make([]ast.Expr, 0, len(fn.Params))
	for i := range fn.Params {
		callArgs = append(callArgs, ast.NewIdent(fmt.Sprintf("arg%d", i)))
	}
	hostCall := callExpr(ast.NewIdent("typed"), callArgs...)
	switch len(fn.Returns) {
	case 0:
		stmts = append(stmts,
			&ast.ExprStmt{X: hostCall},
			returnStmt(callExpr(selector("bridge", "HostReturnVoid"), selector("extern", "Signature", "Return"))),
		)
	case 1:
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("out0")}, Tok: token.DEFINE, Rhs: []ast.Expr{hostCall}})
		stmts = append(stmts, returnStmt(vmNextReturnCall(fn.Returns[0], "out0")))
	case 2:
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("out0"), ast.NewIdent("out1")}, Tok: token.DEFINE, Rhs: []ast.Expr{hostCall}})
		stmts = append(stmts, returnStmt(callExpr(selector("bridge", "HostReturnValueError"), selector("extern", "Signature", "Return"), ast.NewIdent("out0"), ast.NewIdent("out1"))))
	default:
		stmts = append(stmts, returnStmt(ast.NewIdent("nil"), callExpr(selector("fmt", "Errorf"), stringLit("unsupported generated host adapter return count for %s"), ast.NewIdent("binding"))))
	}

	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params: fields(
				field("bridge", selector("vmnextffi", "Bridge")),
				field("extern", selector("air", "Extern")),
				field("binding", ast.NewIdent("string")),
				field("args", ast.NewIdent("any")),
			),
			Results: fields(
				field("", ast.NewIdent("any")),
				field("", ast.NewIdent("error")),
			),
		},
		Body: &ast.BlockStmt{List: stmts},
	}
}

func vmNextReturnCall(returnType, outName string) ast.Expr {
	if strings.HasPrefix(returnType, "Result[") {
		return callExpr(
			selector("bridge", "HostReturnResult"),
			selector("extern", "Signature", "Return"),
			selector(outName, "Value"),
			selector(outName, "Err"),
			selector(outName, "Ok"),
		)
	}
	if returnType == "error" {
		return callExpr(selector("bridge", "HostReturnError"), selector("extern", "Signature", "Return"), ast.NewIdent(outName))
	}
	return callExpr(selector("bridge", "HostReturnValue"), selector("extern", "Signature", "Return"), ast.NewIdent(outName))
}

func vmNextFuncType(fn hostFunction) string {
	params := make([]string, 0, len(fn.Params))
	for _, param := range fn.Params {
		params = append(params, qualifyVMNextType(param.Type))
	}
	returns := make([]string, 0, len(fn.Returns))
	for _, ret := range fn.Returns {
		returns = append(returns, qualifyVMNextType(ret))
	}
	sig := "func(" + strings.Join(params, ", ") + ")"
	if len(returns) == 1 {
		sig += " " + returns[0]
	} else if len(returns) > 1 {
		sig += " (" + strings.Join(returns, ", ") + ")"
	}
	return sig
}

func qualifyVMNextType(goType string) string {
	var out strings.Builder
	for i := 0; i < len(goType); {
		r := rune(goType[i])
		if r == '_' || unicode.IsLetter(r) {
			start := i
			i++
			for i < len(goType) {
				r = rune(goType[i])
				if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
					break
				}
				i++
			}
			ident := goType[start:i]
			out.WriteString(ident)
			continue
		}
		out.WriteByte(goType[i])
		i++
	}
	return out.String()
}

func parseGoExpr(source string) (ast.Expr, error) {
	return parser.ParseExpr(source)
}

func mustParseGoExpr(source string) ast.Expr {
	expr, err := parseGoExpr(source)
	if err != nil {
		panic(err)
	}
	return expr
}

func fields(fields ...*ast.Field) *ast.FieldList {
	return &ast.FieldList{List: fields}
}

func field(name string, typ ast.Expr) *ast.Field {
	field := &ast.Field{Type: typ}
	if name != "" {
		field.Names = []*ast.Ident{ast.NewIdent(name)}
	}
	return field
}

func selector(names ...string) ast.Expr {
	var out ast.Expr = ast.NewIdent(names[0])
	for _, name := range names[1:] {
		out = &ast.SelectorExpr{X: out, Sel: ast.NewIdent(name)}
	}
	return out
}

func callExpr(fun ast.Expr, args ...ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: fun, Args: args}
}

func indexExpr(x ast.Expr, index ast.Expr) ast.Expr {
	return &ast.IndexExpr{X: x, Index: index}
}

func intLit(value int) ast.Expr {
	return &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(value)}
}

func stringLit(value string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(value)}
}

func returnStmt(results ...ast.Expr) ast.Stmt {
	return &ast.ReturnStmt{Results: results}
}

func collectGenerics(typ parse.DeclaredType, out map[string]struct{}) {
	switch t := typ.(type) {
	case *parse.GenericType:
		out[t.Name] = struct{}{}
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
