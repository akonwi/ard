//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type FFIKind int

const (
	FFIRaw FFIKind = iota
	FFIIdiomatic
)

// GoType represents a supported Go type for idiomatic FFI functions.
type GoType struct {
	Base       string // "string", "int", "float64", "bool", "error", "any"
	IsPtr      bool   // *string, *int, etc. → maps to Maybe
	IsSlice    bool   // []string, []int, etc. → maps to List
	MapValue   string // "string", "int", "float64", "bool" for map[string]T
	IsAnySlice bool   // []any → maps to [Dynamic]
	IsAnyMap   bool   // map[string]any → maps to [Str:Dynamic]
}

type FFIFunction struct {
	Name    string
	Module  string
	File    string
	Kind    FFIKind
	Params  []GoType // only set for idiomatic
	Returns []GoType // only set for idiomatic
}

func main() {
	ffiDir := "../../ffi"
	functions, err := discoverFFIFunctions(ffiDir)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	if err := generateRegistry(functions); err != nil {
		log.Fatalf("Failed to generate registry: %v", err)
	}

	raw, idiomatic := 0, 0
	for _, f := range functions {
		if f.Kind == FFIRaw {
			raw++
		} else {
			idiomatic++
		}
	}
	fmt.Printf("Generated registry with %d FFI functions (%d raw, %d idiomatic)\n", len(functions), raw, idiomatic)
}

// --- Discovery ---

func discoverFFIFunctions(dir string) ([]FFIFunction, error) {
	var functions []FFIFunction

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, ".gen.go") {
			return nil
		}

		fileFunctions, err := parseFFIFunctions(path)
		if err != nil {
			return err
		}
		functions = append(functions, fileFunctions...)
		return nil
	})

	return functions, err
}

func parseFFIFunctions(filename string) ([]FFIFunction, error) {
	fset := token.NewFileSet()
	node, err := goparser.ParseFile(fset, filename, nil, goparser.ParseComments)
	if err != nil {
		return nil, err
	}

	base := filepath.Base(filename)
	module := strings.TrimSuffix(base, ".go")

	var functions []FFIFunction
	var errors []string

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		// Skip unexported, underscore-prefixed, and receiver methods
		if fn.Name == nil || !fn.Name.IsExported() || strings.HasPrefix(fn.Name.Name, "_") || fn.Recv != nil {
			return true
		}

		kind, params, returns, classErr := classifyFunction(fn)
		if classErr != nil {
			errors = append(errors, fmt.Sprintf(
				"%s:%d: %s: %v",
				filename, fset.Position(fn.Pos()).Line, fn.Name.Name, classErr,
			))
			return true
		}

		functions = append(functions, FFIFunction{
			Name:    fn.Name.Name,
			Module:  module,
			File:    filename,
			Kind:    kind,
			Params:  params,
			Returns: returns,
		})
		return true
	})

	if len(errors) > 0 {
		return nil, fmt.Errorf("unsupported FFI function signatures:\n  %s", strings.Join(errors, "\n  "))
	}

	return functions, nil
}

// --- Classification ---

func classifyFunction(fn *ast.FuncDecl) (FFIKind, []GoType, []GoType, error) {
	if isRawFFIFunction(fn) {
		return FFIRaw, nil, nil, nil
	}

	params, paramsOk := parseIdiomaticParams(fn)
	returns, returnsOk := parseIdiomaticReturns(fn)

	if paramsOk && returnsOk {
		return FFIIdiomatic, params, returns, nil
	}

	return 0, nil, nil, fmt.Errorf(
		"unsupported signature — must be either raw (func([]*runtime.Object) *runtime.Object) or idiomatic (plain Go types: string, int, float64, bool, error, pointers, slices of scalars, map[string]string)",
	)
}

func isRawFFIFunction(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		return false
	}
	return isSliceOfPointerToRuntimeObject(fn.Type.Params.List[0].Type) &&
		isPointerToRuntimeObject(fn.Type.Results.List[0].Type)
}

