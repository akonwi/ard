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
