package air

// AIR serialization round-trips within one compiler binary only: node kind
// enums are iota-assigned and not stable across versions. If this format
// ever backs an on-disk cache, it must gain explicit numbering and a format
// version first.

import (
	"bytes"
	"encoding/gob"
)

func init() {
	gob.Register(&TextExprPayload{})
	gob.Register(&EmbeddedBlobExprPayload{})
	gob.Register(&EmbeddedSetExprPayload{})
	gob.Register(&BoolExprPayload{})
	gob.Register(&EnumExprPayload{})
	gob.Register(&LocalExprPayload{})
	gob.Register(&GlobalExprPayload{})
	gob.Register(&CallExprPayload{})
	gob.Register(&ForeignExprPayload{})
	gob.Register(&InterfaceExprPayload{})
	gob.Register(&ReferenceExprPayload{})
	gob.Register(&FieldExprPayload{})
	gob.Register(&TagExprPayload{})
	gob.Register(&TraitExprPayload{})
	gob.Register(&AggregateExprPayload{})
	gob.Register(&BinaryExprPayload{})
	gob.Register(&BlockExprPayload{})
	gob.Register(&IfExprPayload{})
	gob.Register(&EnumMatchExprPayload{})
	gob.Register(&IntMatchExprPayload{})
	gob.Register(&StrMatchExprPayload{})
	gob.Register(&UnionMatchExprPayload{})
	gob.Register(&ForeignMatchExprPayload{})
	gob.Register(&MaybeMatchExprPayload{})
	gob.Register(&ResultMatchExprPayload{})
	gob.Register(&TryExprPayload{})
	gob.Register(&SelectExprPayload{})
	gob.Register(&MaybeCallExprPayload{})
	gob.Register(&UnsafeCastExprPayload{})
}

func SerializeProgram(program *Program) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(program); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DeserializeProgram(data []byte) (*Program, error) {
	var program Program
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&program); err != nil {
		return nil, err
	}
	return &program, nil
}