func isPointerToRuntimeObject(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	if sel, ok := star.X.(*ast.SelectorExpr); ok {
		pkg, ok := sel.X.(*ast.Ident)
		return ok && pkg.Name == "runtime" && sel.Sel.Name == "Object"
	}
	if ident, ok := star.X.(*ast.Ident); ok {
		return ident.Name == "Object"
	}
	return false
}

func isSliceOfPointerToRuntimeObject(expr ast.Expr) bool {
	array, ok := expr.(*ast.ArrayType)
	if !ok {
		return false
	}
	return isPointerToRuntimeObject(array.Elt)
}

// --- Idiomatic Type Parsing ---

func parseIdiomaticParams(fn *ast.FuncDecl) ([]GoType, bool) {
	if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return nil, true
	}

	var params []GoType
	for _, field := range fn.Type.Params.List {
		t, ok := parseGoType(field.Type)
		if !ok || t.Base == "error" {
			return nil, false
		}
		nameCount := len(field.Names)
		if nameCount == 0 {
			nameCount = 1
		}
		for i := 0; i < nameCount; i++ {
			params = append(params, t)
		}
	}
	return params, true
}

func parseIdiomaticReturns(fn *ast.FuncDecl) ([]GoType, bool) {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return nil, true // void
	}

	var returns []GoType
	for _, field := range fn.Type.Results.List {
		t, ok := parseGoType(field.Type)
		if !ok {
			return nil, false
		}
		nameCount := len(field.Names)
		if nameCount == 0 {
			nameCount = 1
		}
		for i := 0; i < nameCount; i++ {
			returns = append(returns, t)
		}
	}

	// Valid return patterns:
	//   1 return:  any supported type (scalar, ptr, slice, error)
	//   2 returns: (T, error) where T is not error
	switch len(returns) {
	case 1:
		return returns, true
	case 2:
		if returns[1].Base != "error" || returns[0].Base == "error" {
			return nil, false
		}
		return returns, true
	default:
		return nil, false
	}
}

func parseGoType(expr ast.Expr) (GoType, bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "string", "int", "float64", "bool", "error":
			return GoType{Base: t.Name}, true
		case "any":
			return GoType{Base: "any"}, true
		}
	case *ast.InterfaceType:
		// interface{} is equivalent to any
		if t.Methods == nil || len(t.Methods.List) == 0 {
			return GoType{Base: "any"}, true
		}
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			switch ident.Name {
			case "string", "int", "float64", "bool":
				return GoType{Base: ident.Name, IsPtr: true}, true
			}
		}
	case *ast.ArrayType:
		if t.Len == nil { // slice, not fixed-size array
			if ident, ok := t.Elt.(*ast.Ident); ok {
				switch ident.Name {
				case "string", "int", "float64", "bool":
					return GoType{Base: ident.Name, IsSlice: true}, true
				case "any":
					return GoType{IsAnySlice: true}, true
				}
			}
		}
	case *ast.MapType:
		keyIdent, keyOK := t.Key.(*ast.Ident)
		valueIdent, valueOK := t.Value.(*ast.Ident)
		if keyOK && keyIdent.Name == "string" {
			if valueOK {
				switch valueIdent.Name {
				case "string", "int", "float64", "bool":
					return GoType{MapValue: valueIdent.Name}, true
				case "any":
					return GoType{IsAnyMap: true}, true
				}
			}
		}
	}
	return GoType{}, false
}

// --- Code Generation ---

