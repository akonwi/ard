package checker

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/akonwi/ard/parse"
	"golang.org/x/tools/go/packages"
)

type GoPackageResolver interface {
	LoadPackage(importPath string) (*GoPackage, error)
}

type GoPackage struct {
	ImportPath string
	Name       string
	Functions  map[string]GoFunction
	Types      map[string]GoType
	Constants  map[string]GoConstant
}

type GoFunction struct {
	Name      string
	Signature GoSignature
}

type GoMethod struct {
	Name      string
	Signature GoSignature
}

type GoSignature struct {
	Receiver *GoValueType
	Params   []GoValueType
	Results  []GoValueType
	Variadic bool
}

type GoValueKind string

const (
	GoValueInvalid GoValueKind = ""
	GoValueBool    GoValueKind = "bool"
	GoValueString  GoValueKind = "string"
	GoValueInt     GoValueKind = "int"
	GoValueUint    GoValueKind = "uint"
	GoValueFloat   GoValueKind = "float"
	GoValueAny     GoValueKind = "any"
	GoValueSlice   GoValueKind = "slice"
	GoValueMap     GoValueKind = "map"
	GoValuePointer GoValueKind = "pointer"
	GoValueError   GoValueKind = "error"
	GoValueOther   GoValueKind = "other"
)

type GoValueType struct {
	Expr       string
	Kind       GoValueKind
	Bits       int
	Named      bool
	ImportPath string
	Package    string
	Name       string
	Elem       *GoValueType
	Key        *GoValueType
	Value      *GoValueType
}

type GoType struct {
	Name    string
	Methods map[string]GoMethod
}

type GoConstant struct {
	Name string
}

type directGoImport struct {
	alias      string
	importPath string
	pkg        *GoPackage
}

type GoPackagesResolver struct {
	Dir   string
	cache map[string]*GoPackage
}

func NewGoPackagesResolver(dir string) *GoPackagesResolver {
	return &GoPackagesResolver{Dir: dir, cache: map[string]*GoPackage{}}
}

func (r *GoPackagesResolver) LoadPackage(importPath string) (*GoPackage, error) {
	if r.cache == nil {
		r.cache = map[string]*GoPackage{}
	}
	if cached, ok := r.cache[importPath]; ok {
		return cached, nil
	}
	cfg := &packages.Config{
		Dir:        r.Dir,
		Mode:       packages.NeedName | packages.NeedTypes,
		BuildFlags: []string{"-mod=readonly"},
	}
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("package not found")
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		parts := make([]string, len(pkg.Errors))
		for i, pkgErr := range pkg.Errors {
			parts[i] = pkgErr.Msg
		}
		return nil, fmt.Errorf("%s", strings.Join(parts, "; "))
	}
	if pkg.Types == nil {
		return nil, fmt.Errorf("package has no type information")
	}
	resolved := goPackageFromTypes(importPath, pkg.Name, pkg.Types)
	r.cache[importPath] = resolved
	return resolved, nil
}

func goPackageFromTypes(importPath string, name string, pkg *types.Package) *GoPackage {
	out := &GoPackage{
		ImportPath: importPath,
		Name:       name,
		Functions:  map[string]GoFunction{},
		Types:      map[string]GoType{},
		Constants:  map[string]GoConstant{},
	}
	if pkg == nil || pkg.Scope() == nil {
		return out
	}
	for _, name := range pkg.Scope().Names() {
		obj := pkg.Scope().Lookup(name)
		if obj == nil || !obj.Exported() {
			continue
		}
		switch obj := obj.(type) {
		case *types.Func:
			out.Functions[name] = GoFunction{Name: name, Signature: goSignature(obj.Type())}
		case *types.TypeName:
			out.Types[name] = GoType{Name: name, Methods: exportedMethods(obj.Type())}
		case *types.Const:
			out.Constants[name] = GoConstant{Name: name}
		}
	}
	return out
}

