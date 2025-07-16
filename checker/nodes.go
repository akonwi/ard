package checker

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/akonwi/ard/ast"
)

/* can either produce a value or not */
type Statement struct {
	Break bool
	Expr  Expression
	Stmt  NonProducing
}

type NonProducing interface {
	NonProducing()
}

type Expression interface {
	Type() Type
}

type StrLiteral struct {
	Value string
}

func (s *StrLiteral) String() string {
	return fmt.Sprintf(`"%s"`, s.Value)
}
func (s *StrLiteral) Type() Type {
	return Str
}

type TemplateStr struct {
	Chunks []Expression
}

func (t *TemplateStr) String() string {
	return "TemplateStr"
}
func (t *TemplateStr) Type() Type {
	return Str
}

type BoolLiteral struct {
	Value bool
}

func (b *BoolLiteral) String() string {
	return strconv.FormatBool(b.Value)
}

func (b *BoolLiteral) Type() Type {
	return Bool
}

type IntLiteral struct {
	Value int
}

func (i *IntLiteral) String() string {
	return strconv.Itoa(i.Value)
}

func (i *IntLiteral) Type() Type {
	return Int
}

type FloatLiteral struct {
	Value float64
}

func (f *FloatLiteral) String() string {
	return strconv.FormatFloat(f.Value, 'g', 10, 64)
}

func (f *FloatLiteral) Type() Type {
	return Float
}

type ListLiteral struct {
	Elements []Expression
	_type    Type
}

func (l *ListLiteral) Type() Type {
	return l._type
}

type MapLiteral struct {
	Keys   []Expression
	Values []Expression
	_type  Type
}

func (m *MapLiteral) Type() Type {
	return m._type
}

type VariableDef struct {
	Mutable bool
	Name    string
	__type  Type
	Value   Expression
}

func (v *VariableDef) NonProducing() {}
func (v *VariableDef) name() string {
	return v.Name
}
func (v *VariableDef) _type() Type {
	return v.__type
}

type Reassignment struct {
	Target Expression
	Value  Expression
}

func (r *Reassignment) NonProducing() {}

type Identifier struct {
	Name string
	sym  symbol
}

func (i *Identifier) Type() Type {
	return i.sym._type()
}

type Variable struct {
	sym symbol
}

func (v *Variable) isMutable() bool {
	if def, ok := v.sym.(*VariableDef); ok {
		return def.Mutable
	}
	return false
}

func (v Variable) String() string {
	return v.Name()
}
func (v Variable) Name() string {
	return v.sym.name()
}
func (v Variable) Type() Type {
	return v.sym._type()
}

type InstanceProperty struct {
	Subject  Expression
	Property string
	_type    Type
}

func (i *InstanceProperty) Type() Type {
	return i._type
}

// String returns a string representation of the instance property
func (i *InstanceProperty) String() string {
	// Special case for self-reference using @
	if v, ok := i.Subject.(*Variable); ok && v.Name() == "@" {
		return fmt.Sprintf("@%s", i.Property)
	}
	return fmt.Sprintf("%s.%s", i.Subject, i.Property)
}

type InstanceMethod struct {
	Subject Expression
	Method  *FunctionCall
}

func (i *InstanceMethod) Type() Type {
	return i.Method.Type()
}

type Negation struct {
	Value Expression
}

func (n *Negation) String() string {
	return fmt.Sprintf("-%s", n.Value)
}
func (n *Negation) Type() Type {
	return n.Value.Type()
}

type Not struct {
	Value Expression
}

func (n *Not) String() string {
	return fmt.Sprintf("not %s", n.Value)
}
func (n *Not) Type() Type {
	return Bool
}

type IntAddition struct {
	Left  Expression
	Right Expression
}

func (n *IntAddition) Type() Type {
	return Int
}

type IntSubtraction struct {
	Left  Expression
	Right Expression
}

func (n *IntSubtraction) Type() Type {
	return Int
}

type IntMultiplication struct {
	Left  Expression
	Right Expression
}

func (n *IntMultiplication) Type() Type {
	return Int
}

type IntDivision struct {
	Left  Expression
	Right Expression
}

func (n *IntDivision) Type() Type {
	return Int
}

type IntModulo struct {
	Left  Expression
	Right Expression
}