func generateRegistry(functions []FFIFunction) error {
	var sb strings.Builder

	// Determine needed imports
	hasIdiomatic := false
	needsChecker := false
	for _, fn := range functions {
		if fn.Kind != FFIIdiomatic {
			continue
		}
		hasIdiomatic = true
		for _, t := range fn.Returns {
			if t.IsPtr || t.IsSlice || t.MapValue != "" || t.IsAnySlice || t.IsAnyMap {
				needsChecker = true
			}
		}
		for _, t := range fn.Params {
			if t.IsPtr || t.IsSlice || t.MapValue != "" || t.IsAnySlice || t.IsAnyMap {
				needsChecker = true
			}
		}
	}

	sb.WriteString("// Code generated by generate.go; DO NOT EDIT.\n\npackage vm\n\nimport (\n")
	sb.WriteString("\t\"fmt\"\n\n")
	if needsChecker {
		sb.WriteString("\t\"github.com/akonwi/ard/checker\"\n")
	}
	sb.WriteString("\t\"github.com/akonwi/ard/ffi\"\n")
	if hasIdiomatic {
		sb.WriteString("\t\"github.com/akonwi/ard/runtime\"\n")
	}
	sb.WriteString(")\n\n")

	// Generate wrapper functions for idiomatic functions
	for _, fn := range functions {
		if fn.Kind == FFIIdiomatic {
			generateWrapper(&sb, fn)
		}
	}

	// Generate registration function
	sb.WriteString("// RegisterGeneratedFFIFunctions registers all discovered FFI functions\n")
	sb.WriteString("func (r *RuntimeFFIRegistry) RegisterGeneratedFFIFunctions() error {\n")

	for _, fn := range functions {
		ref := fmt.Sprintf("ffi.%s", fn.Name)
		if fn.Kind == FFIIdiomatic {
			ref = fmt.Sprintf("_ffi_%s", fn.Name)
		}
		sb.WriteString(fmt.Sprintf("\tif err := r.Register(%q, %s); err != nil {\n", fn.Name, ref))
		sb.WriteString(fmt.Sprintf("\t\treturn fmt.Errorf(\"failed to register %s: %%w\", err)\n", fn.Name))
		sb.WriteString("\t}\n")
	}

	sb.WriteString("\n\treturn nil\n}\n")

	return os.WriteFile("registry.gen.go", []byte(sb.String()), 0644)
}

func generateWrapper(sb *strings.Builder, fn FFIFunction) {
	sb.WriteString(fmt.Sprintf("func _ffi_%s(args []*runtime.Object) *runtime.Object {\n", fn.Name))

	// 1. Unwrap params
	for i, p := range fn.Params {
		generateParamUnwrap(sb, i, p)
	}

	// 2. Build call expression
	argNames := make([]string, len(fn.Params))
	for i := range fn.Params {
		argNames[i] = fmt.Sprintf("arg%d", i)
	}
	callExpr := fmt.Sprintf("ffi.%s(%s)", fn.Name, strings.Join(argNames, ", "))

	// 3. Call + return wrapping
	hasError := len(fn.Returns) > 0 && fn.Returns[len(fn.Returns)-1].Base == "error"

	switch {
	case len(fn.Returns) == 0:
		// Void
		sb.WriteString(fmt.Sprintf("\t%s\n", callExpr))
		sb.WriteString("\treturn runtime.Void()\n")

	case len(fn.Returns) == 1 && !hasError:
		// Single value return
		sb.WriteString(fmt.Sprintf("\tresult := %s\n", callExpr))
		generateReturnWrap(sb, fn.Returns[0], "result", false)

	case len(fn.Returns) == 1 && hasError:
		// error-only → Result<Void, Str>
		sb.WriteString(fmt.Sprintf("\terr := %s\n", callExpr))
		sb.WriteString("\tif err != nil {\n")
		sb.WriteString("\t\treturn runtime.MakeErr(runtime.MakeStr(err.Error()))\n")
		sb.WriteString("\t}\n")
		sb.WriteString("\treturn runtime.MakeOk(runtime.Void())\n")

	case len(fn.Returns) == 2 && hasError:
		// (T, error) → Result<T, Str>
		sb.WriteString(fmt.Sprintf("\tresult, err := %s\n", callExpr))
		sb.WriteString("\tif err != nil {\n")
		sb.WriteString("\t\treturn runtime.MakeErr(runtime.MakeStr(err.Error()))\n")
		sb.WriteString("\t}\n")
		generateReturnWrap(sb, fn.Returns[0], "result", true)
	}

	sb.WriteString("}\n\n")
}

