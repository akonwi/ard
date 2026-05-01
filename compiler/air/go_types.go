package air

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"strings"
	"unicode"
)

const defaultGoRuntimeQualifier = "ardrt"

type GoTypeOptions struct {
	RuntimeQualifier string
}

func GenerateGoStructDeclarations(program *Program, options GoTypeOptions) ([]byte, error) {
	if program == nil {
		return nil, fmt.Errorf("AIR program is nil")
	}
	runtimeQualifier := options.RuntimeQualifier
	if runtimeQualifier == "" {
		runtimeQualifier = defaultGoRuntimeQualifier
	}

	var out bytes.Buffer
	for _, typ := range program.Types {
		if typ.Kind != TypeStruct {
			continue
		}
		decl, err := goStructDecl(program, typ, runtimeQualifier)
		if err != nil {
			return nil, err
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		if err := format.Node(&out, token.NewFileSet(), decl); err != nil {
			return nil, err
		}
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func goStructDecl(program *Program, typ TypeInfo, runtimeQualifier string) (ast.Decl, error) {
	fields := make([]*ast.Field, 0, len(typ.Fields))
	for _, field := range typ.Fields {
		fieldType, err := goTypeExpr(program, field.Type, runtimeQualifier)
		if err != nil {
			return nil, fmt.Errorf("field %s type: %w", field.Name, err)
		}
		fields = append(fields, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(goExportedName(field.Name))},
			Type:  fieldType,
		})
	}
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name: ast.NewIdent(goExportedName(typ.Name)),
		Type: &ast.StructType{Fields: &ast.FieldList{List: fields}},
	}}}, nil
}

func goTypeExpr(program *Program, typeID TypeID, runtimeQualifier string) (ast.Expr, error) {
	typ, err := goTypeInfo(program, typeID)
	if err != nil {
		return nil, err
	}

	switch typ.Kind {
	case TypeVoid:
		return &ast.StructType{Fields: &ast.FieldList{}}, nil
	case TypeInt:
		return ast.NewIdent("int"), nil
	case TypeFloat:
		return ast.NewIdent("float64"), nil
	case TypeBool:
		return ast.NewIdent("bool"), nil
	case TypeStr:
		return ast.NewIdent("string"), nil
	case TypeList:
		elem, err := goTypeExpr(program, typ.Elem, runtimeQualifier)
		if err != nil {
			return nil, err
		}
		return &ast.ArrayType{Elt: elem}, nil
	case TypeMap:
		key, err := goTypeExpr(program, typ.Key, runtimeQualifier)
		if err != nil {
			return nil, err
		}
		value, err := goTypeExpr(program, typ.Value, runtimeQualifier)
		if err != nil {
			return nil, err
		}
		return &ast.MapType{Key: key, Value: value}, nil
	case TypeStruct, TypeEnum, TypeUnion, TypeExtern, TypeTraitObject:
		return ast.NewIdent(goExportedName(typ.Name)), nil
	case TypeMaybe:
		elem, err := goTypeExpr(program, typ.Elem, runtimeQualifier)
		if err != nil {
			return nil, err
		}
		return goRuntimeGeneric(runtimeQualifier, "Maybe", elem), nil
	case TypeResult:
		value, err := goTypeExpr(program, typ.Value, runtimeQualifier)
		if err != nil {
			return nil, err
		}
		errType, err := goTypeExpr(program, typ.Error, runtimeQualifier)
		if err != nil {
			return nil, err
		}
		return goRuntimeGeneric(runtimeQualifier, "Result", value, errType), nil
	case TypeDynamic:
		return ast.NewIdent("any"), nil
	case TypeFunction:
		params := make([]*ast.Field, 0, len(typ.Params))
		for _, param := range typ.Params {
			paramType, err := goTypeExpr(program, param, runtimeQualifier)
			if err != nil {
				return nil, err
			}
			params = append(params, &ast.Field{Type: paramType})
		}
		fnType := &ast.FuncType{Params: &ast.FieldList{List: params}}
		if typ.Return != NoType && !goIsVoidType(program, typ.Return) {
			returnType, err := goTypeExpr(program, typ.Return, runtimeQualifier)
			if err != nil {
				return nil, err
			}
			fnType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
		}
		return fnType, nil
	case TypeFiber:
		elem, err := goTypeExpr(program, typ.Elem, runtimeQualifier)
		if err != nil {
			return nil, err
		}
		return goRuntimeGeneric(runtimeQualifier, "Fiber", elem), nil
	default:
		return nil, fmt.Errorf("unsupported AIR type kind %d", typ.Kind)
	}
}

func goRuntimeGeneric(runtimeQualifier, name string, args ...ast.Expr) ast.Expr {
	var base ast.Expr = ast.NewIdent(name)
	if runtimeQualifier != "" {
		base = &ast.SelectorExpr{
			X:   ast.NewIdent(runtimeQualifier),
			Sel: ast.NewIdent(name),
		}
	}
	if len(args) == 1 {
		return &ast.IndexExpr{X: base, Index: args[0]}
	}
	return &ast.IndexListExpr{X: base, Indices: args}
}

func goTypeInfo(program *Program, id TypeID) (TypeInfo, error) {
	if id <= 0 || int(id) > len(program.Types) {
		return TypeInfo{}, fmt.Errorf("invalid type id %d", id)
	}
	return program.Types[id-1], nil
}

func goIsVoidType(program *Program, id TypeID) bool {
	typ, err := goTypeInfo(program, id)
	return err == nil && typ.Kind == TypeVoid
}

func goExportedName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == ':'
	})
	if len(parts) == 0 {
		return "Value"
	}
	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
		parts[i] = goUpperFirst(parts[i])
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
