package air

type ModuleID int
type TypeID int
type FunctionID int
type GlobalID int
type LocalID int
type TraitID int
type ImplID int

const (
	NoType     TypeID     = 0
	NoFunction FunctionID = -1
	NoGlobal   GlobalID   = -1
)

type Program struct {
	Modules   []Module
	Types     []TypeInfo
	Traits    []Trait
	Impls     []Impl
	Globals   []Global
	Tests     []Test
	Functions []Function
	Entry     FunctionID
	Script    FunctionID
}

type Module struct {
	ID        ModuleID
	Path      string
	Imports   []ModuleID
	Types     []TypeID
	Globals   []GlobalID
	Functions []FunctionID
}

type Global struct {
	ID      GlobalID
	Module  ModuleID
	Name    string
	Type    TypeID
	Mutable bool
	Private bool
	Value   Expr
}

type Function struct {
	ID        FunctionID
	Module    ModuleID
	Name      string
	Signature Signature
	Locals    []Local
	Captures  []Capture
	Body      Block
	IsTest    bool
	IsScript  bool
	Private   bool

	// TypeParams names the generic parameters for a generic function definition
	// (ADR 0031). When set, the function is emitted as `func Name[T any](...)`
	// and its body/signature reference TypeParam-kind types. Call sites carry
	// concrete type arguments (Expr.TypeArgs).
	TypeParams []string

	// Receiver and MethodName are set for Ard impl methods. They let targets
	// optionally expose a host-language method shape while preserving the
	// standalone function lowering used by AIR calls.
	Receiver   TypeID
	MethodName string
}

type Signature struct {
	Params []Param
	Return TypeID
}

type Param struct {
	Name    string
	Type    TypeID
	Mutable bool
}

type Local struct {
	ID      LocalID
	Name    string
	Type    TypeID
	Mutable bool
	// Reference marks a local bound to live mutable storage owned elsewhere
	// (an Ard `mut T` value produced by a foreign call). The Go backend keeps
	// the local pointer-backed so mutations flow through to the owner.
	Reference bool
}

type Capture struct {
	Name  string
	Type  TypeID
	Local LocalID
}

type Test struct {
	Name     string
	Function FunctionID
}

type TypeKind uint8

const (
	TypeVoid TypeKind = iota
	TypeInt
	TypeScalar
	TypeForeignType
	TypeFloat64
	TypeBool
	TypeByte
	TypeRune
	TypeStr
	TypeList
	TypeFixedArray
	TypeMap
	TypeStruct
	TypeEnum
	TypeMaybe
	TypeResult
	TypeUnion
	TypeAny
	TypeFunction
	TypeChannel
	TypeReceiver
	TypeSender
	TypeTraitObject
	// TypeParam is a reference to a generic type parameter inside a generic
	// definition (e.g. the `T` in `struct Partition { selected: [$T] }`). It only appears in
	// the fields/signature of a generic definition, never in a concrete value.
	TypeParam
)

type TypeInfo struct {
	ID         TypeID
	Kind       TypeKind
	Name       string
	ModulePath string
	Private    bool

	Elem        TypeID
	ElemMutable bool
	Length      int
	Key         TypeID
	Value       TypeID
	Error       TypeID

	ForeignTarget    string
	ForeignNamespace string
	ForeignQualifier string
	ForeignSymbol    string
	ForeignPointer   bool
	ForeignInterface bool

	Fields   []FieldInfo
	Variants []VariantInfo
	EnumOpen bool
	Members  []UnionMember

	Params       []TypeID
	ParamMutable []bool
	Return       TypeID
	Trait        TraitID

	// Generic representation (ADR 0031). A generic definition sets TypeParams
	// (the parameter names) and references them via TypeParam-kind fields. A
	// concrete instantiation keeps its concrete Fields for AIR typing but also
	// records Generic (the definition's TypeID) and GenericArgs (the type
	// arguments), so the backend can emit it as `Def[args...]`.
	TypeParams  []string
	ParamIndex  int
	Generic     TypeID
	GenericArgs []TypeID
}

type FieldInfo struct {
	Name    string
	Type    TypeID
	Index   int
	Mutable bool
}

type VariantInfo struct {
	Name         string
	Discriminant int
}

type UnionMember struct {
	Type TypeID
	Tag  uint32
	Name string
}

type Trait struct {
	ID         TraitID
	Name       string
	ModulePath string
	Private    bool
	Methods    []TraitMethod
}

type TraitMethod struct {
	Name      string
	Signature Signature
}

type Impl struct {
	ID      ImplID
	Trait   TraitID
	ForType TypeID
	Methods []FunctionID
}
