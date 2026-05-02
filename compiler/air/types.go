package air

type ModuleID int
type TypeID int
type FunctionID int
type ExternID int
type LocalID int
type TraitID int
type ImplID int

const (
	NoType     TypeID     = 0
	NoFunction FunctionID = -1
)

type Program struct {
	Modules   []Module
	Types     []TypeInfo
	Traits    []Trait
	Impls     []Impl
	Externs   []Extern
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
	Functions []FunctionID
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
	TypeFloat
	TypeBool
	TypeStr
	TypeList
	TypeMap
	TypeStruct
	TypeEnum
	TypeMaybe
	TypeResult
	TypeUnion
	TypeDynamic
	TypeExtern
	TypeFunction
	TypeFiber
	TypeTraitObject
)

type TypeInfo struct {
	ID   TypeID
	Kind TypeKind
	Name string

	Elem  TypeID
	Key   TypeID
	Value TypeID
	Error TypeID

	Fields   []FieldInfo
	Variants []VariantInfo
	Members  []UnionMember

	Params []TypeID
	Return TypeID
	Trait  TraitID
}

type FieldInfo struct {
	Name  string
	Type  TypeID
	Index int
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
	ID      TraitID
	Name    string
	Methods []TraitMethod
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

type Extern struct {
	ID        ExternID
	Module    ModuleID
	Name      string
	Signature Signature
	Bindings  map[string]string
}
