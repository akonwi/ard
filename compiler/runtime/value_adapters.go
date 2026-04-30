package runtime

import (
	"fmt"
	"sort"

	"github.com/akonwi/ard/checker"
)

func ValueToObject(v any, t checker.Type) *Object {
	if obj, ok := v.(*Object); ok {
		return obj
	}
	if t == nil {
		switch value := v.(type) {
		case nil:
			return MakeDynamic(nil)
		case VoidValue:
			return Void()
		case string:
			return MakeStr(value)
		case int:
			return MakeInt(value)
		case float64:
			return MakeFloat(value)
		case bool:
			return MakeBool(value)
		case MaybeValue:
			if value.None {
				return MakeNone(checker.Dynamic)
			}
			return MakeNone(checker.Dynamic).ToSome(ValueToObject(value.Value, nil).Raw())
		case ResultValue:
			if value.IsErr {
				return MakeErr(ValueToObject(value.Err, nil))
			}
			return MakeOk(ValueToObject(value.Ok, nil))
		case ListValue:
			elemType := inferLegacyElementType([]any(value))
			items := make([]*Object, len(value))
			for i := range value {
				items[i] = ValueToObject(value[i], elemType)
			}
			return MakeList(elemType, items...)
		case MapValue:
			keyType, valueType := inferLegacyMapTypes(value)
			obj := MakeMap(keyType, valueType)
			if value.Storage != nil {
				for _, key := range value.Storage.Keys() {
					entry, _ := value.Storage.GetAny(key)
					obj.Map_Set(ValueToObject(key, keyType), ValueToObject(entry, valueType))
				}
			}
			return obj
		case StructValue:
			return MakeDynamic(value)
		case EnumValue:
			return MakeDynamic(value)
		default:
			return MakeDynamic(value)
		}
	}

	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			return ValueToObject(v, actual)
		}
		return Make(v, typed)
	case *checker.Maybe:
		maybe, ok := v.(MaybeValue)
		if !ok {
			panic(fmt.Errorf("expected MaybeValue for %s, got %T", t, v))
		}
		if maybe.None {
			return MakeNone(typed.Of())
		}
		inner := ValueToObject(maybe.Value, typed.Of())
		return MakeNone(typed.Of()).ToSome(inner.Raw())
	case *checker.Result:
		result, ok := v.(ResultValue)
		if !ok {
			panic(fmt.Errorf("expected ResultValue for %s, got %T", t, v))
		}
		if result.IsErr {
			return MakeErr(ValueToObject(result.Err, typed.Err()))
		}
		return MakeOk(ValueToObject(result.Ok, typed.Val()))
	case *checker.List:
		values := listFromAny(v)
		items := make([]*Object, len(values))
		for i := range values {
			items[i] = ValueToObject(values[i], typed.Of())
		}
		return MakeList(typed.Of(), items...)
	case *checker.Map:
		mapped, ok := v.(MapValue)
		if !ok {
			panic(fmt.Errorf("expected MapValue for %s, got %T", t, v))
		}
		obj := MakeMap(typed.Key(), typed.Value())
		for _, key := range mapped.Storage.Keys() {
			value, _ := mapped.Storage.GetAny(key)
			obj.Map_Set(ValueToObject(key, typed.Key()), ValueToObject(value, typed.Value()))
		}
		return obj
	case *checker.StructDef:
		structValue, ok := v.(StructValue)
		if !ok {
			panic(fmt.Errorf("expected StructValue for %s, got %T", t, v))
		}
		fieldNames := sortedStructFieldNames(typed)
		if len(structValue.Fields) != len(fieldNames) {
			panic(fmt.Errorf("expected %d struct fields for %s, got %d", len(fieldNames), t, len(structValue.Fields)))
		}
		fields := make(map[string]*Object, len(fieldNames))
		for i, name := range fieldNames {
			fields[name] = ValueToObject(structValue.Fields[i], typed.Fields[name])
		}
		return MakeStruct(typed, fields)
	case *checker.Enum:
		switch value := v.(type) {
		case EnumValue:
			return Make(value.Tag, typed)
		case int:
			return Make(value, typed)
		default:
			panic(fmt.Errorf("expected EnumValue or int for %s, got %T", t, v))
		}
	case *checker.ExternType:
		return Make(v, typed)
	case *checker.FunctionDef:
		return Make(v, typed)
	}

	switch t {
	case checker.Void:
		return Void()
	case checker.Str:
		return MakeStr(v.(string))
	case checker.Int:
		return MakeInt(v.(int))
	case checker.Float:
		return MakeFloat(v.(float64))
	case checker.Bool:
		return MakeBool(v.(bool))
	case checker.Dynamic:
		return MakeDynamic(v)
	default:
		return Make(v, t)
	}
}

