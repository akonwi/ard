package checker

import "go/types"

// ForeignType is a named type owned by a foreign target. It is distinct from
// its underlying Ard representation. When Underlying is nil, the value is opaque
// to Ard and can only be stored or passed back across compatible foreign boundaries.
type ForeignType struct {
	Target                    string
	Namespace                 string
	Qualifier                 string
	Name                      string
	Underlying                Type
	Pointer                   bool
	Struct                    bool
	Interface                 bool
	GoType                    types.Type
	TypeArgs                  []Type
	MapKey                    Type
	MapValue                  Type
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
	return types.AssignableTo(actual.GoType, expected.GoType)
}
