package vm_next

type valueAllocKind uint8

const (
	valueAllocMaybe valueAllocKind = iota
	valueAllocResult
	valueAllocStruct
	valueAllocList
	valueAllocMap
	valueAllocUnion
	valueAllocDynamic
	valueAllocClosure
	valueAllocKindCount
)

type refAccessKind uint8

const (
	refAccessStruct refAccessKind = iota
	refAccessList
	refAccessMap
	refAccessMaybe
	refAccessResult
	refAccessUnion
	refAccessTraitObject
	refAccessExtern
	refAccessDynamic
	refAccessClosure
	refAccessFiber
	refAccessKindCount
)

type zeroValueKind uint8

const (
	zeroValueVoid zeroValueKind = iota
	zeroValueScalar
	zeroValueList
	zeroValueMap
	zeroValueDynamic
	zeroValueFiber
	zeroValueEnum
	zeroValueMaybe
	zeroValueStruct
	zeroValueResult
	zeroValueUnion
	zeroValueTraitObject
	zeroValueExtern
	zeroValueOther
	zeroValueKindCount
)
