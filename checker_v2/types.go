package checker_v2

type Type interface {
	String() string
	get(name string) Type
	equal(other Type) bool
}

type str struct{}

func (s str) String() string { return "Str" }
func (s str) get(name string) Type {
	switch name {
	case "size":
		return Int
	default:
		return nil
	}
}
func (s *str) equal(other Type) bool {
	return s == other
}

var Str = &str{}

type _int struct{}

func (i _int) String() string { return "Int" }
func (i _int) get(name string) Type {
	switch name {
	case "to_str":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Str,
		}
	default:
		return nil
	}
}
func (i *_int) equal(other Type) bool {
	return i == other
}

var Int = &_int{}

type float struct{}

func (f float) String() string { return "Float" }
func (f float) get(name string) Type {
	switch name {
	default:
		return nil
	}
}
func (f *float) equal(other Type) bool {
	return f == other
}

var Float = &float{}

type _bool struct{}

func (b _bool) String() string { return "Bool" }
func (b _bool) get(name string) Type {
	switch name {
	default:
		return nil
	}
}
func (b *_bool) equal(other Type) bool {
	return b == other
}

var Bool = &_bool{}

type void struct{}

func (v void) String() string         { return "Void" }
func (v void) get(name string) Type   { return nil }
func (v *void) equal(other Type) bool { return v == other }

var Void = &void{}

type List struct {
	of Type
}

func MakeList(of Type) *List {
	return &List{of}
}
func (l List) String() string {
	return "[" + l.of.String() + "]"
}
func (l List) get(name string) Type {
	switch name {
	case "size":
		return &FunctionDef{
			Name:       name,
			ReturnType: Int,
		}
	case "push":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "value", Type: l.of}},
			Mutates:    true,
			ReturnType: Int,
		}
	default:
		return nil
	}
}
func (l *List) equal(other Type) bool {
	if o, ok := other.(*List); ok {
		return o.of.equal(o.of)
	}

	return false
}
