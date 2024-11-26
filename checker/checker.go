package checker

import (
	"fmt"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type Type interface {
	String() string
	GetProperty(name string) Type
}

type PrimitiveType struct {
	Name string
}

// impl Type for PrimitiveType
func (p PrimitiveType) String() string {
	return p.Name
}
func (p PrimitiveType) GetProperty(name string) Type {
	switch name {
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

// func (p PrimitiveType) Equals(other Type) bool {
// 	if otherPrim, ok := other.(PrimitiveType); ok {
// 		return p.Name == otherPrim.Name
// 	}
// 	return false
// }

var (
	StrType  = PrimitiveType{"Str"}
	NumType  = PrimitiveType{"Num"}
	BoolType = PrimitiveType{"Bool"}
	VoidType = PrimitiveType{"Void"}
)

type FunctionType struct {
	Mutates    bool
	Parameters []Type
	ReturnType Type
}

// impl StaticType for FunctionType
func (f FunctionType) String() string {
	return fmt.Sprintf("(%v) %v", f.Parameters, f.ReturnType)
}
func (f FunctionType) GetProperty(name string) Type {
	return nil
}
func (f FunctionType) Equals(other Type) bool {
	return f.String() == other.String()
	// if otherFunc, ok := other.(FunctionType); ok {
	// 	if len(f.Parameters) != len(otherFunc.Parameters) {
	// 		return false
	// 	}
	// 	for i, param := range f.Parameters {
	// 		if !param.Equals(otherFunc.Parameters[i]) {
	// 			return false
	// 		}
	// 	}
	// 	return f.ReturnType.Equals(otherFunc.ReturnType)
	// }
	// return false
}

type StructType struct {
	Name   string
	Fields map[string]Type
}

func (s StructType) String() string {
	return s.Name
}
func (s StructType) GetProperty(name string) Type {
	if field, ok := s.Fields[name]; ok {
		return field
	}
	return nil
}

type EnumType struct {
	Name     string
	Variants map[string]int
}

func (e EnumType) String() string {
	return e.Name
}
func (e EnumType) GetProperty(name string) Type {
	return nil
}

type ListType struct {
	ItemType Type
}

func (l ListType) String() string {
	return fmt.Sprintf("[%v]", l.ItemType)
}
func (l ListType) GetProperty(name string) Type {
	switch name {
	case "size":
		return NumType
	default:
		return nil
	}
}

type Symbol struct {
	Name     string
	Type     Type
	Mutable  bool
	Declared bool // what's the purpose of this?
}

type Scope struct {
	parent  *Scope
	symbols map[string]Symbol
	structs map[string]StructType
	enums   map[string]EnumType
}

func (s Scope) GetParent() *Scope {
	return s.parent
}
func NewScope(parent *Scope) *Scope {
	return &Scope{
		parent:  parent,
		symbols: make(map[string]Symbol),
		structs: make(map[string]StructType),
		enums:   make(map[string]EnumType),
	}
}

func (s *Scope) Declare(sym Symbol) error {
	if existing, ok := s.symbols[sym.Name]; ok {
		return fmt.Errorf("symbol %s already declared as %v", existing.Name, existing.Type)
	}
	s.symbols[sym.Name] = sym
	return nil
}

func (s *Scope) DeclareStruct(strct StructType) error {
	if existing, ok := s.structs[strct.Name]; ok {
		return fmt.Errorf("struct %s is already defined", existing.Name)
	}
	s.structs[strct.Name] = strct
	return nil
}

func (s *Scope) DeclareEnum(enum EnumType) error {
	if existing, ok := s.enums[enum.Name]; ok {
		return fmt.Errorf("enum %s is already defined", existing.Name)
	}
	s.enums[enum.Name] = enum
	return nil
}

func (s *Scope) Lookup(name string) *Symbol {
	if sym, ok := s.symbols[name]; ok {
		return &sym
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