func (c *Checker) resolveDirectGoType(ty *parse.CustomType) Type {
	if ty == nil || ty.Type.Target == nil {
		return nil
	}
	alias, ok := ty.Type.Target.(*parse.Identifier)
	if !ok {
		return nil
	}
	property, ok := ty.Type.Property.(*parse.Identifier)
	if !ok {
		return nil
	}
	goImport, ok := c.directGoImports[alias.Name]
	if !ok {
		return nil
	}
	if goImport.pkg != nil {
		if _, ok := goImport.pkg.Types[property.Name]; !ok {
			c.addError(fmt.Sprintf("Go package %q has no exported type %q", goImport.importPath, property.Name), ty.GetLocation())
			return &TypeVar{name: "unknown"}
		}
	}
	binding := canonicalDirectGoBinding(goImport.importPath, []string{property.Name})
	return &ExternType{Name_: ty.GetName(), ExternalBinding: binding, ExternalBindings: map[string]string{"go": binding}}
}

func (c *Checker) resolveDirectGoExternBinding(binding string, loc parse.Location) string {
	parts, ok := directGoBindingParts(binding)
	if !ok {
		return binding
	}
	if len(parts) != 2 && len(parts) != 3 {
		c.addError(fmt.Sprintf("Direct Go extern binding %q must be package::Function or package::Type::Method", binding), loc)
		return binding
	}
	goImport, ok := c.directGoImports[parts[0]]
	if !ok {
		c.addError(fmt.Sprintf("Unknown Go import alias %q in extern binding %q", parts[0], binding), loc)
		return binding
	}
	if goImport.pkg != nil {
		if len(parts) == 2 {
			if _, ok := goImport.pkg.Functions[parts[1]]; !ok {
				c.addError(fmt.Sprintf("Go package %q has no exported function %q", goImport.importPath, parts[1]), loc)
			}
		} else {
			typ, ok := goImport.pkg.Types[parts[1]]
			if !ok {
				c.addError(fmt.Sprintf("Go package %q has no exported type %q", goImport.importPath, parts[1]), loc)
			} else if _, ok := typ.Methods[parts[2]]; !ok {
				c.addError(fmt.Sprintf("Go type %q in package %q has no exported method %q", parts[1], goImport.importPath, parts[2]), loc)
			}
		}
	}
	return canonicalDirectGoBinding(goImport.importPath, parts[1:])
}

func canonicalDirectGoBinding(importPath string, symbolParts []string) string {
	return "go:" + importPath + "::" + strings.Join(symbolParts, "::")
}

func directGoBindingParts(binding string) ([]string, bool) {
	if !strings.Contains(binding, "::") {
		return nil, false
	}
	parts := strings.Split(binding, "::")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return parts, true
		}
	}
	return parts, true
}

type canonicalDirectGoBindingInfo struct {
	ImportPath string
	Symbols    []string
}

func parseCanonicalDirectGoBinding(binding string) (canonicalDirectGoBindingInfo, bool) {
	if !strings.HasPrefix(binding, "go:") {
		return canonicalDirectGoBindingInfo{}, false
	}
	parts := strings.Split(strings.TrimPrefix(binding, "go:"), "::")
	if len(parts) != 2 && len(parts) != 3 {
		return canonicalDirectGoBindingInfo{}, false
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return canonicalDirectGoBindingInfo{}, false
		}
	}
	return canonicalDirectGoBindingInfo{ImportPath: parts[0], Symbols: parts[1:]}, true
}

type directGoSignatureTarget struct {
	Name      string
	Signature GoSignature
	Method    bool
}

