package vm_next

import (
	"fmt"

	"github.com/akonwi/ard/air"
)

type ValueKind uint8

const (
	ValueVoid ValueKind = iota
	ValueInt
	ValueFloat
	ValueBool
	ValueStr
	ValueEnum
	ValueStruct
	ValueResult
)

type Value struct {
	Kind ValueKind
	Type air.TypeID

	Int   int
	Float float64
	Bool  bool
	Str   string
	Ref   any
}

type StructValue struct {
	Type   air.TypeID
	Fields []Value
}

type ResultValue struct {
	Type  air.TypeID
	Ok    bool
	Value Value
}

func Void(typeID air.TypeID) Value {
	return Value{Kind: ValueVoid, Type: typeID}
}

func Int(typeID air.TypeID, value int) Value {
	return Value{Kind: ValueInt, Type: typeID, Int: value}
}

func Float(typeID air.TypeID, value float64) Value {
	return Value{Kind: ValueFloat, Type: typeID, Float: value}
}

func Bool(typeID air.TypeID, value bool) Value {
	return Value{Kind: ValueBool, Type: typeID, Bool: value}
}

func Str(typeID air.TypeID, value string) Value {
	return Value{Kind: ValueStr, Type: typeID, Str: value}
}

func Enum(typeID air.TypeID, discriminant int) Value {
	return Value{Kind: ValueEnum, Type: typeID, Int: discriminant}
}

func Struct(typeID air.TypeID, fields []Value) Value {
	return Value{Kind: ValueStruct, Type: typeID, Ref: &StructValue{Type: typeID, Fields: fields}}
}

func Result(typeID air.TypeID, ok bool, value Value) Value {
	return Value{Kind: ValueResult, Type: typeID, Ref: &ResultValue{Type: typeID, Ok: ok, Value: value}}
}

func (v Value) GoValue() any {
	switch v.Kind {
	case ValueVoid:
		return nil
	case ValueInt:
		return v.Int
	case ValueFloat:
		return v.Float
	case ValueBool:
		return v.Bool
	case ValueStr:
		return v.Str
	case ValueEnum:
		return v.Int
	case ValueStruct:
		structValue, ok := v.Ref.(*StructValue)
		if !ok {
			return nil
		}
		out := make([]any, len(structValue.Fields))
		for i, field := range structValue.Fields {
			out[i] = field.GoValue()
		}
		return out
	case ValueResult:
		resultValue, ok := v.Ref.(*ResultValue)
		if !ok {
			return nil
		}
		return resultValue.Value.GoValue()
	default:
		return nil
	}
}

func (v Value) GoValueString() string {
	if v.Kind == ValueStr {
		return v.Str
	}
	return fmt.Sprint(v.GoValue())
}

func (v Value) structValue() (*StructValue, error) {
	if v.Kind != ValueStruct {
		return nil, fmt.Errorf("expected struct value, got kind %d", v.Kind)
	}
	structValue, ok := v.Ref.(*StructValue)
	if !ok || structValue == nil {
		return nil, fmt.Errorf("struct value has invalid payload %T", v.Ref)
	}
	return structValue, nil
}

func (v Value) resultValue() (*ResultValue, error) {
	if v.Kind != ValueResult {
		return nil, fmt.Errorf("expected result value, got kind %d", v.Kind)
	}
	resultValue, ok := v.Ref.(*ResultValue)
	if !ok || resultValue == nil {
		return nil, fmt.Errorf("result value has invalid payload %T", v.Ref)
	}
	return resultValue, nil
}
