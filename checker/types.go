package checker

import (
	"fmt"
)

// todo: this can return an error with more detailed messaging for the scenario
func areCompatible(expected Type, actual Type) bool {
	if trait, ok := expected.(*Trait); ok {
		return actual.hasTrait(trait)
	}

	return expected.equal(actual)
}

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

	// hasTrait checks if this type implements the given trait
	hasTrait(trait *Trait) bool
}

type Trait struct {
	Name    string
	methods []FunctionDef
	private bool
}

func (t Trait) String() string {
	return t.Name
}

func (t Trait) name() string {
	return t.Name
}

func (t Trait) _type() Type {
	return t
}

func (t Trait) get(name string) Type {
	for _, method := range t.methods {
		if method.Name == name {
			return &method
		}
	}
	return nil
}

func (t Trait) equal(other Type) bool {
	o, ok := other.(*Trait)
	if !ok {
		return false
	}
	if t.Name != o.Name {
		return false
	}
	if len(t.methods) != len(o.methods) {
		return false
	}
	for i := range t.methods {
		if !t.methods[i].equal(&o.methods[i]) {
			return false
		}
	}
	return true
}

func (t Trait) GetMethods() []FunctionDef {
	return t.methods
}

func (t Trait) hasTrait(trait *Trait) bool {
	return t.equal(trait)
}

type str struct{}

func (s str) String() string { return "Str" }
func (s str) get(name string) Type {
	switch name {
	case "contains":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "sub", Type: Str}},
			ReturnType: Bool,
		}
	case "is_empty":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Bool,
		}
	case "size":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Int,
		}
	case "split":
		return &FunctionDef{
			Name: name,
			Parameters: []Parameter{
				{Name: "delimeter", Type: Str},
			},
			ReturnType: MakeList(Str),
		}
	case "starts_with":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "str", Type: Str}},
			ReturnType: Bool,
		}
	case "to_str":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Str,
		}
	case "trim":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Str,
		}
	default:
		return nil
	}
}
func (s *str) equal(other Type) bool {
	if o, ok := other.(*Any); ok {
		if o.actual == nil {
			return true
		}
		return s == o.actual
	}
	return s == other
}

func (s *str) hasTrait(trait *Trait) bool {
	return trait == strMod.symbols["ToString"].Type
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
	if i == other {
		return true
	}
	if any, ok := other.(*Any); ok {
		if any.actual == nil {
			return true
		}
		return i.equal(any.actual)
	}

	if union, ok := other.(*Union); ok {
		return union.equal(i)
	}
	if trait, ok := other.(*Trait); ok {
		return i.hasTrait(trait)
	}
	return false
}

func (i *_int) hasTrait(trait *Trait) bool {
	return trait == strMod.symbols["ToString"].Type
}

var Int = &_int{}

type float struct{}

func (f float) String() string { return "Float" }
func (f float) get(name string) Type {
	switch name {
	case "to_str":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Str,
		}
	case "to_int":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Int,
		}
	default:
		return nil
	}
}
func (f *float) equal(other Type) bool {
	if f == other {
		return true
	}
	if o, ok := other.(*Any); ok {
		if o.actual == nil {
			return true
		}
		return f == o.actual
	}
	if union, ok := other.(*Union); ok {
		return union.equal(f)
	}
	return false
}

func (f *float) hasTrait(trait *Trait) bool {
	return trait == strMod.symbols["ToString"].Type
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
	if b == other {
		return true
	}
	if o, ok := other.(*Any); ok {
		if o.actual == nil {
			return true
		}
		return b == o.actual
	}
	if union, ok := other.(*Union); ok {
		return union.equal(b)
	}
	return false
}

func (b *_bool) hasTrait(trait *Trait) bool {
	return trait == strMod.symbols["ToString"].Type
}

var Bool = &_bool{}

type void struct{}

