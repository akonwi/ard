package runtime

import "github.com/akonwi/ard/checker"

type Kind uint8

const (
	KindUnknown Kind = iota
	KindVoid
	KindStr
	KindInt
	KindFloat
	KindBool
	KindList
	KindMap
	KindMaybe
	KindResult
	KindStruct
	KindEnum
	KindFunction
	KindDynamic
)

func kindForType(t checker.Type) Kind {
	if t == nil {
		return KindUnknown
	}
	if t == checker.Void {
		return KindVoid
	}
	if t == checker.Str {
		return KindStr
	}
	if t == checker.Int {
		return KindInt
	}
	if t == checker.Float {
		return KindFloat
	}
	if t == checker.Bool {
		return KindBool
	}
	if t == checker.Dynamic {
		return KindDynamic
	}

	switch t.(type) {
	case *checker.List:
		return KindList
	case *checker.Map:
		return KindMap
	case *checker.Maybe:
		return KindMaybe
	case *checker.Result:
		return KindResult
	case *checker.StructDef:
		return KindStruct
	case *checker.Enum:
		return KindEnum
	case *checker.FunctionDef:
		return KindFunction
	default:
		return KindUnknown
	}
}

func (k Kind) String() string {
	switch k {
	case KindVoid:
		return "Void"
	case KindStr:
		return "Str"
	case KindInt:
		return "Int"
	case KindFloat:
		return "Float"
	case KindBool:
		return "Bool"
	case KindList:
		return "List"
	case KindMap:
		return "Map"
	case KindMaybe:
		return "Maybe"
	case KindResult:
		return "Result"
	case KindStruct:
		return "Struct"
	case KindEnum:
		return "Enum"
	case KindFunction:
		return "Function"
	case KindDynamic:
		return "Dynamic"
	default:
		return "Unknown"
	}
}
