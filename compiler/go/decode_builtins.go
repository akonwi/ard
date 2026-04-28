package ardgo

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/akonwi/ard/ffi"
)

type builtinDecodeError struct {
	Expected string
	Found    string
	Path     []string
}

func makeBuiltinDecodeError(expected, found string) builtinDecodeError {
	return builtinDecodeError{
		Expected: expected,
		Found:    found,
		Path:     []string{},
	}
}

func builtinDecodeString(data any) Result[string, builtinDecodeError] {
	data = builtinDynamicValue(data)
	if data == nil {
		return Err[string, builtinDecodeError](makeBuiltinDecodeError("Str", "null"))
	}
	if value, ok := data.(string); ok {
		return Ok[string, builtinDecodeError](value)
	}
	return Err[string, builtinDecodeError](makeBuiltinDecodeError("Str", formatBuiltinRawValueForError(data)))
}

func builtinDecodeInt(data any) Result[int, builtinDecodeError] {
	data = builtinDynamicValue(data)
	if data == nil {
		return Err[int, builtinDecodeError](makeBuiltinDecodeError("Int", "null"))
	}
	switch value := data.(type) {
	case float64:
		intValue := int(value)
		if value == float64(intValue) {
			return Ok[int, builtinDecodeError](intValue)
		}
	case int:
		return Ok[int, builtinDecodeError](value)
	case int64:
		return Ok[int, builtinDecodeError](int(value))
	}
	return Err[int, builtinDecodeError](makeBuiltinDecodeError("Int", formatBuiltinRawValueForError(data)))
}

func builtinDecodeFloat(data any) Result[float64, builtinDecodeError] {
	data = builtinDynamicValue(data)
	if data == nil {
		return Err[float64, builtinDecodeError](makeBuiltinDecodeError("Float", "null"))
	}
	switch value := data.(type) {
	case float64:
		return Ok[float64, builtinDecodeError](value)
	case int:
		return Ok[float64, builtinDecodeError](float64(value))
	case int64:
		return Ok[float64, builtinDecodeError](float64(value))
	}
	return Err[float64, builtinDecodeError](makeBuiltinDecodeError("Float", formatBuiltinRawValueForError(data)))
}

func builtinDecodeBool(data any) Result[bool, builtinDecodeError] {
	data = builtinDynamicValue(data)
	if data == nil {
		return Err[bool, builtinDecodeError](makeBuiltinDecodeError("Bool", "null"))
	}
	if value, ok := data.(bool); ok {
		return Ok[bool, builtinDecodeError](value)
	}
	return Err[bool, builtinDecodeError](makeBuiltinDecodeError("Bool", formatBuiltinRawValueForError(data)))
}

func builtinDynamicToList(data any) Result[[]any, string] {
	data = builtinDynamicValue(data)
	if data == nil {
		return Err[[]any, string]("null")
	}
	if items, ok := data.([]any); ok {
		return Ok[[]any, string](items)
	}
	value := reflect.ValueOf(data)
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return Err[[]any, string]("null")
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return Err[[]any, string](formatBuiltinRawValueForError(data))
	}
	out := make([]any, value.Len())
	for i := 0; i < value.Len(); i++ {
		out[i] = builtinDynamicValue(value.Index(i).Interface())
	}
	return Ok[[]any, string](out)
}

func builtinDynamicToMap(data any) Result[map[any]any, string] {
	data = builtinDynamicValue(data)
	if data == nil {
		return Err[map[any]any, string]("null")
	}
	if items, ok := data.(map[string]any); ok {
		out := make(map[any]any, len(items))
		for key, value := range items {
			out[key] = value
		}
		return Ok[map[any]any, string](out)
	}
	if items, ok := data.(map[any]any); ok {
		return Ok[map[any]any, string](items)
	}
	value := reflect.ValueOf(data)
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return Err[map[any]any, string]("null")
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Map {
		return Err[map[any]any, string](formatBuiltinRawValueForError(data))
	}
	out := make(map[any]any, value.Len())
	iter := value.MapRange()
	for iter.Next() {
		keyValue := iter.Key()
		for keyValue.Kind() == reflect.Interface {
			if keyValue.IsNil() {
				return Err[map[any]any, string](formatBuiltinRawValueForError(data))
			}
			keyValue = keyValue.Elem()
		}
		if keyValue.Kind() != reflect.String {
			return Err[map[any]any, string](formatBuiltinRawValueForError(data))
		}
		out[keyValue.String()] = builtinDynamicValue(iter.Value().Interface())
	}
	return Ok[map[any]any, string](out)
}

