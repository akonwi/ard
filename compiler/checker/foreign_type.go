package checker

import "go/types"

// ForeignType is a named type owned by a foreign target. It is distinct from
// its underlying Ard representation. When Underlying is nil, the value is opaque
// to Ard and can only be stored or passed back across compatible foreign boundaries.
type ForeignType struct {
	Target     string
	Namespace  string
	Qualifier  string
	Name       string
	Underlying Type
	Pointer    bool
	Struct     bool
	Interface  bool
	GoType     types.Type
	TypeArgs   []Type
	MapKey     Type
	MapValue   Type
	// Elem is set for named Go slice types (`type Nums []int`); the foreign
	// value then behaves like an Ard list of Elem.
	Elem                      Type
	Fields                    map[string]Type
	UnsupportedFields         map[string]string
	FieldsLoaded              bool
	Methods                   map[string]*FunctionDef
	UnsupportedMethods        map[string]string
	PointerMethods            map[string]*FunctionDef
	UnsupportedPointerMethods map[string]string
	MethodsLoaded             bool
	LoadFields                func() (map[string]Type, map[string]string)
	LoadMethods               func(pointer bool) (map[string]*FunctionDef, map[string]string)
}

func (f *ForeignType) String() string {
	name := f.Name
	if f.Qualifier != "" {
		name = f.Qualifier + "::" + f.Name
	}
	if len(f.TypeArgs) > 0 {
		name += "<"
		for i, arg := range f.TypeArgs {
			if i > 0 {
				name += ", "
			}
			name += arg.String()
		}
		name += ">"
	}
	if f.Pointer {
		return "mut " + name
	}
	return name
}

func (f *ForeignType) get(name string) Type {
	if f.MapKey != nil && f.MapValue != nil {
		if method := MakeMap(f.MapKey, f.MapValue).get(name); method != nil {
			return method
		}
	}
	if f.Elem != nil {
		if method := MakeList(f.Elem).get(name); method != nil {
			return method
		}
	}
	if !f.FieldsLoaded && f.LoadFields != nil {
		f.Fields, f.UnsupportedFields = f.LoadFields()
		f.FieldsLoaded = true
	}
	if field := f.Fields[name]; field != nil {
		return field
	}
	if !f.MethodsLoaded && f.LoadMethods != nil {
		f.Methods, f.UnsupportedMethods = f.LoadMethods(f.Pointer)
		if !f.Pointer {
			f.PointerMethods, f.UnsupportedPointerMethods = f.LoadMethods(true)
		}
		f.MethodsLoaded = true
	}
	method := f.Methods[name]
	if method == nil {
		return nil
	}
	return method
}

func (f *ForeignType) equal(other Type) bool {
	if f == other {
		return true
	}
	o, ok := other.(*ForeignType)
	if !ok {
		if typeVar, ok := other.(*TypeVar); ok && typeVar.actual == nil {
			return true
		}
		return false
	}
	if f.Target != o.Target || f.Namespace != o.Namespace || f.Name != o.Name || f.Pointer != o.Pointer || len(f.TypeArgs) != len(o.TypeArgs) {
		return false
	}
	for i := range f.TypeArgs {
		if !f.TypeArgs[i].equal(o.TypeArgs[i]) {
			return false
		}
	}
	return true
}

func (f *ForeignType) hasTrait(trait *Trait) bool { return false }

// EmptyInterface reports whether f is a Go interface type with an empty
// method set (for example `type Event interface{}`). Any value is assignable
// to it, matching Go's own assignability.
func (f *ForeignType) EmptyInterface() bool {
	if !f.Interface || f.GoType == nil {
		return false
	}
	iface, ok := f.GoType.Underlying().(*types.Interface)
	return ok && iface.Empty()
}

func isPointerForeign(t Type) bool {
	foreign, ok := t.(*ForeignType)
	return ok && foreign.Pointer
}

// PointerForm returns the pointer-shaped form of a foreign named type, or nil
// when the type has no supported pointer form (interfaces, maps, already
// pointer-shaped values, or types without Go metadata).
func (f *ForeignType) PointerForm() *ForeignType {
	if f == nil || f.Pointer || f.Interface || f.Target != "go" || f.GoType == nil {
		return nil
	}
	named, ok := f.GoType.(*types.Named)
	if !ok {
		return nil
	}
	if reason := unsupportedForeignNamedUnderlying(named.Underlying(), true); reason != "" {
		return nil
	}
	pointer, _ := foreignNamedTypeFromGo(named, true, false).(*ForeignType)
	return pointer
}