func (n *IntModulo) Type() Type {
	return Int
}

type IntGreater struct {
	Left  Expression
	Right Expression
}

func (n *IntGreater) Type() Type {
	return Bool
}

type IntGreaterEqual struct {
	Left  Expression
	Right Expression
}

func (n *IntGreaterEqual) Type() Type {
	return Bool
}

type IntLess struct {
	Left  Expression
	Right Expression
}

func (n *IntLess) Type() Type {
	return Bool
}

type IntLessEqual struct {
	Left  Expression
	Right Expression
}

func (n *IntLessEqual) Type() Type {
	return Bool
}

type FloatAddition struct {
	Left  Expression
	Right Expression
}

func (n *FloatAddition) Type() Type {
	return Float
}

type Match struct {
	Pattern *Identifier
	Body    *Block
}

type OptionMatch struct {
	Subject Expression
	Some    *Match
	None    *Block
}

func (o *OptionMatch) Type() Type {
	return o.Some.Body.Type()
}

type EnumMatch struct {
	Subject  Expression
	Cases    []*Block
	CatchAll *Block
}

func (e *EnumMatch) Type() Type {
	// Find the first non-nil case
	for _, c := range e.Cases {
		if c != nil {
			return c.Type()
		}
	}
	// If all cases are nil, use the catch-all
	if e.CatchAll != nil {
		return e.CatchAll.Type()
	}
	return Void
}

type BoolMatch struct {
	Subject Expression
	True    *Block
	False   *Block
}

func (b *BoolMatch) Type() Type {
	return b.True.Type()
}

type IntRange struct {
	Start int
	End   int
}

type IntMatch struct {
	Subject    Expression
	IntCases   map[int]*Block      // keys are integer values
	RangeCases map[IntRange]*Block // keys are integer ranges
	CatchAll   *Block
}

func (i *IntMatch) Type() Type {
	// Find the first non-nil case and return its type
	for _, block := range i.IntCases {
		if block != nil {
			return block.Type()
		}
	}

	for _, block := range i.RangeCases {
		if block != nil {
			return block.Type()
		}
	}

	// If no int cases are defined, use the catch-all case type
	if i.CatchAll != nil {
		return i.CatchAll.Type()
	}

	return Void
}

type UnionMatch struct {
	Subject   Expression
	TypeCases map[string]*Block
	CatchAll  *Block
}

func (u *UnionMatch) Type() Type {
	// Find the first non-nil case and return its type
	for _, block := range u.TypeCases {
		if block != nil {
			return block.Type()
		}
	}

	// If no type cases are defined, use the catch-all case type
	if u.CatchAll != nil {
		return u.CatchAll.Type()
	}

	return Void
}

type FloatSubtraction struct {
	Left  Expression
	Right Expression
}

func (n *FloatSubtraction) Type() Type {
	return Float
}

type FloatMultiplication struct {
	Left  Expression
	Right Expression
}

func (n *FloatMultiplication) Type() Type {
	return Float
}

type FloatDivision struct {
	Left  Expression
	Right Expression
}

func (n *FloatDivision) Type() Type {
	return Float
}

type FloatGreater struct {
	Left  Expression
	Right Expression
}

func (n *FloatGreater) Type() Type {
	return Bool
}

type FloatGreaterEqual struct {
	Left  Expression
	Right Expression
}

func (n *FloatGreaterEqual) Type() Type {
	return Bool
}

type FloatLess struct {
	Left  Expression
	Right Expression
}

func (n *FloatLess) Type() Type {
	return Bool
}

type FloatLessEqual struct {
	Left  Expression
	Right Expression
}

func (n *FloatLessEqual) Type() Type {
	return Bool
}

type StrAddition struct {
	Left  Expression
	Right Expression
}

func (n *StrAddition) Type() Type {
	return Str
}

type Equality struct {
	Left, Right Expression
}

func (n *Equality) Type() Type {
	return Bool
}

type And struct {
	Left, Right Expression
}

func (a *And) Type() Type {
	return Bool
}

type Or struct {
	Left, Right Expression
}

func (o *Or) Type() Type {
	return Bool
}

type Block struct {
	Stmts []Statement
}

func (b *Block) Type() Type {
	if len(b.Stmts) == 0 {
		return Void
	}
	last := b.Stmts[len(b.Stmts)-1]
	if last.Expr != nil {
		return last.Expr.Type()
	}
	return Void
}

