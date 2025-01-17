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

func makeList(element Type) List {
	return List{element: element}
}

type Enum struct {
	Name     string
	Variants []string
}

// impl Type interface
func (e Enum) String() string {
	return e.Name
}
func (e Enum) Is(other Type) bool {
	if otherEnum, isEnum := other.(Enum); isEnum {
		if len(e.Variants) != len(otherEnum.Variants) {
			return false
		}

		for i, v := range otherEnum.Variants {
			if e.Variants[i] != v {
				return false
			}
		}
		return true
	}
	return false
}
func (e Enum) GetProperty(name string) Type {
	return nil
}

func (e Enum) GetVariant(name string) (EnumVariant, bool) {
	for i, v := range e.Variants {
		if v == name {
			return EnumVariant{enum: &e, Enum: e.Name, Variant: name, Value: i}, true
		}
	}
	return EnumVariant{}, false
}

// impl symbol interface
func (e Enum) GetName() string {
	return e.Name
}
func (e Enum) asFunction() (function, bool) {
	return function{}, false
}

type EnumVariant struct {
	enum    *Enum
	Enum    string
	Variant string
	Value   int
}

func (e EnumVariant) String() string {
	return e.Enum + "::" + e.Variant
}

// impl Expression interface
func (e EnumVariant) GetType() Type {
	return *e.enum
}

type Struct struct {
	Name   string
	Fields map[string]Type
}

// impl Type interface
func (s Struct) String() string {
	return s.Name
}

func (s Struct) GetProperty(name string) Type {
	return s.Fields[name]
}
func (s Struct) Is(other Type) bool {
	otherStruct, ok := other.(Struct)
	if !ok {
		return false
	}
	if s.Name != otherStruct.Name {
		return false
	}
	if len(s.Fields) != len(otherStruct.Fields) {
		return false
	}
	for k, v := range s.Fields {
		if ov, ok := otherStruct.Fields[k]; !ok || !v.Is(ov) {
			return false
		}
	}
	return true
}

func (s Struct) GetName() string {
	return s.String()
}
func (s Struct) GetType() Type {
	return s
}
func (s Struct) asFunction() (function, bool) {
	return function{}, false
}

type Tuple struct {
	elements []Type
}

func (l Tuple) String() string {
	elements := make([]string, len(l.elements))
	for i, it := range l.elements {
		elements[i] = it.String()
	}
	return fmt.Sprintf("[%s]", strings.Join(elements, ","))
}

func (l Tuple) GetProperty(name string) Type {
	switch name {
	case "$0":
		return l.elements[0]
	case "$1":
		return l.elements[1]
	case "$2":
		return l.elements[2]
	default:
		return nil
	}
}

func (l Tuple) Is(other Type) bool {
	return l.String() == other.String()
}

type Option struct {
	inner Type
}

func (g Option) GetInnerType() Type {
	return g.inner
}
func (g Option) String() string {
	if g.inner == nil {
		return "??"
	}
	return g.inner.String() + "?"
}
func (g Option) Is(other Type) bool {
	if otherOption, ok := other.(Option); ok {
		if g.inner == nil || otherOption.inner == nil {
			return true
		}
		return g.inner.Is(otherOption.inner)
	}
	return false
}
func (g Option) GetProperty(name string) Type {
	switch name {
	case "some":
		return function{
			name: "some",
			parameters: []variable{
				{name: "value", _type: g.inner},
			},
			returns: Void{},
		}
	case "none":
		return function{
			name:       name,
			parameters: []variable{},
			returns:    Void{},
		}
	default:
		return nil
	}
}
