package air

// ExprPayload is kind-specific AIR expression state. Keeping it separate from
// Expr prevents every expression from reserving space for every possible kind.
type ExprPayload interface {
	exprPayload()
}

func exprPayloadAs[T ExprPayload](expr *Expr) T {
	var zero T
	if expr == nil {
		return zero
	}
	payload, _ := expr.Payload.(T)
	return payload
}

type TextExprPayload struct {
	Value string
}

func (*TextExprPayload) exprPayload() {}

type BoolExprPayload struct {
	Value bool
}

func (*BoolExprPayload) exprPayload() {}

type EnumExprPayload struct {
	Variant      int
	Discriminant int
}

func (*EnumExprPayload) exprPayload() {}

type LocalExprPayload struct {
	Local LocalID
}

func (*LocalExprPayload) exprPayload() {}

type GlobalExprPayload struct {
	Global GlobalID
}

func (*GlobalExprPayload) exprPayload() {}

type SpreadExprPayload struct {
	Element  TypeID
	Callable TypeID
}

func newSpreadExprPayload(enabled bool, element, callable TypeID) *SpreadExprPayload {
	if !enabled {
		return nil
	}
	return &SpreadExprPayload{Element: element, Callable: callable}
}

func spreadExprPayload(expr *Expr) *SpreadExprPayload {
	switch payload := expr.Payload.(type) {
	case *CallExprPayload:
		if payload != nil {
			return payload.Spread
		}
	case *ForeignExprPayload:
		if payload != nil {
			return payload.Spread
		}
	}
	return nil
}

type CallExprPayload struct {
	Function      FunctionID
	TypeArgs      []TypeID
	ArgOrder      []int
	CaptureLocals []LocalID
	Spread        *SpreadExprPayload
}

func (*CallExprPayload) exprPayload() {}

type ForeignExprPayload struct {
	Target      string
	Namespace   string
	Qualifier   string
	Symbol      string
	Receiver    string
	Pointer     bool
	ResultShape ForeignResultShape
	ArgABI      []ABIParamMode
	TypeArgs    []TypeID
	Fields      []StructFieldValue
	Spread      *SpreadExprPayload
}

func (*ForeignExprPayload) exprPayload() {}

type InterfaceExprPayload struct {
	Mode InterfaceConversionMode
}

func (*InterfaceExprPayload) exprPayload() {}

type ReferenceExprPayload struct {
	Mode          ReferenceMode
	Observational bool
}

func (*ReferenceExprPayload) exprPayload() {}

type FieldExprPayload struct {
	Field int
}

func (*FieldExprPayload) exprPayload() {}

type TagExprPayload struct {
	Tag uint32
}

func (*TagExprPayload) exprPayload() {}

type TraitExprPayload struct {
	Impl   ImplID
	Trait  TraitID
	Method int
}

func (*TraitExprPayload) exprPayload() {}

type AggregateExprPayload struct {
	Entries []MapEntry
	Fields  []StructFieldValue
}

func (*AggregateExprPayload) exprPayload() {}

type BinaryExprPayload struct {
	Left  *Expr
	Right *Expr
}

func (*BinaryExprPayload) exprPayload() {}

type BlockExprPayload struct {
	Body Block
}

func (*BlockExprPayload) exprPayload() {}

type IfExprPayload struct {
	Condition *Expr
	Then      Block
	Else      Block
}

func (*IfExprPayload) exprPayload() {}

type EnumMatchExprPayload struct {
	Cases    []EnumMatchCase
	CatchAll Block
}

func (*EnumMatchExprPayload) exprPayload() {}

type IntMatchExprPayload struct {
	Cases      []IntMatchCase
	RangeCases []IntRangeMatchCase
	CatchAll   Block
}

func (*IntMatchExprPayload) exprPayload() {}

type StrMatchExprPayload struct {
	Cases    []StrMatchCase
	CatchAll Block
}

func (*StrMatchExprPayload) exprPayload() {}

type UnionMatchExprPayload struct {
	Cases    []UnionMatchCase
	CatchAll Block
}

func (*UnionMatchExprPayload) exprPayload() {}

type ForeignMatchExprPayload struct {
	Cases    []ForeignTypeMatchCase
	CatchAll Block
}

func (*ForeignMatchExprPayload) exprPayload() {}

type MaybeMatchExprPayload struct {
	SomeLocal LocalID
	Some      Block
	None      Block
}

func (*MaybeMatchExprPayload) exprPayload() {}

type ResultMatchExprPayload struct {
	OkLocal  LocalID
	ErrLocal LocalID
	Ok       Block
	Err      Block
}

func (*ResultMatchExprPayload) exprPayload() {}

type TryExprPayload struct {
	CatchLocal LocalID
	Catch      Block
}

func (*TryExprPayload) exprPayload() {}

type SelectExprPayload struct {
	Cases []SelectMatchCase
}

func (*SelectExprPayload) exprPayload() {}

type MaybeCallExprPayload struct {
	ReturnsReference bool
}

func (*MaybeCallExprPayload) exprPayload() {}

type UnsafeCastExprPayload struct {
	TargetType TypeID
	Pointer    bool
}

func (*UnsafeCastExprPayload) exprPayload() {}

func exprPayloadIsTypedNil(payload ExprPayload) bool {
	switch payload := payload.(type) {
	case *TextExprPayload:
		return payload == nil
	case *BoolExprPayload:
		return payload == nil
	case *EnumExprPayload:
		return payload == nil
	case *LocalExprPayload:
		return payload == nil
	case *GlobalExprPayload:
		return payload == nil
	case *CallExprPayload:
		return payload == nil
	case *ForeignExprPayload:
		return payload == nil
	case *InterfaceExprPayload:
		return payload == nil
	case *ReferenceExprPayload:
		return payload == nil
	case *FieldExprPayload:
		return payload == nil
	case *TagExprPayload:
		return payload == nil
	case *TraitExprPayload:
		return payload == nil
	case *AggregateExprPayload:
		return payload == nil
	case *BinaryExprPayload:
		return payload == nil
	case *BlockExprPayload:
		return payload == nil
	case *IfExprPayload:
		return payload == nil
	case *EnumMatchExprPayload:
		return payload == nil
	case *IntMatchExprPayload:
		return payload == nil
	case *StrMatchExprPayload:
		return payload == nil
	case *UnionMatchExprPayload:
		return payload == nil
	case *ForeignMatchExprPayload:
		return payload == nil
	case *MaybeMatchExprPayload:
		return payload == nil
	case *ResultMatchExprPayload:
		return payload == nil
	case *TryExprPayload:
		return payload == nil
	case *SelectExprPayload:
		return payload == nil
	case *MaybeCallExprPayload:
		return payload == nil
	case *UnsafeCastExprPayload:
		return payload == nil
	default:
		return false
	}
}
