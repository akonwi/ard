package bytecode

type Opcode uint8

const (
	OpNoop Opcode = iota
	OpConstVoid
	OpConstInt
	OpConstFloat
	OpConstBool
	OpConstStr
	OpLoadLocal
	OpStoreLocal
	OpPop
	OpJump
	OpJumpIfFalse
	OpCall
	OpReturn
	OpIntAdd
	OpIntSub
	OpIntMul
	OpIntDiv
	OpIntMod
	OpFloatAdd
	OpFloatSub
	OpFloatMul
	OpFloatDiv
	OpStrConcat
	OpEq
	OpNotEq
	OpLt
	OpLte
	OpGt
	OpGte
	OpAnd
	OpOr
	OpNot
	OpNeg
	OpBlock
	OpMakeStruct
	OpGetField
	OpToStr
)

func (op Opcode) String() string {
	switch op {
	case OpNoop:
		return "Noop"
	case OpConstVoid:
		return "ConstVoid"
	case OpConstInt:
		return "ConstInt"
	case OpConstFloat:
		return "ConstFloat"
	case OpConstBool:
		return "ConstBool"
	case OpConstStr:
		return "ConstStr"
	case OpLoadLocal:
		return "LoadLocal"
	case OpStoreLocal:
		return "StoreLocal"
	case OpPop:
		return "Pop"
	case OpJump:
		return "Jump"
	case OpJumpIfFalse:
		return "JumpIfFalse"
	case OpCall:
		return "Call"
	case OpReturn:
		return "Return"
	case OpIntAdd:
		return "IntAdd"
	case OpIntSub:
		return "IntSub"
	case OpIntMul:
		return "IntMul"
	case OpIntDiv:
		return "IntDiv"
	case OpIntMod:
		return "IntMod"
	case OpFloatAdd:
		return "FloatAdd"
	case OpFloatSub:
		return "FloatSub"
	case OpFloatMul:
		return "FloatMul"
	case OpFloatDiv:
		return "FloatDiv"
	case OpStrConcat:
		return "StrConcat"
	case OpEq:
		return "Eq"
	case OpNotEq:
		return "NotEq"
	case OpLt:
		return "Lt"
	case OpLte:
		return "Lte"
	case OpGt:
		return "Gt"
	case OpGte:
		return "Gte"
	case OpAnd:
		return "And"
	case OpOr:
		return "Or"
	case OpNot:
		return "Not"
	case OpNeg:
		return "Neg"
	case OpBlock:
		return "Block"
	case OpMakeStruct:
		return "MakeStruct"
	case OpGetField:
		return "GetField"
	case OpToStr:
		return "ToStr"
	default:
		return "Opcode(?)"
	}
}
