package air

type Block struct {
	Stmts  []Stmt
	Result *Expr
}

type StmtKind uint8

const (
	StmtLet StmtKind = iota
	StmtAssign
	StmtAssignGlobal
	StmtSetField
	StmtSetForeignField
	StmtSetForeignValue
	StmtExpr
	StmtWhile
	StmtForMap
	StmtBreak
	StmtDefer
)

type Stmt struct {
	Kind       StmtKind
	Local      LocalID
	Global     GlobalID
	ValueLocal LocalID
	Name       string
	Type       TypeID
	Mutable    bool
	Value      *Expr
	Expr       *Expr
	Target     *Expr
	Field      int
	FieldName  string

	ForeignTarget    string
	ForeignNamespace string
	ForeignQualifier string
	ForeignSymbol    string

	Condition *Expr
	Body      Block
}

// ExprKind values are iota-assigned and NOT stable across compiler
// versions: kinds are added and removed freely. They must never be
// persisted; AIR gob serialization is only valid within a single binary.
type ExprKind uint8

const (
	ExprConstVoid ExprKind = iota
	ExprConstInt
	ExprConstFloat
	ExprConstBool
	ExprConstStr
	ExprPanic
	ExprLoadLocal
	ExprLoadGlobal
	ExprFunctionRef
	ExprCall
	ExprForeignCall
	ExprForeignMethodCall
	ExprForeignMethodValue
	ExprForeignFieldAccess
	ExprForeignStructInstance
	ExprForeignValue
	ExprForeignInterfaceUpcast
	// ExprDiscardingFunctionCoercion wraps Target in the Void-returning
	// function type named by Type, evaluating Target once and discarding calls' results.
	ExprDiscardingFunctionCoercion
	ExprUnsafeCast
	ExprUnsafeIsNil
	// ExprMutRef is the explicit `mut <operand>` expression (ADR 0045). Target
	// is the referenced place (or the value expression when Bool marks fresh
	// storage); Type is the referent type. The backend chooses per
	// representation whether a Go pointer is involved.
	ExprMutRef
	ExprMatchForeignType
	// ExprScalarConvert converts Target's foreign named scalar value to the
	// primitive scalar named by Type (for example Go's string(v)).
	ExprScalarConvert
	ExprMakeClosure
	ExprCallClosure
	ExprUnionWrap
	ExprMatchUnion
	ExprTraitUpcast
	ExprCallTrait
	ExprMakeList
	ExprMakeFixedArray
	ExprListAt
	// ExprListAtChecked is the user-facing list.at: a bounds-checked access
	// producing Maybe(elem). ExprListAt is raw indexing used by internal
	// desugaring such as for-loop lowering.
	ExprListAtChecked
	ExprListPrepend
	ExprListPush
	ExprListSet
	ExprListSize
	ExprListSort
	ExprListSwap
	ExprMakeMap
	ExprAsyncStart
	ExprMakeChannel
	ExprChannelSend
	ExprChannelRecv
	ExprChannelClose
	ExprChannelNarrow
	ExprSelect
	ExprMapKeys
	ExprMapSize
	ExprMapGet
	ExprMapSet
	ExprMapDelete
	ExprMapHas
	ExprMapKeyAt
	ExprMapValueAt
	ExprMakeStruct
	ExprGetField
	ExprIntAdd
	ExprIntSub
	ExprIntMul
	ExprIntDiv
	ExprIntMod
	ExprFloatAdd
	ExprFloatSub
	ExprFloatMul
	ExprFloatDiv
	ExprStrConcat
	ExprToStr
	ExprToInt
	ExprToF64
	ExprStrAt
	ExprStrBytes
	ExprStrRunes
	ExprStrSize
	ExprStrIsEmpty
	ExprStrContains
	ExprStrReplace
	ExprStrReplaceAll
	ExprStrStartsWith
	ExprStrEndsWith
	ExprToAny
	ExprStrTrim
	ExprEq
	ExprNotEq
	ExprLt
	ExprLte
	ExprGt
	ExprGte
	ExprAnd
	ExprOr
	ExprNot
	ExprNeg
	ExprBlock
	ExprUnsafeBlock
	ExprIf
	ExprMakeResultOk
	ExprMakeResultErr
	ExprEnumVariant
	ExprMatchEnum
	ExprMatchInt
	ExprMatchStr
	ExprMakeMaybeSome
	ExprMakeMaybeNone
	ExprMakeMaybeNew
	ExprMatchMaybe
	ExprMaybeExpect
	ExprMaybeIsNone
	ExprMaybeIsSome
	ExprMaybeOr
	ExprMaybeMap
	ExprMaybeAndThen
	ExprMaybeSet
	ExprMaybeClear
	ExprMatchResult
	ExprResultExpect
	ExprResultOr
	ExprResultIsOk
	ExprResultIsErr
	ExprResultMap
	ExprResultMapErr
	ExprResultAndThen
	ExprTryResult
	ExprTryMaybe
)

