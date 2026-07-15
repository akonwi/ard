package checker

import (
	"fmt"
	"strings"
)

// todo: this can return an error with more detailed messaging for the scenario
func areCompatible(expected Type, actual Type) bool {
	if trait, ok := expected.(*Trait); ok {
		return actual.hasTrait(trait)
	}
	return expected.equal(actual)
}

func commonResultType(a Type, b Type) (Type, bool) {
	if a == nil || b == nil {
		return nil, false
	}
	if a.equal(b) {
		return a, true
	}
	if a == Void || b == Void {
		return Void, true
	}
	if areCompatible(a, b) {
		return a, true
	}
	if areCompatible(b, a) {
		return b, true
	}
	return nil, false
}

func HasTrait(t Type, trait *Trait) bool {
	if t == nil || trait == nil {
		return false
	}
	return t.hasTrait(trait)
}

type Type interface {
	String() string
	get(name string) Type

	/* A.K.A 'compatible()'
	  The Ard type system only allows generics in parameters.
		This means equal is called as `expected.equal(actual)`,
		where `expected` is the declared parameter type and `actual` is the provided any type.
		The exception is when resolving generics in a function call based on inferred types.
	 	In this scenario, the generic is the `other` argument, so that the callee type can fill in the resolved type.
	*/
	equal(other Type) bool

	// hasTrait checks if this type implements the given trait
	hasTrait(trait *Trait) bool
}

type MutableRef struct {
	of Type
}

func MakeMutableRef(of Type) *MutableRef {
	return &MutableRef{of: of}
}

func (m *MutableRef) Of() Type { return m.of }
func (m *MutableRef) String() string {
	if m == nil || m.of == nil {
		return "mut ?"
	}
	return "mut " + m.of.String()
}
func (m *MutableRef) get(name string) Type {
	if m == nil || m.of == nil {
		return nil
	}
	return m.of.get(name)
}
func (m *MutableRef) equal(other Type) bool {
	r, ok := other.(*MutableRef)
	return ok && equalTypes(m.of, r.of)
}
func (m *MutableRef) hasTrait(trait *Trait) bool {
	return m != nil && m.of != nil && m.of.hasTrait(trait)
}

func mutableRefBase(t Type) (Type, bool) {
	if ref, ok := t.(*MutableRef); ok {
		return ref.of, true
	}
	return t, false
}

func derefMutableRef(t Type) Type {
	base, _ := mutableRefBase(t)
	return base
}

type Trait struct {
	Name       string
	ModulePath string
	methods    []FunctionDef
	private    bool
}

// Error is Ard's builtin contract for values that intentionally implement
// Go's predeclared error interface.
var BuiltinError = &Trait{
	Name:       "Error",
	ModulePath: "builtin/Error",
	methods: []FunctionDef{{
		Name:       "error",
		ReturnType: Str,
	}},
}

func IsBuiltinError(t Type) bool {
	trait, ok := t.(*Trait)
	return ok && trait.ModulePath == BuiltinError.ModulePath && trait.Name == BuiltinError.Name
}

func (t Trait) String() string {
	return t.Name
}

func (t Trait) IsPrivate() bool {
	return t.private
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
	return equalTypes(t, other)
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
	case "at":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "index", Type: Int}},
			ReturnType: MakeMaybe(Rune),
		}
	case "bytes":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: MakeList(Byte),
		}
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
	case "replace", "replace_all":
		return &FunctionDef{
			Name: name,
			Parameters: []Parameter{
				{Name: "old", Type: Str},
				{Name: "new", Type: Str},
			},
			ReturnType: Str,
		}
	case "size":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Int,
		}
	case "runes":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: MakeList(Rune),
		}
	case "starts_with", "ends_with":
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
	if o, ok := other.(*TypeVar); ok {
		if o.actual == nil {
			return true
		}
		return s == o.actual
	}
	return s == other
}

