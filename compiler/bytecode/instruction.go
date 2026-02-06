package bytecode

type Instruction struct {
	Op  Opcode
	A   int
	B   int
	Imm int
	C   int
}