func builtinExtractField(data any, name string) Result[any, string] {
	data = builtinDynamicValue(data)
	if data == nil {
		return Err[any, string]("null")
	}
	if mapped, ok := data.(map[string]any); ok {
		value, ok := mapped[name]
		if !ok {
			return Ok[any, string](nil)
		}
		return Ok[any, string](value)
	}
	mapped := builtinDynamicToMap(data)
	if mapped.IsErr() {
		return Err[any, string](mapped.UnwrapErr())
	}
	value, ok := mapped.UnwrapOk()[name]
	if !ok {
		return Ok[any, string](nil)
	}
	return Ok[any, string](value)
}

func formatBuiltinRawValueForError(v any) string {
	switch val := v.(type) {
	case string:
		if len(val) > 50 {
			return fmt.Sprintf("\"%s...\"", val[:47])
		}
		return fmt.Sprintf("\"%s\"", val)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	case []any:
		if len(val) == 0 {
			return "[]"
		}
		if len(val) <= 3 {
			var preview strings.Builder
			preview.WriteString("[")
			for i, item := range val {
				if i > 0 {
					preview.WriteString(", ")
				}
				preview.WriteString(formatBuiltinRawValueForError(item))
			}
			preview.WriteString("]")
			return preview.String()
		}
		return fmt.Sprintf("[array with %d elements]", len(val))
	case map[string]any:
		if len(val) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		if len(keys) <= 3 {
			var preview strings.Builder
			preview.WriteString("{")
			for i, key := range keys {
				if i > 0 {
					preview.WriteString(", ")
				}
				preview.WriteString(fmt.Sprintf("\"%s\": %s", key, formatBuiltinRawValueForError(val[key])))
			}
			preview.WriteString("}")
			return preview.String()
		}
		return fmt.Sprintf("{object with %d fields}", len(val))
	default:
		return fmt.Sprintf("%v", v)
	}
}

func DecodeStringExtern[E any](data any) Result[string, E] {
	result := builtinDecodeString(data)
	if result.ok {
		return Result[string, E]{value: result.value, ok: true}
	}
	return Result[string, E]{err: CoerceExtern[E](result.err)}
}

func DecodeIntExtern[E any](data any) Result[int, E] {
	result := builtinDecodeInt(data)
	if result.ok {
		return Result[int, E]{value: result.value, ok: true}
	}
	return Result[int, E]{err: CoerceExtern[E](result.err)}
}

func DecodeFloatExtern[E any](data any) Result[float64, E] {
	result := builtinDecodeFloat(data)
	if result.ok {
		return Result[float64, E]{value: result.value, ok: true}
	}
	return Result[float64, E]{err: CoerceExtern[E](result.err)}
}

func DecodeBoolExtern[E any](data any) Result[bool, E] {
	result := builtinDecodeBool(data)
	if result.ok {
		return Result[bool, E]{value: result.value, ok: true}
	}
	return Result[bool, E]{err: CoerceExtern[E](result.err)}
}

func DynamicToListExtern(data any) Result[[]any, string] {
	return builtinDynamicToList(data)
}

func DynamicToMapExtern(data any) Result[map[any]any, string] {
	return builtinDynamicToMap(data)
}

func ExtractFieldExtern(data any, name string) Result[any, string] {
	return builtinExtractField(data, name)
}

func JsonToDynamicExtern(jsonString string) Result[any, string] {
	return builtinJsonToDynamic(jsonString)
}

func builtinJsonToDynamic(jsonString string) Result[any, string] {
	value, err := ffi.JsonToDynamic(jsonString)
	if err != nil {
		return Err[any, string](err.Error())
	}
	return Ok[any, string](value)
}
