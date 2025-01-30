package checker

import (
	"fmt"
	"strings"
)

// A static type, must be printable, have properties, and be comparable
type Type interface {
	String() string
	GetProperty(name string) Type
	matches(other Type) bool
}

type Static interface {
	GetStaticProperty(name string) Type
}

/*
check if a and b are coherent, i.e. they are the same type or one can be used as the other
*/
func AreCoherent(a, b Type) bool {
	if a == nil || b == nil {
		return false
	}
	if a.String() == b.String() {
		return true
	}

	if aOption, ok := a.(Option); ok {
		return aOption.matches(b)
	}

	if aUnion, ok := a.(Union); ok {
		return aUnion.matches(b)
	}

	return false
}

func IsVoid(t Type) bool {
	if t == nil {
		return true
	}
	_, ok := t.(Void)
	return ok
}

type Void struct{}

func (v Void) String() string {
	return "Void"
}
func (v Void) GetProperty(name string) Type {
	return nil
}
func (v Void) matches(other Type) bool {
	return AreCoherent(v, other)
}

type Str struct{}

func (s Str) String() string {
	return "Str"
}
func (s Str) GetProperty(name string) Type {
	switch name {
	case "size":
		return Num{}
	case "is_empty":
		return Bool{}
	default:
		return nil
	}
}
func (s Str) matches(other Type) bool {
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
func (n Num) matches(other Type) bool {
	return n.String() == other.String()
}
func (n Num) GetStaticProperty(name string) Type {
	switch name {
	case "from_str":
		return function{
			name:       "from_str",
			parameters: []variable{{name: "str", _type: Str{}}},
			returns:    Option{Num{}},
		}
	default:
		return nil
	}
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
func (b Bool) matches(other Type) bool {
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
func (f function) matches(other Type) bool {
	return f.String() == other.String()
}

type List struct {
	element Type
}

func (l List) GetElementType() Type {
	return l.element
}

func (l List) String() string {
	if l.element == nil {
		return "[?]"
	}
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
	case "at":
		return function{
			name:       "at",
			parameters: []variable{{name: "index", _type: Num{}}},
			returns:    Option{l.element},
		}
	case "set":
		return function{
			name: name,
			parameters: []variable{
				{name: "index", _type: Num{}},
				{name: "value", _type: l.element},
			},
			returns: Bool{},
		}
	default:
		return nil
	}
}

func (l List) matches(other Type) bool {
	if otherList, ok := other.(List); ok {
		// if either list is still open, then they are compatible
		if l.element == nil || otherList.element == nil {
			return true
		}
		return l.element.matches(otherList.element)
	}
	return false
}

type Map struct {
	key   Type
	value Type
}

func (m Map) GetTypes() (Type, Type) {
	return m.key, m.value
}

func (m Map) String() string {
	return fmt.Sprintf("[%s:%s]", m.key, m.value)
}

func (m Map) GetProperty(name string) Type {
	switch name {
	case "size":
		return Num{}
	case "set":
		return function{
			name: name,
			parameters: []variable{
				{name: "key", _type: m.key},
				{name: "val", _type: m.value},
			},
			returns: Void{},
		}
	case "get":
		return function{
			name:       name,
			parameters: []variable{{name: "key", _type: m.key}},
			returns:    Option{m.value},
		}
	case "drop":
		return function{
			name:       name,
			parameters: []variable{{name: "key", _type: m.key}},
			returns:    Void{},
		}
	case "has":
		return function{
			name:       name,
			parameters: []variable{{name: "key", _type: m.key}},
			returns:    Bool{},
		}
	default:
		return nil
	}
}

func (m Map) matches(other Type) bool {
	if otherMap, ok := other.(Map); ok {
		return m.key.matches(otherMap.key) && m.value.matches(otherMap.value)
	}
	return false
}

type Enum struct {
	Name     string
	Variants []string
}

// impl Type interface
func (e Enum) String() string {
	return e.Name
}
func (e Enum) matches(other Type) bool {
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

func (e Enum) GetStaticProperty(name string) Type {
	if _, ok := e.GetVariant(name); ok {
		return e
	}
	return nil
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
	Name     string
	Fields   map[string]Type
	methods  map[string]FunctionDeclaration
	selfName string
}

// impl Type interface
func (s *Struct) String() string {
	return s.Name
}

func (s Struct) GetProperty(name string) Type {
	if field, ok := s.Fields[name]; ok {
		return field
	}
	if method := s.methods[name]; method.Name != "" {
		return method.GetType()
	}
	return nil
}
func (s *Struct) addMethod(id string, method FunctionDeclaration) {
	s.selfName = id
	s.methods[method.Name] = method
}
func (s Struct) GetMethod(name string) (FunctionDeclaration, bool) {
	method, ok := s.methods[name]
	return method, ok
}
func (s Struct) GetInstanceId() string {
	return s.selfName
}

func (s *Struct) matches(other Type) bool {
	otherStruct, ok := other.(*Struct)
	if !ok {
		return false
	}
	if s.Name != otherStruct.Name {
		return false
	}
	if len(s.Fields) != len(otherStruct.Fields) {
		return false
	}
	for field, fieldType := range s.Fields {
		if otherField, ok := otherStruct.Fields[field]; !ok || !fieldType.matches(otherField) {
			return false
		}
	}
	return true
}

func (s Struct) GetName() string {
	return s.String()
}
func (s Struct) GetType() Type {
	return &s
}
func (s Struct) asFunction() (function, bool) {
	return function{}, false
}

type Option struct {
	inner Type
}

func MakeOption(inner Type) Option {
	return Option{inner: inner}
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
func (g Option) matches(other Type) bool {
	if otherOption, ok := other.(Option); ok {
		if g.inner == nil || otherOption.inner == nil {
			return true
		}
		return g.inner.matches(otherOption.inner)
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

type Union struct {
	name  string
	types []Type
}

func (u Union) GetName() string {
	return u.name
}
func (u Union) GetType() Type {
	return u
}
func (u Union) asFunction() (function, bool) {
	return function{}, false
}
func (u Union) String() string {
	types := make([]string, len(u.types))
	for i, t := range u.types {
		types[i] = t.String()
	}
	return fmt.Sprintf("%s", strings.Join(types, "|"))
}
func (u Union) GetProperty(name string) Type {
	return nil
}
func (u Union) matches(other Type) bool {
	if otherUnion, ok := other.(Union); ok {
		if len(u.types) != len(otherUnion.types) {
			return false
		}
		for i, t := range u.types {
			if !t.matches(otherUnion.types[i]) {
				return false
			}
		}
		return true
	}
	for _, t := range u.types {
		if t.matches(other) {
			return true
		}
	}
	return false
}
func (u Union) getFor(string string) Type {
	for _, t := range u.types {
		if t.String() == string {
			return t
		}
	}
	return nil
}

func areComparable(a, b Type) bool {
	if a.matches(Num{}) || a.matches(Str{}) || a.matches(Bool{}) {
		return a.matches(b)
	}
	if a.matches(Option{}) {
		return a.matches(b) || a.(Option).inner.matches(b)
	}

	return false
}