type Expr struct {
	Kind ExprKind
	Type TypeID

	Int   string
	Float string
	Bool  bool
	Str   string

	Variant      int
	Discriminant int
	Tag          uint32

	Local  LocalID
	Global GlobalID

	Function                FunctionID
	ForeignTarget           string
	ForeignNamespace        string
	ForeignQualifier        string
	ForeignSymbol           string
	ForeignReceiver         string
	ForeignPointer          bool
	ForeignInterfacePointer bool
	TypeArgs                []TypeID
	Impl                    ImplID
	Trait                   TraitID
	Method                  int
	Args                    []Expr
	Entries                 []MapEntry
	CaptureLocals           []LocalID

	Fields []StructFieldValue
	Target *Expr
	Field  int

	Left  *Expr
	Right *Expr

	Condition *Expr
	Body      Block
	Then      Block
	Else      Block

	EnumCases    []EnumMatchCase
	IntCases     []IntMatchCase
	StrCases     []StrMatchCase
	RangeCases   []IntRangeMatchCase
	UnionCases   []UnionMatchCase
	ForeignCases []ForeignTypeMatchCase
	CatchAll     Block

	SomeLocal LocalID
	Some      Block
	None      Block

	OkLocal  LocalID
	ErrLocal LocalID
	Ok       Block
	Err      Block

	HasCatch   bool
	CatchLocal LocalID
	Catch      Block

	SelectCases []SelectMatchCase
}

// SelectArmKind distinguishes the lowered select arm forms (ADR 0032).
type SelectArmKind int

const (
	SelectArmRecv SelectArmKind = iota
	SelectArmSend
	SelectArmDefault
)

// SelectMatchCase is one arm of an ExprSelect. Recv/send arms carry the
// Channel; send arms carry the Value; recv arms with a binding set HasBind and
// bind BindLocal (of type Maybe<elem>) before running Body.
type SelectMatchCase struct {
	Kind      SelectArmKind
	Channel   *Expr
	Value     *Expr
	HasBind   bool
	BindLocal LocalID
	Body      Block
}

type StructFieldValue struct {
	Index int
	Name  string
	Value Expr
}

type MapEntry struct {
	Key   Expr
	Value Expr
}

type EnumMatchCase struct {
	Variant      int
	Discriminant int
	Body         Block
}

type IntMatchCase struct {
	Value int
	Body  Block
}

type StrMatchCase struct {
	Value string
	Body  Block
}

type IntRangeMatchCase struct {
	Start int
	End   int
	Body  Block
}

type UnionMatchCase struct {
	Tag   uint32
	Local LocalID
	Body  Block
}

// ForeignTypeMatchCase is one arm of a dynamic foreign type test (ADR 0042).
// Type names the concrete foreign type asserted by the arm. Bound reports
// whether Local should carry the narrowed value into Body.
type ForeignTypeMatchCase struct {
	Type  TypeID
	Local LocalID
	Bound bool
	Body  Block
}