func generateParamUnwrap(sb *strings.Builder, idx int, t GoType) {
	argRef := fmt.Sprintf("args[%d]", idx)
	varName := fmt.Sprintf("arg%d", idx)

	switch {
	case t.IsPtr:
		sb.WriteString(fmt.Sprintf("\tvar %s *%s\n", varName, t.Base))
		sb.WriteString(fmt.Sprintf("\tif !%s.IsNone() {\n", argRef))
		sb.WriteString(fmt.Sprintf("\t\t_v%d := %s\n", idx, unwrapScalar(argRef, t.Base)))
		sb.WriteString(fmt.Sprintf("\t\t%s = &_v%d\n", varName, idx))
		sb.WriteString("\t}\n")

	case t.IsSlice:
		sb.WriteString(fmt.Sprintf("\t_sl%d := %s.AsList()\n", idx, argRef))
		sb.WriteString(fmt.Sprintf("\t%s := make([]%s, len(_sl%d))\n", varName, t.Base, idx))
		sb.WriteString(fmt.Sprintf("\tfor _i%d, _e%d := range _sl%d {\n", idx, idx, idx))
		sb.WriteString(fmt.Sprintf("\t\t%s[_i%d] = %s\n", varName, idx, unwrapScalar(fmt.Sprintf("_e%d", idx), t.Base)))
		sb.WriteString("\t}\n")

	case t.IsAnySlice:
		sb.WriteString(fmt.Sprintf("\t_sl%d := %s.AsList()\n", idx, argRef))
		sb.WriteString(fmt.Sprintf("\t%s := make([]any, len(_sl%d))\n", varName, idx))
		sb.WriteString(fmt.Sprintf("\tfor _i%d, _e%d := range _sl%d {\n", idx, idx, idx))
		sb.WriteString(fmt.Sprintf("\t\t%s[_i%d] = _e%d.Raw()\n", varName, idx, idx))
		sb.WriteString("\t}\n")

	case t.MapValue != "":
		sb.WriteString(fmt.Sprintf("\t_rawMap%d := %s.AsMap()\n", idx, argRef))
		sb.WriteString(fmt.Sprintf("\t%s := make(map[string]%s, len(_rawMap%d))\n", varName, t.MapValue, idx))
		sb.WriteString(fmt.Sprintf("\tfor _k%d, _v%d := range _rawMap%d {\n", idx, idx, idx))
		sb.WriteString(fmt.Sprintf("\t\t%s[_k%d] = %s\n", varName, idx, unwrapScalar(fmt.Sprintf("_v%d", idx), t.MapValue)))
		sb.WriteString("\t}\n")

	case t.IsAnyMap:
		sb.WriteString(fmt.Sprintf("\t_rawMap%d := %s.AsMap()\n", idx, argRef))
		sb.WriteString(fmt.Sprintf("\t%s := make(map[string]any, len(_rawMap%d))\n", varName, idx))
		sb.WriteString(fmt.Sprintf("\tfor _k%d, _v%d := range _rawMap%d {\n", idx, idx, idx))
		sb.WriteString(fmt.Sprintf("\t\t%s[_k%d] = _v%d.Raw()\n", varName, idx, idx))
		sb.WriteString("\t}\n")

	default:
		sb.WriteString(fmt.Sprintf("\t%s := %s\n", varName, unwrapScalar(argRef, t.Base)))
	}
}

func unwrapScalar(expr, base string) string {
	switch base {
	case "string":
		return expr + ".AsString()"
	case "int":
		return expr + ".AsInt()"
	case "float64":
		return expr + ".AsFloat()"
	case "bool":
		return expr + ".AsBool()"
	case "any":
		return expr + ".Raw()"
	default:
		panic("unsupported base type for unwrap: " + base)
	}
}

