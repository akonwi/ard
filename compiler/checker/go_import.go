package checker

import (
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
)

// GoPackage is target metadata for a directly imported Go package. It is kept
// separate from Ard modules so Go symbols do not become core Ard declarations.
type GoPackage struct {
	Path                 string
	TypesName            string
	Functions            map[string]*FunctionDef
	Types                map[string]Type
	Constants            map[string]Type
	UnsupportedConstants map[string]string
	UnsupportedFunctions map[string]string
}

// ResolveGoPackage loads exported function signatures for a Go package.
// This first slice intentionally supports only the types needed for simple
// package functions such as fmt.Println.
func ResolveGoPackage(path string) (*GoPackage, error) {
	pkg, err := importer.Default().Import(path)
	if err != nil {
		return nil, err
	}
	goPkg := &GoPackage{
		Path:                 path,
		TypesName:            pkg.Name(),
		Functions:            map[string]*FunctionDef{},
		Types:                map[string]Type{},
		Constants:            map[string]Type{},
		UnsupportedConstants: map[string]string{},
		UnsupportedFunctions: map[string]string{},
	}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if !token.IsExported(name) {
			continue
		}
		obj := scope.Lookup(name)
		if typeName, ok := obj.(*types.TypeName); ok {
			if typ, reason := exportedNamedTypeFromGo(typeName); reason == "" {
				goPkg.Types[name] = typ
			}
			continue
		}
		if constant, ok := obj.(*types.Const); ok {
			if typ, reason := constTypeFromGo(constant.Type()); reason == "" {
				goPkg.Constants[name] = typ
			} else {
				goPkg.UnsupportedConstants[name] = reason
			}
			continue
		}
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		def, reason := functionDefFromGoSignature(name, fn.Type().(*types.Signature))
		if reason == "" {
			goPkg.Functions[name] = def
		} else {
			goPkg.UnsupportedFunctions[name] = reason
		}
	}
	return goPkg, nil
}

func functionDefFromGoSignature(name string, sig *types.Signature) (*FunctionDef, string) {
	return functionDefFromGoSignatureWithMethods(name, sig, true)
}

func functionDefFromGoSignatureWithMethods(name string, sig *types.Signature, includeMethods bool) (*FunctionDef, string) {
	params := make([]Parameter, 0, sig.Params().Len())
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		goType := param.Type()
		mutable := false
		if sig.Variadic() && i == sig.Params().Len()-1 {
			slice, ok := goType.(*types.Slice)
			if !ok {
				return nil, fmt.Sprintf("variadic parameter %d is not a slice", i+1)
			}
			goType = slice.Elem()
		} else if _, ok := goType.Underlying().(*types.Slice); ok {
			mutable = true
		} else if _, ok := goType.Underlying().(*types.Map); ok {
			mutable = true
		}
		ardType, reason := typeFromGoWithMethods(goType, includeMethods)
		if reason != "" {
			return nil, fmt.Sprintf("parameter %d has unsupported type %s: %s", i+1, goType.String(), reason)
		}
		paramName := param.Name()
		if paramName == "" {
			paramName = fmt.Sprintf("arg%d", i+1)
		}
		params = append(params, Parameter{Name: paramName, Type: ardType, Mutable: mutable})
	}

	ret, reason := returnTypeFromGoWithMethods(sig.Results(), includeMethods)
	if reason != "" {
		return nil, reason
	}
	return &FunctionDef{Name: name, Parameters: params, ReturnType: ret}, ""
}