func (s *str) hasTrait(trait *Trait) bool {
	return trait.name() == "ToString"
}

var Str = &str{}

type byteType struct{}

func (b byteType) String() string { return "Byte" }
func (b byteType) get(name string) Type {
	switch name {
	case "to_int":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Int,
		}
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
func (b *byteType) equal(other Type) bool {
	if b == other {
		return true
	}
	if typeVar, ok := other.(*TypeVar); ok {
		if typeVar.actual == nil {
			return true
		}
		return b.equal(typeVar.actual)
	}
	if union, ok := other.(*Union); ok {
		return union.equal(b)
	}
	if trait, ok := other.(*Trait); ok {
		return b.hasTrait(trait)
	}
	return false
}
func (b *byteType) hasTrait(trait *Trait) bool {
	return trait.name() == "ToString"
}

var Byte = &byteType{}

type runeType struct{}

func (r runeType) String() string { return "Rune" }
func (r runeType) get(name string) Type {
	switch name {
	case "to_int":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Int,
		}
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
func (r *runeType) equal(other Type) bool {
	if r == other {
		return true
	}
	if typeVar, ok := other.(*TypeVar); ok {
		if typeVar.actual == nil {
			return true
		}
		return r.equal(typeVar.actual)
	}
	if union, ok := other.(*Union); ok {
		return union.equal(r)
	}
	if trait, ok := other.(*Trait); ok {
		return r.hasTrait(trait)
	}
	return false
}
func (r *runeType) hasTrait(trait *Trait) bool {
	return trait.name() == "ToString"
}

var Rune = &runeType{}

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
	case "to_f64":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Float64,
		}
	default:
		return nil
	}
}
func (i *_int) equal(other Type) bool {
	if i == other {
		return true
	}
	if typeVar, ok := other.(*TypeVar); ok {
		if typeVar.actual == nil {
			return true
		}
		return i.equal(typeVar.actual)
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
	return trait.name() == "ToString"
}

var Int = &_int{}

type scalarType struct {
	name string
}

func (s *scalarType) String() string { return s.name }
func (s *scalarType) get(name string) Type {
	switch name {
	case "to_str":
		return &FunctionDef{Name: name, Parameters: []Parameter{}, ReturnType: Str}
	default:
		return nil
	}
}
func (s *scalarType) equal(other Type) bool {
	if s == other {
		return true
	}
	if typeVar, ok := other.(*TypeVar); ok {
		if typeVar.actual == nil {
			return true
		}
		return s.equal(typeVar.actual)
	}
	if union, ok := other.(*Union); ok {
		return union.equal(s)
	}
	if trait, ok := other.(*Trait); ok {
		return s.hasTrait(trait)
	}
	return false
}
func (s *scalarType) hasTrait(trait *Trait) bool { return trait.name() == "ToString" }

var (
	Int8    = &scalarType{name: "Int8"}
	Int16   = &scalarType{name: "Int16"}
	Int32   = &scalarType{name: "Int32"}
	Int64   = &scalarType{name: "Int64"}
	Uint    = &scalarType{name: "Uint"}
	Uint8   = &scalarType{name: "Uint8"}
	Uint16  = &scalarType{name: "Uint16"}
	Uint32  = &scalarType{name: "Uint32"}
	Uint64  = &scalarType{name: "Uint64"}
	Uintptr = &scalarType{name: "Uintptr"}
	Float32 = &scalarType{name: "Float32"}
)

type float struct{}

func (f float) String() string { return "Float64" }
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
	if o, ok := other.(*TypeVar); ok {
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
	return trait.name() == "ToString"
}

var Float64 = &float{}

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
	if o, ok := other.(*TypeVar); ok {
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
	return trait.name() == "ToString"
}

var Bool = &_bool{}

type void struct{}

func (v void) String() string       { return "Void" }
func (v void) get(name string) Type { return nil }
func (v *void) equal(other Type) bool {
	// pass when comparing with an open generic
	if typeVar, isTypeVar := other.(*TypeVar); isTypeVar && typeVar.actual == nil {
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
		// Bounds-checked access, symmetric with Str.at: Some(element) when the
		// index is in range, None otherwise.
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "index", Type: Int}},
			ReturnType: MakeMaybe(l.of),
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
	return equalTypes(l, other)
}

