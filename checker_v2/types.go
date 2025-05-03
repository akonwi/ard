package checker_v2

import "fmt"

type Type interface {
	String() string
	get(name string) Type

	/* A.K.A 'compatible()'
	  The Ard type system only allows generics in parameters.
		This means equal is called as `expected.equal(actual)`,
		where `expected` is the declared parameter type and `actual` is the provided dynamic type.
		The exception is when resolving generics in a function call based on inferred types.
	 	In this scenario, the generic is the `other` argument, so that the callee type can fill in the resolved type.
	*/
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
	// coerce other if it's an open Any
	// todo: implement in other Types
	if o, ok := other.(*Any); ok {
		if o.actual == nil {
			o.actual = s
			return true
		}
		return s == o.actual
	}
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
	case "at":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "index", Type: Int}},
			ReturnType: &Maybe{l.of},
		}
	case "push":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "value", Type: l.of}},
			Mutates:    true,
			ReturnType: Int,
		}
	case "set":
		return &FunctionDef{
			Name: name,
			Parameters: []Parameter{
				{Name: "index", Type: Int},
				{Name: "value", Type: l.of},
			},
			Mutates:    true,
			ReturnType: Bool,
		}
	case "size":
		return &FunctionDef{
			Name:       name,
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

type Map struct {
	key   Type
	value Type
}

func MakeMap(key, value Type) *Map {
	return &Map{key, value}
}

func (m Map) String() string {
	return fmt.Sprintf("[%s:%s]", m.key.String(), m.value.String())
}
func (m Map) equal(other Type) bool {
	if o, ok := other.(*Map); ok {
		return m.key.equal(o.key) && m.value.equal(o.value)
	}

	return false
}
func (m Map) get(name string) Type {
	switch name {
	case "get":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "key", Type: m.key}},
			ReturnType: &Maybe{m.value},
		}
	case "set":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "key", Type: m.key}, {Name: "value", Type: m.value}},
			Mutates:    true,
			ReturnType: Bool,
		}
	case "drop":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "key", Type: m.key}},
			Mutates:    true,
			ReturnType: Void,
		}
	case "has":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "key", Type: m.key}},
			ReturnType: Bool,
		}
	case "size":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Int,
		}
	default:
		return nil
	}
}

func (m *Map) Value() Type {
	return m.value
}

type Maybe struct {
	of Type
}

func (m *Maybe) String() string {
	return m.of.String() + "?"
}
func (m *Maybe) get(name string) Type {
	switch name {
	case "or":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "default", Type: m.of}},
			ReturnType: m.of,
		}
	default:
		return nil
	}
}
func (m *Maybe) equal(other Type) bool {
	if o, ok := other.(*Maybe); ok {
		return m.of.equal(o.of)
	}

	return false
}
func (m *Maybe) Of() Type {
	return m.of
}

type Any struct {
	name   string
	actual Type
}

func (a Any) String() string { return "$" + a.name }
func (a Any) get(name string) Type {
	if a.actual != nil {
		return a.actual.get(name)
	}
	panic(fmt.Errorf("Cannot look up symbols in unrefined %s", a.String()))
}
func (a *Any) equal(other Type) bool {
	if a.actual == nil {
		a.actual = other
		return true
	}
	return a.actual.equal(other)
}
