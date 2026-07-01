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
	Path      string
	TypesName string
	Functions map[string]*FunctionDef
}

// ResolveGoPackage loads exported function signatures for a Go package.
// This first slice intentionally supports only the types needed for simple
// package functions such as fmt.Println.
func ResolveGoPackage(path string) (*GoPackage, error) {
	pkg, err := importer.Default().Import(path)
	if err != nil {
		return nil, err
	}
	goPkg := &GoPackage{Path: path, TypesName: pkg.Name(), Functions: map[string]*FunctionDef{}}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if !token.IsExported(name) {
			continue
		}
		fn, ok := scope.Lookup(name).(*types.Func)
		if !ok {
			continue
		}
		def, ok := functionDefFromGoSignature(name, fn.Type().(*types.Signature))
		if ok {
			goPkg.Functions[name] = def
		}
	}
	return goPkg, nil
}

func functionDefFromGoSignature(name string, sig *types.Signature) (*FunctionDef, bool) {
	params := make([]Parameter, 0, sig.Params().Len())
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		goType := param.Type()
		if sig.Variadic() && i == sig.Params().Len()-1 {
			slice, ok := goType.(*types.Slice)
			if !ok {
				return nil, false
			}
			goType = slice.Elem()
		}
		ardType, ok := typeFromGo(goType)
		if !ok {
			return nil, false
		}
		paramName := param.Name()
		if paramName == "" {
			paramName = fmt.Sprintf("arg%d", i+1)
		}
		params = append(params, Parameter{Name: paramName, Type: ardType})
	}

	ret, ok := returnTypeFromGo(sig.Results())
	if !ok {
		return nil, false
	}
	return &FunctionDef{Name: name, Parameters: params, ReturnType: ret}, true
}

func returnTypeFromGo(results *types.Tuple) (Type, bool) {
	switch results.Len() {
	case 0:
		return Void, true
	case 1:
		return typeFromGo(results.At(0).Type())
	case 2:
		if isGoError(results.At(1).Type()) {
			val, ok := typeFromGo(results.At(0).Type())
			if !ok {
				return nil, false
			}
			return MakeResult(val, Str), true
		}
	}
	return nil, false
}

func typeFromGo(t types.Type) (Type, bool) {
	if isGoAny(t) {
		return Any, true
	}
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return nil, false
	}
	switch basic.Kind() {
	case types.Bool:
		return Bool, true
	case types.String:
		return Str, true
	case types.Int:
		return Int, true
	case types.Float64:
		return Float, true
	}
	return nil, false
}

func isGoAny(t types.Type) bool {
	iface, ok := t.Underlying().(*types.Interface)
	return ok && iface.Empty()
}

func isGoError(t types.Type) bool {
	named, ok := t.(*types.Named)
	return ok && named.Obj().Pkg() == nil && named.Obj().Name() == "error"
}