func functionDefFromGoCallbackSignature(name string, sig *types.Signature) (*FunctionDef, string) {
	params := make([]Parameter, 0, sig.Params().Len())
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		goType := param.Type()
		if sig.Variadic() && i == sig.Params().Len()-1 {
			slice, ok := goType.(*types.Slice)
			if !ok {
				return nil, fmt.Sprintf("variadic parameter %d is not a slice", i+1)
			}
			goType = slice.Elem()
		}
		ardType, reason := typeFromGoWithMethods(goType, false)
		if reason != "" {
			return nil, fmt.Sprintf("parameter %d has unsupported type %s: %s", i+1, goType.String(), reason)
		}
		paramName := param.Name()
		if paramName == "" {
			paramName = fmt.Sprintf("arg%d", i+1)
		}
		params = append(params, Parameter{Name: paramName, Type: ardType})
	}
	ret, reason := callbackReturnTypeFromGo(sig.Results())
	if reason != "" {
		return nil, reason
	}
	return &FunctionDef{Name: name, Parameters: params, ReturnType: ret}, ""
}

func callbackReturnTypeFromGo(results *types.Tuple) (Type, string) {
	switch results.Len() {
	case 0:
		return Void, ""
	case 1:
		if isGoError(results.At(0).Type()) {
			return nil, "callback error returns are not supported yet"
		}
		return typeFromGoWithMethods(results.At(0).Type(), false)
	default:
		return nil, fmt.Sprintf("callback multi-result shape %s is not supported yet", results.String())
	}
}

func returnTypeFromGo(results *types.Tuple) (Type, string) {
	return returnTypeFromGoWithMethods(results, true)
}

func returnTypeFromGoWithMethods(results *types.Tuple, includeMethods bool) (Type, string) {
	switch results.Len() {
	case 0:
		return Void, ""
	case 1:
		if isGoError(results.At(0).Type()) {
			return MakeResult(Void, Str), ""
		}
		return typeFromGoWithMethods(results.At(0).Type(), includeMethods)
	case 2:
		if isGoError(results.At(1).Type()) {
			val, reason := typeFromGoWithMethods(results.At(0).Type(), includeMethods)
			if reason != "" {
				return nil, fmt.Sprintf("result 1 has unsupported type %s: %s", results.At(0).Type().String(), reason)
			}
			return MakeResult(val, Str), ""
		}
		if isGoBool(results.At(1).Type()) {
			val, reason := typeFromGoWithMethods(results.At(0).Type(), includeMethods)
			if reason != "" {
				return nil, fmt.Sprintf("result 1 has unsupported type %s: %s", results.At(0).Type().String(), reason)
			}
			return MakeMaybe(val), ""
		}
	}
	return nil, fmt.Sprintf("unsupported result shape %s", results.String())
}

func constTypeFromGo(t types.Type) (Type, string) {
	if basic, ok := t.(*types.Basic); ok {
		switch basic.Kind() {
		case types.UntypedBool:
			return Bool, ""
		case types.UntypedString:
			return Str, ""
		case types.UntypedInt, types.UntypedRune:
			return Int, ""
		case types.UntypedFloat:
			return Float64, ""
		}
	}
	return typeFromGo(t)
}

func typeFromGo(t types.Type) (Type, string) {
	return typeFromGoWithMethods(t, true)
}

