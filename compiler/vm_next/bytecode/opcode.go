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
	OpUnionWrap
	OpMakeList
	OpListAt
	OpListPrepend
	OpListPush
	OpListSet
	OpListSize
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
	OpMakeMaybeSome
	OpMakeMaybeNone
	OpMakeResultOk
	OpMakeResultErr
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
	case OpUnionWrap:
		return "UnionWrap"
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
	case OpMakeMaybeSome:
		return "MakeMaybeSome"
	case OpMakeMaybeNone:
		return "MakeMaybeNone"
	case OpMakeResultOk:
		return "MakeResultOk"
	case OpMakeResultErr:
		return "MakeResultErr"
	case OpToStr:
		return "ToStr"
	default:
		return "Opcode(?)"
	}
}
