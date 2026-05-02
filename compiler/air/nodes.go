package air

type Block struct {
	Stmts  []Stmt
	Result *Expr
}

type StmtKind uint8

const (
	StmtLet StmtKind = iota
	StmtAssign
	StmtSetField
	StmtExpr
	StmtWhile
	StmtBreak
)

type Stmt struct {
	Kind    StmtKind
	Local   LocalID
	Name    string
	Type    TypeID
	Mutable bool
	Value   *Expr
	Expr    *Expr
	Target  *Expr
	Field   int

	Condition *Expr
	Body      Block
}

type ExprKind uint8

const (
	ExprConstVoid ExprKind = iota
	ExprConstInt
	ExprConstFloat
	ExprConstBool
	ExprConstStr
	ExprPanic
	ExprLoadLocal
	ExprCall
	ExprCallExtern
	ExprMakeClosure
	ExprCallClosure
	ExprSpawnFiber
	ExprFiberGet
	ExprFiberJoin
	ExprUnionWrap
	ExprMatchUnion
	ExprTraitUpcast
	ExprCallTrait
	ExprCopy
	ExprMakeList
	ExprListAt
	ExprListPrepend
	ExprListPush
	ExprListSet
	ExprListSize
	ExprListSort
	ExprListSwap
	ExprMakeMap
	ExprMapKeys
	ExprMapSize
	ExprMapGet
	ExprMapSet
	ExprMapDrop
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
	ExprStrAt
	ExprStrSize
	ExprStrIsEmpty
	ExprStrContains
	ExprStrReplace
	ExprStrReplaceAll
	ExprStrSplit
	ExprStrStartsWith
	ExprToDynamic
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
	ExprIf
	ExprMakeResultOk
	ExprMakeResultErr
	ExprEnumVariant
	ExprMatchEnum
	ExprMatchInt
	ExprMakeMaybeSome
	ExprMakeMaybeNone
	ExprMatchMaybe
	ExprMaybeExpect
	ExprMaybeIsNone
	ExprMaybeIsSome
	ExprMaybeOr
	ExprMaybeMap
	ExprMaybeAndThen
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

	Int   int
	Float float64
	Bool  bool
	Str   string

	Variant      int
	Discriminant int
	Tag          uint32

	Local LocalID

	Function      FunctionID
	Extern        ExternID
	Impl          ImplID
	Trait         TraitID
	Method        int
	Args          []Expr
	Entries       []MapEntry
	CaptureLocals []LocalID

	Fields []StructFieldValue
	Target *Expr
	Field  int

	Left  *Expr
	Right *Expr

	Condition *Expr
	Body      Block
	Then      Block
	Else      Block

	EnumCases  []EnumMatchCase
	IntCases   []IntMatchCase
	RangeCases []IntRangeMatchCase
	UnionCases []UnionMatchCase
	CatchAll   Block

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
