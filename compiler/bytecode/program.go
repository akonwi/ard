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

type Function struct {
	Name  string
	Arity int
	Code  []Instruction
}

type Program struct {
	Constants []Constant
	Functions []Function
}
