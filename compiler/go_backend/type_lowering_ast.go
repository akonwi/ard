// type_lowering_ast.go centralizes lowering from checked Ard types and
// function-signature metadata into Go AST type nodes. Declaration and body
// lowering code uses these helpers to build type expressions, generic type
// parameter lists, and parameter/result field lists without duplicating
// Go-specific type construction logic.
package go_backend

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/akonwi/ard/checker"
)

func identExpr(name string) ast.Expr {
	return ast.NewIdent(name)
}

func selectorExpr(x ast.Expr, sel string) ast.Expr {
	return &ast.SelectorExpr{X: x, Sel: ast.NewIdent(sel)}
}

func indexExpr(base ast.Expr, args []ast.Expr) ast.Expr {
	switch len(args) {
	case 0:
		return base
	case 1:
		return &ast.IndexExpr{X: base, Index: args[0]}
	default:
		return &ast.IndexListExpr{X: base, Indices: args}
	}
}

func typeParamFieldList(order []string, mapping map[string]string, constraints map[string]string) *ast.FieldList {
	if len(order) == 0 || len(mapping) == 0 {
		return nil
	}
	fields := make([]*ast.Field, 0, len(order))
	for _, name := range order {
		emitted := mapping[name]
		if emitted == "" {
			continue
		}
		constraint := constraints[name]
		if constraint == "" {
			constraint = "any"
		}
		fields = append(fields, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(emitted)},
			Type:  ast.NewIdent(constraint),
		})
	}
	if len(fields) == 0 {
		return nil
	}
	return &ast.FieldList{List: fields}
}

func (e *emitter) lowerFunctionParamFields(params []checker.Parameter, includeNames bool) ([]*ast.Field, error) {
	fields := make([]*ast.Field, 0, len(params))
	for _, param := range params {
		typeExpr, err := e.lowerTypeExpr(param.Type)
		if err != nil {
			return nil, err
		}
		if param.Mutable && mutableParamNeedsPointer(param.Type) {
			typeExpr = &ast.StarExpr{X: typeExpr}
		}
		field := &ast.Field{Type: typeExpr}
		if includeNames {
			field.Names = []*ast.Ident{ast.NewIdent(goName(param.Name, false))}
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func (e *emitter) lowerBoundFunctionParamFields(params []checker.Parameter) ([]*ast.Field, error) {
	fields := make([]*ast.Field, 0, len(params))
	for _, param := range params {
		typeExpr, err := e.lowerTypeExpr(param.Type)
		if err != nil {
			return nil, err
		}
		usePointer := param.Mutable && mutableParamNeedsPointer(param.Type)
		name := e.bindLocalWithPointer(param.Name, usePointer)
		if usePointer {
			typeExpr = &ast.StarExpr{X: typeExpr}
		}
		fields = append(fields, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(name)},
			Type:  typeExpr,
		})
	}
	return fields, nil
}

func funcResults(returnType ast.Expr) *ast.FieldList {
	if returnType == nil {
		return nil
	}
	return &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
}

func (e *emitter) lowerTraitTypeExpr(trait *checker.Trait, typeParams map[string]string, namedTypeRef func(string, checker.Type) ast.Expr) (ast.Expr, error) {
	if trait == nil {
		return nil, fmt.Errorf("nil trait")
	}
	switch trait.Name {
	case "ToString":
		return selectorExpr(ast.NewIdent(helperImportAlias), "ToString"), nil
	case "Encodable":
		return selectorExpr(ast.NewIdent(helperImportAlias), "Encodable"), nil
	}
	methods := trait.GetMethods()
	fields := make([]*ast.Field, 0, len(methods))
	for _, method := range methods {
		params, err := e.lowerFunctionParamFieldsWithOptions(method.Parameters, false, typeParams, namedTypeRef)
		if err != nil {
			return nil, err
		}
		var results *ast.FieldList
		if method.ReturnType != checker.Void {
			resultType, err := e.lowerTypeExprWithOptions(method.ReturnType, typeParams, namedTypeRef)
			if err != nil {
				return nil, err
			}
			results = funcResults(resultType)
		}
		fields = append(fields, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(goName(method.Name, true))},
			Type: &ast.FuncType{
				Params:  &ast.FieldList{List: params},
				Results: results,
			},
		})
	}
	return &ast.InterfaceType{Methods: &ast.FieldList{List: fields}}, nil
}

