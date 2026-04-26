package ir

type Module struct {
	Path        string
	PackageName string
	Decls       []Decl
	Entrypoint  *Block
}

type Decl interface {
	declNode()
}

type Field struct {
	Name string
	Type Type
}

type Param struct {
	Name    string
	Type    Type
	Mutable bool
}

type StructDecl struct {
	Name   string
	Fields []Field
}

func (*StructDecl) declNode() {}

type EnumValue struct {
	Name  string
	Value int
}

type EnumDecl struct {
	Name   string
	Values []EnumValue
}

func (*EnumDecl) declNode() {}

type UnionDecl struct {
	Name  string
	Types []Type
}

func (*UnionDecl) declNode() {}

type ExternTypeDecl struct {
	Name string
	Args []Type
}

func (*ExternTypeDecl) declNode() {}

type FuncDecl struct {
	Name          string
	Params        []Param
	Return        Type
	Body          *Block
	ExternBinding string
	IsExtern      bool
	IsPrivate     bool
	IsTest        bool
}

func (*FuncDecl) declNode() {}

type VarDecl struct {
	Name    string
	Type    Type
	Value   Expr
	Mutable bool
}

func (*VarDecl) declNode() {}

type Block struct {
	Stmts []Stmt
}

type Stmt interface {
	stmtNode()
}

type ReturnStmt struct {
	Value Expr
}

func (*ReturnStmt) stmtNode() {}

type ExprStmt struct {
	Value Expr
}

func (*ExprStmt) stmtNode() {}

type BreakStmt struct{}

func (*BreakStmt) stmtNode() {}

type AssignStmt struct {
	Target string
	Value  Expr
}

func (*AssignStmt) stmtNode() {}

type MemberAssignStmt struct {
	Subject Expr
	Field   string
	Value   Expr
}

func (*MemberAssignStmt) stmtNode() {}

type ForIntRangeStmt struct {
	Cursor string
	Index  string
	Start  Expr
	End    Expr
	Body   *Block
}

func (*ForIntRangeStmt) stmtNode() {}

type ForLoopStmt struct {
	InitName  string
	InitValue Expr
	Cond      Expr
	Update    Stmt
	Body      *Block
}

func (*ForLoopStmt) stmtNode() {}

type ForInStrStmt struct {
	Cursor string
	Index  string
	Value  Expr
	Body   *Block
}

func (*ForInStrStmt) stmtNode() {}

type ForInListStmt struct {
	Cursor string
	Index  string
	List   Expr
	Body   *Block
}

func (*ForInListStmt) stmtNode() {}

type ForInMapStmt struct {
	Key   string
	Value string
	Map   Expr
	Body  *Block
}

func (*ForInMapStmt) stmtNode() {}

type WhileStmt struct {
	Cond Expr
	Body *Block
}

func (*WhileStmt) stmtNode() {}

type IfStmt struct {
	Cond Expr
	Then *Block
	Else *Block
}

func (*IfStmt) stmtNode() {}

type Expr interface {
	exprNode()
}

type IdentExpr struct {
	Name string
}

func (*IdentExpr) exprNode() {}

type LiteralExpr struct {
	Kind  string
	Value string
}

func (*LiteralExpr) exprNode() {}

type SelectorExpr struct {
	Subject Expr
	Name    string
}

func (*SelectorExpr) exprNode() {}

type CallExpr struct {
	Callee Expr
	Args   []Expr
}

func (*CallExpr) exprNode() {}

type ListLiteralExpr struct {
	Type     Type
	Elements []Expr
}

func (*ListLiteralExpr) exprNode() {}

type MapEntry struct {
	Key   Expr
	Value Expr
}

type MapLiteralExpr struct {
	Type    Type
	Entries []MapEntry
}

func (*MapLiteralExpr) exprNode() {}

type StructFieldValue struct {
	Name  string
	Value Expr
}

type StructLiteralExpr struct {
	Type   Type
	Fields []StructFieldValue
}

func (*StructLiteralExpr) exprNode() {}

type EnumVariantExpr struct {
	Type         Type
	Discriminant int
}

func (*EnumVariantExpr) exprNode() {}

type IfExpr struct {
	Cond Expr
	Then *Block
	Else *Block
	Type Type
}

func (*IfExpr) exprNode() {}

type UnionMatchCase struct {
	Type    Type
	Pattern string
	Body    *Block
}

type UnionMatchExpr struct {
	Subject  Expr
	Cases    []UnionMatchCase
	CatchAll *Block
	Type     Type
}

func (*UnionMatchExpr) exprNode() {}

type TryExpr struct {
	Kind     string
	Subject  Expr
	CatchVar string
	Catch    *Block
	Type     Type
}

func (*TryExpr) exprNode() {}

type PanicExpr struct {
	Message Expr
	Type    Type
}

func (*PanicExpr) exprNode() {}

type CopyExpr struct {
	Value Expr
	Type  Type
}

func (*CopyExpr) exprNode() {}

// BlockExpr evaluates Setup statements once for their side effects, then
// returns the Value expression. It is used to express single-evaluation
// semantics for expression-level constructs (such as match expressions with
// non-trivial subjects) without resorting to marker fallback.
type BlockExpr struct {
	Setup []Stmt
	Value Expr
	Type  Type
}

func (*BlockExpr) exprNode() {}

type Type interface {
	typeNode()
}

type PrimitiveType struct {
	Name string
}

func (*PrimitiveType) typeNode() {}

type DynamicType struct{}

func (*DynamicType) typeNode() {}

type VoidType struct{}

func (*VoidType) typeNode() {}

type TypeVarType struct {
	Name string
}

func (*TypeVarType) typeNode() {}

type NamedType struct {
	Name string
	Args []Type
}

func (*NamedType) typeNode() {}

type ListType struct {
	Elem Type
}

func (*ListType) typeNode() {}

type MapType struct {
	Key   Type
	Value Type
}

func (*MapType) typeNode() {}

type MaybeType struct {
	Of Type
}

func (*MaybeType) typeNode() {}

type ResultType struct {
	Val Type
	Err Type
}

func (*ResultType) typeNode() {}

type FuncType struct {
	Params []Type
	Return Type
}

func (*FuncType) typeNode() {}

var (
	IntType     Type = &PrimitiveType{Name: "Int"}
	FloatType   Type = &PrimitiveType{Name: "Float"}
	StrType     Type = &PrimitiveType{Name: "Str"}
	BoolType    Type = &PrimitiveType{Name: "Bool"}
	Dynamic     Type = &DynamicType{}
	Void        Type = &VoidType{}
	UnknownType Type = &NamedType{Name: "Unknown"}
)