func (v void) String() string       { return "Void" }
func (v void) get(name string) Type { return nil }
func (v *void) equal(other Type) bool {
	// pass when comparing with an open generic
	if any, isAny := other.(*Any); isAny && any.actual == nil {
		return true
	}
	return v == other
}
func (v *void) hasTrait(trait *Trait) bool { return false }

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
			ReturnType: l.of,
		}
	case "prepend":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "value", Type: l.of}},
			Mutates:    true,
			ReturnType: Int,
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
	case "sort":
		param := Parameter{
			Name: "cmp",
			Type: &FunctionDef{
				Parameters: []Parameter{{Name: "a", Type: l.of}, {Name: "b", Type: l.of}},
				ReturnType: Bool,
			},
		}
		return &FunctionDef{
			Mutates:    true,
			Name:       name,
			Parameters: []Parameter{param},
			ReturnType: Void,
		}
	case "swap":
		return &FunctionDef{
			Mutates: true,
			Name:    name,
			Parameters: []Parameter{
				{Name: "l", Type: Int},
				{Name: "r", Type: Int},
			},
			ReturnType: Void,
		}
	default:
		return nil
	}
}
func (l *List) equal(other Type) bool {
	if o, ok := other.(*List); ok {
		return l.of.equal(o.of)
	}
	if any, ok := other.(*Any); ok {
		if any.actual == nil {
			return true
		}
		return l.equal(any.actual)
	}
	if union, ok := other.(*Union); ok {
		return union.equal(l)
	}

	return false
}

func (l *List) hasTrait(trait *Trait) bool {
	return false // Lists don't implement any traits by default
}
func (l List) Of() Type {
	return l.of
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
	if any, ok := other.(*Any); ok {
		if any.actual == nil {
			return true
		}
		return m.equal(any.actual)
	}

	if union, ok := other.(*Union); ok {
		return union.equal(m)
	}
	return false
}

func (m Map) hasTrait(trait *Trait) bool {
	return false // Maps don't implement any traits by default
}
func (m Map) get(name string) Type {
	switch name {
	case "get":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "key", Type: m.key}},
			ReturnType: &Maybe{m.value},
		}
	case "keys":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: MakeList(m.key),
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

func (m *Map) Key() Type {
	return m.key
}

func (m *Map) Value() Type {
	return m.value
}

type Maybe struct {
	of Type
}

func IsMaybe(t Type) bool {
	_, ok := t.(*Maybe)
	return ok
}

func MakeMaybe(of Type) *Maybe {
	return &Maybe{of}
}

func (m *Maybe) String() string {
	return m.of.String() + "?"
}
func (m *Maybe) get(name string) Type {
	switch name {
	case "is_none":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Bool,
		}
	case "is_some":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Bool,
		}
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

func (m *Maybe) hasTrait(trait *Trait) bool {
	return false // Maybe types don't implement traits by default
}
func (m *Maybe) Of() Type {
	return m.of
}

type Any struct {
	name   string
	actual Type
}

func (a Any) String() string {
	if a.actual != nil {
		return a.actual.String()
	}
	return "$" + a.name
}
func (a Any) Actual() Type {
	return a.actual
}

func (a Any) get(name string) Type {
	if a.actual != nil {
		return a.actual.get(name)
	}
	panic(fmt.Errorf("Cannot look up symbols in unrefined %s", a.String()))
}
func (a *Any) equal(other Type) bool {
	if a == other {
		return true
	}
	if a.actual == nil {
		return true
	}
	return a.actual.equal(other)
}

func (a *Any) hasTrait(trait *Trait) bool {
	if a.actual == nil {
		return false
	}
	return a.actual.hasTrait(trait)
}

type Result struct {
	val Type
	err Type
}

func MakeResult(val, err Type) *Result {
	return &Result{val, err}
}

func (r Result) String() string {
	return fmt.Sprintf("%s!%s", r.val.String(), r.err.String())
}

func (r Result) get(name string) Type {
	switch name {
	case "expect":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "message", Type: Str}},
			ReturnType: r.val,
		}
	case "or":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "default", Type: r.val}},
			ReturnType: r.val,
		}
	case "is_ok":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Bool,
		}
	case "is_err":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Bool,
		}
	default:
		return nil
	}
}

func (r *Result) equal(other Type) bool {
	if o, ok := other.(*Result); ok {
		return r.val.equal(o.val) && r.err.equal(o.err)
	}
	return false
}

func (r *Result) hasTrait(trait *Trait) bool {
	return false // Result types don't implement traits by default
}

func (r *Result) Val() Type {
	return r.val
}

func (r *Result) Err() Type {
	return r.err
}

func getGenerics(types ...Type) []Type {
	all := []Type{}
	for _, t := range types {
		switch t := t.(type) {
		case *List:
			all = append(all, getGenerics(t.of)...)
		case *Result:
			all = append(all, getGenerics(t.val, t.err)...)
		case *Any:
			if t.actual == nil {
				all = append(all, t)
			}
		}
	}
	return all
}

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
