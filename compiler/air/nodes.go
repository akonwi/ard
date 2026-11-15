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
	// StmtSetPointee replaces the whole pointee of the pointer-valued Target
	// with Value (`pointer.* = value`, ADR 0073). Type is the pointee type.
	StmtSetPointee
)

type Stmt struct {
	Kind       StmtKind
	Local      LocalID
	Global     GlobalID
	ValueLocal LocalID
	Name       string
	Type       TypeID
	Mutable    bool
	// Predeclare makes a StmtLet binding available while its initializer is
	// evaluated. Recursive local functions use it so their closure can capture
	// the stable self slot before the closure value is assigned.
	Predeclare bool
	// LocalFunction identifies a lexical named-function binding. Targets may
	// need a unique storage name when expression scopes are flattened.
	LocalFunction bool
	Value         *Expr
	Expr          *Expr
	Target        *Expr
	Field         int
	FieldName     string

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
	ExprEmbeddedText
	ExprEmbeddedBytes
	ExprMakeEmbeddedFS
	ExprEmbeddedFSReadFile
	ExprEmbeddedFSReadText
	ExprEmbeddedFSReadDir
	ExprEmbeddedFSStat
	ExprEmbeddedFSSub
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
	ExprInterfaceConversion
	// ExprDiscardingFunctionCoercion wraps Target in the Void-returning
	// function type named by Type, evaluating Target once and discarding calls' results.
	ExprDiscardingFunctionCoercion
	ExprUnsafeCast
	ExprUnsafeIsNil
	// ExprMutRef is the explicit `mut <operand>` expression. ReferenceMode
	// determines whether Target is an existing handle, an addressable place,
	// or a value requiring fresh stable storage (ADR 0057).
	ExprMutRef
	// ExprDeref performs one shallow read through a reference. It is distinct
	// from reference creation and is never an assignment place (ADR 0057).
	ExprDeref
	// ExprTraitRefProject widens a concrete mutable reference to the trait's
	// native interface while preserving its concrete pointer (ADR 0061).
	ExprTraitRefProject
	// ExprMatch runs a compiled pattern match (ADR 0083). Args are the
	// subjects and the MatchExprPayload carries the decision tree.
	ExprMatch
	// ExprScalarConvert converts Target's foreign named scalar value to the
	// primitive scalar named by Type (for example Go's string(v)). It also
	// carries the lossless `T::from` tier and implicit foreign scalar
	// coercions, all of which lower to a plain Go conversion.
	ExprScalarConvert
	// ExprScalarTryConvert is a checked numeric conversion `T::try(x)`
	// (ADR 0072). Type is the Maybe wrapping the target scalar; Target is the
	// source value. It evaluates to none when the value is not representable.
	ExprScalarTryConvert
	// ExprScalarFitConvert is a forced numeric conversion `T::fit(x)`
	// (ADR 0072). Type is the target scalar. Integer targets wrap and float
	// targets round, both as Go does; a float source saturates rather than
	// inheriting Go's implementation-defined overflow.
	ExprScalarFitConvert
	ExprMakeClosure
	ExprCallClosure
	ExprUnionWrap
	ExprTraitUpcast
	ExprCallTrait
	ExprMakeList
	ExprMakeListSized
	ExprMakeFixedArray
	ExprListAt
	// ExprListAtChecked is the user-facing list.at: a bounds-checked access
	// producing Maybe(elem). ExprListAt is raw indexing used by internal
	// desugaring such as for-loop lowering.
	ExprListAtChecked
	ExprListSlice
	ExprListIsEmpty
	ExprListToList
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
	// ExprMakeTuple constructs the tuple named by Type from Args, one per
	// element in order (ADR 0077).
	ExprMakeTuple
	// ExprTupleElem reads one element of the tuple-valued Target. The
	// FieldExprPayload carries the element index.
	ExprTupleElem
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
	ExprStrSlice
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
	ExprMakeMaybeSome
	ExprMakeMaybeNone
	ExprMakeMaybeNew
	ExprMakeError
	ExprMatchMaybe
	ExprMaybeExpect
	ExprMaybeIsNone
	ExprMaybeIsSome
	ExprMaybeOr
	ExprMaybeMap
	ExprMaybeAndThen
	ExprMaybeSet
	ExprMaybeClear
	ExprResultExpect
	ExprResultOr
	ExprResultIsOk
	ExprResultIsErr
	ExprResultMap
	ExprResultMapErr
	ExprResultAndThen
	ExprTryResult
	ExprTryMaybe
	// Bitwise operators on integers (ADR 0080). The binary kinds carry a
	// BinaryExprPayload; for the shifts the right operand (the count) may
	// have a different integer type than the left and the result.
	ExprIntBitAnd
	ExprIntBitOr
	ExprIntBitXor
	ExprIntShl
	ExprIntShr
	// ExprIntBitNot complements its integer Target.
	ExprIntBitNot
)

// lastExprKind is the highest valid ExprKind; keep it at the end of the list.
const lastExprKind = ExprIntBitNot

type ForeignResultShape uint8

const (
	ForeignResultUnknown ForeignResultShape = iota
	ForeignResultDirect
	ForeignResultValueError
	ForeignResultErrorOnly
	ForeignResultValueBool
	// ForeignResultTuple is two or more Go results packed into one tuple.
	ForeignResultTuple
	// ForeignResultTupleError is three or more Go results whose last is an
	// error: the leading results are the tuple of a Result.
	ForeignResultTupleError
)

type InterfaceConversionMode uint8

const (
	InterfaceValue InterfaceConversionMode = iota
	InterfaceOwnedPointer
	InterfaceReference
)

// ReferenceMode classifies the operand of an explicit mutable-reference
// expression. InvalidReferenceMode is reserved for validation failures.
type ReferenceMode uint8

const (
	InvalidReferenceMode ReferenceMode = iota
	ExistingReference
	AddressablePlace
	FreshValue
)

type Expr struct {
	Kind    ExprKind
	Type    TypeID
	Target  *Expr
	Args    []Expr
	Payload ExprPayload
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
