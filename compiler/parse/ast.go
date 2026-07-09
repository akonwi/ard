package parse

import (
	"fmt"
	"strings"
)

func renderTypeParams(params []string) string {
	if len(params) == 0 {
		return ""
	}
	formatted := make([]string, len(params))
	for i, param := range params {
		formatted[i] = "$" + param
	}
	return "<" + strings.Join(formatted, ", ") + ">"
}

type Statement interface {
	String() string
	GetLocation() Location
}

type Expression interface {
	Statement
}

type Point struct {
	Row int
	Col int
}

func (p Point) String() string {
	return fmt.Sprintf("[%d:%d]", p.Row, p.Col)
}

type Location struct {
	Start Point
	End   Point
}

func (l Location) String() string {
	return l.Start.String() + "-" + l.End.String()
}

func (l Location) GetLocation() Location {
	return l
}

func (l Location) GetStart() Point {
	return l.Start
}

type ImportKind string

const (
	ImportKindModule ImportKind = ""
	ImportKindGo     ImportKind = "go"
)

type Import struct {
	Path string
	Name string
	Kind ImportKind
	Location
}

func (p Import) String() string {
	return p.Name
}

// Alias returns the name the import is referenced by: the explicit alias
// when present, otherwise the final path segment.
func (p Import) Alias() string {
	if p.Name != "" {
		return p.Name
	}
	if idx := strings.LastIndex(p.Path, "/"); idx >= 0 {
		return p.Path[idx+1:]
	}
	return p.Path
}

type Program struct {
	Imports    []Import
	Statements []Statement
}

type Break struct{ Location }

func (b Break) String() string {
	return "break"
}

type Comment struct {
	Location
	Value string
}

func (c Comment) String() string {
	return fmt.Sprintf("Comment(%s)", c.Value)
}

type VariableDeclaration struct {
	Location
	Name    string
	Mutable bool
	Value   Expression
	Type    DeclaredType
}

type DeclaredType interface {
	GetName() string
	IsNullable() bool
	GetLocation() Location
}

type StringType struct {
	Location
	nullable bool
}

func (v StringType) IsNullable() bool {
	return v.nullable
}

func (s StringType) GetName() string {
	return "String"
}

type MutableType struct {
	Location
	Inner    DeclaredType
	nullable bool
}

func (m MutableType) IsNullable() bool {
	return m.nullable
}

func (m MutableType) GetName() string {
	if m.Inner == nil {
		return "mut"
	}
	return m.Inner.GetName()
}

type FunctionType struct {
	Location
	Nullable        bool
	Params          []DeclaredType
	ParamMutability []bool
	Return          DeclaredType
}

func (f FunctionType) IsNullable() bool {
	return f.Nullable
}

func (f FunctionType) GetName() string {
	return "Function"
}

type IntType struct {
	Location
	nullable bool
}

func (s IntType) GetName() string {
	return "Int"
}

func (v IntType) IsNullable() bool {
	return v.nullable
}

type FloatType struct {
	Location
	nullable bool
}

func (f FloatType) GetName() string {
	return "Float64"
}
func (f FloatType) IsNullable() bool {
	return f.nullable
}

type BooleanType struct {
	Location
	nullable bool
}

func (s BooleanType) GetName() string {
	return "Boolean"
}

func (v BooleanType) IsNullable() bool {
	return v.nullable
}

type VoidType struct {
	Location
	nullable bool
}

func (v VoidType) GetName() string {
	return "Void"
}

func (v VoidType) IsNullable() bool {
	return v.nullable
}

type TypeDeclaration struct {
	Location
	Name    Identifier
	Type    []DeclaredType
	Private bool
}

func (t TypeDeclaration) String() string {
	return fmt.Sprintf("TypeDeclaration(%s)", t.Name)
}

type List struct {
	Location
	Element  DeclaredType
	nullable bool
}

func (s List) GetName() string {
	return "List"
}

func (v List) IsNullable() bool {
	return v.nullable
}

type FixedArray struct {
	Location
	Element  DeclaredType
	Length   int
	nullable bool
}

func (s FixedArray) GetName() string {
	return "FixedArray"
}

func (v FixedArray) IsNullable() bool {
	return v.nullable
}

type Map struct {
	Location
	Key      DeclaredType
	Value    DeclaredType
	nullable bool
}

func (s Map) GetName() string {
	return "Map"
}

