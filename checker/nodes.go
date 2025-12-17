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
	GetTypeID() TypeID
}

type StrLiteral struct {
	typeID TypeID
	Value string
}

func (s *StrLiteral) String() string {
	return fmt.Sprintf(`"%s"`, s.Value)
}
func (s *StrLiteral) Type() Type {
	return Str
}

type TemplateStr struct {
	typeID TypeID
	Chunks []Expression
}

func (t *TemplateStr) String() string {
	return "TemplateStr"
}
func (t *TemplateStr) Type() Type {
	return Str
}

type BoolLiteral struct {
	typeID TypeID
	Value bool
}

func (b *BoolLiteral) String() string {
	return strconv.FormatBool(b.Value)
}

func (b *BoolLiteral) Type() Type {
	return Bool
}

type VoidLiteral struct {
	typeID TypeID
}

func (v *VoidLiteral) String() string {
	return "()"
}

func (v *VoidLiteral) Type() Type {
	return Void
}

type IntLiteral struct {
	typeID TypeID
	Value int
}

func (i *IntLiteral) String() string {
	return strconv.Itoa(i.Value)
}

func (i *IntLiteral) Type() Type {
	return Int
}

type FloatLiteral struct {
	typeID TypeID
	Value float64
}

func (f *FloatLiteral) String() string {
	return strconv.FormatFloat(f.Value, 'g', 10, 64)
}

func (f *FloatLiteral) Type() Type {
	return Float
}

type ListLiteral struct {
	typeID TypeID
	Elements []Expression
	_type    Type
}

func (l *ListLiteral) Type() Type {
	return l._type
}

type MapLiteral struct {
	typeID TypeID
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

func (v *VariableDef) Type() Type {
	return v.__type
}

type Reassignment struct {
	Target Expression
	Value  Expression
}

func (r *Reassignment) NonProducing() {}

type Identifier struct {
	Name string
	sym  Symbol
}

func (i *Identifier) Type() Type {
	return i.sym.Type
}

type Variable struct {
	typeID TypeID
	sym Symbol
}

func (v Variable) String() string {
	return v.Name()
}
func (v Variable) Name() string {
	return v.sym.Name
}
func (v Variable) Type() Type {
	return v.sym.Type
}

type InstanceProperty struct {
	typeID TypeID
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
	typeID TypeID
	Subject Expression
	Method  *FunctionCall
}

func (i *InstanceMethod) Type() Type {
	return i.Method.Type()
}

func (i *InstanceMethod) String() string {
	return fmt.Sprintf("%s.%s", i.Subject, i.Method.Name)
}

type Negation struct {
	typeID TypeID
	Value Expression
}

func (n *Negation) String() string {
	return fmt.Sprintf("-%s", n.Value)
}
func (n *Negation) Type() Type {
	return n.Value.Type()
}

type Not struct {
	typeID TypeID
	Value Expression
}

func (n *Not) String() string {
	return fmt.Sprintf("not %s", n.Value)
}
func (n *Not) Type() Type {
	return Bool
}

type IntAddition struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *IntAddition) Type() Type {
	return Int
}

type IntSubtraction struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *IntSubtraction) Type() Type {
	return Int
}

type IntMultiplication struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *IntMultiplication) Type() Type {
	return Int
}

type IntDivision struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *IntDivision) Type() Type {
	return Int
}

type IntModulo struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *IntModulo) Type() Type {
	return Int
}

type IntGreater struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *IntGreater) Type() Type {
	return Bool
}

type IntGreaterEqual struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *IntGreaterEqual) Type() Type {
	return Bool
}

type IntLess struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *IntLess) Type() Type {
	return Bool
}

type IntLessEqual struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *IntLessEqual) Type() Type {
	return Bool
}

type FloatAddition struct {
	typeID TypeID
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
	typeID TypeID
	Subject Expression
	Some    *Match
	None    *Block
}

func (o *OptionMatch) Type() Type {
	return o.Some.Body.Type()
}

type EnumMatch struct {
	typeID TypeID
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
	typeID TypeID
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
	typeID TypeID
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
	typeID TypeID
	Subject   Expression
	TypeCases map[string]*Match
	CatchAll  *Block
}

func (u *UnionMatch) Type() Type {
	// Find the first non-nil case and return its type
	for _, block := range u.TypeCases {
		if block != nil {
			return block.Body.Type()
		}
	}

	// If no type cases are defined, use the catch-all case type
	if u.CatchAll != nil {
		return u.CatchAll.Type()
	}

	return Void
}

