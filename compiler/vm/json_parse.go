package vm

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/akonwi/ard/air"
)

type jsonDecodeError struct {
	expected string
	found    string
	path     []string
}

func (e jsonDecodeError) Error() string {
	if len(e.path) == 0 {
		return fmt.Sprintf("got %s, expected %s", e.found, e.expected)
	}
	return fmt.Sprintf("%s: got %s, expected %s", strings.Join(e.path, "."), e.found, e.expected)
}

func (vm *VM) callJSONParse(extern air.Extern, args []Value) (Value, error) {
	returnInfo, err := vm.typeInfo(extern.Signature.Return)
	if err != nil {
		return Value{}, err
	}
	if returnInfo.Kind != air.TypeResult {
		return Value{}, fmt.Errorf("JsonParse return type must be Result, got %s", returnInfo.Name)
	}
	if len(args) != 1 {
		return Value{}, fmt.Errorf("JsonParse expects 1 arg, got %d", len(args))
	}
	if args[0].Kind != ValueStr {
		return vm.jsonParseErr(extern.Signature.Return, jsonDecodeError{expected: "Str", found: args[0].GoValueString()}), nil
	}

	dec := jsontext.NewDecoder(strings.NewReader(args[0].Str))
	value, decodeErr := vm.jsonDecodeValue(dec, returnInfo.Value, nil)
	if decodeErr != nil {
		return vm.jsonParseErr(extern.Signature.Return, *decodeErr), nil
	}
	if kind := dec.PeekKind(); kind != jsontext.KindInvalid {
		return vm.jsonParseErr(extern.Signature.Return, jsonDecodeError{expected: "end of JSON", found: kind.String()}), nil
	}
	return Result(extern.Signature.Return, true, value), nil
}

func (vm *VM) jsonDecodeValue(dec *jsontext.Decoder, typeID air.TypeID, path []string) (Value, *jsonDecodeError) {
	typeInfo, err := vm.typeInfo(typeID)
	if err != nil {
		return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: err.Error(), path: path}
	}

	if dec.PeekKind() == jsontext.KindNull {
		if _, err := dec.ReadToken(); err != nil {
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: err.Error(), path: path}
		}
		if typeInfo.Kind == air.TypeMaybe {
			vm.recordMaybeDetailAlloc(false)
			return Maybe(typeID, false, vm.zeroValue(typeInfo.Elem)), nil
		}
		if typeInfo.Kind == air.TypeDynamic {
			return Dynamic(typeID, nil), nil
		}
		return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: "null", path: path}
	}

	switch typeInfo.Kind {
	case air.TypeInt:
		tok, err := dec.ReadToken()
		if err != nil {
			return Value{}, jsonTokenErr(typeInfo.Name, path, err)
		}
		if tok.Kind() != jsontext.KindNumber {
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonKindFound(tok.Kind()), path: path}
		}
		parsed, err := strconv.ParseInt(tok.String(), 10, 0)
		if err != nil {
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: "Number", path: path}
		}
		return Int(typeID, int(parsed)), nil
	case air.TypeFloat:
		tok, err := dec.ReadToken()
		if err != nil {
			return Value{}, jsonTokenErr(typeInfo.Name, path, err)
		}
		if tok.Kind() != jsontext.KindNumber {
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonKindFound(tok.Kind()), path: path}
		}
		parsed, err := strconv.ParseFloat(tok.String(), 64)
		if err != nil {
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: "Number", path: path}
		}
		return Float(typeID, parsed), nil
	case air.TypeBool:
		tok, err := dec.ReadToken()
		if err != nil {
			return Value{}, jsonTokenErr(typeInfo.Name, path, err)
		}
		switch tok.Kind() {
		case jsontext.KindTrue:
			return Bool(typeID, true), nil
		case jsontext.KindFalse:
			return Bool(typeID, false), nil
		default:
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonKindFound(tok.Kind()), path: path}
		}
	case air.TypeStr:
		tok, err := dec.ReadToken()
		if err != nil {
			return Value{}, jsonTokenErr(typeInfo.Name, path, err)
		}
		if tok.Kind() != jsontext.KindString {
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonKindFound(tok.Kind()), path: path}
		}
		return Str(typeID, tok.String()), nil
	case air.TypeDynamic:
		raw, decodeErr := jsonDecodeDynamic(dec, path)
		if decodeErr != nil {
			return Value{}, decodeErr
		}
		return Dynamic(typeID, raw), nil
	case air.TypeMaybe:
		inner, decodeErr := vm.jsonDecodeValue(dec, typeInfo.Elem, path)
		if decodeErr != nil {
			return Value{}, decodeErr
		}
		vm.recordMaybeDetailAlloc(true)
		return Maybe(typeID, true, inner), nil
	case air.TypeList:
		if tok, err := dec.ReadToken(); err != nil {
			return Value{}, jsonTokenErr(typeInfo.Name, path, err)
		} else if tok.Kind() != jsontext.KindBeginArray {
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonKindFound(tok.Kind()), path: path}
		}
		items := []Value{}
		for dec.PeekKind() != jsontext.KindEndArray {
			item, decodeErr := vm.jsonDecodeValue(dec, typeInfo.Elem, appendPath(path, fmt.Sprintf("[%d]", len(items))))
			if decodeErr != nil {
				return Value{}, decodeErr
			}
			items = append(items, item)
		}
		if _, err := dec.ReadToken(); err != nil {
			return Value{}, jsonTokenErr(typeInfo.Name, path, err)
		}
		return List(typeID, items), nil
	case air.TypeMap:
		keyInfo, err := vm.typeInfo(typeInfo.Key)
		if err != nil || keyInfo.Kind != air.TypeStr {
			return Value{}, &jsonDecodeError{expected: "Str map key", found: keyInfo.Name, path: path}
		}
		if tok, err := dec.ReadToken(); err != nil {
			return Value{}, jsonTokenErr(typeInfo.Name, path, err)
		} else if tok.Kind() != jsontext.KindBeginObject {
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonKindFound(tok.Kind()), path: path}
		}
		entries := []MapEntryValue{}
		for dec.PeekKind() != jsontext.KindEndObject {
			keyTok, err := dec.ReadToken()
			if err != nil {
				return Value{}, jsonTokenErr("Str", path, err)
			}
			if keyTok.Kind() != jsontext.KindString {
				return Value{}, &jsonDecodeError{expected: "Str", found: jsonKindFound(keyTok.Kind()), path: path}
			}
			key := keyTok.String()
			item, decodeErr := vm.jsonDecodeValue(dec, typeInfo.Value, appendPath(path, key))
			if decodeErr != nil {
				return Value{}, decodeErr
			}
			entries = append(entries, MapEntryValue{Key: Str(typeInfo.Key, key), Value: item})
		}
		if _, err := dec.ReadToken(); err != nil {
			return Value{}, jsonTokenErr(typeInfo.Name, path, err)
		}
		return Map(typeID, entries), nil
	case air.TypeStruct:
		return vm.jsonDecodeStruct(dec, typeID, typeInfo, path)
	}

	return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonKindFound(dec.PeekKind()), path: path}
}

