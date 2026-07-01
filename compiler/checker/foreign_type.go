package checker

// ForeignType is a named type owned by a foreign target. It is distinct from
// its underlying Ard representation. When Underlying is nil, the value is opaque
// to Ard and can only be stored or passed back across compatible foreign boundaries.
type ForeignType struct {
	Target        string
	Namespace     string
	Qualifier     string
	Name          string
	Underlying    Type
	Pointer       bool
	Methods       map[string]*FunctionDef
	MethodsLoaded bool
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
	if !f.MethodsLoaded {
		f.Methods = loadForeignTypeMethods(f)
		f.MethodsLoaded = true
	}
	return f.Methods[name]
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
