package bytecode

type StackEffect struct {
	Pop  int
	Push int
}

func (o Opcode) String() string {
	switch o {
	case OpNoop:
		return "NOOP"
	case OpConstInt:
		return "CONST_INT"
	case OpConstFloat:
		return "CONST_FLOAT"
	case OpConstStr:
		return "CONST_STR"
	case OpConstBool:
		return "CONST_BOOL"
	case OpConstVoid:
		return "CONST_VOID"
	case OpConst:
		return "CONST"
	case OpLoadLocal:
		return "LOAD_LOCAL"
	case OpStoreLocal:
		return "STORE_LOCAL"
	case OpPop:
		return "POP"
	case OpDup:
		return "DUP"
	case OpSwap:
		return "SWAP"
	case OpAdd:
		return "ADD"
	case OpSub:
		return "SUB"
	case OpMul:
		return "MUL"
	case OpDiv:
		return "DIV"
	case OpMod:
		return "MOD"
	case OpNeg:
		return "NEG"
	case OpNot:
		return "NOT"
	case OpEq:
		return "EQ"
	case OpNeq:
		return "NEQ"
	case OpLt:
		return "LT"
	case OpLte:
		return "LTE"
	case OpGt:
		return "GT"
	case OpGte:
		return "GTE"
	case OpJump:
		return "JUMP"
	case OpJumpIfFalse:
		return "JUMP_IF_FALSE"
	case OpJumpIfTrue:
		return "JUMP_IF_TRUE"
	case OpCall:
		return "CALL"
	case OpCallExtern:
		return "CALL_EXTERN"
	case OpCallModule:
		return "CALL_MODULE"
	case OpMakeList:
		return "MAKE_LIST"
	case OpMakeMap:
		return "MAKE_MAP"
	case OpMakeStruct:
		return "MAKE_STRUCT"
	case OpMakeEnum:
		return "MAKE_ENUM"
	case OpGetField:
		return "GET_FIELD"
	case OpSetField:
		return "SET_FIELD"
	case OpCallMethod:
		return "CALL_METHOD"
	case OpMatchBool:
		return "MATCH_BOOL"
	case OpMatchInt:
		return "MATCH_INT"
	case OpMatchEnum:
		return "MATCH_ENUM"
	case OpMatchUnion:
		return "MATCH_UNION"
	case OpMatchMaybe:
		return "MATCH_MAYBE"
	case OpMatchResult:
		return "MATCH_RESULT"
	case OpTryResult:
		return "TRY_RESULT"
	case OpTryMaybe:
		return "TRY_MAYBE"
	case OpAsyncStart:
		return "ASYNC_START"
	case OpAsyncEval:
		return "ASYNC_EVAL"
	case OpReturn:
		return "RETURN"
	default:
		return "UNKNOWN"
	}
}

func (o Opcode) StackEffect() StackEffect {
	switch o {
	case OpNoop:
		return StackEffect{}
	case OpConstInt, OpConstFloat, OpConstStr, OpConstBool, OpConstVoid, OpConst:
		return StackEffect{Pop: 0, Push: 1}
	case OpLoadLocal:
		return StackEffect{Pop: 0, Push: 1}
	case OpStoreLocal:
		return StackEffect{Pop: 1, Push: 0}
	case OpPop:
		return StackEffect{Pop: 1, Push: 0}
	case OpDup:
		return StackEffect{Pop: 1, Push: 2}
	case OpSwap:
		return StackEffect{Pop: 2, Push: 2}
	case OpAdd, OpSub, OpMul, OpDiv, OpMod, OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte:
		return StackEffect{Pop: 2, Push: 1}
	case OpNeg, OpNot:
		return StackEffect{Pop: 1, Push: 1}
	case OpJump:
		return StackEffect{Pop: 0, Push: 0}
	case OpJumpIfFalse, OpJumpIfTrue:
		return StackEffect{Pop: 1, Push: 0}
	case OpReturn:
		return StackEffect{Pop: 1, Push: 0}
	default:
		return StackEffect{}
	}
}