func (e *emitter) lowerTypeArgExprWithOptions(t checker.Type, typeParams map[string]string, namedTypeRef func(string, checker.Type) ast.Expr) (ast.Expr, error) {
	typeExpr, err := e.lowerTypeExprWithOptions(t, typeParams, namedTypeRef)
	if err != nil {
		return nil, err
	}
	if typeExpr == nil {
		return &ast.StructType{Fields: &ast.FieldList{}}, nil
	}
	return typeExpr, nil
}

func (e *emitter) lowerFunctionParamFieldsWithOptions(params []checker.Parameter, includeNames bool, typeParams map[string]string, namedTypeRef func(string, checker.Type) ast.Expr) ([]*ast.Field, error) {
	fields := make([]*ast.Field, 0, len(params))
	for _, param := range params {
		typeExpr, err := e.lowerTypeExprWithOptions(param.Type, typeParams, namedTypeRef)
		if err != nil {
			return nil, err
		}
		if param.Mutable && mutableParamNeedsPointer(param.Type) {
			typeExpr = &ast.StarExpr{X: typeExpr}
		}
		field := &ast.Field{Type: typeExpr}
		if includeNames {
			field.Names = []*ast.Ident{ast.NewIdent(goName(param.Name, false))}
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func (e *emitter) lowerTypeExpr(t checker.Type) (ast.Expr, error) {
	if structDef, ok := t.(*checker.StructDef); ok {
		return e.lowerStructTypeExpr(structDef)
	}
	return e.lowerTypeExprWithOptions(t, e.typeParams, func(name string, typ checker.Type) ast.Expr {
		alias := e.importedTypeAlias(name, typ)
		if alias == "" {
			return ast.NewIdent(goName(name, true))
		}
		return selectorExpr(ast.NewIdent(alias), goName(name, true))
	})
}

func (e *emitter) lowerStructTypeExpr(def *checker.StructDef) (ast.Expr, error) {
	base := ast.Expr(ast.NewIdent(goName(def.Name, true)))
	if alias := e.importedTypeAlias(def.Name, def); alias != "" {
		base = selectorExpr(ast.NewIdent(alias), goName(def.Name, true))
	}
	template := e.structTypeTemplate(def)
	order := structTypeParamOrder(template)
	if len(order) == 0 {
		return base, nil
	}
	bindings := inferStructBoundTypeArgs(def, order, nil)
	args := make([]ast.Expr, 0, len(order))
	for _, name := range order {
		if e.typeParams != nil {
			if resolved := e.typeParams[name]; resolved != "" {
				args = append(args, ast.NewIdent(resolved))
				continue
			}
		}
		bound := bindings[name]
		if tv, ok := bound.(*checker.TypeVar); ok {
			if actual := tv.Actual(); actual != nil {
				bound = actual
			} else {
				bound = nil
			}
		}
		if bound != nil {
			typeExpr, err := e.lowerTypeArgExprWithOptions(bound, e.typeParams, nil)
			if err != nil {
				return nil, err
			}
			args = append(args, typeExpr)
			continue
		}
		args = append(args, ast.NewIdent("any"))
	}
	return indexExpr(base, args), nil
}

func (e *emitter) lowerTypeExprWithOptions(t checker.Type, typeParams map[string]string, namedTypeRef func(string, checker.Type) ast.Expr) (ast.Expr, error) {
	switch t {
	case checker.Int:
		return ast.NewIdent("int"), nil
	case checker.Float:
		return ast.NewIdent("float64"), nil
	case checker.Str:
		return ast.NewIdent("string"), nil
	case checker.Bool:
		return ast.NewIdent("bool"), nil
	case checker.Void:
		return nil, nil
	case checker.Dynamic:
		return ast.NewIdent("any"), nil
	}

	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			return e.lowerTypeExprWithOptions(actual, typeParams, namedTypeRef)
		}
		if typeParams != nil {
			if resolved := typeParams[typeVarName(typed)]; resolved != "" {
				return ast.NewIdent(resolved), nil
			}
		}
		return ast.NewIdent("any"), nil
	case *checker.Result:
		valueType, err := e.lowerTypeArgExprWithOptions(typed.Val(), typeParams, namedTypeRef)
		if err != nil {
			return nil, err
		}
		errType, err := e.lowerTypeArgExprWithOptions(typed.Err(), typeParams, namedTypeRef)
		if err != nil {
			return nil, err
		}
		return indexExpr(selectorExpr(ast.NewIdent(helperImportAlias), "Result"), []ast.Expr{valueType, errType}), nil
	case *checker.Enum:
		if len(typed.Methods) > 0 {
			if namedTypeRef != nil {
				return namedTypeRef(typed.Name, typed), nil
			}
			return ast.NewIdent(goName(typed.Name, true)), nil
		}
		return &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("Tag")}, Type: ast.NewIdent("int")}}}}, nil
	case *checker.Maybe:
		innerType, err := e.lowerTypeArgExprWithOptions(typed.Of(), typeParams, namedTypeRef)
		if err != nil {
			return nil, err
		}
		return indexExpr(selectorExpr(ast.NewIdent(helperImportAlias), "Maybe"), []ast.Expr{innerType}), nil
	case *checker.List:
		elementType, err := e.lowerTypeExprWithOptions(typed.Of(), typeParams, namedTypeRef)
		if err != nil {
			return nil, err
		}
		return &ast.ArrayType{Elt: elementType}, nil
	case *checker.Map:
		keyType, err := e.lowerTypeExprWithOptions(typed.Key(), typeParams, namedTypeRef)
		if err != nil {
			return nil, err
		}
		valueType, err := e.lowerTypeExprWithOptions(typed.Value(), typeParams, namedTypeRef)
		if err != nil {
			return nil, err
		}
		return &ast.MapType{Key: keyType, Value: valueType}, nil
	case *checker.StructDef:
		base := ast.Expr(ast.NewIdent(goName(typed.Name, true)))
		if namedTypeRef != nil {
			base = namedTypeRef(typed.Name, typed)
		}
		order := structTypeParamOrder(typed)
		if len(order) == 0 {
			return base, nil
		}
		bindings := inferStructBoundTypeArgs(typed, order, nil)
		args := make([]ast.Expr, 0, len(order))
		for _, name := range order {
			if typeParams != nil {
				if resolved := typeParams[name]; resolved != "" {
					args = append(args, ast.NewIdent(resolved))
					continue
				}
			}
			bound := bindings[name]
			if tv, ok := bound.(*checker.TypeVar); ok {
				if actual := tv.Actual(); actual != nil {
					bound = actual
				} else {
					bound = nil
				}
			}
			if bound != nil {
				typeExpr, err := e.lowerTypeArgExprWithOptions(bound, typeParams, namedTypeRef)
				if err != nil {
					return nil, err
				}
				args = append(args, typeExpr)
				continue
			}
			args = append(args, ast.NewIdent("any"))
		}
		return indexExpr(base, args), nil
	case *checker.FunctionDef:
		return e.lowerFuncTypeExprWithOptions(typed, typeParams, namedTypeRef)
	case *checker.Trait:
		return e.lowerTraitTypeExpr(typed, typeParams, namedTypeRef)
	case *checker.ExternType, *checker.Union:
		return ast.NewIdent("any"), nil
	default:
		return nil, fmt.Errorf("unsupported type: %s", t.String())
	}
}

