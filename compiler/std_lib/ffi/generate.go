//go:build ignore

package main

import (
	"fmt"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"slices"
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
}

type skippedExtern struct {
	Binding string
	Reason  string
}

func main() {
	c, err := loadContract("..")
	if err != nil {
		panic(err)
	}
	source, err := render(c)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("ard.gen.go", source, 0o644); err != nil {
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
	for _, param := range fn.Parameters {
		goType, err := typeGoType(param.Type, aliases, definedTypes)
		if err != nil {
			return hostFunction{}, fmt.Errorf("parameter %s: %w", param.Name, err)
		}
		params = append(params, fmt.Sprintf("%s %s", goIdentifier(param.Name), goType))
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
	}, nil
}

func returnGoTypes(typ parse.DeclaredType, aliases map[string]string, definedTypes map[string]struct{}) ([]string, error) {
	if result, ok := typ.(*parse.ResultType); ok {
		if !isStringType(result.Err) {
			return nil, fmt.Errorf("Result error type %s is not generated yet", result.Err.GetName())
		}
		if isVoidType(result.Val) {
			return []string{"error"}, nil
		}
		valueType, err := typeGoType(result.Val, aliases, definedTypes)
		if err != nil {
			return nil, err
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
	args = append(args, returnType)
	return fmt.Sprintf("Callback%d[%s]", len(fn.Params), strings.Join(args, ", ")), nil
}

func render(c contract) ([]byte, error) {
	var out strings.Builder
	out.WriteString("// Code generated by ard; DO NOT EDIT.\n\n")
	out.WriteString("package ffi\n\n")
	out.WriteString("type Maybe[T any] struct {\n\tValue T\n\tSome  bool\n}\n\n")
	out.WriteString("func Some[T any](value T) Maybe[T] {\n\treturn Maybe[T]{Value: value, Some: true}\n}\n\n")
	out.WriteString("func None[T any]() Maybe[T] {\n\treturn Maybe[T]{}\n}\n\n")
	out.WriteString("type Callback0[R any] struct{}\n")
	out.WriteString("type Callback1[A, R any] struct{}\n")
	out.WriteString("type Callback2[A, B, R any] struct{}\n\n")

	for _, typ := range c.ExternTypes {
		fmt.Fprintf(&out, "type %s struct {\n\tHandle any\n}\n\n", typ.Name)
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
