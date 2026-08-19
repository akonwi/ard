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
	// RequiredGoMethodName is set for explicit imported Go interface impls.
	// Targets must emit this exact receiver method or reject lowering; a
	// standalone helper cannot satisfy the foreign interface.
	RequiredGoMethodName string
}

type Signature struct {
	Params []Param
	Return TypeID
}

// ABIParamMode describes the host ABI projection applied to one canonical AIR
// parameter. Reference identity remains in the parameter TypeID; the mode only
// records boundary behavior that cannot be derived from that identity.
type ABIParamMode uint8

const (
	// ABIParamExact passes the canonical AIR value representation unchanged.
	ABIParamExact ABIParamMode = iota
	// ABIParamDescriptorValue requires a TypeReference to a slice/map
	// descriptor, but passes the current descriptor value to the host ABI.
	ABIParamDescriptorValue
)

type Param struct {
	Name string
	Type TypeID
	ABI  ABIParamMode
}

type Local struct {
	ID      LocalID
	Name    string
	Type    TypeID
	Mutable bool
	// Reference marks a local bound to live mutable storage owned elsewhere.
	// The Go backend preserves the referent's sharing representation so
	// mutations flow through to the owner.
	Reference bool
}

type CaptureMode uint8

const (
	// CaptureValue snapshots an ordinary value when the closure is created.
	CaptureValue CaptureMode = iota
	// CaptureReference snapshots a first-class reference handle.
	CaptureReference
	// CaptureSlot captures the binding slot itself for rebinding or address use.
	CaptureSlot
)

type Capture struct {
	Name  string
	Type  TypeID
	Local LocalID
	Mode  CaptureMode
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
	TypeSlice
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
	// TypeReference is the canonical pointer-shaped mutable-reference type.
	// Elem names the referent at every nesting position (ADR 0057).
	TypeReference
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

	Elem   TypeID
	Length int
	Key    TypeID
	Value  TypeID
	Error  TypeID

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

	Params []TypeID
	Return TypeID
	// Variadic marks the final TypeFunction parameter as a repeated element.
	// Ard declarations remain fixed-arity; this is carried by foreign callable
	// values and named Go function types.
	Variadic bool
	Trait    TraitID

	// Generic representation (ADR 0031). A generic definition sets TypeParams
	// (the parameter names) and references them via TypeParam-kind fields. A
	// concrete instantiation keeps its concrete Fields for AIR typing but also
	// records Generic (the definition's TypeID) and GenericArgs (the type
	// arguments), so the backend can emit it as `Def[args...]`.
	TypeParams        []string
	ParamIndex        int
	Generic           TypeID
	GenericArgs       []TypeID
	GenericComparable []bool
}

type FieldInfo struct {
	Name  string
	Type  TypeID
	Index int
	JSON  JSONFieldInfo
}

// JSONFieldInfo carries normalized, target-neutral JSON field semantics across
// the frontend/backend boundary.
type JSONFieldInfo struct {
	Name     string
	HasName  bool
	OmitNone bool
	Skip     bool
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
	ID                  TraitID
	Name                string
	ModulePath          string
	Private             bool
	BuiltinError        bool
	GoInterfaceFallback bool
	Methods             []TraitMethod
}

type TraitMethod struct {
	Name      string
	Mutates   bool
	Signature Signature
}

type Impl struct {
	ID      ImplID
	Trait   TraitID
	ForType TypeID
	Methods []FunctionID
}
