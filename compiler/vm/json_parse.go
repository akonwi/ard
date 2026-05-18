package vm

import (
	"encoding/json/v2"
	"fmt"
	"math"
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

	raw, err := vm.jsonParseInput(args[0])
	if err != nil {
		return vm.jsonParseErr(extern.Signature.Return, returnInfo.Error, jsonDecodeError{expected: "JSON", found: err.Error()}), nil
	}

	value, decodeErr := vm.jsonValueToValue(raw, returnInfo.Value, nil)
	if decodeErr != nil {
		return vm.jsonParseErr(extern.Signature.Return, returnInfo.Error, *decodeErr), nil
	}
	return Result(extern.Signature.Return, true, value), nil
}

func (vm *VM) jsonParseInput(value Value) (any, error) {
	if value.Kind == ValueUnion {
		unionValue, err := value.unionValue()
		if err != nil {
			return nil, err
		}
		value = unionValue.Value
	}
	if value.Kind == ValueStr {
		var raw any
		if err := json.Unmarshal([]byte(value.Str), &raw); err != nil {
			return nil, err
		}
		return raw, nil
	}
	if value.Kind == ValueDynamic {
		return value.Ref, nil
	}
	return nil, fmt.Errorf("%s", value.GoValueString())
}

func (vm *VM) jsonValueToValue(raw any, typeID air.TypeID, path []string) (Value, *jsonDecodeError) {
	typeInfo, err := vm.typeInfo(typeID)
	if err != nil {
		return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: err.Error(), path: path}
	}

	if raw == nil {
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
		if n, ok := jsonNumber(raw); ok && math.Trunc(n) == n {
			return Int(typeID, int(n)), nil
		}
	case air.TypeFloat:
		if n, ok := jsonNumber(raw); ok {
			return Float(typeID, n), nil
		}
	case air.TypeBool:
		if b, ok := raw.(bool); ok {
			return Bool(typeID, b), nil
		}
	case air.TypeStr:
		if s, ok := raw.(string); ok {
			return Str(typeID, s), nil
		}
	case air.TypeDynamic:
		return Dynamic(typeID, raw), nil
	case air.TypeMaybe:
		inner, decodeErr := vm.jsonValueToValue(raw, typeInfo.Elem, path)
		if decodeErr != nil {
			return Value{}, decodeErr
		}
		vm.recordMaybeDetailAlloc(true)
		return Maybe(typeID, true, inner), nil
	case air.TypeList:
		items, ok := raw.([]any)
		if !ok {
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonFound(raw), path: path}
		}
		out := make([]Value, len(items))
		for i, item := range items {
			converted, decodeErr := vm.jsonValueToValue(item, typeInfo.Elem, appendPath(path, fmt.Sprintf("[%d]", i)))
			if decodeErr != nil {
				return Value{}, decodeErr
			}
			out[i] = converted
		}
		return List(typeID, out), nil
	case air.TypeMap:
		m, ok := raw.(map[string]any)
		if !ok {
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonFound(raw), path: path}
		}
		keyInfo, err := vm.typeInfo(typeInfo.Key)
		if err != nil || keyInfo.Kind != air.TypeStr {
			return Value{}, &jsonDecodeError{expected: "Str map key", found: keyInfo.Name, path: path}
		}
		entries := make([]MapEntryValue, 0, len(m))
		for k, item := range m {
			converted, decodeErr := vm.jsonValueToValue(item, typeInfo.Value, appendPath(path, k))
			if decodeErr != nil {
				return Value{}, decodeErr
			}
			entries = append(entries, MapEntryValue{Key: Str(typeInfo.Key, k), Value: converted})
		}
		return Map(typeID, entries), nil
	case air.TypeStruct:
		m, ok := raw.(map[string]any)
		if !ok {
			return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonFound(raw), path: path}
		}
		fields := make([]Value, len(typeInfo.Fields))
		for _, field := range typeInfo.Fields {
			fieldInfo, err := vm.typeInfo(field.Type)
			if err != nil {
				return Value{}, &jsonDecodeError{expected: field.Name, found: err.Error(), path: path}
			}
			item, exists := m[field.Name]
			if !exists {
				if fieldInfo.Kind == air.TypeMaybe {
					vm.recordMaybeDetailAlloc(false)
					fields[field.Index] = Maybe(field.Type, false, vm.zeroValue(fieldInfo.Elem))
					continue
				}
				return Value{}, &jsonDecodeError{expected: fieldInfo.Name, found: "missing", path: appendPath(path, field.Name)}
			}
			converted, decodeErr := vm.jsonValueToValue(item, field.Type, appendPath(path, field.Name))
			if decodeErr != nil {
				return Value{}, decodeErr
			}
			fields[field.Index] = converted
		}
		return Struct(typeID, fields), nil
	}

	return Value{}, &jsonDecodeError{expected: typeInfo.Name, found: jsonFound(raw), path: path}
}

func (vm *VM) jsonParseErr(resultType, errorType air.TypeID, decodeErr jsonDecodeError) Value {
	errorInfo, err := vm.typeInfo(errorType)
	if err != nil || errorInfo.Kind != air.TypeStruct {
		return Result(resultType, false, Str(errorType, decodeErr.Error()))
	}
	fields := make([]Value, len(errorInfo.Fields))
	for _, field := range errorInfo.Fields {
		switch field.Name {
		case "expected":
			fields[field.Index] = Str(field.Type, decodeErr.expected)
		case "found":
			fields[field.Index] = Str(field.Type, decodeErr.found)
		case "path":
			pathInfo, _ := vm.typeInfo(field.Type)
			items := make([]Value, len(decodeErr.path))
			for i, segment := range decodeErr.path {
				items[i] = Str(pathInfo.Elem, segment)
			}
			fields[field.Index] = List(field.Type, items)
		default:
			fields[field.Index] = vm.zeroValue(field.Type)
		}
	}
	return Result(resultType, false, Struct(errorType, fields))
}

func appendPath(path []string, segment string) []string {
	out := make([]string, len(path), len(path)+1)
	copy(out, path)
	out = append(out, segment)
	return out
}

func jsonNumber(raw any) (float64, bool) {
	switch n := raw.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}

func jsonFound(raw any) string {
	switch raw.(type) {
	case nil:
		return "null"
	case string:
		return "Str"
	case bool:
		return "Bool"
	case float64, float32, int, int64, uint64:
		return "Number"
	case []any:
		return "List"
	case map[string]any:
		return "Map"
	default:
		return fmt.Sprintf("%T", raw)
	}
}
