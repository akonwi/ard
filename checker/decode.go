package checker

// Dynamic type for external/untyped data
type dynamicType struct{}

func (d dynamicType) String() string       { return "Dynamic" }
func (d dynamicType) get(name string) Type { return nil }
func (d dynamicType) equal(other Type) bool {
	_, ok := other.(*dynamicType)
	return ok
}
func (d dynamicType) hasTrait(trait *Trait) bool { return false }

var Dynamic = &dynamicType{}
