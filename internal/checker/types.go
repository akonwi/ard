package checker

import (
	"fmt"
	"strings"
)

// A static type, must be printable, have properties
type Type interface {
	String() string
	GetProperty(name string) Type
}

type Static interface {
	GetStaticProperty(name string) Type
}

/*
check if a and b are coherent, i.e. they are the same type or one can be used as the other
*/
func AreCoherent(a, b Type) bool {
	if a == nil && b == nil {
		return true
	}

	if aOption, ok := a.(Option); ok {
		if bOption, ok := b.(Option); ok {
			if aOption.inner == nil || bOption.inner == nil {
				return true
			}
			return AreCoherent(aOption.inner, bOption.inner)
		}
		return false
	}

	if aAny, ok := a.(Any); ok {
		return aAny.refine(b)
	}
	if bAny, ok := b.(Any); ok {
		return bAny.refine(a)
	}

	if a.String() == b.String() {
		return true
	}

	if aList, ok := a.(List); ok {
		if bList, ok := b.(List); ok {
			return AreCoherent(aList.element, bList.element)
		}
		return false
	}

	if aMap, ok := a.(Map); ok {
		if bMap, ok := b.(Map); ok {
			return AreCoherent(aMap.key, bMap.key) && AreCoherent(aMap.value, bMap.value)
		}
		return false
	}

	if aUnion, ok := a.(Union); ok {
		if bUnion, ok := b.(Union); ok {
			if len(aUnion.types) != len(bUnion.types) {
				return false
			}
			for i, t := range aUnion.types {
				if !AreCoherent(aUnion.types[i], t) {
					return false
				}
			}
			return true
		}
		return aUnion.allows(b)
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

type Str struct{}

func (s Str) String() string {
	return "Str"
}
func (s Str) GetProperty(name string) Type {
	switch name {
	case "size":
		return function{
			name:    name,
			returns: Num{},
		}
	case "is_empty":
		return Bool{}
	default:
		return nil
	}
}

type Num struct{}

func (n Num) String() string {
	return "Num"
}
func (n Num) GetProperty(name string) Type {
	switch name {
	case "to_str":
		return function{
			name:    name,
			returns: Str{},
		}
	default:
		return nil
	}
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
	case "to_str":
		return function{
			name:    name,
			returns: Str{},
		}
	default:
		return nil
	}
}

// also doubles as a symbol in scope
type function struct {
	name       string
	parameters []variable
	returns    Type
	mutates    bool
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
			mutates:    true,
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
			name:    name,
			mutates: true,
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
			name:    name,
			mutates: true,
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
			mutates:    true,
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

type Enum struct {
	Name     string
	Variants []string
}

// impl Type interface
func (e Enum) String() string {
	return e.Name
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

func (g Option) GetProperty(name string) Type {
	switch name {
	case "or":
		return function{
			name:       name,
			parameters: []variable{{name: "default", _type: g.inner}},
			returns:    g.inner,
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
func (u Union) allows(other Type) bool {
	for _, t := range u.types {
		if AreCoherent(t, other) {
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

/*
Any is like a wildcard that initially is coherent with any type.
Once it passes coherence with another concrete type, it becomes that type.
*/
type Any struct {
	inner Type
}

func (a Any) String() string {
	if a.inner == nil {
		return "Any"
	}
	return a.inner.String()
}

func (a Any) GetProperty(name string) Type {
	if a.inner == nil {
		return nil
	}
	return a.GetProperty(name)
}

func (a *Any) refine(t Type) bool {
	if a.inner != nil {
		return AreCoherent(a.inner, t)
	}
	if t == nil {
		return true
	}
	a.inner = t
	return true
}

func areComparable(a, b Type) bool {
	_, aIsNum := a.(Num)
	_, aIsStr := a.(Str)
	_, aIsBool := a.(Bool)
	if aIsBool || aIsNum || aIsStr {
		return AreCoherent(a, b)
	}

	_, aIsOption := a.(Option)
	if aIsOption {
		return AreCoherent(a, b)
	}

	return false
}