func (e *emitter) lowerFuncTypeExprWithOptions(def *checker.FunctionDef, typeParams map[string]string, namedTypeRef func(string, checker.Type) ast.Expr) (ast.Expr, error) {
	params, err := e.lowerFunctionParamFieldsWithOptions(def.Parameters, false, typeParams, namedTypeRef)
	if err != nil {
		return nil, err
	}
	var results *ast.FieldList
	returnType := effectiveFunctionReturnType(def)
	if returnType != checker.Void {
		resultExpr, err := e.lowerTypeExprWithOptions(returnType, typeParams, namedTypeRef)
		if err != nil {
			return nil, err
		}
		results = funcResults(resultExpr)
	}
	return &ast.FuncType{Params: &ast.FieldList{List: params}, Results: results}, nil
}

func astValueSpec(name string, typ ast.Expr, value ast.Expr) *ast.ValueSpec {
	spec := &ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(name)}}
	if typ != nil {
		spec.Type = typ
	}
	if value != nil {
		spec.Values = []ast.Expr{value}
	}
	return spec
}

func astVarDecl(specs ...*ast.ValueSpec) ast.Decl {
	parts := make([]ast.Spec, 0, len(specs))
	for _, spec := range specs {
		parts = append(parts, spec)
	}
	return &ast.GenDecl{Tok: token.VAR, Specs: parts}
}
