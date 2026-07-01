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
		UnsupportedFunctions: map[string]string{},
	}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if !token.IsExported(name) {
			continue
		}
		fn, ok := scope.Lookup(name).(*types.Func)
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
		ardType, reason := typeFromGo(goType)
		if reason != "" {
			return nil, fmt.Sprintf("parameter %d has unsupported type %s: %s", i+1, goType.String(), reason)
		}
		paramName := param.Name()
		if paramName == "" {
			paramName = fmt.Sprintf("arg%d", i+1)
		}
		params = append(params, Parameter{Name: paramName, Type: ardType})
	}

	ret, reason := returnTypeFromGo(sig.Results())
	if reason != "" {
		return nil, reason
	}
	return &FunctionDef{Name: name, Parameters: params, ReturnType: ret}, ""
}

func returnTypeFromGo(results *types.Tuple) (Type, string) {
	switch results.Len() {
	case 0:
		return Void, ""
	case 1:
		if isGoError(results.At(0).Type()) {
			return MakeResult(Void, Str), ""
		}
		return typeFromGo(results.At(0).Type())
	case 2:
		if isGoError(results.At(1).Type()) {
			val, reason := typeFromGo(results.At(0).Type())
			if reason != "" {
				return nil, fmt.Sprintf("result 1 has unsupported type %s: %s", results.At(0).Type().String(), reason)
			}
			return MakeResult(val, Str), ""
		}
	}
	return nil, fmt.Sprintf("unsupported result shape %s", results.String())
}

func typeFromGo(t types.Type) (Type, string) {
	if isGoAny(t) {
		return Any, ""
	}
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return nil, "only basic scalar and any types are supported"
	}
	switch basic.Kind() {
	case types.Bool:
		return Bool, ""
	case types.String:
		return Str, ""
	case types.Int:
		return Int, ""
	case types.Float64:
		return Float, ""
	}
	return nil, fmt.Sprintf("unsupported basic type %s", basic.Name())
}

func isGoAny(t types.Type) bool {
	iface, ok := t.Underlying().(*types.Interface)
	return ok && iface.Empty()
}

func isGoError(t types.Type) bool {
	named, ok := t.(*types.Named)
	return ok && named.Obj().Pkg() == nil && named.Obj().Name() == "error"
}