type If struct {
	Condition Expression
	Body      *Block
	ElseIf    *If
	Else      *Block
}

func (i *If) Type() Type {
	return i.Body.Type()
}

type ForIntRange struct {
	Cursor string
	Index  string
	Start  Expression
	End    Expression
	Body   *Block
}

func (f ForIntRange) NonProducing() {}

type ForInStr struct {
	Cursor string
	Index  string
	Value  Expression
	Body   *Block
}

func (f ForInStr) NonProducing() {}

type ForInList struct {
	Cursor string
	Index  string
	List   Expression
	Body   *Block
}

func (f ForInList) NonProducing() {}

type ForInMap struct {
	Key  string
	Val  string
	Map  Expression
	Body *Block
}

func (f ForInMap) NonProducing() {}

type ForLoop struct {
	Init      *VariableDef
	Condition Expression
	Update    *Reassignment
	Body      *Block
}

func (f ForLoop) NonProducing() {}

type WhileLoop struct {
	Condition Expression
	Body      *Block
}

func (w WhileLoop) NonProducing() {}

type Parameter struct {
	Name    string
	Type    Type
	Mutable bool
}

type FunctionDef struct {
	Name       string
	Parameters []Parameter
	ReturnType Type
	Mutates    bool
	Body       *Block
	Private    bool
}

func (f FunctionDef) String() string {
	paramStrs := make([]string, len(f.Parameters))
	for i := range f.Parameters {
		paramStrs[i] = f.Parameters[i].Type.String()
	}

	return fmt.Sprintf("fn (%s) %s", strings.Join(paramStrs, ","), f.ReturnType.String())
}

func (f FunctionDef) get(name string) Type { return nil }

func (f FunctionDef) name() string {
	return f.Name
}
func (f *FunctionDef) _type() Type {
	return f
}
func (f *FunctionDef) Type() Type {
	return f
}
func (f FunctionDef) equal(other Type) bool {
	// Check if it's another FunctionDef
	if otherF, ok := other.(*FunctionDef); ok {
		if len(f.Parameters) != len(otherF.Parameters) {
			return false
		}

		for i := range f.Parameters {
			if !f.Parameters[i].Type.equal(otherF.Parameters[i].Type) {
				return false
			}
		}

		return f.Mutates == otherF.Mutates && f.ReturnType.equal(otherF.ReturnType)
	}

	return false
}

func (f FunctionDef) hasTrait(trait *Trait) bool {
	return false
}
func (f *FunctionDef) hasGenerics() bool {
	for i := range f.Parameters {
		if strings.HasPrefix(f.Parameters[i].Type.String(), "$") {
			return true
		}
	}
	return strings.Contains(f.ReturnType.String(), "$")
}

type FunctionCall struct {
	Name string
	Args []Expression
	fn   *FunctionDef
}

func CreateCall(name string, args []Expression, fn FunctionDef) *FunctionCall {
	return &FunctionCall{
		Name: name,
		Args: args,
		fn:   &fn,
	}
}

func (f *FunctionCall) Type() Type {
	return f.fn.ReturnType
}

type ModuleStructInstance struct {
	Module   string
	Property *StructInstance
}

func (p *ModuleStructInstance) Type() Type {
	return p.Property._type
}

type ModuleFunctionCall struct {
	Module string
	Call   *FunctionCall
}

func (p *ModuleFunctionCall) Type() Type {
	return p.Call.Type()
}

type StaticFunctionCall struct {
	Scope *StructDef
	Call  *FunctionCall
}

func (p *StaticFunctionCall) Type() Type {
	return p.Call.Type()
}

type ModuleStaticFunctionCall struct {
	Module string
	Struct string
	Call   *FunctionCall
}

func (p *ModuleStaticFunctionCall) Type() Type {
	return p.Call.Type()
}

type Enum struct {
	Name     string
	Variants []string
	private  bool
}

func (e Enum) variant(name string) int8 {
	for i, v := range e.Variants {
		if v == name {
			return int8(i)
		}
	}
	return -1
}

func (e Enum) NonProducing() {}

func (e Enum) _type() Type {
	return e
}
func (e Enum) name() string {
	return e.Name
}