type ConditionalMatch struct {
	typeID TypeID
	Cases    []ConditionalCase
	CatchAll *Block
}

func (c *ConditionalMatch) Type() Type {
	if len(c.Cases) > 0 {
		return c.Cases[0].Body.Type()
	}
	if c.CatchAll != nil {
		return c.CatchAll.Type()
	}
	return Void
}

type ConditionalCase struct {
	Condition Expression // Must be type Bool
	Body      *Block
}

type FloatSubtraction struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *FloatSubtraction) Type() Type {
	return Float
}

type FloatMultiplication struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *FloatMultiplication) Type() Type {
	return Float
}

type FloatDivision struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *FloatDivision) Type() Type {
	return Float
}

type FloatGreater struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *FloatGreater) Type() Type {
	return Bool
}

type FloatGreaterEqual struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *FloatGreaterEqual) Type() Type {
	return Bool
}

type FloatLess struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *FloatLess) Type() Type {
	return Bool
}

type FloatLessEqual struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *FloatLessEqual) Type() Type {
	return Bool
}

type StrAddition struct {
	typeID TypeID
	Left  Expression
	Right Expression
}

func (n *StrAddition) Type() Type {
	return Str
}

type Equality struct {
	typeID TypeID
	Left, Right Expression
}

func (n *Equality) Type() Type {
	return Bool
}

type And struct {
	typeID TypeID
	Left, Right Expression
}

func (a *And) Type() Type {
	return Bool
}

type Or struct {
	typeID TypeID
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
	typeID    TypeID
	Condition Expression
	Body      *Block
	ElseIf    *If
	Else      *Block
}

func (i *If) Type() Type {
	return i.Body.Type()
}

func (i *If) GetTypeID() TypeID {
	return i.typeID
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
	typeID     TypeID
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

	return fmt.Sprintf("fn %s(%s) %s", f.Name, strings.Join(paramStrs, ","), f.ReturnType.String())
}

func (f FunctionDef) get(name string) Type { return nil }

func (f *FunctionDef) GetTypeID() TypeID {
	return f.typeID
}

type ExternalFunctionDef struct {
	typeID          TypeID
	Name            string
	Parameters      []Parameter
	ReturnType      Type
	ExternalBinding string
	Private         bool
}

func (e ExternalFunctionDef) String() string {
	paramStrs := make([]string, len(e.Parameters))
	for i := range e.Parameters {
		paramStrs[i] = e.Parameters[i].Type.String()
	}

	return fmt.Sprintf("extern fn (%s) %s = %q", strings.Join(paramStrs, ","), e.ReturnType.String(), e.ExternalBinding)
}

func (e ExternalFunctionDef) get(name string) Type { return nil }

func (e *ExternalFunctionDef) Type() Type {
	return e
}

func (e *ExternalFunctionDef) GetTypeID() TypeID {
	return e.typeID
}

func (e ExternalFunctionDef) equal(other Type) bool {
	// Check if it's another ExternalFunctionDef
	if otherE, ok := other.(*ExternalFunctionDef); ok {
		if len(e.Parameters) != len(otherE.Parameters) {
			return false
		}

		for i := range e.Parameters {
			if !e.Parameters[i].Type.equal(otherE.Parameters[i].Type) {
				return false
			}
		}

		return e.ReturnType.equal(otherE.ReturnType) && e.ExternalBinding == otherE.ExternalBinding
	}

	// Also check if it's compatible with a regular FunctionDef (type-wise)
	if otherF, ok := other.(*FunctionDef); ok {
		if len(e.Parameters) != len(otherF.Parameters) {
			return false
		}

		for i := range e.Parameters {
			if !e.Parameters[i].Type.equal(otherF.Parameters[i].Type) {
				return false
			}
		}

		return e.ReturnType.equal(otherF.ReturnType)
	}

	return false
}

func (e ExternalFunctionDef) hasTrait(trait *Trait) bool {
	return false
}

func (e *ExternalFunctionDef) hasGenerics() bool {
	for i := range e.Parameters {
		if strings.HasPrefix(e.Parameters[i].Type.String(), "$") {
			return true
		}
	}
	return strings.Contains(e.ReturnType.String(), "$")
}

func (f FunctionDef) name() string {
	return f.Name
}

func (f *FunctionDef) Type() Type {
	return f
}

