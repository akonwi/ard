package checker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/akonwi/ard/parse"
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

type RuneLiteral struct {
	Value rune
}

func (r *RuneLiteral) String() string {
	return strconv.QuoteRune(r.Value)
}
func (r *RuneLiteral) Type() Type {
	return Rune
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

type VoidLiteral struct{}

func (v *VoidLiteral) String() string {
	return "()"
}

func (v *VoidLiteral) Type() Type {
	return Void
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
	return Float64
}

type ListLiteral struct {
	Elements []Expression
	_type    Type
	ListType Type // Pre-computed by checker
}

func (l *ListLiteral) Type() Type {
	return l._type
}

type MapLiteral struct {
	Keys      []Expression
	Values    []Expression
	_type     Type
	KeyType   Type // Pre-computed by checker
	ValueType Type // Pre-computed by checker
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

func (r *Reassignment) Type() Type {
	return Void
}

type Identifier struct {
	Name string
	sym  Symbol
}

func (i *Identifier) Type() Type {
	return i.sym.Type
}

type Variable struct {
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

type SubjectKind uint8

const (
	StructSubject SubjectKind = iota
)

type InstanceProperty struct {
	Subject  Expression
	Property string
	_type    Type
	Kind     SubjectKind // Pre-computed by checker based on subject type
}

func (i *InstanceProperty) Type() Type {
	return derefMutableRef(i._type)
}

// String returns a string representation of the instance property
func (i *InstanceProperty) String() string {
	return fmt.Sprintf("%s.%s", i.Subject, i.Property)
}

type InstanceMethod struct {
	Subject      Expression
	Method       *FunctionCall
	ReceiverKind InstanceReceiverKind
	StructType   *StructDef
	EnumType     *Enum
	TraitType    *Trait
}

func (i *InstanceMethod) Type() Type {
	return i.Method.Type()
}

func (i *InstanceMethod) String() string {
	return fmt.Sprintf("%s.%s", i.Subject, i.Method.Name)
}

type InstanceReceiverKind uint8

const (
	ReceiverUnknown InstanceReceiverKind = iota
	ReceiverStruct
	ReceiverEnum
	ReceiverTrait
)

// Primitive method types with enum-based dispatch

type StrMethodKind uint8

const (
	StrSize StrMethodKind = iota
	StrAt
	StrBytes
	StrRunes
	StrIsEmpty
	StrContains
	StrReplace
	StrReplaceAll
	StrSplit
	StrStartsWith
	StrEndsWith
	StrToStr
	StrTrim
)

type StrMethod struct {
	Subject Expression
	Kind    StrMethodKind
	Args    []Expression
}

func (s *StrMethod) Type() Type {
	switch s.Kind {
	case StrSize:
		return Int
	case StrAt:
		return MakeMaybe(Rune)
	case StrBytes:
		return MakeList(Byte)
	case StrRunes:
		return MakeList(Rune)
	case StrIsEmpty:
		return Bool
	case StrContains:
		return Bool
	case StrReplace, StrReplaceAll:
		return Str
	case StrStartsWith, StrEndsWith:
		return Bool
	case StrToStr:
		return Str
	case StrTrim:
		return Str
	default:
		return Void
	}
}

type ByteMethodKind uint8

const (
	ByteToInt ByteMethodKind = iota
	ByteToStr
)

type ByteMethod struct {
	Subject Expression
	Kind    ByteMethodKind
}

func (m *ByteMethod) Type() Type {
	switch m.Kind {
	case ByteToInt:
		return Int
	case ByteToStr:
		return Str
	default:
		return Void
	}
}

type RuneMethodKind uint8

const (
	RuneToInt RuneMethodKind = iota
	RuneToStr
)

type RuneMethod struct {
	Subject Expression
	Kind    RuneMethodKind
}

func (m *RuneMethod) Type() Type {
	switch m.Kind {
	case RuneToInt:
		return Int
	case RuneToStr:
		return Str
	default:
		return Void
	}
}

type IntMethodKind uint8

const (
	IntToStr IntMethodKind = iota
)

type IntMethod struct {
	Subject Expression
	Kind    IntMethodKind
}

func (m *IntMethod) Type() Type {
	switch m.Kind {
	case IntToStr:
		return Str
	default:
		return Void
	}
}

type FloatMethodKind uint8

const (
	FloatToStr FloatMethodKind = iota
	FloatToInt
)

type FloatMethod struct {
	Subject Expression
	Kind    FloatMethodKind
}

func (m *FloatMethod) Type() Type {
	switch m.Kind {
	case FloatToStr:
		return Str
	case FloatToInt:
		return Int
	default:
		return Void
	}
}

type BoolMethodKind uint8

const (
	BoolToStr BoolMethodKind = iota
)

type BoolMethod struct {
	Subject Expression
	Kind    BoolMethodKind
}

func (m *BoolMethod) Type() Type {
	switch m.Kind {
	case BoolToStr:
		return Str
	default:
		return Void
	}
}

// Collection method types with enum-based dispatch

type ListMethodKind uint8

const (
	ListAt ListMethodKind = iota
	ListPrepend
	ListPush
	ListSet
	ListSize
	ListSort
	ListSwap
)

type ListMethod struct {
	Subject     Expression
	Kind        ListMethodKind
	Args        []Expression
	ElementType Type         // Pre-computed element type
	fn          *FunctionDef // Function definition for return type resolution
}

func (m *ListMethod) Type() Type {
	// Use function return type if available (handles generics properly)
	if m.fn != nil {
		return m.fn.ReturnType
	}
	// Fallback to computed type (for backwards compatibility)
	switch m.Kind {
	case ListAt:
		return m.ElementType
	case ListPrepend, ListPush:
		return MakeList(m.ElementType)
	case ListSet:
		return Bool
	case ListSize:
		return Int
	case ListSort, ListSwap:
		return Void
	default:
		return Void
	}
}

type MapMethodKind uint8

const (
	MapKeys MapMethodKind = iota
	MapSize
	MapGet
	MapSet
	MapDrop
	MapHas
)

type MapMethod struct {
	Subject   Expression
	Kind      MapMethodKind
	Args      []Expression
	KeyType   Type         // Pre-computed key type
	ValueType Type         // Pre-computed value type
	fn        *FunctionDef // Function definition for return type resolution
}

func (m *MapMethod) Type() Type {
	// Use function return type if available (handles generics properly)
	if m.fn != nil {
		return m.fn.ReturnType
	}
	// Fallback to computed type (for backwards compatibility)
	switch m.Kind {
	case MapKeys:
		return MakeList(m.KeyType)
	case MapSize:
		return Int
	case MapGet:
		return MakeMaybe(m.ValueType)
	case MapSet:
		return Bool
	case MapDrop:
		return Void
	case MapHas:
		return Bool
	default:
		return Void
	}
}

type MaybeMethodKind uint8

const (
	MaybeExpect MaybeMethodKind = iota
	MaybeIsNone
	MaybeIsSome
	MaybeOr
	MaybeMap
	MaybeAndThen
)

type MaybeMethod struct {
	Subject    Expression
	Kind       MaybeMethodKind
	Args       []Expression
	InnerType  Type         // Pre-computed inner type
	fn         *FunctionDef // Function definition for return type resolution
	ReturnType Type         // Pre-computed by checker
}

func (m *MaybeMethod) Type() Type {
	if m.ReturnType != nil {
		return m.ReturnType
	}
	if m.fn != nil {
		return m.fn.ReturnType
	}
	switch m.Kind {
	case MaybeExpect, MaybeOr:
		return m.InnerType
	case MaybeIsNone, MaybeIsSome:
		return Bool
	default:
		return Void
	}
}

type ResultMethodKind uint8

const (
	ResultExpect ResultMethodKind = iota
	ResultOr
	ResultIsOk
	ResultIsErr
	ResultMap
	ResultMapErr
	ResultAndThen
)

type ResultMethod struct {
	Subject    Expression
	Kind       ResultMethodKind
	Args       []Expression
	OkType     Type         // Pre-computed OK type
	ErrType    Type         // Pre-computed Error type
	fn         *FunctionDef // Function definition for return type resolution
	ReturnType Type         // Pre-computed by checker
}

func (m *ResultMethod) Type() Type {
	// Use function return type if available (handles generics properly)
	if m.ReturnType != nil {
		return m.ReturnType
	}
	if m.fn != nil {
		return m.fn.ReturnType
	}
	// Fallback to computed type (for backwards compatibility)
	switch m.Kind {
	case ResultExpect, ResultOr:
		return m.OkType
	case ResultIsOk, ResultIsErr:
		return Bool
	default:
		return Void
	}
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
	return n.Left.Type()
}

type IntSubtraction struct {
	Left  Expression
	Right Expression
}

func (n *IntSubtraction) Type() Type {
	return n.Left.Type()
}

type IntMultiplication struct {
	Left  Expression
	Right Expression
}

func (n *IntMultiplication) Type() Type {
	return n.Left.Type()
}

type IntDivision struct {
	Left  Expression
	Right Expression
}

func (n *IntDivision) Type() Type {
	return n.Left.Type()
}

type IntModulo struct {
	Left  Expression
	Right Expression
}

func (n *IntModulo) Type() Type {
	return n.Left.Type()
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
	return n.Left.Type()
}

type Match struct {
	Pattern *Identifier
	Body    *Block
}

type OptionMatch struct {
	Subject    Expression
	Some       *Match
	None       *Block
	InnerType  Type // Pre-computed inner type of Maybe
	ResultType Type
}

func (o *OptionMatch) Type() Type {
	if o.ResultType != nil {
		return o.ResultType
	}
	return o.Some.Body.Type()
}

type EnumMatch struct {
	Subject             Expression
	Cases               []*Block
	CatchAll            *Block
	DiscriminantToIndex map[int]int // Pre-computed discriminant lookup
	ResultType          Type
}

func (e *EnumMatch) Type() Type {
	if e.ResultType != nil {
		return e.ResultType
	}
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
	Subject    Expression
	True       *Block
	False      *Block
	ResultType Type
}

func (b *BoolMatch) Type() Type {
	if b.ResultType != nil {
		return b.ResultType
	}
	return b.True.Type()
}

// SelectArmKind distinguishes the channel-multiplexing arm forms (ADR 0032).
type SelectArmKind int

const (
	SelectArmRecv SelectArmKind = iota
	SelectArmSend
	SelectArmDefault
)

// SelectArm is one arm of a Select. For recv/send arms Channel is the channel
// expression and ElemType is its element type. Recv arms may carry a Binding
// (`let name = ch.recv()`) that binds `ElemType?` in the arm body. Send arms
// carry the Value to send. The default arm has Kind SelectArmDefault.
type SelectArm struct {
	Kind     SelectArmKind
	Channel  Expression
	Binding  *Identifier
	ElemType Type
	Value    Expression
	Body     *Block
}

type Select struct {
	Arms       []SelectArm
	ResultType Type
}

func (s *Select) Type() Type {
	if s.ResultType != nil {
		return s.ResultType
	}
	return Void
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
	ResultType Type
}

type StrMatch struct {
	Subject    Expression
	Cases      map[string]*Block
	CatchAll   *Block
	ResultType Type
}

func (s *StrMatch) Type() Type {
	if s.ResultType != nil {
		return s.ResultType
	}
	for _, block := range s.Cases {
		if block != nil {
			return block.Type()
		}
	}
	if s.CatchAll != nil {
		return s.CatchAll.Type()
	}
	return Void
}

func (i *IntMatch) Type() Type {
	if i.ResultType != nil {
		return i.ResultType
	}
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
	Subject         Expression
	TypeCases       map[string]*Match
	TypeCasesByType map[Type]*Match // Pre-computed type lookup
	CatchAll        *Block
	ResultType      Type
}

func (u *UnionMatch) Type() Type {
	if u.ResultType != nil {
		return u.ResultType
	}
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
	Cases      []ConditionalCase
	CatchAll   *Block
	ResultType Type
}

func (c *ConditionalMatch) Type() Type {
	if c.ResultType != nil {
		return c.ResultType
	}
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
	Left  Expression
	Right Expression
}

func (n *FloatSubtraction) Type() Type {
	return n.Left.Type()
}

type FloatMultiplication struct {
	Left  Expression
	Right Expression
}

func (n *FloatMultiplication) Type() Type {
	return n.Left.Type()
}

type FloatDivision struct {
	Left  Expression
	Right Expression
}

func (n *FloatDivision) Type() Type {
	return n.Left.Type()
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

type Inequality struct {
	Left, Right Expression
}

func (n *Inequality) Type() Type {
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
	Stmts             []Statement
	DiscardFinalValue bool
}

func (b *Block) Type() Type {
	if b.DiscardFinalValue {
		return Void
	}
	if len(b.Stmts) == 0 {
		return Void
	}
	// Find the last non-empty statement (skip trailing empty statements from blank lines)
	for i := len(b.Stmts) - 1; i >= 0; i-- {
		if b.Stmts[i].Expr != nil {
			return b.Stmts[i].Expr.Type()
		}
	}
	return Void
}

type UnsafeBlock struct {
	Body       *Block
	ValueType  Type
	ResultType *Result
}

func (u *UnsafeBlock) Type() Type {
	if u.ResultType != nil {
		return u.ResultType
	}
	valueType := u.ValueType
	if valueType == nil && u.Body != nil {
		valueType = u.Body.Type()
	}
	if valueType == nil {
		valueType = Void
	}
	return MakeResult(valueType, Str)
}

func (u *UnsafeBlock) OkType() Type {
	if u.ValueType != nil {
		return u.ValueType
	}
	if u.Body != nil {
		return u.Body.Type()
	}
	return Void
}

type IfBranch struct {
	Condition Expression
	Body      *Block
}

type If struct {
	Branches []IfBranch
	Else     *Block
}

func (i *If) Type() Type {
	if len(i.Branches) == 0 || i.Branches[0].Body == nil {
		return Void
	}
	return i.Branches[0].Body.Type()
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
	Name                    string
	Receiver                string
	GenericParams           []string
	Parameters              []Parameter
	ReturnType              Type
	InferReturnTypeFromBody bool
	Mutates                 bool
	IsTest                  bool
	Body                    *Block
	Private                 bool
	GenericBindings         map[string]Type
}

func (f FunctionDef) String() string {
	paramStrs := make([]string, len(f.Parameters))
	for i := range f.Parameters {
		paramStrs[i] = f.Parameters[i].Type.String()
	}

	return fmt.Sprintf("fn %s(%s) %s", f.Name, strings.Join(paramStrs, ","), f.ReturnType.String())
}

func (f FunctionDef) get(name string) Type { return nil }

func (f FunctionDef) name() string {
	return f.Name
}

func (f *FunctionDef) Type() Type {
	return f
}

func (f FunctionDef) equal(other Type) bool {
	return equalTypes(f, other)
}

func (f FunctionDef) hasTrait(trait *Trait) bool {
	return false
}
func (f *FunctionDef) hasGenerics() bool {
	if len(f.GenericParams) > 0 {
		return true
	}
	for _, param := range f.Parameters {
		if hasGenericsInType(param.Type) {
			return true
		}
	}
	return hasGenericsInType(f.ReturnType)
}

type FunctionCall struct {
	Name       string
	Args       []Expression
	TypeArgs   []Type
	fn         *FunctionDef
	ReturnType Type // Pre-computed by checker
}

func CreateCall(name string, args []Expression, fn FunctionDef) *FunctionCall {
	return &FunctionCall{
		Name:       name,
		Args:       args,
		fn:         &fn,
		ReturnType: fn.ReturnType,
	}
}

func (f *FunctionCall) Type() Type {
	return f.ReturnType
}

func (f *FunctionCall) Definition() *FunctionDef {
	return f.fn
}

type FunctionValueCall struct {
	Callee       Expression
	Args         []Expression
	FunctionType *FunctionDef
	ReturnType   Type
}

func (f *FunctionValueCall) Type() Type {
	return f.ReturnType
}

type ModuleStructInstance struct {
	Module     string
	Property   *StructInstance
	FieldTypes map[string]Type // Pre-computed by checker
	StructType Type            // Pre-computed by checker
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

type ForeignFunctionCall struct {
	Target    string
	Namespace string
	Qualifier string
	Symbol    string
	Call      *FunctionCall
}

func (p *ForeignFunctionCall) Type() Type {
	return p.Call.Type()
}

type ForeignMethodCall struct {
	Subject   Expression
	Target    string
	Namespace string
	Qualifier string
	Receiver  string
	Pointer   bool
	Symbol    string
	Call      *FunctionCall
}

func (p *ForeignMethodCall) Type() Type {
	return p.Call.Type()
}

type ForeignValue struct {
	Target    string
	Namespace string
	Qualifier string
	Symbol    string
	ValueType Type
}

func (p *ForeignValue) Type() Type {
	return p.ValueType
}

type ModuleSymbol struct {
	Module string
	Symbol Symbol
}

func (p *ModuleSymbol) Type() Type {
	return p.Symbol.Type
}

type EnumValue struct {
	Name  string
	Value int // The computed integer discriminant
}

type Enum struct {
	Name       string
	ModulePath string
	Private    bool
	Values     []EnumValue // The discriminant values for each variant
	Methods    map[string]*FunctionDef
	Traits     []*Trait
	Location   parse.Location
	Open       bool
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
		if tv, ok := other.(*TypeVar); ok {
			if tv.actual == nil {
				return true
			}
			return e.equal(tv.actual)
		}
		return false
	}
	if e.Name != o.Name || namedTypeOwnersDiffer(e.ModulePath, o.ModulePath) {
		return false
	}
	if len(e.Values) != len(o.Values) {
		return false
	}
	for i := range e.Values {
		if e.Values[i].Name != o.Values[i].Name || e.Values[i].Value != o.Values[i].Value {
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
	enum         *Enum
	Variant      int
	EnumType     Type // Pre-computed by checker
	Discriminant int  // Pre-computed by checker
}

func (ev EnumVariant) Type() Type {
	return ev.enum
}

func (ev EnumVariant) hasTrait(trait *Trait) bool {
	return ev.enum.hasTrait(trait)
}

func (ev EnumVariant) String() string {
	return fmt.Sprintf("%s::%s", ev.enum.Name, ev.enum.Values[ev.Variant].Name)
}

type Union struct {
	Name       string
	ModulePath string
	Types      []Type
	Private    bool
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
	return equalTypes(u, other)
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
	Name          string
	ModulePath    string
	Fields        map[string]Type
	Self          string
	Traits        []*Trait
	GenericParams []string
	TypeArgs      []Type
	Private       bool
}

func (def StructDef) NonProducing() {}

func (def *StructDef) name() string {
	return def.Name
}

func (def StructDef) String() string {
	if len(def.TypeArgs) == 0 || strings.Contains(def.Name, "<") {
		return def.name()
	}
	parts := make([]string, len(def.TypeArgs))
	for i, arg := range def.TypeArgs {
		parts[i] = arg.String()
	}
	return fmt.Sprintf("%s<%s>", def.name(), strings.Join(parts, ", "))
}
func (def StructDef) get(name string) Type {
	// Struct type identity describes value shape. Method namespaces live on the
	// checked Program side table and are resolved by the checker with module context.
	if field, ok := def.Fields[name]; ok {
		return field
	}
	return nil
}
func (def StructDef) equal(other Type) bool {
	return equalTypes(def, other)
}

func (def StructDef) hasGenerics() bool {
	if len(def.GenericParams) > 0 {
		return true
	}
	for _, typeArg := range def.TypeArgs {
		if hasGenericsInType(typeArg) {
			return true
		}
	}
	for _, fieldType := range def.Fields {
		if hasGenericsInType(fieldType) {
			return true
		}
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
	Name       string
	Fields     map[string]Expression
	_type      *StructDef
	FieldTypes map[string]Type // Pre-computed by checker
	StructType Type            // Pre-computed by checker
}

func (s StructInstance) Type() Type {
	return s._type
}

type ResultMatch struct {
	Subject    Expression
	Ok         *Match
	Err        *Match
	OkType     Type // Pre-computed ok type
	ErrType    Type // Pre-computed err type
	ResultType Type
}

func (r ResultMatch) Type() Type {
	if r.ResultType != nil {
		return r.ResultType
	}
	return r.Ok.Body.Type()
}

type Panic struct {
	Message Expression
	node    *parse.FunctionCall
}

func (p Panic) GetLocation() parse.Location {
	return p.node.GetLocation()
}

func (p Panic) Type() Type {
	// realistically, this is Void but that would break expectations when using `panic()` to signal unreachable code
	// in a function or block that is declared to return or be a non-Void value.
	// using TypeVar technically allows empty panicking functions to work; e.g. the `async:start()` function in async.ard
	return &TypeVar{name: "Unreachable"}
}

type TryKind uint8

const (
	TryResult TryKind = iota
	TryMaybe
)

type TryOp struct {
	expr       Expression
	ok         Type
	OkType     Type
	ErrType    Type
	CatchBlock *Block
	CatchVar   string
	Kind       TryKind // Pre-computed by checker: TryResult or TryMaybe
}

func (t TryOp) Expr() Expression {
	return t.expr
}

func (t TryOp) Type() Type {
	return t.ok
}