func (c *Checker) validateDirectGoExternSignature(name string, params []Parameter, returnType Type, binding string, loc parse.Location) {
	target, ok := c.directGoSignatureTarget(binding)
	if !ok {
		return
	}
	if target.Signature.Variadic {
		c.addError(fmt.Sprintf("Go function %s is variadic; variadic direct Go externs are not supported yet", target.Name), loc)
		return
	}
	expectedParams := len(target.Signature.Params)
	paramOffset := 0
	if target.Method {
		expectedParams++
		paramOffset = 1
	}
	if len(params) != expectedParams {
		c.addError(fmt.Sprintf("Go function %s expects %d parameter(s), extern %s declares %d", target.Name, expectedParams, name, len(params)), loc)
		return
	}
	if target.Method && target.Signature.Receiver != nil {
		if ok, reason := c.directGoAssignableCompatible(params[0].Type, *target.Signature.Receiver); !ok {
			c.addError(fmt.Sprintf("receiver for %s: %s", target.Name, reason), loc)
		}
	}
	for i, goParam := range target.Signature.Params {
		ardParam := params[i+paramOffset]
		if ok, reason := c.directGoParamCompatible(ardParam.Type, goParam, true); !ok {
			c.addError(fmt.Sprintf("parameter %d for %s: %s", i+1+paramOffset, target.Name, reason), loc)
		}
	}
	c.validateDirectGoExternReturn(name, returnType, target, loc)
}

func (c *Checker) directGoSignatureTarget(binding string) (directGoSignatureTarget, bool) {
	info, ok := parseCanonicalDirectGoBinding(binding)
	if !ok {
		return directGoSignatureTarget{}, false
	}
	var pkg *GoPackage
	for _, imp := range c.directGoImports {
		if imp.importPath == info.ImportPath {
			pkg = imp.pkg
			break
		}
	}
	if pkg == nil {
		return directGoSignatureTarget{}, false
	}
	switch len(info.Symbols) {
	case 1:
		fn, ok := pkg.Functions[info.Symbols[0]]
		if !ok {
			return directGoSignatureTarget{}, false
		}
		return directGoSignatureTarget{Name: pkgQualifiedName(pkg, info.Symbols), Signature: fn.Signature}, true
	case 2:
		typ, ok := pkg.Types[info.Symbols[0]]
		if !ok {
			return directGoSignatureTarget{}, false
		}
		method, ok := typ.Methods[info.Symbols[1]]
		if !ok {
			return directGoSignatureTarget{}, false
		}
		return directGoSignatureTarget{Name: pkgQualifiedName(pkg, info.Symbols), Signature: method.Signature, Method: true}, true
	default:
		return directGoSignatureTarget{}, false
	}
}

func pkgQualifiedName(pkg *GoPackage, symbols []string) string {
	qualifier := ""
	if pkg != nil {
		qualifier = pkg.Name
		if qualifier == "" {
			qualifier = pkg.ImportPath
		}
	}
	if qualifier == "" {
		return strings.Join(symbols, ".")
	}
	return qualifier + "." + strings.Join(symbols, ".")
}

func (c *Checker) validateDirectGoExternReturn(name string, returnType Type, target directGoSignatureTarget, loc parse.Location) {
	results := target.Signature.Results
	if len(results) == 0 {
		if !equalTypes(returnType, Void) {
			c.addError(fmt.Sprintf("return for %s: Go returns nothing, extern %s declares %s", target.Name, name, returnType.String()), loc)
		}
		return
	}
	if len(results) != 1 {
		c.addError(fmt.Sprintf("return for %s: Go returns %s; multiple-return direct Go adapters are not supported yet", target.Name, goSignatureResultsString(results)), loc)
		return
	}
	if results[0].Kind == GoValueError {
		c.addError(fmt.Sprintf("return for %s: Go return error requires an adapter; direct Go error adapters are not supported yet", target.Name), loc)
		return
	}
	if ok, reason := c.directGoAssignableCompatible(returnType, results[0]); !ok {
		c.addError(fmt.Sprintf("return for %s: %s", target.Name, reason), loc)
	}
}

