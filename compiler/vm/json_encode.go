package vm

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strings"

	"github.com/akonwi/ard/air"
)

func (vm *VM) callJSONEncode(extern air.Extern, args []Value) (Value, error) {
	returnInfo, err := vm.typeInfo(extern.Signature.Return)
	if err != nil {
		return Value{}, err
	}
	if returnInfo.Kind != air.TypeResult {
		return Value{}, fmt.Errorf("JsonEncode return type must be Result, got %s", returnInfo.Name)
	}
	if len(args) != 1 {
		return Value{}, fmt.Errorf("JsonEncode expects 1 arg, got %d", len(args))
	}

	var buf bytes.Buffer
	enc := jsontext.NewEncoder(&buf)
	if err := vm.jsonEncodeValue(enc, args[0]); err != nil {
		return Result(extern.Signature.Return, false, Str(returnInfo.Error, err.Error())), nil
	}
	out := strings.TrimSuffix(buf.String(), "\n")
	return Result(extern.Signature.Return, true, Str(returnInfo.Value, out)), nil
}

func (vm *VM) jsonEncodeValue(enc *jsontext.Encoder, value Value) error {
	switch value.Kind {
	case ValueVoid:
		return enc.WriteToken(jsontext.Null)
	case ValueInt:
		return enc.WriteToken(jsontext.Int(int64(value.Int)))
	case ValueFloat:
		return enc.WriteToken(jsontext.Float(value.Float))
	case ValueBool:
		if value.Bool {
			return enc.WriteToken(jsontext.True)
		}
		return enc.WriteToken(jsontext.False)
	case ValueStr:
		return enc.WriteToken(jsontext.String(value.Str))
	case ValueEnum:
		return enc.WriteToken(jsontext.Int(int64(value.Int)))
	}

	typeInfo, err := vm.typeInfo(value.Type)
	if err != nil {
		return err
	}
	switch value.Kind {
	case ValueDynamic:
		bytes, err := json.Marshal(value.Ref)
		if err != nil {
			return err
		}
		return enc.WriteValue(jsontext.Value(bytes))
	case ValueMaybe:
		maybeValue, err := value.maybeValue()
		if err != nil {
			return err
		}
		if !maybeValue.Some {
			return enc.WriteToken(jsontext.Null)
		}
		return vm.jsonEncodeValue(enc, maybeValue.Value)
	case ValueResult, ValueResultInt, ValueResultStr, ValueResultBool, ValueResultFloat:
		ok, resultValue, err := value.resultParts()
		if err != nil {
			return err
		}
		_ = ok
		return vm.jsonEncodeValue(enc, resultValue)
	case ValueList:
		listValue, err := value.listValue()
		if err != nil {
			return err
		}
		if err := enc.WriteToken(jsontext.BeginArray); err != nil {
			return err
		}
		for _, item := range listValue.Items {
			if err := vm.jsonEncodeValue(enc, item); err != nil {
				return err
			}
		}
		return enc.WriteToken(jsontext.EndArray)
	case ValueMap:
		mapValue, err := value.mapValue()
		if err != nil {
			return err
		}
		if err := enc.WriteToken(jsontext.BeginObject); err != nil {
			return err
		}
		for _, entry := range mapValue.Entries {
			key, err := jsonEncodeMapKey(entry.Key)
			if err != nil {
				return err
			}
			if err := enc.WriteToken(jsontext.String(key)); err != nil {
				return err
			}
			if err := vm.jsonEncodeValue(enc, entry.Value); err != nil {
				return err
			}
		}
		return enc.WriteToken(jsontext.EndObject)
	case ValueStruct:
		structValue, err := value.structValue()
		if err != nil {
			return err
		}
		if typeInfo.Kind != air.TypeStruct {
			return fmt.Errorf("expected struct type, got %s", typeInfo.Name)
		}
		if err := enc.WriteToken(jsontext.BeginObject); err != nil {
			return err
		}
		for _, field := range typeInfo.Fields {
			if err := enc.WriteToken(jsontext.String(field.Name)); err != nil {
				return err
			}
			if err := vm.jsonEncodeValue(enc, structValue.Fields[field.Index]); err != nil {
				return err
			}
		}
		return enc.WriteToken(jsontext.EndObject)
	case ValueUnion:
		unionValue, err := value.unionValue()
		if err != nil {
			return err
		}
		return vm.jsonEncodeValue(enc, unionValue.Value)
	case ValueTraitObject:
		traitObject, err := value.traitObjectValue()
		if err != nil {
			return err
		}
		return vm.jsonEncodeValue(enc, traitObject.Value)
	case ValueExtern:
		externValue, err := value.externValue()
		if err != nil {
			return err
		}
		bytes, err := json.Marshal(externValue.Handle)
		if err != nil {
			return err
		}
		return enc.WriteValue(jsontext.Value(bytes))
	default:
		return fmt.Errorf("cannot JSON encode %s", typeInfo.Name)
	}
}

func jsonEncodeMapKey(value Value) (string, error) {
	if value.Kind == ValueStr {
		return value.Str, nil
	}
	return fmt.Sprint(value.GoValue()), nil
}
