package ast

import "fmt"

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

type Import struct {
	Path string
	Name string
	Location
}

func (p Import) String() string {
	return p.Name
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
	return "Float"
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

type TypeDeclaration struct {
	Location
	Name Identifier
	Type []DeclaredType
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

// impl interfaces
func (v VariableAssignment) String() string {
	return fmt.Sprintf("%v = %s", v.Target, v.Value)
}

type Parameter struct {
	Location
	Name    string
	Type    DeclaredType
	Mutable bool
}

func (p Parameter) String() string {
	return p.Name
}

type FunctionDeclaration struct {
	Location
	Name       string
	Mutates    bool
	Parameters []Parameter
	ReturnType DeclaredType
	Body       []Statement
}

func (f FunctionDeclaration) String() string {
	return fmt.Sprintf("%s(%v) %s", f.Name, f.Parameters, f.ReturnType.GetName())
}

type AnonymousFunction struct {
	Location
	Parameters []Parameter
	ReturnType DeclaredType
	Body       []Statement
}

func (a AnonymousFunction) String() string {
	return fmt.Sprintf("AnonymousFunction")
}

type StructDefinition struct {
	Location
	Name   Identifier
	Fields []StructField
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
	Target  Identifier
	Methods []FunctionDeclaration
}

type TraitDefinition struct {
	Location
	Name    Identifier
	Methods []FunctionDeclaration
}

type TraitImplementation struct {
	Location
	Trait   Identifier
	ForType Identifier
	Methods []FunctionDeclaration
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
	Properties []StructValue
}

func (s StructInstance) String() string {
	return fmt.Sprintf("StructInstance(%s)", s.Name)
}

type EnumDefinition struct {
	Location
	Name     string
	Variants []string
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
	Cursor Identifier
	Start  Expression
	End    Expression
	Body   []Statement
}

func (r RangeLoop) String() string {
	return fmt.Sprintf("for range %s..%s", r.Start, r.End)
}

type ForInLoop struct {
	Location
	Cursor   Identifier
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

type FunctionCall struct {
	Location
	Name     string
	TypeArgs []DeclaredType
	Args     []Expression
}

func (f FunctionCall) String() string {
	return fmt.Sprintf("FunctionCall(%s)", f.Name)
}

type InstanceProperty struct {
	Location
	Target   Expression
	Property Identifier
}

func (ip InstanceProperty) String() string {
	// Special case for self-reference using @
	if id, ok := ip.Target.(*Identifier); ok && id.Name == "@" {
		return fmt.Sprintf("@%s", ip.Property)
	}
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

type EnumAccess struct {
	Location
	Enum    Identifier
	Variant Identifier
}

func (m EnumAccess) String() string {
	return fmt.Sprintf("EnumAccess(%s::%s)", m.Enum, m.Variant)
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

type ListLiteral struct {
	Location
	Items []Expression
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
	Entries []MapEntry
}

func (m MapLiteral) String() string {
	return fmt.Sprintf("MapLiteral { %v }", m.Entries)
}

type MatchExpression struct {
	Location
	Subject Expression
	Cases   []MatchCase
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

type Try struct {
	keyword    Identifier
	Expression Expression
}

func (t Try) String() string {
	return fmt.Sprintf("try %s", t.Expression)
}
func (t Try) GetLocation() Location {
	return t.keyword.Location
}
