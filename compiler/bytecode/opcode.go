package bytecode

type Opcode uint8

const (
	OpNoop Opcode = iota
	OpConstInt
	OpConstFloat
	OpConstStr
	OpConstBool
	OpConstVoid
	OpConst
	OpLoadLocal
	OpStoreLocal
	OpPop
	OpDup
	OpSwap
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	OpNeg
	OpNot
	OpEq
	OpNeq
	OpLt
	OpLte
	OpGt
	OpGte
	OpJump
	OpJumpIfFalse
	OpJumpIfTrue
	OpCall
	OpCallExtern
	OpCallModule
	OpMakeList
	OpMakeMap
	OpMakeStruct
	OpMakeEnum
	OpGetField
	OpSetField
	OpCallMethod
	OpMatchBool
	OpMatchInt
	OpMatchEnum
	OpMatchUnion
	OpMatchMaybe
	OpMatchResult
	OpTryResult
	OpTryMaybe
	OpAsyncStart
	OpAsyncEval
	OpReturn
)
