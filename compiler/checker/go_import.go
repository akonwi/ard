package checker

import (
	"fmt"
	"go/token"
	"go/types"
	"math"
)

// GoPackage is target metadata for a directly imported Go package. It is kept
// separate from Ard modules so Go symbols do not become core Ard declarations.
type GoPackage struct {
	Path                 string
	TypesName            string
	Functions            map[string]*FunctionDef
	Generics             map[string]*types.Func
	Types                map[string]Type
	Constants            map[string]Type
	Variables            map[string]Type
	UnsupportedTypes     map[string]string
	UnsupportedConstants map[string]string
	UnsupportedVariables map[string]string
	UnsupportedFunctions map[string]string
}

type GoPackageResolver interface {
	ResolveGoPackage(path string) (*GoPackage, error)
}

func goPackageFromTypesPackage(path string, pkg *types.Package) *GoPackage {
	goPkg := &GoPackage{
		Path:                 path,
		TypesName:            pkg.Name(),
		Functions:            map[string]*FunctionDef{},
		Generics:             map[string]*types.Func{},
		Types:                map[string]Type{},
		Constants:            map[string]Type{},
		Variables:            map[string]Type{},
		UnsupportedTypes:     map[string]string{},
		UnsupportedConstants: map[string]string{},
		UnsupportedVariables: map[string]string{},
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
			} else {
				goPkg.UnsupportedTypes[name] = reason
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
		if variable, ok := obj.(*types.Var); ok {
			if typ, reason := typeFromGo(variable.Type()); reason == "" {
				goPkg.Variables[name] = typ
			} else {
				goPkg.UnsupportedVariables[name] = reason
			}
			continue
		}
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		sig := fn.Type().(*types.Signature)
		if sig.TypeParams().Len() > 0 {
			// Generic Go functions are mapped lazily at each call site, once
			// type arguments are known (explicit or inferred).
			goPkg.Generics[name] = fn
			continue
		}
		def, reason := functionDefFromGoSignature(name, sig)
		if reason == "" {
			goPkg.Functions[name] = def
		} else {
			goPkg.UnsupportedFunctions[name] = reason
		}
	}
	return goPkg
}

func functionDefFromGoSignature(name string, sig *types.Signature) (*FunctionDef, string) {
	return functionDefFromGoSignatureWithMethods(name, sig, true)
}

func foreignResultShape(results *types.Tuple) ForeignResultShape {
	switch {
	case results.Len() == 1 && isGoError(results.At(0).Type()):
		return ForeignResultErrorOnly
	case results.Len() == 2 && isGoError(results.At(1).Type()):
		return ForeignResultValueError
	case results.Len() == 2 && isGoBool(results.At(1).Type()):
		return ForeignResultValueBool
	default:
		return ForeignResultDirect
	}
}

func functionDefFromGoSignatureWithMethods(name string, sig *types.Signature, includeMethods bool) (*FunctionDef, string) {
	params := make([]Parameter, 0, sig.Params().Len())
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		goType := param.Type()
		mutable := false
		foreignABI := ForeignParameterExact
		variadic := false
		if sig.Variadic() && i == sig.Params().Len()-1 {
			slice, ok := goType.(*types.Slice)
			if !ok {
				return nil, fmt.Sprintf("variadic parameter %d is not a slice", i+1)
			}
			goType = slice.Elem()
			variadic = true
		}
		if _, ok := goType.Underlying().(*types.Slice); ok {
			mutable = true
			foreignABI = ForeignParameterDescriptorValue
		} else if _, ok := goType.Underlying().(*types.Map); ok {
			mutable = true
			foreignABI = ForeignParameterDescriptorValue
		}
		ardType, reason := typeFromGoWithMethods(goType, includeMethods)
		if reason != "" {
			return nil, fmt.Sprintf("parameter %d has unsupported type %s: %s", i+1, goType.String(), reason)
		}
		if mutable && !isReferenceType(ardType) {
			// Every Go slice/map parameter is an explicit-reference-required
			// descriptor boundary: only an actual list/map reference flows in,
			// while lowering still projects the exact descriptor value the Go
			// ABI requires (ADR 0057).
			ardType = MakeMutableRef(ardType)
		}
		paramName := param.Name()
		if paramName == "" {
			paramName = fmt.Sprintf("arg%d", i+1)
		}
		params = append(params, Parameter{Name: paramName, Type: ardType, Mutable: mutable, ForeignABI: foreignABI, Variadic: variadic})
	}

	ret, reason := returnTypeFromGoWithMethods(sig.Results(), includeMethods)
	if reason != "" {
		return nil, reason
	}
	return &FunctionDef{Name: name, Parameters: params, ReturnType: ret, ForeignResultShape: foreignResultShape(sig.Results())}, ""
}