func (e Enum) Type() Type {
	return e
}
func (e Enum) String() string {
	return e.Name
}
func (e Enum) equal(other Type) bool {
	o, ok := other.(*Enum)
	if !ok {
		return false
	}
	if e.Name != o.Name {
		return false
	}
	if len(e.Variants) != len(o.Variants) {
		return false
	}
	for i := range e.Variants {
		if e.Variants[i] != o.Variants[i] {
			return false
		}
	}
	return true
}
func (e Enum) get(name string) Type { return nil }

func (e Enum) hasTrait(trait *Trait) bool {
	return false
}

type EnumVariant struct {
	enum    *Enum
	Variant int8
}

func (ev EnumVariant) Type() Type {
	return ev.enum
}

func (ev EnumVariant) hasTrait(trait *Trait) bool {
	return ev.enum.hasTrait(trait)
}

func (ev EnumVariant) String() string {
	return fmt.Sprintf("%s::%s", ev.enum.Name, ev.enum.Variants[ev.Variant])
}

type Union struct {
	Name  string
	Types []Type
}

func (u Union) NonProducing() {}
func (u Union) String() string {
	strs := make([]string, len(u.Types))
	for i, t := range u.Types {
		strs[i] = t.String()
	}
	return strings.Join(strs, "|")
}
func (u Union) get(name string) Type { return nil }

// Implement the symbol interface
func (u Union) name() string {
	return u.Name
}
func (u Union) _type() Type {
	return u
}
func (u Union) Type() Type {
	return u
}
func (u Union) equal(other Type) bool {
	if otherUnion, ok := other.(*Union); ok {
		if len(u.Types) != len(otherUnion.Types) {
			return false
		}

		// Check that all types in the union match
		for _, uType := range u.Types {
			found := slices.ContainsFunc(otherUnion.Types, uType.equal)
			if !found {
				return false
			}
		}
		return true
	}

	// Check if the other type matches any type in this union
	for _, t := range u.Types {
		if t.equal(other) {
			return true
		}
	}

	return false
}

func (u Union) hasTrait(trait *Trait) bool {
	// A union has a trait only if all of its types have that trait
	for _, t := range u.Types {
		if !t.hasTrait(trait) {
			return false
		}
	}
	return len(u.Types) > 0
}

type StructDef struct {
	Name    string
	Fields  map[string]Type
	Self    string
	Traits  []*Trait
	Private bool
	Statics map[string]*FunctionDef
}

func (def StructDef) NonProducing() {}

func (def *StructDef) name() string {
	return def.Name
}
func (def StructDef) _type() Type {
	return def
}
func (def StructDef) String() string {
	return def.name()
}
func (def StructDef) get(name string) Type {
	field, ok := def.Fields[name]
	if !ok {
		return nil
	}
	return field
}
func (def StructDef) equal(other Type) bool {
	if otherDef, ok := other.(*StructDef); ok {
		if def.Name != otherDef.Name {
			return false
		}
		if len(def.Fields) != len(otherDef.Fields) {
			return false
		}
		for name, fieldType := range def.Fields {
			if otherFieldType, ok := otherDef.Fields[name]; !ok || !fieldType.equal(otherFieldType) {
				return false
			}
		}
		return true
	}
	if o, ok := other.(*Any); ok {
		if o.actual == nil {
			return true
		}
		return def.equal(o.actual)
	}
	return false
}

func (def StructDef) hasTrait(trait *Trait) bool {
	for i := range def.Traits {
		t := def.Traits[i]
		if t.equal(trait) {
			return true
		}
	}
	return false
}

type StructInstance struct {
	Name   string
	Fields map[string]Expression
	_type  *StructDef
}

func (s StructInstance) Type() Type {
	return s._type
}

type ResultMatch struct {
	Subject Expression
	Ok      *Match
	Err     *Match
}

func (r ResultMatch) Type() Type {
	return r.Ok.Body.Type()
}

type Panic struct {
	Message Expression
	node    *ast.FunctionCall
}

func (p Panic) GetLocation() ast.Location {
	return p.node.GetLocation()
}

func (p Panic) Type() Type {
	return Void
}

type TryOp struct {
	expr Expression
	ok   Type
}

func (t TryOp) Expr() Expression {
	return t.expr
}

func (t TryOp) Type() Type {
	return t.ok
}