func goSignatureResultsString(results []GoValueType) string {
	parts := make([]string, len(results))
	for i, result := range results {
		parts[i] = result.String()
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func (c *Checker) directGoParamCompatible(ard Type, goType GoValueType, topLevel bool) (bool, string) {
	ard = derefType(ard)
	if directGoNamedTypeMatches(ard, goType) {
		return true, ""
	}
	if goType.Kind == GoValuePointer {
		return false, fmt.Sprintf("Go type %s is a pointer; direct Go pointer bindings are not supported yet", goType.String())
	}
	if topLevel && directGoScalarCompatible(ard, goType) {
		return true, ""
	}
	if goType.Named {
		return false, fmt.Sprintf("Ard type %s is not compatible with Go named type %s", typeSyntaxString(ard), goType.String())
	}
	switch goType.Kind {
	case GoValueBool, GoValueString, GoValueInt, GoValueUint, GoValueFloat:
		if directGoAssignableScalarCompatible(ard, goType) {
			return true, ""
		}
	case GoValueAny:
		if equalTypes(ard, Dynamic) {
			return true, ""
		}
	case GoValueSlice:
		list, ok := ard.(*List)
		if ok && goType.Elem != nil {
			if compatible, reason := c.directGoParamCompatible(list.Of(), *goType.Elem, false); compatible {
				return true, ""
			} else {
				return false, "list element: " + reason
			}
		}
	case GoValueMap:
		m, ok := ard.(*Map)
		if ok && goType.Key != nil && goType.Value != nil {
			if compatible, reason := c.directGoParamCompatible(m.Key(), *goType.Key, false); !compatible {
				return false, "map key: " + reason
			}
			if compatible, reason := c.directGoParamCompatible(m.Value(), *goType.Value, false); !compatible {
				return false, "map value: " + reason
			}
			return true, ""
		}
	case GoValueError:
		return false, "Go error values require an adapter; direct Go error adapters are not supported yet"
	}
	return false, fmt.Sprintf("Ard type %s is not compatible with Go type %s", typeSyntaxString(ard), goType.String())
}

func (c *Checker) directGoAssignableCompatible(ard Type, goType GoValueType) (bool, string) {
	ard = derefType(ard)
	if directGoNamedTypeMatches(ard, goType) {
		return true, ""
	}
	if goType.Kind == GoValuePointer {
		return false, fmt.Sprintf("Go type %s is a pointer; direct Go pointer bindings are not supported yet", goType.String())
	}
	if goType.Named {
		return false, fmt.Sprintf("Ard type %s is not compatible with Go named type %s", typeSyntaxString(ard), goType.String())
	}
	switch goType.Kind {
	case GoValueBool, GoValueString, GoValueInt, GoValueUint, GoValueFloat:
		if directGoAssignableScalarCompatible(ard, goType) {
			return true, ""
		}
	case GoValueAny:
		if equalTypes(ard, Dynamic) {
			return true, ""
		}
	case GoValueSlice:
		list, ok := ard.(*List)
		if ok && goType.Elem != nil {
			if compatible, reason := c.directGoAssignableCompatible(list.Of(), *goType.Elem); compatible {
				return true, ""
			} else {
				return false, "list element: " + reason
			}
		}
	case GoValueMap:
		m, ok := ard.(*Map)
		if ok && goType.Key != nil && goType.Value != nil {
			if compatible, reason := c.directGoAssignableCompatible(m.Key(), *goType.Key); !compatible {
				return false, "map key: " + reason
			}
			if compatible, reason := c.directGoAssignableCompatible(m.Value(), *goType.Value); !compatible {
				return false, "map value: " + reason
			}
			return true, ""
		}
	case GoValueError:
		return false, "Go error values require an adapter; direct Go error adapters are not supported yet"
	}
	return false, fmt.Sprintf("Ard type %s is not compatible with Go type %s", typeSyntaxString(ard), goType.String())
}

func directGoScalarCompatible(ard Type, goType GoValueType) bool {
	switch goType.Kind {
	case GoValueBool:
		return equalTypes(ard, Bool)
	case GoValueString:
		return equalTypes(ard, Str)
	case GoValueInt:
		return equalTypes(ard, Int) || (goType.Bits == 32 && equalTypes(ard, Rune))
	case GoValueUint:
		return equalTypes(ard, Int) || (goType.Bits == 8 && equalTypes(ard, Byte))
	case GoValueFloat:
		return equalTypes(ard, Float)
	default:
		return false
	}
}

func directGoAssignableScalarCompatible(ard Type, goType GoValueType) bool {
	switch goType.Kind {
	case GoValueBool:
		return equalTypes(ard, Bool)
	case GoValueString:
		return equalTypes(ard, Str)
	case GoValueInt:
		return equalTypes(ard, Int) && goType.Bits == 0 || equalTypes(ard, Rune) && goType.Bits == 32
	case GoValueUint:
		return equalTypes(ard, Byte) && goType.Bits == 8
	case GoValueFloat:
		return equalTypes(ard, Float) && goType.Bits == 64
	default:
		return false
	}
}

func directGoNamedTypeMatches(ard Type, goType GoValueType) bool {
	if !goType.Named || goType.ImportPath == "" || goType.Name == "" {
		return false
	}
	extern, ok := ard.(*ExternType)
	if !ok {
		return false
	}
	return extern.ExternalBinding == canonicalDirectGoBinding(goType.ImportPath, []string{goType.Name})
}

func (t GoValueType) String() string {
	if t.Expr != "" {
		return t.Expr
	}
	switch t.Kind {
	case GoValueBool:
		return "bool"
	case GoValueString:
		return "string"
	case GoValueInt:
		if t.Bits > 0 {
			return fmt.Sprintf("int%d", t.Bits)
		}
		return "int"
	case GoValueUint:
		if t.Bits > 0 {
			return fmt.Sprintf("uint%d", t.Bits)
		}
		return "uint"
	case GoValueFloat:
		if t.Bits == 32 {
			return "float32"
		}
		return "float64"
	case GoValueAny:
		return "any"
	case GoValueSlice:
		if t.Elem != nil {
			return "[]" + t.Elem.String()
		}
		return "[]?"
	case GoValueMap:
		if t.Key != nil && t.Value != nil {
			return "map[" + t.Key.String() + "]" + t.Value.String()
		}
		return "map[?]?"
	case GoValuePointer:
		if t.Elem != nil {
			return "*" + t.Elem.String()
		}
		return "*?"
	case GoValueError:
		return "error"
	default:
		return "?"
	}
}

func goSignature(typ types.Type) GoSignature {
	sig, ok := typ.(*types.Signature)
	if !ok || sig == nil {
		return GoSignature{}
	}
	out := GoSignature{Params: goTuple(sig.Params()), Results: goTuple(sig.Results()), Variadic: sig.Variadic()}
	if recv := sig.Recv(); recv != nil {
		receiver := goValueType(recv.Type())
		out.Receiver = &receiver
	}
	return out
}

func goTuple(tuple *types.Tuple) []GoValueType {
	if tuple == nil || tuple.Len() == 0 {
		return nil
	}
	out := make([]GoValueType, tuple.Len())
	for i := 0; i < tuple.Len(); i++ {
		out[i] = goValueType(tuple.At(i).Type())
	}
	return out
}

func goValueType(typ types.Type) GoValueType {
	return goValueTypeSeen(typ, map[types.Type]bool{})
}

func goValueTypeSeen(typ types.Type, seen map[types.Type]bool) GoValueType {
	if typ == nil {
		return GoValueType{Kind: GoValueInvalid}
	}
	if seen[typ] {
		return goOpaqueType(typ)
	}
	seen[typ] = true
	defer delete(seen, typ)

	out := GoValueType{Expr: goTypeExpr(typ), Kind: GoValueOther}
	if out.Expr == "error" {
		out.Kind = GoValueError
		return out
	}
	if named, ok := typ.(*types.Named); ok {
		underlying := goValueTypeSeen(named.Underlying(), seen)
		underlying.Expr = goTypeExpr(typ)
		underlying.Named = true
		underlying.Name = named.Obj().Name()
		if pkg := named.Obj().Pkg(); pkg != nil {
			underlying.ImportPath = pkg.Path()
			underlying.Package = pkg.Name()
		}
		return underlying
	}
	switch underlying := typ.Underlying().(type) {
	case *types.Basic:
		switch underlying.Kind() {
		case types.Bool:
			out.Kind = GoValueBool
		case types.String:
			out.Kind = GoValueString
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
			out.Kind = GoValueInt
			out.Bits = basicIntBits(underlying.Kind())
		case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			out.Kind = GoValueUint
			out.Bits = basicIntBits(underlying.Kind())
		case types.Float32:
			out.Kind = GoValueFloat
			out.Bits = 32
		case types.Float64:
			out.Kind = GoValueFloat
			out.Bits = 64
		}
	case *types.Pointer:
		elem := goValueTypeSeen(underlying.Elem(), seen)
		out.Kind = GoValuePointer
		out.Elem = &elem
	case *types.Slice:
		elem := goValueTypeSeen(underlying.Elem(), seen)
		out.Kind = GoValueSlice
		out.Elem = &elem
	case *types.Map:
		key := goValueTypeSeen(underlying.Key(), seen)
		value := goValueTypeSeen(underlying.Elem(), seen)
		out.Kind = GoValueMap
		out.Key = &key
		out.Value = &value
	case *types.Interface:
		if underlying.Empty() {
			out.Kind = GoValueAny
		}
	}
	return out
}

func goOpaqueType(typ types.Type) GoValueType {
	out := GoValueType{Expr: goTypeExpr(typ), Kind: GoValueOther}
	if named, ok := typ.(*types.Named); ok {
		out.Named = true
		out.Name = named.Obj().Name()
		if pkg := named.Obj().Pkg(); pkg != nil {
			out.ImportPath = pkg.Path()
			out.Package = pkg.Name()
		}
	}
	return out
}

func goTypeExpr(typ types.Type) string {
	if named, ok := typ.(*types.Named); ok && named.Obj() != nil {
		name := named.Obj().Name()
		if pkg := named.Obj().Pkg(); pkg != nil && pkg.Name() != "" {
			return pkg.Name() + "." + name
		}
		return name
	}
	return types.TypeString(typ, packageQualifier)
}

func packageQualifier(pkg *types.Package) string {
	if pkg == nil {
		return ""
	}
	return pkg.Name()
}

func basicIntBits(kind types.BasicKind) int {
	switch kind {
	case types.Int8, types.Uint8:
		return 8
	case types.Int16, types.Uint16:
		return 16
	case types.Int32, types.Uint32:
		return 32
	case types.Int64, types.Uint64:
		return 64
	default:
		return 0
	}
}

func exportedMethods(typ types.Type) map[string]GoMethod {
	methods := map[string]GoMethod{}
	addMethods := func(methodSet *types.MethodSet) {
		if methodSet == nil {
			return
		}
		for i := 0; i < methodSet.Len(); i++ {
			selection := methodSet.At(i)
			if selection == nil || selection.Obj() == nil || !selection.Obj().Exported() {
				continue
			}
			// A promoted method's signature keeps the embedded type as its receiver,
			// while Ard names the outer type in bindings like pkg::Outer::Method.
			// Skip promoted methods until direct Go FFI can model promotion explicitly.
			if len(selection.Index()) != 1 {
				continue
			}
			name := selection.Obj().Name()
			methods[name] = GoMethod{Name: name, Signature: goSignature(selection.Obj().Type())}
		}
	}
	addMethods(types.NewMethodSet(typ))
	addMethods(types.NewMethodSet(types.NewPointer(typ)))
	return methods
}
