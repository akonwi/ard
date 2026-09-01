package air

func (e Expr) TextPayload() *TextExprPayload {
	payload, _ := e.Payload.(*TextExprPayload)
	return payload
}

func (e Expr) BoolPayload() *BoolExprPayload {
	payload, _ := e.Payload.(*BoolExprPayload)
	return payload
}

func (e Expr) EnumPayload() *EnumExprPayload {
	payload, _ := e.Payload.(*EnumExprPayload)
	return payload
}

func (e Expr) LocalPayload() *LocalExprPayload {
	payload, _ := e.Payload.(*LocalExprPayload)
	return payload
}

func (e Expr) GlobalPayload() *GlobalExprPayload {
	payload, _ := e.Payload.(*GlobalExprPayload)
	return payload
}

func (e Expr) CallPayload() *CallExprPayload {
	payload, _ := e.Payload.(*CallExprPayload)
	return payload
}

func (e Expr) ForeignPayload() *ForeignExprPayload {
	payload, _ := e.Payload.(*ForeignExprPayload)
	return payload
}

func (e Expr) InterfacePayload() *InterfaceExprPayload {
	payload, _ := e.Payload.(*InterfaceExprPayload)
	return payload
}

func (e Expr) ReferencePayload() *ReferenceExprPayload {
	payload, _ := e.Payload.(*ReferenceExprPayload)
	return payload
}

func (e Expr) FieldPayload() *FieldExprPayload {
	payload, _ := e.Payload.(*FieldExprPayload)
	return payload
}

func (e Expr) TagPayload() *TagExprPayload {
	payload, _ := e.Payload.(*TagExprPayload)
	return payload
}

func (e Expr) TraitPayload() *TraitExprPayload {
	payload, _ := e.Payload.(*TraitExprPayload)
	return payload
}

func (e Expr) AggregatePayload() *AggregateExprPayload {
	payload, _ := e.Payload.(*AggregateExprPayload)
	return payload
}

func (e Expr) BinaryPayload() *BinaryExprPayload {
	payload, _ := e.Payload.(*BinaryExprPayload)
	return payload
}

func (e Expr) BlockPayload() *BlockExprPayload {
	payload, _ := e.Payload.(*BlockExprPayload)
	return payload
}

func (e Expr) IfPayload() *IfExprPayload {
	payload, _ := e.Payload.(*IfExprPayload)
	return payload
}

func (e Expr) EnumMatchPayload() *EnumMatchExprPayload {
	payload, _ := e.Payload.(*EnumMatchExprPayload)
	return payload
}

func (e Expr) IntMatchPayload() *IntMatchExprPayload {
	payload, _ := e.Payload.(*IntMatchExprPayload)
	return payload
}

func (e Expr) StrMatchPayload() *StrMatchExprPayload {
	payload, _ := e.Payload.(*StrMatchExprPayload)
	return payload
}

func (e Expr) UnionMatchPayload() *UnionMatchExprPayload {
	payload, _ := e.Payload.(*UnionMatchExprPayload)
	return payload
}

func (e Expr) ForeignMatchPayload() *ForeignMatchExprPayload {
	payload, _ := e.Payload.(*ForeignMatchExprPayload)
	return payload
}

func (e Expr) MaybeMatchPayload() *MaybeMatchExprPayload {
	payload, _ := e.Payload.(*MaybeMatchExprPayload)
	return payload
}

func (e Expr) ResultMatchPayload() *ResultMatchExprPayload {
	payload, _ := e.Payload.(*ResultMatchExprPayload)
	return payload
}

func (e Expr) TryPayload() *TryExprPayload {
	payload, _ := e.Payload.(*TryExprPayload)
	return payload
}

func (e Expr) SelectPayload() *SelectExprPayload {
	payload, _ := e.Payload.(*SelectExprPayload)
	return payload
}

func (e Expr) MaybeCallPayload() *MaybeCallExprPayload {
	payload, _ := e.Payload.(*MaybeCallExprPayload)
	return payload
}

func (e Expr) UnsafeCastPayload() *UnsafeCastExprPayload {
	payload, _ := e.Payload.(*UnsafeCastExprPayload)
	return payload
}
