package checker

import "reflect"

// A static type, must be printable, have properties, and be comparable
type Type interface {
	String() string
	GetProperty(name string) Type
	Is(other Type) bool
}

func areSameType(a, b Type) bool {
	return reflect.TypeOf(a) == reflect.TypeOf(b)
}

type Str struct{}

func (s Str) String() string {
	return "Str"
}
func (s Str) GetProperty(name string) Type {
	switch name {
	case "size":
		return Num{}
	default:
		return nil
	}
}
func (s Str) Is(other Type) bool {
	return s.String() == other.String()
}

type Num struct{}

func (n Num) String() string {
	return "Num"
}
func (n Num) GetProperty(name string) Type {
	return nil
}
func (n Num) Is(other Type) bool {
	return n.String() == other.String()
}

type Bool struct{}

func (b Bool) String() string {
	return "Bool"
}
func (b Bool) GetProperty(name string) Type {
	return nil
}
func (b Bool) Is(other Type) bool {
	return b.String() == other.String()
}