func (v Map) IsNullable() bool {
	return v.nullable
}

type CustomType struct {
	Location
	Name     string
	Type     StaticProperty
	nullable bool
	Private  bool
	TypeArgs []DeclaredType
}

func (u CustomType) GetName() string {
	return u.Name
}

func (u CustomType) IsNullable() bool {
	return u.nullable
}
func (u CustomType) String() string {
	return u.Name
}

type GenericType struct {
	Location
	Name     string
	nullable bool
}

func (g GenericType) GetName() string {
	return g.Name
}

func (g GenericType) IsNullable() bool {
	return g.nullable
}

func (g GenericType) String() string {
	return g.Name
}

type ResultType struct {
	Location
	Val, Err DeclaredType
	nullable bool
}

func (r ResultType) GetName() string {
	return "Result"
}
func (r ResultType) IsNullable() bool {
	return r.nullable
}

func (v VariableDeclaration) String() string {
	binding := "let"
	if v.Mutable {
		binding = "mut"
	}
	return fmt.Sprintf("%s %s: %s", binding, v.Name, v.Type)
}

type VariableAssignment struct {
	Location
	Target   Expression
	Operator Operator
	Value    Expression
}

type Defer struct {
	Location
	Expr Expression
	Body []Statement
}

func (d Defer) String() string {
	if d.Expr != nil {
		return fmt.Sprintf("defer %s", d.Expr)
	}
	return "defer { ... }"
}

// impl interfaces
func (v VariableAssignment) String() string {
	return fmt.Sprintf("%v = %s", v.Target, v.Value)
}

type Parameter struct {
	Location
	Name string
	Type DeclaredType
}

func (p Parameter) String() string {
	return p.Name
}

type FunctionDeclaration struct {
	Location
	Name       string
	TypeParams []string // Legacy/constructed generic parameter metadata; source function declaration lists are rejected.
	Mutates    bool
	IsTest     bool
	Parameters []Parameter
	ReturnType DeclaredType
	Body       []Statement
	Private    bool
	Comments   []Comment // Comments found within the function declaration
}

func (f FunctionDeclaration) String() string {
	return fmt.Sprintf("%s(%v) %s", f.Name, f.Parameters, f.ReturnType.GetName())
}

type StaticFunctionDeclaration struct {
	FunctionDeclaration
	Path StaticProperty
}

func (f StaticFunctionDeclaration) String() string {
	return fmt.Sprintf("%s(%v) %s", f.Path, f.Parameters, f.ReturnType.GetName())
}

type AnonymousFunction struct {
	Location
	Parameters []Parameter
	ReturnType DeclaredType
	Body       []Statement
}

func (a AnonymousFunction) String() string {
	return "AnonymousFunction"
}

type StructDefinition struct {
	Location
	Name       Identifier
	TypeParams []string
	Fields     []StructField
	Private    bool
	Comments   []Comment // Comments found within the struct definition
}

type StructField struct {
	Name Identifier
	Type DeclaredType
}

func (s StructDefinition) String() string {
	return fmt.Sprintf("StructDefinition(%s)", s.Name)
}

type ImplBlock struct {
	Location
	Target   Identifier
	Receiver Identifier
	Methods  []FunctionDeclaration
	Comments []Comment // Comments found within the impl block
}

type TraitDefinition struct {
	Location
	Name     Identifier
	Methods  []FunctionDeclaration
	Private  bool
	Comments []Comment // Comments found within the trait definition
}

type TraitImplementation struct {
	Location
	Trait    Expression // Identifier | StaticProperty
	ForType  Identifier
	Receiver Identifier
	Methods  []FunctionDeclaration
}

func (i ImplBlock) String() string {
	return fmt.Sprintf("ImplBlock(%s)", i.Target)
}

func (t TraitDefinition) String() string {
	return fmt.Sprintf("TraitDefinition(%s)", t.Name)
}

func (t TraitImplementation) String() string {
	return fmt.Sprintf("TraitImplementation(%s for %s)", t.Trait, t.ForType)
}

type StructValue struct {
	Location
	Name  Identifier
	Value Expression
}

type StructInstance struct {
	Location
	Name       Identifier
	TypeArgs   []DeclaredType
	Properties []StructValue
	Comments   []Comment // Comments found within the struct instance
}

func (s StructInstance) String() string {
	return fmt.Sprintf("StructInstance(%s)", s.Name)
}