func (l *List) hasTrait(trait *Trait) bool {
	return false // Lists don't implement any traits by default
}
func (l *List) Of() Type {
	return l.of
}

type FixedArray struct {
	of     Type
	length int
}

func MakeFixedArray(of Type, length int) *FixedArray {
	return &FixedArray{of: of, length: length}
}

func (a FixedArray) String() string {
	return fmt.Sprintf("[%s; %d]", typeSyntaxString(a.of), a.length)
}

func (a FixedArray) get(name string) Type {
	switch name {
	case "at":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "index", Type: Int}},
			ReturnType: MakeMaybe(a.of),
		}
	case "size":
		return &FunctionDef{Name: name, ReturnType: Int}
	default:
		return nil
	}
}

func (a *FixedArray) equal(other Type) bool {
	return equalTypes(a, other)
}

func (a *FixedArray) hasTrait(trait *Trait) bool {
	return false
}

func (a *FixedArray) Of() Type {
	return a.of
}

func (a *FixedArray) Len() int {
	return a.length
}

// Chan is a typed channel for communicating between concurrent tasks. It
// lowers to a native Go `chan T`.
type Chan struct {
	of Type
}

func MakeChan(of Type) *Chan {
	return &Chan{of}
}
func (c Chan) String() string {
	return "Chan<" + c.of.String() + ">"
}
func (c Chan) get(name string) Type {
	switch name {
	case "send":
		return chanSendMethod(c.of)
	case "recv":
		return chanRecvMethod(c.of)
	case "close":
		return chanCloseMethod()
	case "receiver":
		return &FunctionDef{Name: "receiver", ReturnType: MakeReceiver(c.of)}
	case "sender":
		return &FunctionDef{Name: "sender", ReturnType: MakeSender(c.of)}
	}
	return nil
}
func (c *Chan) equal(other Type) bool {
	return equalTypes(c, other)
}
func (c *Chan) hasTrait(trait *Trait) bool {
	return false
}
func (c *Chan) Of() Type {
	return c.of
}

// channelElementType returns the element type of any channel-like type (Chan,
// Receiver, Sender) and whether the type is a channel at all.
func channelElementType(t Type) (Type, bool) {
	switch ch := t.(type) {
	case *Chan:
		return ch.of, true
	case *Receiver:
		return ch.of, true
	case *Sender:
		return ch.of, true
	}
	return nil, false
}

// channelCanRecv reports whether the channel type permits receiving.
func channelCanRecv(t Type) bool {
	switch t.(type) {
	case *Chan, *Receiver:
		return true
	}
	return false
}

// channelCanSend reports whether the channel type permits sending.
func channelCanSend(t Type) bool {
	switch t.(type) {
	case *Chan, *Sender:
		return true
	}
	return false
}

// chanSendMethod / chanRecvMethod / chanCloseMethod build the channel method
// signatures shared by the bidirectional and directional channel types.
func chanSendMethod(of Type) Type {
	return &FunctionDef{Name: "send", Parameters: []Parameter{{Name: "value", Type: of}}, ReturnType: Void}
}
func chanRecvMethod(of Type) Type {
	return &FunctionDef{Name: "recv", ReturnType: &Maybe{of}}
}
func chanCloseMethod() Type {
	return &FunctionDef{Name: "close", ReturnType: Void}
}

// Receiver is a receive-only channel view (Go `<-chan T`). Its only method is
// recv. Created by Chan.receiver() and produced by mapping Go `<-chan T`.
type Receiver struct {
	of Type
}

