package checker

// ForeignType is a named type owned by a foreign target. It is distinct from
// its underlying Ard representation. When Underlying is nil, the value is opaque
// to Ard and can only be stored or passed back across compatible foreign boundaries.
type ForeignType struct {
	Target             string
	Namespace          string
	Qualifier          string
	Name               string
	Underlying         Type
	Pointer            bool
	Struct             bool
	MapKey             Type
	MapValue           Type
	Fields             map[string]Type
	UnsupportedFields  map[string]string
	FieldsLoaded       bool
	Methods            map[string]*FunctionDef
	UnsupportedMethods map[string]string
	MethodsLoaded      bool
}

func (f *ForeignType) String() string {
	name := f.Name
	if f.Qualifier != "" {
		name = f.Qualifier + "::" + f.Name
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
	if !f.FieldsLoaded {
		f.Fields, f.UnsupportedFields = loadForeignTypeFields(f)
		f.FieldsLoaded = true
	}
	if field := f.Fields[name]; field != nil {
		return field
	}
	if !f.MethodsLoaded {
		f.Methods, f.UnsupportedMethods = loadForeignTypeMethods(f)
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
	return f.Target == o.Target && f.Namespace == o.Namespace && f.Name == o.Name && f.Pointer == o.Pointer
}

func (f *ForeignType) hasTrait(trait *Trait) bool { return false }
