package checker

import (
	"fmt"
	"go/types"
)

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
	// ReadOnly marks a read-only `&pkg::T` pointer form (ADR 0073). It is only
	// set together with Pointer and is a checker-only restriction: writes and
	// pointer-receiver method calls through the pointer are rejected, while
	// the runtime representation is the ordinary Go pointer.
	ReadOnly  bool
	Struct    bool
	Interface bool
	GoType    types.Type
	TypeArgs  []Type
	MapKey    Type
	MapValue  Type
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

// ComparableTypeArgs identifies foreign generic parameters whose Go
// constraints require comparable arguments.
func (f *ForeignType) ComparableTypeArgs() []bool {
	if f == nil {
		return nil
	}
	goType := f.GoType
	if pointer, ok := goType.(*types.Pointer); ok {
		goType = pointer.Elem()
	}
	named, ok := goType.(*types.Named)
	if !ok {
		return nil
	}
	params := named.Origin().TypeParams()
	if params == nil {
		return nil
	}
	out := make([]bool, params.Len())
	for i := 0; i < params.Len(); i++ {
		if constraint, ok := params.At(i).Constraint().Underlying().(*types.Interface); ok {
			out[i] = constraint.Complete().IsComparable()
		}
	}
	return out
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
		if f.ReadOnly {
			return "&" + name
		}
		return "&mut " + name
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
	if arrayType, ok := f.Underlying.(*FixedArray); ok {
		if method := arrayType.get(name); method != nil {
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
	if f.Target != o.Target || f.Namespace != o.Namespace || f.Name != o.Name || f.Pointer != o.Pointer || f.ReadOnly != o.ReadOnly || len(f.TypeArgs) != len(o.TypeArgs) {
		return false
	}
	for i := range f.TypeArgs {
		if !f.TypeArgs[i].equal(o.TypeArgs[i]) {
			return false
		}
	}
	return true
}

// hasTrait reports Ard trait conformance for a foreign type. Ard impls cannot
// target foreign types, so only builtin Error applies: it is Go's predeclared
// `error`, which Go values satisfy under Go's own rules (#513).
func (f *ForeignType) hasTrait(trait *Trait) bool {
	return IsBuiltinError(trait) && f.implementsGoError()
}

// goErrorType is Go's predeclared `error` interface, which builtin Error
// represents.
var goErrorType = types.Universe.Lookup("error").Type()

// implementsGoError reports whether the Go value f represents is assignable to
// Go's `error`. A pointer form's Go type is `*T`, so Go's pointer method set
// applies, as it does when a `&T` or `&mut T` reaches any Go interface.
func (f *ForeignType) implementsGoError() bool {
	return f != nil && f.Target == "go" && f.GoType != nil && types.AssignableTo(f.GoType, goErrorType)
}

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

// readOnlyPointerForm returns the read-only form of a pointer-shaped foreign
// type (ADR 0073).
func (f *ForeignType) readOnlyPointerForm() *ForeignType {
	if f == nil || !f.Pointer {
		return nil
	}
	readOnly := *f
	readOnly.ReadOnly = true
	return &readOnly
}

// namedGoSliceElem returns the element type of a named Go slice value
// (`type Nums []int`), which behaves like an Ard list of that element.
func namedGoSliceElem(t Type) (Type, bool) {
	foreign, ok := t.(*ForeignType)
	if !ok || foreign.Pointer || foreign.Elem == nil {
		return nil, false
	}
	return foreign.Elem, true
}

// namedGoMapEntry returns the key and value types of a named Go map value
// (`type Header map[string][]string`), which behaves like an Ard map.
func namedGoMapEntry(t Type) (Type, Type, bool) {
	foreign, ok := t.(*ForeignType)
	if !ok || foreign.Pointer || foreign.MapKey == nil || foreign.MapValue == nil {
		return nil, nil, false
	}
	return foreign.MapKey, foreign.MapValue, true
}

func isPointerForeign(t Type) bool {
	foreign, ok := t.(*ForeignType)
	return ok && foreign.Pointer
}

// PointerForm returns the pointer-shaped form of a foreign named type, or nil
// when the type has no supported pointer form (interfaces, maps, already
// pointer-shaped values, or types without Go metadata).
func (f *ForeignType) PointerForm() *ForeignType {
	if f == nil || f.Pointer || f.Interface || f.Target != "go" {
		return nil
	}
	if f.GoType == nil {
		// A synthetic foreign type without go/types metadata (embedded or
		// test resolvers) takes a shallow pointer form so explicit
		// references still work.
		pointer := *f
		pointer.Pointer = true
		pointer.Methods = f.PointerMethods
		pointer.UnsupportedMethods = f.UnsupportedPointerMethods
		pointer.MethodsLoaded = pointer.Methods != nil || pointer.UnsupportedMethods != nil
		return &pointer
	}
	named, ok := f.GoType.(*types.Named)
	if !ok {
		return nil
	}
	if reason := unsupportedForeignNamedUnderlying(named.Underlying(), true); reason != "" {
		return nil
	}
	pointer, _ := foreignNamedTypeFromGo(named, true, false).(*ForeignType)
	if pointer != nil && len(f.TypeArgs) > 0 {
		pointer.TypeArgs = append([]Type(nil), f.TypeArgs...)
		pointer.Fields = f.Fields
		pointer.UnsupportedFields = f.UnsupportedFields
		pointer.FieldsLoaded = f.FieldsLoaded
	}
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
	named, ok := types.Unalias(pointer.Elem()).(*types.Named)
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
	// Both types ordinarily come from the resolver's single primed go/packages
	// session (ADR 0044), so object identity makes plain assignability enough.
	if types.AssignableTo(actual.GoType, expected.GoType) {
		return true
	}
	// Symbolic Ard arguments are represented by local placeholders while type
	// metadata is loaded. Re-instantiate both sides in one go/types context so
	// equal Ard declaration generics share identity and Go retains the complete
	// method ABI, including unexported methods and pointer/descriptor shapes.
	context := newCheckerGoTypeContext()
	collectForeignGoTypeConstraints(actual, context, map[*ForeignType]struct{}{})
	collectForeignGoTypeConstraints(expected, context, map[*ForeignType]struct{}{})
	actualGo, err := foreignGoTypeWithContext(actual, context)
	if err != nil {
		return false
	}
	expectedGo, err := foreignGoTypeWithContext(expected, context)
	if err != nil {
		return false
	}
	return types.AssignableTo(actualGo, expectedGo)
}

func foreignGoTypeWithContext(foreign *ForeignType, context *checkerGoTypeContext) (types.Type, error) {
	if foreign == nil || foreign.GoType == nil {
		return nil, fmt.Errorf("foreign type has no Go representation")
	}
	goType := foreign.GoType
	pointer := false
	if ptr, ok := goType.(*types.Pointer); ok {
		goType = ptr.Elem()
		pointer = true
	}
	named, ok := goType.(*types.Named)
	if !ok || len(foreign.TypeArgs) == 0 {
		return foreign.GoType, nil
	}
	params := named.Origin().TypeParams()
	if params == nil || params.Len() != len(foreign.TypeArgs) {
		return nil, fmt.Errorf("Go type argument count mismatch")
	}
	collectForeignGoTypeConstraints(foreign, context, map[*ForeignType]struct{}{})
	args := make([]types.Type, len(foreign.TypeArgs))
	for i, argument := range foreign.TypeArgs {
		converted, ok := checkerTypeToGoTypeWithContext(argument, context)
		if !ok {
			return nil, fmt.Errorf("type argument %s has no Go representation", argument)
		}
		args[i] = converted
	}
	instantiated, err := types.Instantiate(types.NewContext(), named.Origin(), args, true)
	if err != nil {
		return nil, err
	}
	if pointer {
		return types.NewPointer(instantiated), nil
	}
	return instantiated, nil
}

func collectForeignGoTypeConstraints(foreign *ForeignType, context *checkerGoTypeContext, seen map[*ForeignType]struct{}) {
	if foreign == nil || context == nil {
		return
	}
	if _, ok := seen[foreign]; ok {
		return
	}
	seen[foreign] = struct{}{}
	goType := foreign.GoType
	if pointer, ok := goType.(*types.Pointer); ok {
		goType = pointer.Elem()
	}
	named, _ := goType.(*types.Named)
	if named != nil {
		params := named.Origin().TypeParams()
		for i, argument := range foreign.TypeArgs {
			if params == nil || i >= params.Len() {
				break
			}
			typeVar, ok := derefType(argument).(*TypeVar)
			if !ok || typeVar.actual != nil || typeVar.owner != 0 {
				continue
			}
			constraint, ok := params.At(i).Constraint().Underlying().(*types.Interface)
			if ok && constraint.Complete().IsComparable() {
				context.comparableTypeVars[checkerTypeVarKey(typeVar)] = true
			}
		}
	}
	for _, argument := range foreign.TypeArgs {
		collectCheckerTypeGoConstraints(argument, context, seen)
	}
}

func collectCheckerTypeGoConstraints(t Type, context *checkerGoTypeContext, seen map[*ForeignType]struct{}) {
	switch typ := derefType(t).(type) {
	case *ForeignType:
		collectForeignGoTypeConstraints(typ, context, seen)
	case *MutableRef:
		collectCheckerTypeGoConstraints(typ.Of(), context, seen)
	case *List:
		collectCheckerTypeGoConstraints(typ.Of(), context, seen)
	case *Slice:
		collectCheckerTypeGoConstraints(typ.Of(), context, seen)
	case *FixedArray:
		collectCheckerTypeGoConstraints(typ.Of(), context, seen)
	case *Map:
		collectCheckerTypeGoConstraints(typ.Key(), context, seen)
		collectCheckerTypeGoConstraints(typ.Value(), context, seen)
	case *Maybe:
		collectCheckerTypeGoConstraints(typ.Of(), context, seen)
	case *Result:
		collectCheckerTypeGoConstraints(typ.Val(), context, seen)
		collectCheckerTypeGoConstraints(typ.Err(), context, seen)
	}
}