func MakeReceiver(of Type) *Receiver {
	return &Receiver{of}
}
func (c Receiver) String() string {
	return "Receiver<" + c.of.String() + ">"
}
func (c Receiver) get(name string) Type {
	if name == "recv" {
		return chanRecvMethod(c.of)
	}
	return nil
}
func (c *Receiver) equal(other Type) bool {
	return equalTypes(c, other)
}
func (c *Receiver) hasTrait(trait *Trait) bool {
	return false
}
func (c *Receiver) Of() Type {
	return c.of
}

// Sender is a send-only channel view (Go `chan<- T`). Its methods are send and
// close. Created by Chan.sender() and produced by mapping Go `chan<- T`.
type Sender struct {
	of Type
}

func MakeSender(of Type) *Sender {
	return &Sender{of}
}
func (c Sender) String() string {
	return "Sender<" + c.of.String() + ">"
}
func (c Sender) get(name string) Type {
	switch name {
	case "send":
		return chanSendMethod(c.of)
	case "close":
		return chanCloseMethod()
	}
	return nil
}
func (c *Sender) equal(other Type) bool {
	return equalTypes(c, other)
}
func (c *Sender) hasTrait(trait *Trait) bool {
	return false
}
func (c *Sender) Of() Type {
	return c.of
}

type Map struct {
	key   Type
	value Type
}

func MakeMap(key, value Type) *Map {
	return &Map{key, value}
}

func (m Map) String() string {
	return fmt.Sprintf("[%s: %s]", m.key.String(), m.value.String())
}
func (m Map) equal(other Type) bool {
	return equalTypes(m, other)
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
			ReturnType: Void,
		}
	case "delete":
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
	switch m.of.(type) {
	case *FunctionDef, FunctionDef, *Result, *MutableRef:
		return "(" + typeSyntaxString(m.of) + ")?"
	default:
		return typeSyntaxString(m.of) + "?"
	}
}

func typeSyntaxString(t Type) string {
	switch typ := t.(type) {
	case *FunctionDef:
		return functionTypeString(*typ)
	case FunctionDef:
		return functionTypeString(typ)
	case *Maybe:
		return typ.String()
	case *Result:
		return resultOperandSyntax(typ.val) + "!" + resultOperandSyntax(typ.err)
	case *List:
		return "[" + typeSyntaxString(typ.of) + "]"
	case *FixedArray:
		return fmt.Sprintf("[%s; %d]", typeSyntaxString(typ.of), typ.length)
	case *Map:
		return "[" + typeSyntaxString(typ.key) + ": " + typeSyntaxString(typ.value) + "]"
	default:
		return t.String()
	}
}

func resultOperandSyntax(t Type) string {
	value := typeSyntaxString(t)
	switch t.(type) {
	case *FunctionDef, FunctionDef, *Result:
		return "(" + value + ")"
	default:
		return value
	}
}

func functionTypeString(f FunctionDef) string {
	return callableTypeString(f.Parameters, f.ReturnType)
}