func ObjectToValue(obj *Object, t checker.Type) any {
	if obj == nil {
		return nil
	}
	if t == nil {
		t = obj.Type()
	}

	if typed, ok := t.(*checker.TypeVar); ok {
		if actual := typed.Actual(); actual != nil {
			return ObjectToValue(obj, actual)
		}
		return obj.Raw()
	}

	switch typed := t.(type) {
	case *checker.Maybe:
		if obj.IsNone() {
			return NoneValue()
		}
		innerObj := objectFromLegacyStorage(obj.Raw(), typed.Of())
		return SomeValue(ObjectToValue(innerObj, typed.Of()))
	case *checker.Result:
		innerObj := obj.UnwrapResult()
		if obj.IsErr() {
			return ErrValue(ObjectToValue(innerObj, typed.Err()))
		}
		return OkValue(ObjectToValue(innerObj, typed.Val()))
	case *checker.List:
		raw := obj.AsList()
		values := make(ListValue, len(raw))
		for i := range raw {
			values[i] = ObjectToValue(raw[i], typed.Of())
		}
		return values
	case *checker.Map:
		storage := newVMMapForKeyType(typed.Key())
		for rawKey, valueObj := range obj.AsMap() {
			keyObj := obj.Map_GetKey(rawKey)
			storage.SetAny(ObjectToValue(keyObj, typed.Key()), ObjectToValue(valueObj, typed.Value()))
		}
		return MapValue{KeyType: 0, ValueType: 0, Storage: storage}
	case *checker.StructDef:
		fieldNames := sortedStructFieldNames(typed)
		fields := make([]any, len(fieldNames))
		for i, name := range fieldNames {
			fieldObj := obj.Struct_Get(name)
			if fieldObj == nil {
				panic(fmt.Errorf("missing struct field %s on %s", name, typed.Name))
			}
			fields[i] = ObjectToValue(fieldObj, typed.Fields[name])
		}
		return StructValue{Fields: fields}
	case *checker.Enum:
		return EnumValue{Tag: obj.AsInt()}
	case *checker.ExternType:
		return obj.Raw()
	case *checker.FunctionDef:
		return obj.Raw()
	}

	switch t {
	case checker.Void:
		return NativeVoid
	case checker.Str:
		return obj.AsString()
	case checker.Int:
		return obj.AsInt()
	case checker.Float:
		return obj.AsFloat()
	case checker.Bool:
		return obj.AsBool()
	case checker.Dynamic:
		return obj.Raw()
	default:
		return obj.Raw()
	}
}

func listFromAny(v any) []any {
	switch values := v.(type) {
	case ListValue:
		return []any(values)
	case []any:
		return values
	default:
		panic(fmt.Errorf("expected ListValue or []any, got %T", v))
	}
}

func sortedStructFieldNames(def *checker.StructDef) []string {
	names := make([]string, 0, len(def.Fields))
	for name := range def.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func objectFromLegacyStorage(raw any, t checker.Type) *Object {
	if obj, ok := raw.(*Object); ok {
		return obj
	}
	switch t {
	case checker.Str:
		return MakeStr(raw.(string))
	case checker.Int:
		return MakeInt(raw.(int))
	case checker.Float:
		return MakeFloat(raw.(float64))
	case checker.Bool:
		return MakeBool(raw.(bool))
	case checker.Dynamic:
		return MakeDynamic(raw)
	case checker.Void:
		return Void()
	}
	return Make(raw, t)
}

func inferLegacyElementType(values []any) checker.Type {
	if len(values) == 0 {
		return checker.Dynamic
	}
	first := inferLegacyType(values[0])
	for i := 1; i < len(values); i++ {
		if inferLegacyType(values[i]) != first {
			return checker.Dynamic
		}
	}
	return first
}

func inferLegacyMapTypes(value MapValue) (checker.Type, checker.Type) {
	var keyType checker.Type = checker.Dynamic
	switch value.Storage.(type) {
	case *Map[string]:
		keyType = checker.Str
	case *Map[int]:
		keyType = checker.Int
	case *Map[float64]:
		keyType = checker.Float
	case *Map[bool]:
		keyType = checker.Bool
	}
	var valueType checker.Type = checker.Dynamic
	if value.Storage != nil {
		keys := value.Storage.Keys()
		if len(keys) > 0 {
			entry, _ := value.Storage.GetAny(keys[0])
			valueType = inferLegacyType(entry)
			for _, key := range keys[1:] {
				entry, _ := value.Storage.GetAny(key)
				if inferLegacyType(entry) != valueType {
					valueType = checker.Dynamic
					break
				}
			}
		}
	}
	return keyType, valueType
}

func inferLegacyType(value any) checker.Type {
	switch value.(type) {
	case VoidValue:
		return checker.Void
	case string:
		return checker.Str
	case int:
		return checker.Int
	case float64:
		return checker.Float
	case bool:
		return checker.Bool
	default:
		return checker.Dynamic
	}
}

func newVMMapForKeyType(t checker.Type) VMMap {
	switch t {
	case checker.Str, checker.Dynamic:
		return NewMap[string]()
	case checker.Int:
		return NewMap[int]()
	case checker.Float:
		return NewMap[float64]()
	case checker.Bool:
		return NewMap[bool]()
	default:
		panic(fmt.Errorf("unsupported native map key type: %s", t))
	}
}