type EnumVariant struct {
	Name  string
	Value Expression // nil means auto-assign (0 or previous+1)
}

type EnumDefinition struct {
	Location
	Name string
	// NameLocation is the span of the enum's name token, for tooling that
	// anchors on the declared name rather than the whole declaration.
	NameLocation Location
	Variants     []EnumVariant
	Private      bool
	Comments     []Comment // Comments found within the enum definition
}

func (e EnumDefinition) String() string {
	return fmt.Sprintf("EnumDefinition(%s)", e.Name)
}

type WhileLoop struct {
	Location
	Condition Expression
	Body      []Statement
}

func (w WhileLoop) String() string {
	return "while"
}

type RangeLoop struct {
	Location
	Cursor  Identifier
	Cursor2 Identifier
	Start   Expression
	End     Expression
	Body    []Statement
}

func (r RangeLoop) String() string {
	return fmt.Sprintf("for range %s..%s", r.Start, r.End)
}

type ForInLoop struct {
	Location
	Cursor   Identifier
	Cursor2  Identifier
	Iterable Expression
	Body     []Statement
}

func (f ForInLoop) String() string {
	return fmt.Sprintf("for %s", f.Iterable)
}

type ForLoop struct {
	Location
	Init        *VariableDeclaration
	Condition   Expression
	Incrementer Statement
	Body        []Statement
}

func (f ForLoop) String() string {
	return fmt.Sprintf("for %s", f.Init)
}

type IfStatement struct {
	Location
	Condition Expression
	Body      []Statement
	Else      Statement
}

func (i IfStatement) String() string {
	return "IfStatement"
}

type Argument struct {
	Location
	Name  string
	Value Expression
}

// MutRef is the explicit mutable-reference expression `mut <operand>`
// (ADR 0045). It evaluates to a mutable reference to the operand's storage,
// or to fresh mutable storage when the operand is a value expression.
type MutRef struct {
	Location
	Operand Expression
}

func (m MutRef) String() string {
	return fmt.Sprintf("mut %s", m.Operand)
}

type FunctionCall struct {
	Location
	Name     string
	TypeArgs []DeclaredType
	Args     []Argument
	Comments []Comment // Comments found within the function call
}

func (f FunctionCall) String() string {
	return fmt.Sprintf("FunctionCall(%s)", f.Name)
}

type FunctionValueCall struct {
	Location
	Callee   Expression
	TypeArgs []DeclaredType
	Args     []Argument
	Comments []Comment // Comments found within the function call
}

func (f FunctionValueCall) String() string {
	return fmt.Sprintf("FunctionValueCall(%s)", f.Callee)
}

type InstanceProperty struct {
	Location
	Target   Expression
	Property Identifier
}

func (ip InstanceProperty) String() string {
	return fmt.Sprintf("%s.%s", ip.Target, ip.Property)
}

type InstanceMethod struct {
	Location
	Target Expression
	Method FunctionCall
}

func (im InstanceMethod) String() string {
	return fmt.Sprintf("%s.%s", im.Target, im.Method)
}

type StaticProperty struct {
	Location
	Target   Expression
	Property Expression
}

func (s StaticProperty) String() string {
	return fmt.Sprintf("%s::%s", s.Target, s.Property)
}

type StaticFunction struct {
	Location
	Target   Expression
	Function FunctionCall
}

func (s StaticFunction) String() string {
	return fmt.Sprintf("%s::%s", s.Target, s.Function)
}

type Operator int

const (
	InvalidOp Operator = iota
	Bang
	Minus
	Decrement
	Plus
	Increment
	Divide
	Multiply
	Modulo
	GreaterThan
	GreaterThanOrEqual
	LessThan
	LessThanOrEqual
	Equal
	NotEqual
	And
	Not
	Or
	Range
	Assign
)

type UnaryExpression struct {
	Location
	Operator Operator
	Operand  Expression
}

// impl interfaces
func (u UnaryExpression) String() string {
	return fmt.Sprintf("(%v %v)", u.Operator, u.Operand)
}

type BinaryExpression struct {
	Location
	Operator    Operator
	Left, Right Expression
}

func (b BinaryExpression) String() string {
	return fmt.Sprintf("(%v %v %v)", b.Left, b.Operator, b.Right)
}

