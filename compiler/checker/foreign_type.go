package checker

// ForeignType is a named type owned by a foreign target. It is distinct from
// its underlying Ard representation, but literals may be contextually typed as
// this type when the underlying representation supports literal checking.
type ForeignType struct {
	Target     string
	Namespace  string
	Qualifier  string
	Name       string
	Underlying Type
}

func (f *ForeignType) String() string {
	if f.Qualifier != "" {
		return f.Qualifier + "::" + f.Name
	}
	return f.Name
}

func (f *ForeignType) get(name string) Type { return nil }

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
	return f.Target == o.Target && f.Namespace == o.Namespace && f.Name == o.Name
}

func (f *ForeignType) hasTrait(trait *Trait) bool { return false }
