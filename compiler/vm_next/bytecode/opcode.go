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
	OpCallExtern
	OpCopy
	OpTraitUpcast
	OpCallTrait
	OpMakeClosure
	OpCallClosure
	OpSpawnFiber
	OpFiberGet
	OpFiberJoin
	OpUnionWrap
	OpUnionTag
	OpUnionValue
	OpMakeList
	OpListAt
	OpListPrepend
	OpListPush
	OpListSet
	OpListSize
	OpListSizeLocal
	OpListAtLocal
	OpListIndexLtLocal
	OpListSort
	OpListSwap
	OpMakeMap
	OpMapKeys
	OpMapSize
	OpMapGet
	OpMapSet
	OpMapDrop
	OpMapHas
	OpMapKeyAt
	OpMapValueAt
	OpMakeStruct
	OpGetField
	OpSetField
	OpEnumVariant
	OpStrAt
	OpStrSize
	OpStrIsEmpty
	OpStrContains
	OpStrReplace
	OpStrReplaceAll
	OpStrSplit
	OpStrStartsWith
	OpStrTrim
	OpMakeMaybeSome
	OpMakeMaybeNone
	OpMaybeExpect
	OpMaybeIsNone
	OpMaybeIsSome
	OpMaybeOr
	OpMaybeMap
	OpMaybeAndThen
	OpMakeResultOk
	OpMakeResultErr
	OpResultExpect
	OpResultErrValue
	OpResultOr
	OpResultIsOk
	OpResultIsErr
	OpResultMap
	OpResultMapErr
	OpResultAndThen
	OpTryResult
	OpTryMaybe
	OpToDynamic
	OpPanic
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
	case OpCallExtern:
		return "CallExtern"
	case OpCopy:
		return "Copy"
	case OpTraitUpcast:
		return "TraitUpcast"
	case OpCallTrait:
		return "CallTrait"
	case OpMakeClosure:
		return "MakeClosure"
	case OpCallClosure:
		return "CallClosure"
	case OpSpawnFiber:
		return "SpawnFiber"
	case OpFiberGet:
		return "FiberGet"
	case OpFiberJoin:
		return "FiberJoin"
	case OpUnionWrap:
		return "UnionWrap"
	case OpUnionTag:
		return "UnionTag"
	case OpUnionValue:
		return "UnionValue"
	case OpMakeList:
		return "MakeList"
	case OpListAt:
		return "ListAt"
	case OpListPrepend:
		return "ListPrepend"
	case OpListPush:
		return "ListPush"
	case OpListSet:
		return "ListSet"
	case OpListSize:
		return "ListSize"
	case OpListSizeLocal:
		return "ListSizeLocal"
	case OpListAtLocal:
		return "ListAtLocal"
	case OpListIndexLtLocal:
		return "ListIndexLtLocal"
	case OpListSort:
		return "ListSort"
	case OpListSwap:
		return "ListSwap"
	case OpMakeMap:
		return "MakeMap"
	case OpMapKeys:
		return "MapKeys"
	case OpMapSize:
		return "MapSize"
	case OpMapGet:
		return "MapGet"
	case OpMapSet:
		return "MapSet"
	case OpMapDrop:
		return "MapDrop"
	case OpMapHas:
		return "MapHas"
	case OpMapKeyAt:
		return "MapKeyAt"
	case OpMapValueAt:
		return "MapValueAt"
	case OpMakeStruct:
		return "MakeStruct"
	case OpGetField:
		return "GetField"
	case OpSetField:
		return "SetField"
	case OpEnumVariant:
		return "EnumVariant"
	case OpStrAt:
		return "StrAt"
	case OpStrSize:
		return "StrSize"
	case OpStrIsEmpty:
		return "StrIsEmpty"
	case OpStrContains:
		return "StrContains"
	case OpStrReplace:
		return "StrReplace"
	case OpStrReplaceAll:
		return "StrReplaceAll"
	case OpStrSplit:
		return "StrSplit"
	case OpStrStartsWith:
		return "StrStartsWith"
	case OpStrTrim:
		return "StrTrim"
	case OpMakeMaybeSome:
		return "MakeMaybeSome"
	case OpMakeMaybeNone:
		return "MakeMaybeNone"
	case OpMakeResultOk:
		return "MakeResultOk"
	case OpMaybeExpect:
		return "MaybeExpect"
	case OpMaybeIsNone:
		return "MaybeIsNone"
	case OpMaybeIsSome:
		return "MaybeIsSome"
	case OpMaybeOr:
		return "MaybeOr"
	case OpMaybeMap:
		return "MaybeMap"
	case OpMaybeAndThen:
		return "MaybeAndThen"
	case OpMakeResultErr:
		return "MakeResultErr"
	case OpResultExpect:
		return "ResultExpect"
	case OpResultErrValue:
		return "ResultErrValue"
	case OpResultOr:
		return "ResultOr"
	case OpResultIsOk:
		return "ResultIsOk"
	case OpResultIsErr:
		return "ResultIsErr"
	case OpResultMap:
		return "ResultMap"
	case OpResultMapErr:
		return "ResultMapErr"
	case OpResultAndThen:
		return "ResultAndThen"
	case OpTryResult:
		return "TryResult"
	case OpTryMaybe:
		return "TryMaybe"
	case OpToDynamic:
		return "ToDynamic"
	case OpPanic:
		return "Panic"
	case OpToStr:
		return "ToStr"
	default:
		return "Opcode(?)"
	}
}
