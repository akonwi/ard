package checker

import (
	"fmt"
	"reflect"
	"strings"
)

// A static type, must be printable, have properties, and be comparable
type Type interface {
	String() string
	GetProperty(name string) Type
	Is(other Type) bool
}

func areSameType(a, b Type) bool {
	return reflect.TypeOf(a) == reflect.TypeOf(b)
}

type Void struct{}

func (v Void) String() string {
	return "Void"
}
func (v Void) GetProperty(name string) Type {
	return nil
}
func (v Void) Is(other Type) bool {
	return v.String() == other.String()
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
	switch name {
	case "as_str":
		return Str{}
	default:
		return nil
	}
}
func (n Num) Is(other Type) bool {
	return n.String() == other.String()
}

type Bool struct{}

func (b Bool) String() string {
	return "Bool"
}
func (b Bool) GetProperty(name string) Type {
	switch name {
	case "as_str":
		return Str{}
	default:
		return nil
	}
}
func (b Bool) Is(other Type) bool {
	return b.String() == other.String()
}

// also doubles as a symbol in scope
type function struct {
	name       string
	parameters []variable
	returns    Type
}

func (f function) String() string {
	params := make([]string, len(f.parameters))
	for i, p := range f.parameters {
		params[i] = p.GetName()
	}
	return f.name + "(" + strings.Join(params, ",") + ") " + f.returns.String()
}
func (f function) GetProperty(name string) Type {
	return nil
}
func (f function) Is(other Type) bool {
	return f.String() == other.String()
}

type List struct {
	element Type
}

func (l List) String() string {
	return fmt.Sprintf("[%s]", l.element)
}

func (l List) GetProperty(name string) Type {
	switch name {
	case "size":
		return Num{}
	case "push":
		return function{
			name:       "push",
			parameters: []variable{{name: "item", _type: l.element}},
			returns:    Num{},
		}
	default:
		return nil
	}
}

func (l List) Is(other Type) bool {
	if otherList, ok := other.(List); ok {
		// if either list is still open, then they are compatible
		if l.element == nil || otherList.element == nil {
			return true
		}
		return l.element.Is(otherList.element)
	}
	return false
}
