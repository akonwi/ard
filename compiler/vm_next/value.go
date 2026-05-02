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
	ValueMaybe
	ValueStruct
	ValueList
	ValueMap
	ValueResult
	ValueUnion
	ValueTraitObject
	ValueExtern
	ValueDynamic
	ValueClosure
	ValueFiber
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

type ListValue struct {
	Type  air.TypeID
	Items []Value
}

type MapValue struct {
	Type          air.TypeID
	Entries       []MapEntryValue
	SortedEntries []MapEntryValue
	SortedDirty   bool
}

type MapEntryValue struct {
	Key   Value
	Value Value
}

type ResultValue struct {
	Type  air.TypeID
	Ok    bool
	Value Value
}

type UnionValue struct {
	Type  air.TypeID
	Tag   uint32
	Value Value
}

type TraitObjectValue struct {
	Type  air.TypeID
	Trait air.TraitID
	Impl  air.ImplID
	Value Value
}

type MaybeValue struct {
	Type  air.TypeID
	Some  bool
	Value Value
}

type ExternValue struct {
	Type   air.TypeID
	Handle any
}

type DynamicValue struct {
	Type air.TypeID
	Raw  any
}

type ClosureValue struct {
	Type     air.TypeID
	Function air.FunctionID
	Captures []Value
}

