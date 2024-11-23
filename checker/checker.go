package checker

import (
	"fmt"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type Type interface {
	String() string
}

type PrimitiveType struct {
	Name string
}

// impl StaticType for PrimitiveType
func (p PrimitiveType) String() string {
	return p.Name
}

var (
	StrType  = PrimitiveType{"Str"}
	NumType  = PrimitiveType{"Num"}
	BoolType = PrimitiveType{"Bool"}
	VoidType = PrimitiveType{"Void"}
)

type FunctionType struct {
	Parameters []Type
	ReturnType Type
}

// impl StaticType for FunctionType
func (f FunctionType) String() string {
	return fmt.Sprintf("(%v) %v", f.Parameters, f.ReturnType)
}

type Scope struct {
	parent  *Scope
	symbols map[string]Symbol
}

type Symbol struct {
	Name     string
	Type     Type
	Mutable  bool
	Declared bool // what's the purpose of this?
}

func NewScope(parent *Scope) *Scope {
	return &Scope{
		parent:  parent,
		symbols: make(map[string]Symbol),
	}
}

func (s *Scope) Declare(sym Symbol) error {
	if existing, ok := s.symbols[sym.Name]; ok {
		return fmt.Errorf("symbol %s already declared as %v", existing.Name, existing.Type)
	}
	s.symbols[sym.Name] = sym
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
