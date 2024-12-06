package checker

import (
	"fmt"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type Type interface {
	String() string
	GetProperty(name string) Type
	Equals(other Type) bool
}

type PrimitiveType struct {
	Name string
}

// impl Type for PrimitiveType
func (p PrimitiveType) String() string {
	return p.Name
}
func (p PrimitiveType) GetProperty(name string) Type {
	switch p.Name {
	case "Str":
		switch name {
		case "size":
			return NumType
		default:
			return nil
		}
	}
	return nil
}

func (p PrimitiveType) Equals(other Type) bool {
	if otherPrimitive, ok := other.(PrimitiveType); ok {
		return p.Name == otherPrimitive.Name
	}
	return false
}

var (
	StrType  = PrimitiveType{"Str"}
	NumType  = PrimitiveType{"Num"}
	BoolType = PrimitiveType{"Bool"}
	VoidType = PrimitiveType{"Void"}
)

type FunctionType struct {
	Name       string
	Mutates    bool
	Parameters []Type
	ReturnType Type
}

func (f FunctionType) String() string {
	params := strings.Builder{}
	for i, param := range f.Parameters {
		if i > 0 {
			params.WriteString(", ")
		}
		params.WriteString(param.String())
	}
	return fmt.Sprintf("(%v) %v", params.String(), f.ReturnType)
}
func (f FunctionType) GetProperty(name string) Type {
	return nil
}
func (f FunctionType) Equals(other Type) bool {
	if otherFunc, ok := other.(FunctionType); ok {
		if len(f.Parameters) != len(otherFunc.Parameters) {
			return false
		}
		for i, param := range f.Parameters {
			if !param.Equals(otherFunc.Parameters[i]) {
				return false
			}
		}
		return f.ReturnType.Equals(otherFunc.ReturnType)
	}
	return false
}
func (f FunctionType) GetName() string {
	return f.Name
}
func (f FunctionType) GetType() Type {
	return f
}

type StructType struct {
	Name   string
	Fields map[string]Type
}

func (s StructType) String() string {
	return fmt.Sprintf("Struct(%s)", s.Name)
}
func (s StructType) GetProperty(name string) Type {
	if field, ok := s.Fields[name]; ok {
		return field
	}
	return nil
}
func (s StructType) Equals(other Type) bool {
	return s.String() == other.String()
}
func (s StructType) GetName() string {
	return s.Name
}
func (s StructType) GetType() Type {
	return s
}

type EnumType struct {
	Name     string
	Variants []string
}

func (e EnumType) HasVariant(variant string) bool {
	for _, v := range e.Variants {
		if v == variant {
			return true
		}
	}
	return false
}

func (e EnumType) FormatVariant(variant string) string {
	return fmt.Sprintf("%s::%s", e.Name, variant)
}

func (e EnumType) String() string {
	return e.Name
}
func (e EnumType) GetProperty(name string) Type {
	return nil
}
func (e EnumType) Equals(other Type) bool {
	return e.String() == other.String()
}
func (e EnumType) GetName() string {
	return e.Name
}
func (e EnumType) GetType() Type {
	return e
}

type GenericType struct {
	inner *Type
	name  string
}

func (g GenericType) String() string {
	return fmt.Sprintf("%s?", g.name)
}
func (g GenericType) GetType() Type {
	if g.inner == nil {
		return g
	}
	return *g.inner
}
func (g GenericType) Equals(other Type) bool {
	if g.inner == nil {
		return true
	}
	return (*g.inner).Equals(other)
}
func (g GenericType) GetProperty(name string) Type {
	if g.inner == nil {
		return nil
	}
	return (*g.inner).GetProperty(name)
}
func (g *GenericType) Fill(inner Type) {
	g.inner = &inner
}

type ListType struct {
	ItemType Type
}

func (l ListType) String() string {
	if l.ItemType == nil {
		return "[?]"
	}
	return fmt.Sprintf("[%s]", l.ItemType)
}
func (l ListType) GetProperty(name string) Type {
	switch name {
	case "map":
		outType := GenericType{name: "Out"}
		return FunctionType{
			Mutates: false,
			Name:    "map",
			Parameters: []Type{
				FunctionType{
					Name:       "callback",
					Parameters: []Type{l.ItemType},
					ReturnType: outType,
				},
			},
			// List probably needs to use a pointer to the inner type
			ReturnType: MakeList(outType),
		}
	case "pop":
		// pop is a function that takes no arguments and returns the last item in the list
		// (Item?) Num
		return FunctionType{
			Mutates:    true,
			Name:       "pop",
			Parameters: []Type{},
			ReturnType: l.ItemType,
		}
	case "push":
		// push is a function that takes an item of the same type as the list and returns the new size
		// () Item?
		return FunctionType{
			Mutates:    true,
			Name:       "push",
			Parameters: []Type{l.ItemType},
			ReturnType: NumType,
		}
	case "size":
		return NumType
	default:
		return nil
	}
}
func (l ListType) Equals(other Type) bool {
	if otherList, ok := other.(ListType); ok {
		// if either list is still open, then they are compatible
		if l.ItemType == nil || otherList.ItemType == nil {
			return true
		}
		return l.ItemType.Equals(otherList.ItemType)
	}
	return false
}
func MakeList(itemType Type) ListType {
	return ListType{ItemType: itemType}
}

type MapType struct {
	KeyType   Type
	ValueType Type
}

func (m MapType) String() string {
	value := "?"
	if m.ValueType != nil {
		value = m.ValueType.String()
	}
	return fmt.Sprintf("{%s:%s}", m.KeyType, value)
}
func (m MapType) GetProperty(name string) Type {
	switch name {
	case "size":
		return NumType
	default:
		return nil
	}
}
func (m MapType) Equals(other Type) bool {
	if otherMap, ok := other.(MapType); ok {
		if !m.KeyType.Equals(otherMap.KeyType) {
			return false
		}
		if m.ValueType == nil || otherMap.ValueType == nil {
			return true
		}
		return m.ValueType.Equals(otherMap.ValueType)
	}
	return false
}
func MakeMap(valueType Type) MapType {
	return MapType{KeyType: StrType, ValueType: valueType}
}

type Symbol interface {
	GetName() string
	GetType() Type
}

type Variable struct {
	Name    string
	Type    Type
	Mutable bool
}

func (v Variable) GetName() string {
	return v.Name
}
func (v Variable) GetType() Type {
	return v.Type
}

type ScopeOptions struct {
	IsTop bool
}

type Scope struct {
	parent  *Scope
	symbols map[string]Symbol
	structs map[string]StructType
}

func (s Scope) GetParent() *Scope {
	return s.parent
}
func NewScope(parent *Scope, options ScopeOptions) Scope {
	scope := Scope{
		parent:  parent,
		symbols: make(map[string]Symbol),
		structs: make(map[string]StructType),
	}
	if options.IsTop {
		scope.Declare(FunctionType{
			Name: "print",
			Parameters: []Type{
				StrType,
			},
			ReturnType: VoidType,
		})
	}
	return scope
}

func (s *Scope) Declare(sym Symbol) error {
	if existing, ok := s.symbols[sym.GetName()]; ok {
		return fmt.Errorf("symbol %s already declared as %v", existing.GetName(), existing.GetType())
	}
	s.symbols[sym.GetName()] = sym
	return nil
}

func (s *Scope) Lookup(name string) Symbol {
	if sym, ok := s.symbols[name]; ok {
		return sym
	}
	if s.parent != nil {
		return s.parent.Lookup(name)
	}
	return nil
}

type Diagnostic struct {
	Msg   string
	Range tree_sitter.Range
}

// tree-sitter uses 0-based indexing, so make this human friendly when it's time to show it to humans
// start := Position{
// 	Line:   node.StartPosition().Row + 1,
// 	Column: node.StartByte() + 1,
// }
// end := Position{
// 	Line:   node.EndPosition().Row + 1,
// 	Column: node.EndPosition().Column,
// }

func MakeError(msg string, node *tree_sitter.Node) Diagnostic {
	return Diagnostic{
		Msg:   msg,
		Range: node.Range(),
	}
}