func (f FunctionDef) equal(other Type) bool {
	if otherF, ok := other.(*ExternalFunctionDef); ok {
		if len(f.Parameters) != len(otherF.Parameters) {
			return false
		}

		for i := range f.Parameters {
			if !f.Parameters[i].Type.equal(otherF.Parameters[i].Type) {
				return false
			}
		}
		return true
	}

	otherF, ok := other.(*FunctionDef)
	if !ok {
		return false
	}

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
	typeID TypeID
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
	typeID   TypeID
	Module   string
	Property *StructInstance
}

func (p *ModuleStructInstance) Type() Type {
	return p.Property._type
}

func (p *ModuleStructInstance) GetTypeID() TypeID {
	return p.typeID
}

type ModuleFunctionCall struct {
	typeID TypeID
	Module string
	Call   *FunctionCall
}

func (p *ModuleFunctionCall) Type() Type {
	return p.Call.Type()
}

type ModuleSymbol struct {
	typeID TypeID
	Module string
	Symbol Symbol
}

func (p *ModuleSymbol) Type() Type {
	return p.Symbol.Type
}

func (p *ModuleSymbol) GetTypeID() TypeID {
	return p.typeID
}

type Enum struct {
	Name     string
	Private  bool
	Variants []string
	Methods  map[string]*FunctionDef
	Traits   []*Trait
	Location ast.Location
}

func (e Enum) NonProducing() {}

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
func (e Enum) get(name string) Type {
	if method, ok := e.Methods[name]; ok {
		return method
	}
	// Check if the enum has the ToString trait
	if name == "to_str" {
		for _, trait := range e.Traits {
			if trait.Name == "ToString" {
				return &trait.methods[0]
			}
		}
	}
	return nil
}

func (e Enum) hasTrait(trait *Trait) bool {
	for _, t := range e.Traits {
		if t.equal(trait) {
			return true
		}
	}
	return false
}

type EnumVariant struct {
	typeID  TypeID
	enum    *Enum
	Variant int8
}

func (ev *EnumVariant) Type() Type {
	return ev.enum
}

func (ev *EnumVariant) GetTypeID() TypeID {
	return ev.typeID
}

func (ev *EnumVariant) hasTrait(trait *Trait) bool {
	return ev.enum.hasTrait(trait)
}

func (ev *EnumVariant) String() string {
	return fmt.Sprintf("%s::%s", ev.enum.Name, ev.enum.Variants[ev.Variant])
}

type Union struct {
	Name  string
	Types []Type
}