func callableTypeString(params []Parameter, returnType Type) string {
	paramStrs := make([]string, len(params))
	for i := range params {
		mutable, paramType := normalizedParamMutability(params[i])
		paramStrs[i] = typeSyntaxString(paramType)
		// A pointer-shaped foreign type renders its own `mut` prefix.
		if foreign, ok := paramType.(*ForeignType); mutable && (!ok || !foreign.Pointer) {
			paramStrs[i] = "mut " + paramStrs[i]
		}
	}
	rendered := fmt.Sprintf("fn(%s)", strings.Join(paramStrs, ", "))
	// Ard syntax omits the return type for non-returning functions.
	if returnType == nil || returnType.equal(Void) {
		return rendered
	}
	return rendered + " " + typeSyntaxString(returnType)
}
func (m *Maybe) get(name string) Type {
	switch name {
	case "expect":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "message", Type: Str}},
			ReturnType: m.of,
		}
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
	case "set":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "value", Type: m.of}},
			ReturnType: Void,
		}
	case "clear":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Void,
		}
	case "map":
		mapped := &TypeVar{name: "Mapped"}
		return &FunctionDef{
			Name: name,
			Parameters: []Parameter{{
				Name: "with",
				Type: &FunctionDef{
					Name:       "<function>",
					Parameters: []Parameter{{Name: "value", Type: m.of}},
					ReturnType: mapped,
				},
			}},
			ReturnType: MakeMaybe(mapped),
		}
	case "and_then":
		mapped := &TypeVar{name: "Mapped"}
		return &FunctionDef{
			Name: name,
			Parameters: []Parameter{{
				Name: "with",
				Type: &FunctionDef{
					Name:       "<function>",
					Parameters: []Parameter{{Name: "value", Type: m.of}},
					ReturnType: MakeMaybe(mapped),
				},
			}},
			ReturnType: MakeMaybe(mapped),
		}
	default:
		return nil
	}
}
func (m *Maybe) equal(other Type) bool {
	return equalTypes(m, other)
}

func (m *Maybe) hasTrait(trait *Trait) bool {
	return false // Maybe types don't implement traits by default
}
func (m *Maybe) Of() Type {
	return m.of
}

type TypeVar struct {
	name   string
	actual Type
	bound  bool
}

func (a TypeVar) String() string {
	if a.bound && a.actual != nil {
		return a.actual.String()
	}
	return "$" + a.name
}

func (a *TypeVar) Name() string {
	return a.name
}

func (a *TypeVar) Actual() Type {
	return a.actual
}

func (a TypeVar) get(name string) Type {
	if a.actual == nil {
		return nil
	}
	return a.actual.get(name)
}
func (a *TypeVar) equal(other Type) bool {
	return equalTypes(a, other)
}

func (a *TypeVar) hasTrait(trait *Trait) bool {
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
	case "map":
		mappedVal := &TypeVar{name: "MappedVal"}
		return &FunctionDef{
			Name: name,
			Parameters: []Parameter{{
				Name: "with",
				Type: &FunctionDef{
					Name:       "<function>",
					Parameters: []Parameter{{Name: "value", Type: r.val}},
					ReturnType: mappedVal,
				},
			}},
			ReturnType: MakeResult(mappedVal, r.err),
		}
	case "map_err":
		mappedErr := &TypeVar{name: "MappedErr"}
		return &FunctionDef{
			Name: name,
			Parameters: []Parameter{{
				Name: "with",
				Type: &FunctionDef{
					Name:       "<function>",
					Parameters: []Parameter{{Name: "err", Type: r.err}},
					ReturnType: mappedErr,
				},
			}},
			ReturnType: MakeResult(r.val, mappedErr),
		}
	case "and_then":
		mappedVal := &TypeVar{name: "MappedVal"}
		return &FunctionDef{
			Name: name,
			Parameters: []Parameter{{
				Name: "with",
				Type: &FunctionDef{
					Name:       "<function>",
					Parameters: []Parameter{{Name: "value", Type: r.val}},
					ReturnType: MakeResult(mappedVal, r.err),
				},
			}},
			ReturnType: MakeResult(mappedVal, r.err),
		}
	default:
		return nil
	}
}

func (r *Result) equal(other Type) bool {
	return equalTypes(r, other)
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

// Any type for external/untyped data
type anyType struct{}

func (d anyType) String() string       { return "Any" }
func (d anyType) get(name string) Type { return nil }
func (d anyType) equal(other Type) bool {
	if _, ok := other.(*anyType); ok {
		return true
	}
	if typeVar, ok := other.(*TypeVar); ok && typeVar.actual == nil {
		return true
	}
	return false
}
func (d anyType) hasTrait(trait *Trait) bool { return false }

var Any = &anyType{}