// ValueForm returns the value-shaped form of a pointer foreign named type, or
// nil when the receiver is not a pointer form backed by Go metadata.
func (f *ForeignType) ValueForm() *ForeignType {
	if f == nil || !f.Pointer || f.GoType == nil {
		return nil
	}
	pointer, ok := f.GoType.(*types.Pointer)
	if !ok {
		return nil
	}
	named, ok := pointer.Elem().(*types.Named)
	if !ok {
		return nil
	}
	value, _ := foreignNamedTypeFromGo(named, false, false).(*ForeignType)
	return value
}

func foreignGoAssignableTo(actual *ForeignType, expected *ForeignType) bool {
	if actual == nil || expected == nil || actual.Target != "go" || expected.Target != "go" || actual.GoType == nil || expected.GoType == nil {
		return false
	}
	return goAssignableAcrossUniverses(actual.GoType, expected.GoType)
}

// goAssignableAcrossUniverses reports Go assignability between types that may
// originate from different packages.Load calls. Each load builds its own
// go/types object universe, so the same named type (for example
// http.ResponseWriter) has a distinct identity per load and
// types.AssignableTo alone reports false for types that Go itself accepts.
//
// All loads run with the same project root and build configuration, so an
// import path always denotes a single declaration. That makes it sound to
// canonicalize one side into the other's universe by re-resolving named types
// through (package path, type name) and re-checking assignability within a
// single universe.
func goAssignableAcrossUniverses(actual, expected types.Type) bool {
	if types.AssignableTo(actual, expected) {
		return true
	}
	if universe := goTypeUniverse(actual); universe != nil {
		if translated, ok := translateGoType(expected, universe); ok && types.AssignableTo(actual, translated) {
			return true
		}
	}
	if universe := goTypeUniverse(expected); universe != nil {
		if translated, ok := translateGoType(actual, universe); ok && types.AssignableTo(translated, expected) {
			return true
		}
	}
	return false
}

// goTypeUniverse returns the package whose import graph anchors the given
// type's universe: the declaring package of the principal named type, seen
// through aliases and composite shells. It returns nil when the type has no
// named anchor (predeclared and fully unnamed types), which is fine because
// such types are universe-independent.
func goTypeUniverse(t types.Type) *types.Package {
	switch t := types.Unalias(t).(type) {
	case *types.Named:
		if obj := t.Obj(); obj != nil {
			return obj.Pkg()
		}
	case *types.Pointer:
		return goTypeUniverse(t.Elem())
	case *types.Slice:
		return goTypeUniverse(t.Elem())
	case *types.Array:
		return goTypeUniverse(t.Elem())
	case *types.Chan:
		return goTypeUniverse(t.Elem())
	case *types.Map:
		if universe := goTypeUniverse(t.Key()); universe != nil {
			return universe
		}
		return goTypeUniverse(t.Elem())
	}
	return nil
}

// translateGoType rebuilds t with every named type re-resolved inside the
// target universe's import graph. It returns false when any named type's
// package or declaration cannot be found there, or when the type contains a
// form translation does not support; callers must treat that as "unknown",
// not as assignable.
func translateGoType(t types.Type, universe *types.Package) (types.Type, bool) {
	switch t := types.Unalias(t).(type) {
	case *types.Basic:
		return t, true
	case *types.Named:
		return translateGoNamed(t, universe)
	case *types.Pointer:
		elem, ok := translateGoType(t.Elem(), universe)
		if !ok {
			return nil, false
		}
		return types.NewPointer(elem), true
	case *types.Slice:
		elem, ok := translateGoType(t.Elem(), universe)
		if !ok {
			return nil, false
		}
		return types.NewSlice(elem), true
	case *types.Array:
		elem, ok := translateGoType(t.Elem(), universe)
		if !ok {
			return nil, false
		}
		return types.NewArray(elem, t.Len()), true
	case *types.Map:
		key, ok := translateGoType(t.Key(), universe)
		if !ok {
			return nil, false
		}
		value, ok := translateGoType(t.Elem(), universe)
		if !ok {
			return nil, false
		}
		return types.NewMap(key, value), true
	case *types.Chan:
		elem, ok := translateGoType(t.Elem(), universe)
		if !ok {
			return nil, false
		}
		return types.NewChan(t.Dir(), elem), true
	case *types.Signature:
		return translateGoSignature(t, universe)
	case *types.Struct:
		return translateGoStruct(t, universe)
	case *types.Interface:
		return translateGoInterface(t, universe)
	}
	return nil, false
}