func typeFromGoWithMethods(t types.Type, includeMethods bool) (Type, string) {
	if isGoAny(t) {
		return Any, ""
	}
	if alias, ok := t.(*types.Alias); ok {
		underlying, reason := primitiveTypeFromGo(alias.Underlying())
		if reason != "" {
			return nil, reason
		}
		pkg := alias.Obj().Pkg()
		namespace := ""
		qualifier := ""
		if pkg != nil {
			namespace = pkg.Path()
			qualifier = pkg.Name()
		}
		return &ForeignType{Target: "go", Namespace: namespace, Qualifier: qualifier, Name: alias.Obj().Name(), Underlying: underlying}, ""
	}
	if named, ok := t.(*types.Named); ok && !isGoError(t) {
		if sig, ok := named.Underlying().(*types.Signature); ok {
			fn, reason := functionDefFromGoCallbackSignature("<function>", sig)
			if reason != "" {
				return nil, reason
			}
			return fn, ""
		}
		if reason := unsupportedForeignNamedUnderlying(named.Underlying(), false); reason != "" {
			return nil, reason
		}
		return foreignNamedTypeFromGo(named, false, includeMethods), ""
	}
	if sig, ok := t.Underlying().(*types.Signature); ok {
		fn, reason := functionDefFromGoCallbackSignature("<function>", sig)
		if reason != "" {
			return nil, reason
		}
		return fn, ""
	}
	if ptr, ok := t.(*types.Pointer); ok {
		if named, ok := ptr.Elem().(*types.Named); ok && !isGoError(named) {
			if reason := unsupportedForeignNamedUnderlying(named.Underlying(), true); reason != "" {
				return nil, reason
			}
			return foreignNamedTypeFromGo(named, true, includeMethods), ""
		}
		return nil, "only pointers to named Go types are supported"
	}
	if slice, ok := t.Underlying().(*types.Slice); ok {
		elem, reason := typeFromGoWithMethods(slice.Elem(), includeMethods)
		if reason != "" {
			return nil, "slice element " + reason
		}
		return MakeList(elem), ""
	}
	if goMap, ok := t.Underlying().(*types.Map); ok {
		key, reason := typeFromGoWithMethods(goMap.Key(), includeMethods)
		if reason != "" {
			return nil, "map key " + reason
		}
		value, reason := typeFromGoWithMethods(goMap.Elem(), includeMethods)
		if reason != "" {
			return nil, "map value " + reason
		}
		return MakeMap(key, value), ""
	}
	return primitiveTypeFromGo(t)
}

func exportedNamedTypeFromGo(typeName *types.TypeName) (Type, string) {
	named, ok := typeName.Type().(*types.Named)
	if !ok {
		return nil, "exported Go type is not named"
	}
	if reason := unsupportedForeignNamedUnderlying(named.Underlying(), false); reason != "" {
		return nil, reason
	}
	return foreignNamedTypeFromGo(named, false, true), ""
}

func unsupportedForeignNamedUnderlying(underlying types.Type, pointer bool) string {
	if _, reason := primitiveTypeFromGo(underlying); reason == "" {
		return ""
	}
	if _, ok := underlying.(*types.Struct); ok {
		return ""
	}
	if goMap, ok := underlying.(*types.Map); ok {
		if _, reason := typeFromGoWithMethods(goMap.Key(), false); reason != "" {
			return "map key " + reason
		}
		if _, reason := typeFromGoWithMethods(goMap.Elem(), false); reason != "" {
			return "map value " + reason
		}
		return ""
	}
	if _, ok := underlying.(*types.Interface); ok {
		if pointer {
			return "pointers to Go interface types are not supported"
		}
		return "Go interface types are not supported yet"
	}
	return fmt.Sprintf("named Go types with underlying %s are not supported yet", underlying.String())
}

func foreignNamedTypeFromGo(named *types.Named, pointer bool, includeMethods bool) Type {
	pkg := named.Obj().Pkg()
	namespace := ""
	qualifier := ""
	if pkg != nil {
		namespace = pkg.Path()
		qualifier = pkg.Name()
	}
	underlying, _ := primitiveTypeFromGo(named.Underlying())
	_, isStruct := named.Underlying().(*types.Struct)
	foreign := &ForeignType{Target: "go", Namespace: namespace, Qualifier: qualifier, Name: named.Obj().Name(), Underlying: underlying, Pointer: pointer, Struct: isStruct}
	if !pointer {
		if goMap, ok := named.Underlying().(*types.Map); ok {
			if key, reason := typeFromGoWithMethods(goMap.Key(), false); reason == "" {
				foreign.MapKey = key
			}
			if value, reason := typeFromGoWithMethods(goMap.Elem(), false); reason == "" {
				foreign.MapValue = value
			}
		}
	}
	if includeMethods {
		foreign.Methods, foreign.UnsupportedMethods = goMethodsForNamedType(named, pointer)
		foreign.MethodsLoaded = true
	}
	return foreign
}