func (u Union) NonProducing() {}
func (u Union) String() string {
	return u.Name
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
	Methods map[string]*FunctionDef
	Self    string
	Traits  []*Trait
	Private bool
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
	// Check data fields first
	if field, ok := def.Fields[name]; ok {
		return field
	}
	// Check methods
	if method, ok := def.Methods[name]; ok {
		return method
	}
	return nil
}
func (def StructDef) equal(other Type) bool {
	if otherDef, ok := other.(*StructDef); ok {
		if def.Name != otherDef.Name {
			return false
		}
		if len(def.Fields) != len(otherDef.Fields) {
			return false
		}
		if len(def.Methods) != len(otherDef.Methods) {
			return false
		}
		for name, fieldType := range def.Fields {
			if otherFieldType, ok := otherDef.Fields[name]; !ok || !fieldType.equal(otherFieldType) {
				return false
			}
		}
		for name, methodType := range def.Methods {
			if otherMethodType, ok := otherDef.Methods[name]; !ok || !methodType.equal(otherMethodType) {
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
	typeID TypeID
	Name   string
	Fields map[string]Expression
	_type  *StructDef
}

func (s StructInstance) Type() Type {
	return s._type
}

type ResultMatch struct {
	typeID TypeID
	Subject Expression
	Ok      *Match
	Err     *Match
}

func (r ResultMatch) Type() Type {
	return r.Ok.Body.Type()
}

type Panic struct {
	typeID TypeID
	Message Expression
	node    *ast.FunctionCall
}

func (p Panic) GetLocation() ast.Location {
	return p.node.GetLocation()
}

func (p Panic) Type() Type {
	// realistically, this is Void but that would break expectations when using `panic()` to signal unreachable code
	// in a function or block that is declared to return or be a non-Void value.
	// using Any technically allows empty panicking functions to work; e.g. the `async:start()` function in async.ard
	return &Any{name: "Unreachable"}
}

type TryOp struct {
	typeID TypeID
	expr       Expression
	ok         Type
	CatchBlock *Block
	CatchVar   string
}

func (t TryOp) Expr() Expression {
	return t.expr
}

func (t TryOp) Type() Type {
	return t.ok
}

type CopyExpression struct {
	typeID TypeID
	Expr  Expression
	Type_ Type
}

func (c *CopyExpression) Type() Type {
	return c.Type_
}

// GetTypeID implementations for all Expression types
// Phase 4: Providing access to typeID for registry lookups

func (s *StrLiteral) GetTypeID() TypeID {
	return s.typeID
}

func (t *TemplateStr) GetTypeID() TypeID {
	return t.typeID
}

func (b *BoolLiteral) GetTypeID() TypeID {
	return b.typeID
}

func (v *VoidLiteral) GetTypeID() TypeID {
	return v.typeID
}

func (i *IntLiteral) GetTypeID() TypeID {
	return i.typeID
}

func (f *FloatLiteral) GetTypeID() TypeID {
	return f.typeID
}

func (l *ListLiteral) GetTypeID() TypeID {
	return l.typeID
}

func (m *MapLiteral) GetTypeID() TypeID {
	return m.typeID
}

func (v *Variable) GetTypeID() TypeID {
	return v.typeID
}

func (i *InstanceProperty) GetTypeID() TypeID {
	return i.typeID
}

func (i *InstanceMethod) GetTypeID() TypeID {
	return i.typeID
}

func (n *Negation) GetTypeID() TypeID {
	return n.typeID
}

func (n *Not) GetTypeID() TypeID {
	return n.typeID
}

func (i *IntAddition) GetTypeID() TypeID {
	return i.typeID
}

func (i *IntSubtraction) GetTypeID() TypeID {
	return i.typeID
}

func (i *IntMultiplication) GetTypeID() TypeID {
	return i.typeID
}

func (i *IntDivision) GetTypeID() TypeID {
	return i.typeID
}

func (i *IntModulo) GetTypeID() TypeID {
	return i.typeID
}

func (i *IntGreater) GetTypeID() TypeID {
	return i.typeID
}

func (i *IntGreaterEqual) GetTypeID() TypeID {
	return i.typeID
}

func (i *IntLess) GetTypeID() TypeID {
	return i.typeID
}

func (i *IntLessEqual) GetTypeID() TypeID {
	return i.typeID
}

func (f *FloatAddition) GetTypeID() TypeID {
	return f.typeID
}

func (f *FloatSubtraction) GetTypeID() TypeID {
	return f.typeID
}

func (f *FloatMultiplication) GetTypeID() TypeID {
	return f.typeID
}

func (f *FloatDivision) GetTypeID() TypeID {
	return f.typeID
}

func (f *FloatGreater) GetTypeID() TypeID {
	return f.typeID
}

func (f *FloatGreaterEqual) GetTypeID() TypeID {
	return f.typeID
}

func (f *FloatLess) GetTypeID() TypeID {
	return f.typeID
}

func (f *FloatLessEqual) GetTypeID() TypeID {
	return f.typeID
}

func (s *StrAddition) GetTypeID() TypeID {
	return s.typeID
}

func (e *Equality) GetTypeID() TypeID {
	return e.typeID
}

func (a *And) GetTypeID() TypeID {
	return a.typeID
}

func (o *Or) GetTypeID() TypeID {
	return o.typeID
}

func (o *OptionMatch) GetTypeID() TypeID {
	return o.typeID
}

func (e *EnumMatch) GetTypeID() TypeID {
	return e.typeID
}

func (b *BoolMatch) GetTypeID() TypeID {
	return b.typeID
}

func (i *IntMatch) GetTypeID() TypeID {
	return i.typeID
}

func (u *UnionMatch) GetTypeID() TypeID {
	return u.typeID
}

func (c *ConditionalMatch) GetTypeID() TypeID {
	return c.typeID
}

func (f *FunctionCall) GetTypeID() TypeID {
	return f.typeID
}

func (m *ModuleFunctionCall) GetTypeID() TypeID {
	return m.typeID
}

func (s *StructInstance) GetTypeID() TypeID {
	return s.typeID
}

func (r *ResultMatch) GetTypeID() TypeID {
	return r.typeID
}

func (p *Panic) GetTypeID() TypeID {
	return p.typeID
}

func (t *TryOp) GetTypeID() TypeID {
	return t.typeID
}

func (c *CopyExpression) GetTypeID() TypeID {
	return c.typeID
}