// functionDefFromGoCallbackSignature maps a Go callback parameter's
// signature to the Ard function type that satisfies it. Callback
// compatibility must stay conversion-free: an Ard closure's lowered Go form
// must BE the Go callback type (ADR 0038 ABI), because both direct calls and
// function-value calls (#275 adapters) pass the closure through unchanged.
// A callback shape that would need a call-site wrapper must be rejected
// here, not papered over at one call path.
func functionDefFromGoCallbackSignature(name string, sig *types.Signature) (*FunctionDef, string) {
	params := make([]Parameter, 0, sig.Params().Len())
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		goType := param.Type()
		variadic := sig.Variadic() && i == sig.Params().Len()-1
		if variadic {
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
		if variadic && isDescriptorBoundaryArdType(ardType) {
			return nil, fmt.Sprintf("variadic callback parameter %d has descriptor element type %s, which requires an unsupported call adapter", i+1, goType.String())
		}
		paramName := param.Name()
		if paramName == "" {
			paramName = fmt.Sprintf("arg%d", i+1)
		}
		params = append(params, Parameter{Name: paramName, Type: ardType, Variadic: variadic})
	}
	ret, reason := callbackReturnTypeFromGo(sig.Results())
	if reason != "" {
		return nil, reason
	}
	return &FunctionDef{Name: name, Parameters: params, ReturnType: ret}, ""
}