func generateReturnWrap(sb *strings.Builder, t GoType, varName string, isResult bool) {
	wrap := func(inner string) {
		if isResult {
			sb.WriteString(fmt.Sprintf("\treturn runtime.MakeOk(%s)\n", inner))
		} else {
			sb.WriteString(fmt.Sprintf("\treturn %s\n", inner))
		}
	}

	switch {
	case t.IsPtr:
		checkerType := checkerTypeStr(t.Base)
		sb.WriteString(fmt.Sprintf("\tif %s == nil {\n", varName))
		if isResult {
			sb.WriteString(fmt.Sprintf("\t\treturn runtime.MakeOk(runtime.MakeNone(%s))\n", checkerType))
		} else {
			sb.WriteString(fmt.Sprintf("\t\treturn runtime.MakeNone(%s)\n", checkerType))
		}
		sb.WriteString("\t}\n")
		wrap(fmt.Sprintf("runtime.MakeNone(%s).ToSome(*%s)", checkerType, varName))

	case t.IsSlice:
		checkerType := checkerTypeStr(t.Base)
		sb.WriteString(fmt.Sprintf("\t_items := make([]*runtime.Object, len(%s))\n", varName))
		sb.WriteString(fmt.Sprintf("\tfor _i, _v := range %s {\n", varName))
		sb.WriteString(fmt.Sprintf("\t\t_items[_i] = %s\n", wrapScalar("_v", t.Base)))
		sb.WriteString("\t}\n")
		wrap(fmt.Sprintf("runtime.MakeList(%s, _items...)", checkerType))

	case t.IsAnySlice:
		sb.WriteString(fmt.Sprintf("\t_items := make([]*runtime.Object, len(%s))\n", varName))
		sb.WriteString(fmt.Sprintf("\tfor _i, _v := range %s {\n", varName))
		sb.WriteString("\t\t_items[_i] = runtime.MakeDynamic(_v)\n")
		sb.WriteString("\t}\n")
		wrap("runtime.MakeList(checker.Dynamic, _items...)")

	case t.MapValue != "":
		valueCheckerType := checkerTypeStr(t.MapValue)
		sb.WriteString(fmt.Sprintf("\t_map := runtime.MakeMap(checker.Str, %s)\n", valueCheckerType))
		sb.WriteString(fmt.Sprintf("\tfor _k, _v := range %s {\n", varName))
		sb.WriteString(fmt.Sprintf("\t\t_map.Map_Set(runtime.MakeStr(_k), %s)\n", wrapScalar("_v", t.MapValue)))
		sb.WriteString("\t}\n")
		wrap("_map")

	case t.IsAnyMap:
		sb.WriteString("\t_map := runtime.MakeMap(checker.Str, checker.Dynamic)\n")
		sb.WriteString(fmt.Sprintf("\tfor _k, _v := range %s {\n", varName))
		sb.WriteString("\t\t_map.Map_Set(runtime.MakeStr(_k), runtime.MakeDynamic(_v))\n")
		sb.WriteString("\t}\n")
		wrap("_map")

	default:
		wrap(wrapScalar(varName, t.Base))
	}
}

func wrapScalar(expr, base string) string {
	switch base {
	case "string":
		return fmt.Sprintf("runtime.MakeStr(%s)", expr)
	case "int":
		return fmt.Sprintf("runtime.MakeInt(%s)", expr)
	case "float64":
		return fmt.Sprintf("runtime.MakeFloat(%s)", expr)
	case "bool":
		return fmt.Sprintf("runtime.MakeBool(%s)", expr)
	case "any":
		return fmt.Sprintf("runtime.MakeDynamic(%s)", expr)
	default:
		panic("unsupported base type for wrap: " + base)
	}
}

func checkerTypeStr(base string) string {
	switch base {
	case "string":
		return "checker.Str"
	case "int":
		return "checker.Int"
	case "float64":
		return "checker.Float"
	case "bool":
		return "checker.Bool"
	default:
		panic("unsupported base type for checker type: " + base)
	}
}