func (vm *VM) jsonDecodeStruct(dec *jsontext.Decoder, typeID air.TypeID, typeInfo air.TypeInfo, path []string) (Value, *jsonDecodeError) {
	if tok, err := dec.ReadToken(); err != nil {
		return Value{}, jsonTokenErr(typeInfo.Name, path, err)
	} else if tok.Kind() != jsontext.KindBeginObject {
		return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonKindFound(tok.Kind()), path: path}
	}

	fields := make([]Value, len(typeInfo.Fields))
	seen := make([]bool, len(typeInfo.Fields))
	fieldsByName := make(map[string]air.FieldInfo, len(typeInfo.Fields))
	for _, field := range typeInfo.Fields {
		fieldsByName[field.Name] = field
	}

	for dec.PeekKind() != jsontext.KindEndObject {
		keyTok, err := dec.ReadToken()
		if err != nil {
			return Value{}, jsonTokenErr("Str", path, err)
		}
		if keyTok.Kind() != jsontext.KindString {
			return Value{}, &jsonDecodeError{expected: "Str", found: jsonKindFound(keyTok.Kind()), path: path}
		}
		fieldName := keyTok.String()
		field, ok := fieldsByName[fieldName]
		if !ok {
			if err := dec.SkipValue(); err != nil {
				return Value{}, jsonTokenErr("Dynamic", appendPath(path, fieldName), err)
			}
			continue
		}
		converted, decodeErr := vm.jsonDecodeValue(dec, field.Type, appendPath(path, fieldName))
		if decodeErr != nil {
			return Value{}, decodeErr
		}
		fields[field.Index] = converted
		seen[field.Index] = true
	}
	if _, err := dec.ReadToken(); err != nil {
		return Value{}, jsonTokenErr(typeInfo.Name, path, err)
	}

	for _, field := range typeInfo.Fields {
		if seen[field.Index] {
			continue
		}
		fieldInfo, err := vm.typeInfo(field.Type)
		if err != nil {
			return Value{}, &jsonDecodeError{expected: field.Name, found: err.Error(), path: path}
		}
		if fieldInfo.Kind == air.TypeMaybe {
			vm.recordMaybeDetailAlloc(false)
			fields[field.Index] = Maybe(field.Type, false, vm.zeroValue(fieldInfo.Elem))
			continue
		}
		return Value{}, &jsonDecodeError{expected: fieldInfo.Name, found: "missing", path: appendPath(path, field.Name)}
	}
	return Struct(typeID, fields), nil
}

func jsonDecodeDynamic(dec *jsontext.Decoder, path []string) (any, *jsonDecodeError) {
	value, err := dec.ReadValue()
	if err != nil {
		return nil, jsonTokenErr("Dynamic", path, err)
	}
	var raw any
	if err := json.Unmarshal(value, &raw); err != nil {
		return nil, &jsonDecodeError{expected: "Dynamic", found: err.Error(), path: path}
	}
	return raw, nil
}

func (vm *VM) jsonParseErr(resultType air.TypeID, decodeErr jsonDecodeError) Value {
	return Result(resultType, false, Str(vm.program.Types[resultType-1].Error, decodeErr.Error()))
}

func appendPath(path []string, segment string) []string {
	out := make([]string, len(path), len(path)+1)
	copy(out, path)
	out = append(out, segment)
	return out
}

func jsonTokenErr(expected string, path []string, err error) *jsonDecodeError {
	if err == io.EOF {
		return &jsonDecodeError{expected: expected, found: "end of JSON", path: path}
	}
	return &jsonDecodeError{expected: expected, found: err.Error(), path: path}
}

func jsonKindFound(kind jsontext.Kind) string {
	switch kind {
	case jsontext.KindNull:
		return "null"
	case jsontext.KindString:
		return "Str"
	case jsontext.KindTrue, jsontext.KindFalse:
		return "Bool"
	case jsontext.KindNumber:
		return "Number"
	case jsontext.KindBeginArray:
		return "List"
	case jsontext.KindBeginObject:
		return "Map"
	case jsontext.KindEndArray:
		return "]"
	case jsontext.KindEndObject:
		return "}"
	default:
		return kind.String()
	}
}
