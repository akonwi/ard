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
	case OpCopy:
		return "COPY"
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
	case OpAnd:
		return "AND"
	case OpOr:
		return "OR"
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
	case OpMakeClosure:
		return "MAKE_CLOSURE"
	case OpCallClosure:
		return "CALL_CLOSURE"
	case OpMakeList:
		return "MAKE_LIST"
	case OpMakeMap:
		return "MAKE_MAP"
	case OpMakeStruct:
		return "MAKE_STRUCT"
	case OpMakeEnum:
		return "MAKE_ENUM"
	case OpListLen:
		return "LIST_LEN"
	case OpListGet:
		return "LIST_GET"
	case OpListSet:
		return "LIST_SET"
	case OpListPush:
		return "LIST_PUSH"
	case OpListPrepend:
		return "LIST_PREPEND"
	case OpMapKeys:
		return "MAP_KEYS"
	case OpMapGet:
		return "MAP_GET"
	case OpMapGetValue:
		return "MAP_GET_VALUE"
	case OpMapSet:
		return "MAP_SET"
	case OpMapDrop:
		return "MAP_DROP"
	case OpMapHas:
		return "MAP_HAS"
	case OpMapSize:
		return "MAP_SIZE"
	case OpGetField:
		return "GET_FIELD"
	case OpSetField:
		return "SET_FIELD"
	case OpCallMethod:
		return "CALL_METHOD"
	case OpStrMethod:
		return "STR_METHOD"
	case OpIntMethod:
		return "INT_METHOD"
	case OpFloatMethod:
		return "FLOAT_METHOD"
	case OpBoolMethod:
		return "BOOL_METHOD"
	case OpMaybeMethod:
		return "MAYBE_METHOD"
	case OpResultMethod:
		return "RESULT_METHOD"
	case OpMaybeUnwrap:
		return "MAYBE_UNWRAP"
	case OpResultUnwrap:
		return "RESULT_UNWRAP"
	case OpTypeName:
		return "TYPE_NAME"
	case OpMakeNone:
		return "MAKE_NONE"
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
	case OpPanic:
		return "PANIC"
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
	case OpCopy:
		return StackEffect{Pop: 1, Push: 1}
	case OpAdd, OpSub, OpMul, OpDiv, OpMod, OpAnd, OpOr, OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte:
		return StackEffect{Pop: 2, Push: 1}
	case OpNeg, OpNot:
		return StackEffect{Pop: 1, Push: 1}
	case OpJump:
		return StackEffect{Pop: 0, Push: 0}
	case OpJumpIfFalse, OpJumpIfTrue:
		return StackEffect{Pop: 1, Push: 0}
	case OpReturn:
		return StackEffect{Pop: 1, Push: 0}
	case OpMakeClosure:
		return StackEffect{}
	case OpListLen:
		return StackEffect{Pop: 1, Push: 1}
	case OpListGet:
		return StackEffect{Pop: 2, Push: 1}
	case OpListSet:
		return StackEffect{Pop: 3, Push: 1}
	case OpListPush:
		return StackEffect{Pop: 2, Push: 1}
	case OpListPrepend:
		return StackEffect{Pop: 2, Push: 1}
	case OpMapKeys:
		return StackEffect{Pop: 1, Push: 1}
	case OpMapGet:
		return StackEffect{Pop: 2, Push: 1}
	case OpMapGetValue:
		return StackEffect{Pop: 2, Push: 1}
	case OpMapSet:
		return StackEffect{Pop: 3, Push: 1}
	case OpMapDrop:
		return StackEffect{Pop: 2, Push: 1}
	case OpMapHas:
		return StackEffect{Pop: 2, Push: 1}
	case OpMapSize:
		return StackEffect{Pop: 1, Push: 1}
	case OpStrMethod, OpIntMethod, OpFloatMethod, OpBoolMethod, OpMaybeMethod, OpResultMethod:
		return StackEffect{}
	case OpMaybeUnwrap, OpResultUnwrap, OpTypeName:
		return StackEffect{Pop: 1, Push: 1}
	case OpMakeNone:
		return StackEffect{Pop: 0, Push: 1}
	case OpTryResult, OpTryMaybe:
		return StackEffect{Pop: 1, Push: 1}
	case OpAsyncStart, OpAsyncEval:
		return StackEffect{Pop: 1, Push: 1}
	case OpPanic:
		return StackEffect{Pop: 1, Push: 0}
	default:
		return StackEffect{}
	}
}