type FiberValue struct {
	Type   air.TypeID
	Done   chan struct{}
	Result Value
	Err    error
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

func Maybe(typeID air.TypeID, some bool, value Value) Value {
	return Value{Kind: ValueMaybe, Type: typeID, Ref: &MaybeValue{Type: typeID, Some: some, Value: value}}
}

func Struct(typeID air.TypeID, fields []Value) Value {
	return Value{Kind: ValueStruct, Type: typeID, Ref: &StructValue{Type: typeID, Fields: fields}}
}

func List(typeID air.TypeID, items []Value) Value {
	return Value{Kind: ValueList, Type: typeID, Ref: &ListValue{Type: typeID, Items: items}}
}

func Map(typeID air.TypeID, entries []MapEntryValue) Value {
	return Value{Kind: ValueMap, Type: typeID, Ref: &MapValue{Type: typeID, Entries: entries, SortedDirty: true}}
}

func Result(typeID air.TypeID, ok bool, value Value) Value {
	return Value{Kind: ValueResult, Type: typeID, Ref: &ResultValue{Type: typeID, Ok: ok, Value: value}}
}

func Union(typeID air.TypeID, tag uint32, value Value) Value {
	return Value{Kind: ValueUnion, Type: typeID, Ref: &UnionValue{Type: typeID, Tag: tag, Value: value}}
}

func TraitObject(typeID air.TypeID, trait air.TraitID, impl air.ImplID, value Value) Value {
	return Value{Kind: ValueTraitObject, Type: typeID, Ref: &TraitObjectValue{Type: typeID, Trait: trait, Impl: impl, Value: value}}
}

func Extern(typeID air.TypeID, handle any) Value {
	return Value{Kind: ValueExtern, Type: typeID, Ref: &ExternValue{Type: typeID, Handle: handle}}
}

func Dynamic(typeID air.TypeID, raw any) Value {
	return Value{Kind: ValueDynamic, Type: typeID, Ref: &DynamicValue{Type: typeID, Raw: raw}}
}

func Closure(typeID air.TypeID, function air.FunctionID, captures []Value) Value {
	return Value{Kind: ValueClosure, Type: typeID, Ref: &ClosureValue{Type: typeID, Function: function, Captures: captures}}
}

func Fiber(typeID air.TypeID, fiber *FiberValue) Value {
	return Value{Kind: ValueFiber, Type: typeID, Ref: fiber}
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
	case ValueMaybe:
		maybeValue, ok := v.Ref.(*MaybeValue)
		if !ok || !maybeValue.Some {
			return nil
		}
		return maybeValue.Value.GoValue()
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
	case ValueList:
		listValue, ok := v.Ref.(*ListValue)
		if !ok {
			return nil
		}
		out := make([]any, len(listValue.Items))
		for i, item := range listValue.Items {
			out[i] = item.GoValue()
		}
		return out
	case ValueMap:
		mapValue, ok := v.Ref.(*MapValue)
		if !ok {
			return nil
		}
		out := make(map[any]any, len(mapValue.Entries))
		for _, entry := range mapValue.Entries {
			out[entry.Key.GoValue()] = entry.Value.GoValue()
		}
		return out
	case ValueResult:
		resultValue, ok := v.Ref.(*ResultValue)
		if !ok {
			return nil
		}
		return resultValue.Value.GoValue()
	case ValueUnion:
		unionValue, ok := v.Ref.(*UnionValue)
		if !ok {
			return nil
		}
		return unionValue.Value.GoValue()
	case ValueTraitObject:
		traitObjectValue, ok := v.Ref.(*TraitObjectValue)
		if !ok {
			return nil
		}
		return traitObjectValue.Value.GoValue()
	case ValueExtern:
		externValue, ok := v.Ref.(*ExternValue)
		if !ok {
			return nil
		}
		return externValue.Handle
	case ValueDynamic:
		dynamicValue, ok := v.Ref.(*DynamicValue)
		if !ok {
			return nil
		}
		return dynamicValue.Raw
	case ValueClosure:
		return v.Ref
	case ValueFiber:
		return v.Ref
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

func (v Value) listValue() (*ListValue, error) {
	if v.Kind != ValueList {
		return nil, fmt.Errorf("expected list value, got kind %d", v.Kind)
	}
	listValue, ok := v.Ref.(*ListValue)
	if !ok || listValue == nil {
		return nil, fmt.Errorf("list value has invalid payload %T", v.Ref)
	}
	return listValue, nil
}

func (v Value) mapValue() (*MapValue, error) {
	if v.Kind != ValueMap {
		return nil, fmt.Errorf("expected map value, got kind %d", v.Kind)
	}
	mapValue, ok := v.Ref.(*MapValue)
	if !ok || mapValue == nil {
		return nil, fmt.Errorf("map value has invalid payload %T", v.Ref)
	}
	return mapValue, nil
}

func (v Value) maybeValue() (*MaybeValue, error) {
	if v.Kind != ValueMaybe {
		return nil, fmt.Errorf("expected Maybe value, got kind %d", v.Kind)
	}
	maybeValue, ok := v.Ref.(*MaybeValue)
	if !ok || maybeValue == nil {
		return nil, fmt.Errorf("Maybe value has invalid payload %T", v.Ref)
	}
	return maybeValue, nil
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

func (v Value) unionValue() (*UnionValue, error) {
	if v.Kind != ValueUnion {
		return nil, fmt.Errorf("expected union value, got kind %d", v.Kind)
	}
	unionValue, ok := v.Ref.(*UnionValue)
	if !ok || unionValue == nil {
		return nil, fmt.Errorf("union value has invalid payload %T", v.Ref)
	}
	return unionValue, nil
}

func (v Value) traitObjectValue() (*TraitObjectValue, error) {
	if v.Kind != ValueTraitObject {
		return nil, fmt.Errorf("expected trait object value, got kind %d", v.Kind)
	}
	traitObjectValue, ok := v.Ref.(*TraitObjectValue)
	if !ok || traitObjectValue == nil {
		return nil, fmt.Errorf("trait object value has invalid payload %T", v.Ref)
	}
	return traitObjectValue, nil
}

func (v Value) externValue() (*ExternValue, error) {
	if v.Kind != ValueExtern {
		return nil, fmt.Errorf("expected extern value, got kind %d", v.Kind)
	}
	externValue, ok := v.Ref.(*ExternValue)
	if !ok || externValue == nil {
		return nil, fmt.Errorf("extern value has invalid payload %T", v.Ref)
	}
	return externValue, nil
}

func (v Value) dynamicValue() (*DynamicValue, error) {
	if v.Kind != ValueDynamic {
		return nil, fmt.Errorf("expected Dynamic value, got kind %d", v.Kind)
	}
	dynamicValue, ok := v.Ref.(*DynamicValue)
	if !ok || dynamicValue == nil {
		return nil, fmt.Errorf("Dynamic value has invalid payload %T", v.Ref)
	}
	return dynamicValue, nil
}

func (v Value) closureValue() (*ClosureValue, error) {
	if v.Kind != ValueClosure {
		return nil, fmt.Errorf("expected closure value, got kind %d", v.Kind)
	}
	closureValue, ok := v.Ref.(*ClosureValue)
	if !ok || closureValue == nil {
		return nil, fmt.Errorf("closure value has invalid payload %T", v.Ref)
	}
	return closureValue, nil
}

func (v Value) fiberValue() (*FiberValue, error) {
	if v.Kind != ValueFiber {
		return nil, fmt.Errorf("expected fiber value, got kind %d", v.Kind)
	}
	fiberValue, ok := v.Ref.(*FiberValue)
	if !ok || fiberValue == nil {
		return nil, fmt.Errorf("fiber value has invalid payload %T", v.Ref)
	}
	return fiberValue, nil
}