type ChainedComparison struct {
	Location
	Operands  []Expression // [a, b, c] for `a op1 b op2 c`
	Operators []Operator   // [op1, op2] for `a op1 b op2 c`
}

func (c ChainedComparison) String() string {
	return fmt.Sprintf("ChainedComparison(%v %v)", c.Operands, c.Operators)
}

type RangeExpression struct {
	Location
	Start, End Expression
}

func (b RangeExpression) String() string {
	return "RangeExpression"
}

type Identifier struct {
	Location
	Name string
}

func (i Identifier) String() string {
	return fmt.Sprintf("%s", i.Name)
}

type StrLiteral struct {
	Location
	Value string
}

func (s StrLiteral) String() string {
	return s.Value
}

type RuneLiteral struct {
	Location
	Value string
}

func (r RuneLiteral) String() string {
	return r.Value
}

type InterpolatedStr struct {
	Location
	Chunks []Expression
}

func (i InterpolatedStr) String() string {
	return "InterpolatedStr"
}

type NumLiteral struct {
	Location
	Value string
}

func (n NumLiteral) String() string {
	return n.Value
}

type BoolLiteral struct {
	Location
	Value bool
}

// impl interfaces
func (b BoolLiteral) String() string {
	return fmt.Sprintf("%t", b.Value)
}

type VoidLiteral struct {
	Location
}

func (v VoidLiteral) String() string {
	return "()"
}

type ListLiteral struct {
	Location
	Items    []Expression
	Comments []Comment // Comments found within the list literal
}

func (l ListLiteral) String() string {
	return "ListLiteral"
}

type MapEntry struct {
	Key   Expression
	Value Expression
}

type MapLiteral struct {
	Location
	Entries  []MapEntry
	Comments []Comment // Comments found within the map literal
}

func (m MapLiteral) String() string {
	return fmt.Sprintf("MapLiteral { %v }", m.Entries)
}

type MatchExpression struct {
	Location
	Subject  Expression
	Cases    []MatchCase
	Comments []Comment // Comments found within the match expression
}

func (m MatchExpression) String() string {
	return fmt.Sprintf("MatchExpression(%s)", m.Subject)
}

type MatchCase struct {
	Location
	Pattern Expression
	Body    []Statement
}

func (m MatchCase) String() string {
	return fmt.Sprintf("MatchCase(%s)", m.Pattern)
}

// SelectExpression multiplexes over several channel operations, running the
// arm whose operation can proceed first. See ADR 0032.
type SelectExpression struct {
	Location
	Cases    []SelectCase
	Comments []Comment
}

func (s SelectExpression) String() string {
	return "SelectExpression"
}

// SelectCase is one arm of a select. Op is the channel operation expression
// (e.g. `ch.recv()`, `ch.send(x)`) or the `_` identifier for the default arm.
// Binding is the optional `let name =` capture, valid only on receive arms.
type SelectCase struct {
	Location
	Binding *Identifier
	Op      Expression
	Body    []Statement
}

func (s SelectCase) String() string {
	return fmt.Sprintf("SelectCase(%s)", s.Op)
}

type ConditionalMatchExpression struct {
	Location
	Cases    []ConditionalMatchCase
	Comments []Comment // Comments found within the match expression
}

func (c ConditionalMatchExpression) String() string {
	return "ConditionalMatchExpression"
}

type ConditionalMatchCase struct {
	Location
	Condition Expression // The boolean condition to evaluate (nil for catch-all case)
	Body      []Statement
}

func (c ConditionalMatchCase) String() string {
	if c.Condition == nil {
		return "ConditionalMatchCase(_)"
	}
	return fmt.Sprintf("ConditionalMatchCase(%s)", c.Condition)
}

type Try struct {
	Location
	keyword    Identifier
	Expression Expression
	CatchVar   *Identifier // nil if no catch clause
	CatchBlock []Statement // nil if no catch clause
}

func (t Try) String() string {
	if t.CatchVar != nil {
		return fmt.Sprintf("try %s -> %s { ... }", t.Expression, t.CatchVar.Name)
	}
	return fmt.Sprintf("try %s", t.Expression)
}

type BlockExpression struct {
	Location
	Statements []Statement
}

func (b BlockExpression) String() string {
	return "BlockExpression"
}

type UnsafeBlock struct {
	Location
	Statements []Statement
}

func (u UnsafeBlock) String() string {
	return "unsafe { ... }"
}