func loadForeignTypeFields(f *ForeignType) (map[string]Type, map[string]string) {
	if f == nil || f.Target != "go" || f.Namespace == "" || f.Name == "" {
		return nil, nil
	}
	pkg, err := importer.Default().Import(f.Namespace)
	if err != nil {
		return nil, nil
	}
	typeName, ok := pkg.Scope().Lookup(f.Name).(*types.TypeName)
	if !ok {
		return nil, nil
	}
	named, ok := typeName.Type().(*types.Named)
	if !ok {
		return nil, nil
	}
	return goFieldsForNamedType(named)
}

func goFieldsForNamedType(named *types.Named) (map[string]Type, map[string]string) {
	strct, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil, nil
	}
	fields := map[string]Type{}
	unsupported := map[string]string{}
	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if !field.Exported() || field.Embedded() {
			continue
		}
		typ, reason := typeFromGoWithMethods(field.Type(), false)
		if reason == "" {
			fields[field.Name()] = typ
		} else {
			unsupported[field.Name()] = reason
		}
	}
	return fields, unsupported
}

func loadForeignTypeMethods(f *ForeignType) (map[string]*FunctionDef, map[string]string) {
	if f == nil || f.Target != "go" || f.Namespace == "" || f.Name == "" {
		return nil, nil
	}
	pkg, err := importer.Default().Import(f.Namespace)
	if err != nil {
		return nil, nil
	}
	typeName, ok := pkg.Scope().Lookup(f.Name).(*types.TypeName)
	if !ok {
		return nil, nil
	}
	named, ok := typeName.Type().(*types.Named)
	if !ok {
		return nil, nil
	}
	return goMethodsForNamedType(named, f.Pointer)
}

func goMethodsForNamedType(named *types.Named, pointer bool) (map[string]*FunctionDef, map[string]string) {
	var receiver types.Type = named
	if pointer {
		receiver = types.NewPointer(named)
	}
	methodSet := types.NewMethodSet(receiver)
	methods := map[string]*FunctionDef{}
	unsupported := map[string]string{}
	for i := 0; i < methodSet.Len(); i++ {
		selection := methodSet.At(i)
		method, ok := selection.Obj().(*types.Func)
		if !ok || !token.IsExported(method.Name()) {
			continue
		}
		def, reason := functionDefFromGoSignatureWithMethods(method.Name(), method.Type().(*types.Signature), false)
		if reason == "" {
			methods[method.Name()] = def
		} else {
			unsupported[method.Name()] = reason
		}
	}
	return methods, unsupported
}

func primitiveTypeFromGo(t types.Type) (Type, string) {
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return nil, "only basic scalar, slice, map, and any types are supported"
	}
	switch basic.Kind() {
	case types.Bool:
		return Bool, ""
	case types.String:
		return Str, ""
	case types.Int:
		return Int, ""
	case types.Int8:
		return Int8, ""
	case types.Int16:
		return Int16, ""
	case types.Int32:
		return Int32, ""
	case types.Int64:
		return Int64, ""
	case types.Uint:
		return Uint, ""
	case types.Uint8:
		return Byte, ""
	case types.Uint16:
		return Uint16, ""
	case types.Uint32:
		return Uint32, ""
	case types.Uint64:
		return Uint64, ""
	case types.Uintptr:
		return Uintptr, ""
	case types.Float32:
		return Float32, ""
	case types.Float64:
		return Float64, ""
	}
	return nil, fmt.Sprintf("unsupported basic type %s", basic.Name())
}

func isGoAny(t types.Type) bool {
	iface, ok := t.Underlying().(*types.Interface)
	return ok && iface.Empty()
}

func isGoBool(t types.Type) bool {
	basic, ok := t.(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

func isGoError(t types.Type) bool {
	named, ok := t.(*types.Named)
	return ok && named.Obj().Pkg() == nil && named.Obj().Name() == "error"
}