func translateGoNamed(named *types.Named, universe *types.Package) (types.Type, bool) {
	obj := named.Obj()
	if obj == nil {
		return nil, false
	}
	// Predeclared named types such as error live in the shared universe scope
	// and have no declaring package; they are identical across loads.
	if obj.Pkg() == nil {
		return named, true
	}
	pkg := findGoPackage(universe, obj.Pkg().Path())
	if pkg == nil {
		return nil, false
	}
	typeName, ok := pkg.Scope().Lookup(obj.Name()).(*types.TypeName)
	if !ok {
		return nil, false
	}
	resolved, ok := types.Unalias(typeName.Type()).(*types.Named)
	if !ok {
		return nil, false
	}
	args := named.TypeArgs()
	if args == nil || args.Len() == 0 {
		return resolved, true
	}
	translatedArgs := make([]types.Type, args.Len())
	for i := 0; i < args.Len(); i++ {
		arg, ok := translateGoType(args.At(i), universe)
		if !ok {
			return nil, false
		}
		translatedArgs[i] = arg
	}
	instantiated, err := types.Instantiate(nil, resolved, translatedArgs, false)
	if err != nil {
		return nil, false
	}
	return instantiated, true
}

func translateGoSignature(sig *types.Signature, universe *types.Package) (types.Type, bool) {
	if sig.TypeParams().Len() > 0 || sig.RecvTypeParams().Len() > 0 {
		return nil, false
	}
	params, ok := translateGoTuple(sig.Params(), universe)
	if !ok {
		return nil, false
	}
	results, ok := translateGoTuple(sig.Results(), universe)
	if !ok {
		return nil, false
	}
	return types.NewSignatureType(nil, nil, nil, params, results, sig.Variadic()), true
}

func translateGoTuple(tuple *types.Tuple, universe *types.Package) (*types.Tuple, bool) {
	if tuple == nil {
		return nil, true
	}
	vars := make([]*types.Var, tuple.Len())
	for i := 0; i < tuple.Len(); i++ {
		v := tuple.At(i)
		translated, ok := translateGoType(v.Type(), universe)
		if !ok {
			return nil, false
		}
		// Preserve the declaring package: go/types matches unexported
		// names by package path, so dropping it would break identity.
		vars[i] = types.NewVar(v.Pos(), v.Pkg(), v.Name(), translated)
	}
	return types.NewTuple(vars...), true
}

func translateGoStruct(structType *types.Struct, universe *types.Package) (types.Type, bool) {
	fields := make([]*types.Var, structType.NumFields())
	tags := make([]string, structType.NumFields())
	for i := 0; i < structType.NumFields(); i++ {
		field := structType.Field(i)
		translated, ok := translateGoType(field.Type(), universe)
		if !ok {
			return nil, false
		}
		// Preserve the declaring package: go/types matches unexported
		// field names by package path, so dropping it would break identity.
		fields[i] = types.NewField(field.Pos(), field.Pkg(), field.Name(), translated, field.Embedded())
		tags[i] = structType.Tag(i)
	}
	return types.NewStruct(fields, tags), true
}

func translateGoInterface(iface *types.Interface, universe *types.Package) (types.Type, bool) {
	methods := make([]*types.Func, iface.NumExplicitMethods())
	for i := 0; i < iface.NumExplicitMethods(); i++ {
		method := iface.ExplicitMethod(i)
		sig, ok := translateGoType(method.Signature(), universe)
		if !ok {
			return nil, false
		}
		// Preserve the declaring package: go/types matches unexported
		// method names by package path, so dropping it would prevent
		// sealed interfaces from ever being satisfied after translation.
		methods[i] = types.NewFunc(method.Pos(), method.Pkg(), method.Name(), sig.(*types.Signature))
	}
	embeddeds := make([]types.Type, iface.NumEmbeddeds())
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		translated, ok := translateGoType(iface.EmbeddedType(i), universe)
		if !ok {
			return nil, false
		}
		embeddeds[i] = translated
	}
	translated := types.NewInterfaceType(methods, embeddeds)
	translated.Complete()
	return translated, true
}

// findGoPackage locates a package by import path within a universe's import
// graph, including the root itself. Identity is by pointer because distinct
// universes intentionally contain distinct *types.Package values for the
// same path.
func findGoPackage(root *types.Package, path string) *types.Package {
	if root == nil {
		return nil
	}
	seen := map[*types.Package]bool{}
	queue := []*types.Package{root}
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		if seen[pkg] {
			continue
		}
		seen[pkg] = true
		if pkg.Path() == path {
			return pkg
		}
		queue = append(queue, pkg.Imports()...)
	}
	return nil
}
