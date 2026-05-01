package air

type Block struct {
	Stmts  []Stmt
	Result *Expr
}

type StmtKind uint8

const (
	StmtLet StmtKind = iota
	StmtAssign
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
	ExprLoadLocal
	ExprCall
	ExprCallExtern
	ExprMakeClosure
	ExprCallClosure
	ExprSpawnFiber
	ExprFiberGet
	ExprFiberJoin
	ExprMakeList
	ExprMakeMap
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
	ExprIf
	ExprMakeResultOk
	ExprMakeResultErr
	ExprEnumVariant
	ExprMatchEnum
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

	Local LocalID

	Function      FunctionID
	Extern        ExternID
	Args          []Expr
	Entries       []MapEntry
	CaptureLocals []LocalID

	Fields []StructFieldValue
	Target *Expr
	Field  int

	Left  *Expr
	Right *Expr

	Condition *Expr
	Then      Block
	Else      Block

	EnumCases []EnumMatchCase
	CatchAll  Block

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
