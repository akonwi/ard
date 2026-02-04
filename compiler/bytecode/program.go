package bytecode

type ConstantKind uint8

const (
	ConstInt ConstantKind = iota
	ConstFloat
	ConstStr
	ConstBool
)

type Constant struct {
	Kind  ConstantKind
	Int   int
	Float float64
	Str   string
	Bool  bool
}

type TypeID int

type TypeEntry struct {
	ID   TypeID
	Name string
}

type Function struct {
	Name     string
	Arity    int
	Locals   int
	MaxStack int
	Code     []Instruction
}

type Program struct {
	Constants []Constant
	Types     []TypeEntry
	Functions []Function
}