// callbackReturnTypeFromGo maps a Go callback's results to the Ard return
// type of the closure that satisfies it, mirroring the call-boundary result
// adaptation in reverse: an error result means the Ard callback returns a
// Result whose error arm becomes the Go error, and a comma-ok pair means it
// returns a Maybe. Both rely on those Ard returns already lowering to the
// matching Go ABI shapes (ADR 0038), so no wrapper is generated.
func callbackReturnTypeFromGo(results *types.Tuple) (Type, string) {
	switch results.Len() {
	case 0:
		return Void, ""
	case 1:
		resultType := results.At(0).Type()
		if isGoError(resultType) {
			return MakeResult(Void, BuiltinError), ""
		}
		// Ard Void functions have no Go result. They cannot satisfy a Go
		// callback that returns a value-position struct{} without an adapter,
		// and callback parameters are intentionally conversion-free.
		if isGoEmptyStruct(types.Unalias(resultType)) {
			return nil, "callback result struct{} requires an ABI adapter"
		}
		return typeFromGoWithMethods(resultType, false)
	case 2:
		// isGoError/isGoBool intentionally match only the universe error type
		// and the basic bool: a named bool (`type MyBool bool`) or error-like
		// interface would make the lowered Ard closure's ABI shape
		// (`(T, bool)` / `(T, error)`) un-assignable to the Go callback type,
		// producing uncompilable Go. The restriction is load-bearing.
		if isGoError(results.At(1).Type()) || isGoBool(results.At(1).Type()) {
			if isGoEmptyStruct(types.Unalias(results.At(0).Type())) {
				return nil, "callback result 1 struct{} requires an ABI adapter"
			}
			value, reason := typeFromGoWithMethods(results.At(0).Type(), false)
			if reason != "" {
				return nil, fmt.Sprintf("callback result 1 has unsupported type %s: %s", results.At(0).Type().String(), reason)
			}
			if isGoError(results.At(1).Type()) {
				return MakeResult(value, BuiltinError), ""
			}
			return MakeMaybe(value), ""
		}
		return nil, fmt.Sprintf("callback multi-result shape %s is not supported yet", results.String())
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
			return MakeResult(Void, BuiltinError), ""
		}
		return typeFromGoWithMethods(results.At(0).Type(), includeMethods)
	case 2:
		if isGoError(results.At(1).Type()) {
			val, reason := typeFromGoWithMethods(results.At(0).Type(), includeMethods)
			if reason != "" {
				return nil, fmt.Sprintf("result 1 has unsupported type %s: %s", results.At(0).Type().String(), reason)
			}
			return MakeResult(val, BuiltinError), ""
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
	// A bare type parameter must be checked before the empty-interface test:
	// a `T any` parameter's underlying type is the empty interface, but an
	// uninstantiated type parameter is not Ard `Any`.
	if tp, ok := t.(*types.TypeParam); ok {
		return nil, fmt.Sprintf("type parameter %s requires instantiation", tp.Obj().Name())
	}
	if isGoError(t) {
		return BuiltinError, ""
	}
	if isGoAny(t) {
		return Any, ""
	}
	if alias, ok := t.(*types.Alias); ok {
		// A Go alias is the same type as its target, so aliases of named types
		// (for example `ui.Style = vaxis.Style`) resolve through the aliased
		// type to preserve type identity across packages.
		unaliased := types.Unalias(t)
		if _, isBasic := unaliased.(*types.Basic); !isBasic {
			return typeFromGoWithMethods(unaliased, includeMethods)
		}
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
		return &ForeignType{Target: "go", Namespace: namespace, Qualifier: qualifier, Name: alias.Obj().Name(), Underlying: underlying, GoType: alias}, ""
	}
	if named, ok := t.(*types.Named); ok && !isGoError(t) {
		if sig, ok := named.Underlying().(*types.Signature); ok {
			// A named Go func type (for example `ui.VoidCallback`) keeps its
			// type identity so generated Go names the exact type. Its
			// Underlying carries the signature; Ard closures with a matching
			// signature are assignable, mirroring Go's unnamed-to-named rule.
			fn, reason := functionDefFromGoCallbackSignature(named.Obj().Name(), sig)
			if reason != "" {
				return nil, reason
			}
			pkg := named.Obj().Pkg()
			namespace := ""
			qualifier := ""
			if pkg != nil {
				namespace = pkg.Path()
				qualifier = pkg.Name()
			}
			return &ForeignType{Target: "go", Namespace: namespace, Qualifier: qualifier, Name: named.Obj().Name(), Underlying: fn, GoType: named}, ""
		}
		if reason := unsupportedGoNamedTypeArgs(named); reason != "" {
			return nil, reason
		}
		if reason := unsupportedForeignNamedUnderlying(named.Underlying(), false); reason != "" {
			return nil, reason
		}
		return foreignNamedTypeFromGo(named, false, includeMethods), ""
	}
	// Go's unnamed empty struct carries no values and is the Go
	// representation Ard already uses for value-position Void. Named empty
	// structs are handled above and retain their distinct Go type identity;
	// aliases reach this point through their unaliased target.
	if isGoEmptyStruct(t) {
		return Void, ""
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
			if reason := unsupportedGoNamedTypeArgs(named); reason != "" {
				return nil, reason
			}
			if reason := unsupportedForeignNamedUnderlying(named.Underlying(), true); reason != "" {
				return nil, reason
			}
			return foreignNamedTypeFromGo(named, true, includeMethods), ""
		}
		// Pointer-to-descriptor and multi-level pointers are reference
		// boundaries (ADR 0057): only an actual reference flows in, and
		// lowering projects the exact pointer shape. Ard cannot construct
		// multi-level pointers itself; they flow only from compatible foreign
		// values.
		if _, pointerToInterface := ptr.Elem().Underlying().(*types.Interface); pointerToInterface {
			return nil, "pointers to Go interfaces are unsupported"
		}
		// Every otherwise representable single-level pointee, including Go
		// primitives, is an explicit reference boundary (ADR 0057).
		inner, reason := typeFromGoWithMethods(ptr.Elem(), includeMethods)
		if reason != "" {
			return nil, reason
		}
		return MakeMutableRef(inner), ""
	}
	if slice, ok := t.Underlying().(*types.Slice); ok {
		elem, reason := typeFromGoWithMethods(slice.Elem(), includeMethods)
		if reason != "" {
			return nil, "slice element " + reason
		}
		return MakeList(elem), ""
	}
	if array, ok := t.Underlying().(*types.Array); ok {
		elem, reason := typeFromGoWithMethods(array.Elem(), includeMethods)
		if reason != "" {
			return nil, "array element " + reason
		}
		if array.Len() > int64(math.MaxInt) {
			return nil, "array length too large"
		}
		return MakeFixedArray(elem, int(array.Len())), ""
	}
	if goChan, ok := t.Underlying().(*types.Chan); ok {
		elem, reason := typeFromGoWithMethods(goChan.Elem(), includeMethods)
		if reason != "" {
			return nil, "channel element " + reason
		}
		switch goChan.Dir() {
		case types.SendRecv:
			return MakeChan(elem), ""
		case types.RecvOnly:
			return MakeReceiver(elem), ""
		case types.SendOnly:
			return MakeSender(elem), ""
		default:
			return nil, "unsupported channel direction"
		}
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

func unsupportedGoNamedTypeArgs(named *types.Named) string {
	args := named.TypeArgs()
	if args == nil || args.Len() == 0 {
		return ""
	}
	for i := 0; i < args.Len(); i++ {
		if _, reason := typeFromGoWithMethods(args.At(i), false); reason != "" {
			return fmt.Sprintf("type argument %d has unsupported type %s: %s", i+1, args.At(i).String(), reason)
		}
	}
	return ""
}

func exportedNamedTypeFromGo(typeName *types.TypeName) (Type, string) {
	// Only the unnamed empty interface (and aliases of it) map to Ard's
	// opaque Any; named empty interfaces keep their Go type identity.
	if isGoAny(typeName.Type()) {
		return Any, ""
	}
	if typeName.IsAlias() {
		return typeFromGo(typeName.Type())
	}
	named, ok := typeName.Type().(*types.Named)
	if !ok {
		return nil, "exported Go type is not named"
	}
	// Named func types keep their identity through the general mapping.
	if _, isFunc := named.Underlying().(*types.Signature); isFunc {
		return typeFromGo(named)
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
	if goSlice, ok := underlying.(*types.Slice); ok {
		if _, reason := typeFromGoWithMethods(goSlice.Elem(), false); reason != "" {
			return "slice element " + reason
		}
		return ""
	}
	if goArray, ok := underlying.(*types.Array); ok {
		if _, reason := typeFromGoWithMethods(goArray.Elem(), false); reason != "" {
			return "array element " + reason
		}
		if goArray.Len() > int64(math.MaxInt) {
			return "array length too large"
		}
		return ""
	}
	if _, ok := underlying.(*types.Interface); ok {
		if pointer {
			return "pointers to Go interface types are not supported"
		}
		return ""
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
	if goArray, ok := named.Underlying().(*types.Array); ok {
		if elem, reason := typeFromGoWithMethods(goArray.Elem(), false); reason == "" && goArray.Len() <= int64(math.MaxInt) {
			underlying = MakeFixedArray(elem, int(goArray.Len()))
		}
	}
	_, isStruct := named.Underlying().(*types.Struct)
	_, isInterface := named.Underlying().(*types.Interface)
	goType := types.Type(named)
	if pointer {
		goType = types.NewPointer(named)
	}
	foreign := &ForeignType{Target: "go", Namespace: namespace, Qualifier: qualifier, Name: named.Obj().Name(), Underlying: underlying, Pointer: pointer, Struct: isStruct, Interface: isInterface, GoType: goType}
	if args := named.TypeArgs(); args != nil && args.Len() > 0 {
		for i := 0; i < args.Len(); i++ {
			arg, reason := typeFromGoWithMethods(args.At(i), false)
			if reason == "" {
				foreign.TypeArgs = append(foreign.TypeArgs, arg)
			}
		}
	}
	foreign.LoadFields = func() (map[string]Type, map[string]string) { return goFieldsForNamedType(named) }
	foreign.LoadMethods = func(pointer bool) (map[string]*FunctionDef, map[string]string) {
		return goMethodsForNamedType(named, pointer)
	}
	if !pointer {
		if goMap, ok := named.Underlying().(*types.Map); ok {
			if key, reason := typeFromGoWithMethods(goMap.Key(), false); reason == "" {
				foreign.MapKey = key
			}
			if value, reason := typeFromGoWithMethods(goMap.Elem(), false); reason == "" {
				foreign.MapValue = value
			}
		}
		if goSlice, ok := named.Underlying().(*types.Slice); ok {
			if elem, reason := typeFromGoWithMethods(goSlice.Elem(), false); reason == "" {
				foreign.Elem = elem
			}
		}
	}
	if includeMethods {
		foreign.Methods, foreign.UnsupportedMethods = goMethodsForNamedType(named, pointer)
		if !pointer {
			foreign.PointerMethods, foreign.UnsupportedPointerMethods = goMethodsForNamedType(named, true)
		}
		foreign.MethodsLoaded = true
	}
	return foreign
}

func goFieldsForInstantiatedGenericNamedType(instantiated, origin *types.Named, args []Type) (map[string]Type, map[string]string) {
	fields, unsupported := goFieldsForNamedType(instantiated)
	bound, _ := goFieldsForBoundGenericNamedType(origin, args)
	if fields == nil {
		fields = map[string]Type{}
	}
	if unsupported == nil {
		unsupported = map[string]string{}
	}
	for name, typ := range bound {
		fields[name] = typ
		delete(unsupported, name)
	}
	return fields, unsupported
}

func goFieldsForBoundGenericNamedType(named *types.Named, args []Type) (map[string]Type, map[string]string) {
	if named == nil || named.TypeParams() == nil || named.TypeParams().Len() != len(args) {
		return nil, nil
	}
	strct, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil, nil
	}
	bindings := goTypeParamBindings{}
	for i := 0; i < named.TypeParams().Len(); i++ {
		bindings[named.TypeParams().At(i)] = args[i]
	}
	fields := map[string]Type{}
	unsupported := map[string]string{}
	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if !field.Exported() {
			continue
		}
		bound, reason := boundTypeFromGo(field.Type(), named.TypeParams(), bindings)
		if reason == "" {
			fields[field.Name()] = bound
		} else {
			unsupported[field.Name()] = reason
		}
	}
	return fields, unsupported
}

func goFieldsForNamedType(named *types.Named) (map[string]Type, map[string]string) {
	strct, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil, nil
	}
	return goFieldsForStruct(strct)
}

func goFieldsForStruct(strct *types.Struct) (map[string]Type, map[string]string) {
	fields := map[string]Type{}
	unsupported := map[string]string{}
	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if !field.Exported() {
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
		declared := method.Origin().Type().(*types.Signature)
		instantiated := method.Type().(*types.Signature)
		def, reason := functionDefFromGoMethodSignatures(method.Name(), declared, instantiated)
		if reason == "" {
			methods[method.Name()] = def
		} else {
			unsupported[method.Name()] = reason
		}
	}
	return methods, unsupported
}

// functionDefFromGoMethodSignatures maps concrete selected method types while
// classifying the error convention from the method's declaration. A generic
// value result instantiated with error therefore remains an ordinary Error.
func functionDefFromGoMethodSignatures(name string, declared, instantiated *types.Signature) (*FunctionDef, string) {
	def, reason := functionDefFromGoSignatureWithMethods(name, instantiated, false)
	if reason != "" {
		return nil, reason
	}
	ret, reason := returnTypeFromGoMethodSignatures(declared.Results(), instantiated.Results())
	if reason != "" {
		return nil, reason
	}
	def.ReturnType = ret
	for i := range def.Parameters {
		if i >= declared.Params().Len() || i >= instantiated.Params().Len() {
			break
		}
		declaredParam := declared.Params().At(i).Type()
		instantiatedParam := instantiated.Params().At(i).Type()
		if def.Parameters[i].Variadic {
			if slice, ok := declaredParam.(*types.Slice); ok {
				declaredParam = slice.Elem()
			}
			if slice, ok := instantiatedParam.(*types.Slice); ok {
				instantiatedParam = slice.Elem()
			}
		}
		mapped, reason := remapDeclaredGoCallbackType(declaredParam, instantiatedParam, def.Parameters[i].Type)
		if reason != "" {
			return nil, fmt.Sprintf("parameter %d has unsupported type %s: %s", i+1, instantiated.Params().At(i).Type(), reason)
		}
		def.Parameters[i].Type = mapped
	}
	def.ForeignResultShape = foreignResultShapeFromMethodSignatures(declared.Results(), instantiated.Results())
	return def, ""
}

func returnTypeFromGoMethodSignatures(declared, instantiated *types.Tuple) (Type, string) {
	if declared.Len() != instantiated.Len() {
		return nil, "instantiated method result shape does not match its declaration"
	}
	switch declared.Len() {
	case 0:
		return Void, ""
	case 1:
		if isGoError(declared.At(0).Type()) {
			return MakeResult(Void, BuiltinError), ""
		}
		mapped, reason := typeFromGoWithMethods(instantiated.At(0).Type(), false)
		if reason != "" {
			return nil, reason
		}
		return remapDeclaredGoCallbackType(declared.At(0).Type(), instantiated.At(0).Type(), mapped)
	case 2:
		if isGoError(declared.At(1).Type()) {
			value, reason := typeFromGoWithMethods(instantiated.At(0).Type(), false)
			if reason != "" {
				return nil, fmt.Sprintf("result 1 has unsupported type %s: %s", instantiated.At(0).Type(), reason)
			}
			value, reason = remapDeclaredGoCallbackType(declared.At(0).Type(), instantiated.At(0).Type(), value)
			if reason != "" {
				return nil, fmt.Sprintf("result 1 has unsupported type %s: %s", instantiated.At(0).Type(), reason)
			}
			return MakeResult(value, BuiltinError), ""
		}
		if isGoBool(instantiated.At(1).Type()) {
			value, reason := typeFromGoWithMethods(instantiated.At(0).Type(), false)
			if reason != "" {
				return nil, fmt.Sprintf("result 1 has unsupported type %s: %s", instantiated.At(0).Type(), reason)
			}
			value, reason = remapDeclaredGoCallbackType(declared.At(0).Type(), instantiated.At(0).Type(), value)
			if reason != "" {
				return nil, fmt.Sprintf("result 1 has unsupported type %s: %s", instantiated.At(0).Type(), reason)
			}
			return MakeMaybe(value), ""
		}
	}
	return nil, fmt.Sprintf("unsupported result shape %s", instantiated.String())
}

func remapDeclaredGoCallbackType(declared, instantiated types.Type, mapped Type) (Type, string) {
	declaredSignature, declaredIsCallback := declared.Underlying().(*types.Signature)
	instantiatedSignature, instantiatedIsCallback := instantiated.Underlying().(*types.Signature)
	if !declaredIsCallback || !instantiatedIsCallback {
		return remapDeclaredGoCallbackContainer(declared, instantiated, mapped)
	}
	callback, reason := functionDefFromGoCallbackSignature("<function>", instantiatedSignature)
	if reason != "" {
		return nil, reason
	}
	ret, reason := returnTypeFromGoMethodSignatures(declaredSignature.Results(), instantiatedSignature.Results())
	if reason != "" {
		return nil, reason
	}
	callback.ReturnType = ret
	for i := range callback.Parameters {
		if i >= declaredSignature.Params().Len() || i >= instantiatedSignature.Params().Len() {
			break
		}
		declaredParam := declaredSignature.Params().At(i).Type()
		instantiatedParam := instantiatedSignature.Params().At(i).Type()
		if callback.Parameters[i].Variadic {
			if slice, ok := declaredParam.(*types.Slice); ok {
				declaredParam = slice.Elem()
			}
			if slice, ok := instantiatedParam.(*types.Slice); ok {
				instantiatedParam = slice.Elem()
			}
		}
		parameterType, reason := remapDeclaredGoCallbackType(declaredParam, instantiatedParam, callback.Parameters[i].Type)
		if reason != "" {
			return nil, reason
		}
		callback.Parameters[i].Type = parameterType
	}
	if foreign, ok := mapped.(*ForeignType); ok {
		copy := *foreign
		copy.Underlying = callback
		return &copy, ""
	}
	return callback, ""
}

func remapDeclaredGoCallbackContainer(declared, instantiated types.Type, mapped Type) (Type, string) {
	if declaredPointer, ok := declared.(*types.Pointer); ok {
		instantiatedPointer, ok := instantiated.(*types.Pointer)
		mappedReference, mappedOK := mapped.(*MutableRef)
		if !ok || !mappedOK {
			return mapped, ""
		}
		elem, reason := remapDeclaredGoCallbackType(declaredPointer.Elem(), instantiatedPointer.Elem(), mappedReference.Of())
		if reason != "" {
			return nil, reason
		}
		return MakeMutableRef(elem), ""
	}

	// Slice and map parameters carry an Ard reference wrapper for descriptor
	// mutability. Recurse through that wrapper without confusing it for a Go
	// pointer declaration.
	if mappedReference, ok := mapped.(*MutableRef); ok {
		inner, reason := remapDeclaredGoCallbackContainer(declared, instantiated, mappedReference.Of())
		if reason != "" {
			return nil, reason
		}
		return MakeMutableRef(inner), ""
	}

	switch declaredContainer := declared.Underlying().(type) {
	case *types.Slice:
		instantiatedContainer, ok := instantiated.Underlying().(*types.Slice)
		if !ok {
			return mapped, ""
		}
		var mappedElem Type
		switch container := mapped.(type) {
		case *List:
			mappedElem = container.Of()
		case *ForeignType:
			mappedElem = container.Elem
		}
		if mappedElem == nil {
			return mapped, ""
		}
		elem, reason := remapDeclaredGoCallbackType(declaredContainer.Elem(), instantiatedContainer.Elem(), mappedElem)
		if reason != "" {
			return nil, reason
		}
		if foreign, ok := mapped.(*ForeignType); ok {
			copy := *foreign
			copy.Elem = elem
			return &copy, ""
		}
		return MakeList(elem), ""
	case *types.Array:
		instantiatedContainer, ok := instantiated.Underlying().(*types.Array)
		if !ok {
			return mapped, ""
		}
		var array *FixedArray
		switch container := mapped.(type) {
		case *FixedArray:
			array = container
		case *ForeignType:
			array, _ = container.Underlying.(*FixedArray)
		}
		if array == nil {
			return mapped, ""
		}
		elem, reason := remapDeclaredGoCallbackType(declaredContainer.Elem(), instantiatedContainer.Elem(), array.Of())
		if reason != "" {
			return nil, reason
		}
		remappedArray := MakeFixedArray(elem, array.Len())
		if foreign, ok := mapped.(*ForeignType); ok {
			copy := *foreign
			copy.Underlying = remappedArray
			return &copy, ""
		}
		return remappedArray, ""
	case *types.Map:
		instantiatedContainer, ok := instantiated.Underlying().(*types.Map)
		if !ok {
			return mapped, ""
		}
		var mappedKey, mappedValue Type
		switch container := mapped.(type) {
		case *Map:
			mappedKey, mappedValue = container.Key(), container.Value()
		case *ForeignType:
			mappedKey, mappedValue = container.MapKey, container.MapValue
		}
		if mappedKey == nil || mappedValue == nil {
			return mapped, ""
		}
		key, reason := remapDeclaredGoCallbackType(declaredContainer.Key(), instantiatedContainer.Key(), mappedKey)
		if reason != "" {
			return nil, reason
		}
		value, reason := remapDeclaredGoCallbackType(declaredContainer.Elem(), instantiatedContainer.Elem(), mappedValue)
		if reason != "" {
			return nil, reason
		}
		if foreign, ok := mapped.(*ForeignType); ok {
			copy := *foreign
			copy.MapKey, copy.MapValue = key, value
			return &copy, ""
		}
		return MakeMap(key, value), ""
	case *types.Struct:
		instantiatedContainer, ok := instantiated.Underlying().(*types.Struct)
		foreign, mappedOK := mapped.(*ForeignType)
		if !ok || !mappedOK {
			return mapped, ""
		}
		copy := *foreign
		remapFields := func(fields map[string]Type) (map[string]Type, string) {
			remapped := make(map[string]Type, len(fields))
			for name, field := range fields {
				remapped[name] = field
			}
			for i := 0; i < declaredContainer.NumFields() && i < instantiatedContainer.NumFields(); i++ {
				declaredField := declaredContainer.Field(i)
				instantiatedField := instantiatedContainer.Field(i)
				field, exists := fields[instantiatedField.Name()]
				if !exists {
					continue
				}
				mappedField, reason := remapDeclaredGoCallbackType(declaredField.Type(), instantiatedField.Type(), field)
				if reason != "" {
					return nil, reason
				}
				remapped[instantiatedField.Name()] = mappedField
			}
			return remapped, ""
		}
		if foreign.Fields != nil {
			fields, reason := remapFields(foreign.Fields)
			if reason != "" {
				return nil, reason
			}
			copy.Fields = fields
		}
		if foreign.LoadFields != nil {
			loadFields := foreign.LoadFields
			copy.LoadFields = func() (map[string]Type, map[string]string) {
				fields, unsupported := loadFields()
				remapped, _ := remapFields(fields)
				return remapped, unsupported
			}
		}
		return &copy, ""
	case *types.Chan:
		instantiatedContainer, ok := instantiated.Underlying().(*types.Chan)
		if !ok {
			return mapped, ""
		}
		var mappedElem Type
		switch channel := mapped.(type) {
		case *Chan:
			mappedElem = channel.Of()
		case *Receiver:
			mappedElem = channel.Of()
		case *Sender:
			mappedElem = channel.Of()
		}
		if mappedElem == nil {
			return mapped, ""
		}
		elem, reason := remapDeclaredGoCallbackType(declaredContainer.Elem(), instantiatedContainer.Elem(), mappedElem)
		if reason != "" {
			return nil, reason
		}
		switch instantiatedContainer.Dir() {
		case types.RecvOnly:
			return MakeReceiver(elem), ""
		case types.SendOnly:
			return MakeSender(elem), ""
		default:
			return MakeChan(elem), ""
		}
	default:
		return mapped, ""
	}
}

func foreignResultShapeFromMethodSignatures(declared, instantiated *types.Tuple) ForeignResultShape {
	switch {
	case declared.Len() == 1 && isGoError(declared.At(0).Type()):
		return ForeignResultErrorOnly
	case declared.Len() == 2 && isGoError(declared.At(1).Type()):
		return ForeignResultValueError
	case instantiated.Len() == 2 && isGoBool(instantiated.At(1).Type()):
		return ForeignResultValueBool
	default:
		return ForeignResultDirect
	}
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

func isGoEmptyStruct(t types.Type) bool {
	strct, ok := t.(*types.Struct)
	return ok && strct.NumFields() == 0
}

// isGoAny reports whether a Go type is the unnamed empty interface (`any` /
// `interface{}`), including aliases of it. A *named* empty interface (for
// example `type Event interface{}`) is a distinct Go type identity and maps
// to a foreign interface type instead, so signatures that name it lower to
// the exact Go type.
func isGoAny(t types.Type) bool {
	if _, isNamed := types.Unalias(t).(*types.Named); isNamed {
		return false
	}
	iface, ok := t.Underlying().(*types.Interface)
	return ok && iface.Empty()
}

func isGoBool(t types.Type) bool {
	basic, ok := t.(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

func isGoError(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Pkg() == nil && named.Obj().Name() == "error"
}
