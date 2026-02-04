package bytecode

type Opcode uint8

const (
	OpNoop Opcode = iota
	OpConstInt
	OpConstFloat
	OpConstStr
	OpConstBool
	OpConstVoid
	OpLoadLocal
	OpStoreLocal
	OpJump
	OpJumpIfFalse
	OpCall
	OpReturn
)
