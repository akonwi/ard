package ffi

import (
	"encoding/json/v2"
	"fmt"
	"strconv"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

func StrToDynamic(args []*runtime.Object, _ checker.Type) *runtime.Object {
	strValue := args[0].AsString()
	return runtime.MakeDynamic(strValue)
}

func IntToDynamic(args []*runtime.Object, _ checker.Type) *runtime.Object {
	intValue := args[0].AsInt()
	return runtime.MakeDynamic(intValue)
}

func FloatToDynamic(args []*runtime.Object, _ checker.Type) *runtime.Object {
	floatValue := args[0].AsFloat()
	return runtime.MakeDynamic(floatValue)
}

func BoolToDynamic(args []*runtime.Object, _ checker.Type) *runtime.Object {
	return runtime.MakeDynamic(args[0].Raw())
}

func VoidToDynamic(args []*runtime.Object, _ checker.Type) *runtime.Object {
	return runtime.MakeDynamic(nil)
}

func ListToDynamic(args []*runtime.Object, _ checker.Type) *runtime.Object {
	arg := args[0].AsList()
	raw := make([]any, len(arg))
	for i, item := range arg {
		raw[i] = item.Raw()
	}
	return runtime.MakeDynamic(raw)
}

func MapToDynamic(args []*runtime.Object, _ checker.Type) *runtime.Object {
	arg := args[0].AsMap()
	raw := map[string]any{}
	for key, val := range arg {
		raw[key] = val.Raw()
	}
	return runtime.MakeDynamic(raw)
}

// Parse external data (JSON text) into Dynamic object
func JsonToDynamic(args []*runtime.Object, _ checker.Type) *runtime.Object {
	jsonString := args[0].AsString()
	jsonBytes := []byte(jsonString)

	// Parse JSON into Dynamic object, fallback to nil if parsing fails
	var raw any
	err := json.Unmarshal(jsonBytes, &raw)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("Error parsing JSON: %s", err.Error())))
	}

	return runtime.MakeOk(runtime.MakeDynamic(raw))
}

// fn (Dynamic) Str!Error
func DecodeString(args []*runtime.Object, outType checker.Type) *runtime.Object {
	resultType := outType.(*checker.Result)
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Str", "null", resultType.Err()))
	}
	if str, ok := data.(string); ok {
		return runtime.MakeOk(runtime.MakeStr(str))
	}

	return runtime.MakeErr(makeError("Str", formatRawValueForError(arg.GoValue()), resultType.Err()))
}

// fn (Dynamic) Int!Error
func DecodeInt(args []*runtime.Object, outType checker.Type) *runtime.Object {
	resultType := outType.(*checker.Result)
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Int", "null", resultType.Err()))
	}
	if int, ok := data.(int); ok {
		return runtime.MakeOk(runtime.MakeInt(int))
	}
	// SQLite integers come as int64
	if int64Val, ok := data.(int64); ok {
		return runtime.MakeOk(runtime.MakeInt(int(int64Val)))
	}
	// JSON numbers might come as float64
	if floatVal, ok := data.(float64); ok {
		int := int(floatVal)
		if floatVal == float64(int) { // Check if it's actually an integer without losing precision
			return runtime.MakeOk(runtime.MakeInt(int))
		}
	}

	return runtime.MakeErr(makeError("Int", formatRawValueForError(arg.GoValue()), resultType.Err()))
}

// fn (Dynamic) Float!Error
func DecodeFloat(args []*runtime.Object, outType checker.Type) *runtime.Object {
	resultType := outType.(*checker.Result)
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Float", "null", resultType.Err()))
	}
	if float, ok := data.(float64); ok {
		return runtime.MakeOk(runtime.MakeFloat(float))
	}
	// Allow int to float conversion
	if intVal, ok := data.(int); ok {
		return runtime.MakeOk(runtime.MakeFloat(float64(intVal)))
	}
	if intVal, ok := data.(int64); ok {
		return runtime.MakeOk(runtime.MakeFloat(float64(intVal)))
	}

	return runtime.MakeErr(makeError("Float", formatRawValueForError(arg.GoValue()), resultType.Err()))
}

// fn (Dynamic) Bool!Error
func DecodeBool(args []*runtime.Object, outType checker.Type) *runtime.Object {
	resultType := outType.(*checker.Result)
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Bool", "null", resultType.Err()))
	}
	if val, ok := data.(bool); ok {
		return runtime.MakeOk(runtime.MakeBool(val))
	}

	return runtime.MakeErr(makeError("Bool", formatRawValueForError(arg.GoValue()), resultType.Err()))
}

// fn (Dynamic) Bool
func IsNil(args []*runtime.Object, _ checker.Type) *runtime.Object {
	isNil := args[0].Raw() == nil
	return runtime.MakeBool(isNil)
}

// fn (Dyanmic) [Dynamic]!Str
func DynamicToList(args []*runtime.Object, _ checker.Type) *runtime.Object {
	arg := args[0]
	data := arg.Raw()

	if data == nil {
		return runtime.MakeErr(runtime.MakeStr("null"))
	}

	if dataList, ok := data.([]any); ok {
		items := make([]*runtime.Object, len(dataList))
		for i, item := range dataList {
			items[i] = runtime.Make(item, arg.Type())
		}
		return runtime.MakeOk(runtime.MakeList(arg.Type(), items...))
	}

	return runtime.MakeErr(runtime.MakeStr(formatRawValueForError(arg.GoValue())))
}

// fn (Dyanmic) [Dynamic:Dynamic]!Str
func DynamicToMap(args []*runtime.Object, _ checker.Type) *runtime.Object {
	arg := args[0]
	data := arg.Raw()

	if data == nil {
		return runtime.MakeErr(runtime.MakeStr("null"))
	}

	if dataMap, ok := data.(map[string]any); ok {
		_map := runtime.MakeMap(checker.Dynamic, checker.Dynamic)
		for i, item := range dataMap {
			_map.Map_Set(runtime.MakeDynamic(i), runtime.MakeDynamic(item))
		}
		return runtime.MakeOk(_map)
	}

	return runtime.MakeErr(runtime.MakeStr(formatRawValueForError(arg.GoValue())))
}

// fn (Dynamic, Str) Dynamic!Str
func ExtractField(args []*runtime.Object, _ checker.Type) *runtime.Object {
	arg := args[0]
	data := arg.Raw()

	if data == nil {
		return runtime.MakeErr(runtime.MakeStr("null"))
	}

	dataMap, ok := data.(map[string]any)
	if !ok {
		return runtime.MakeErr(runtime.MakeStr(formatRawValueForError(arg.GoValue())))
	}

	fieldName := args[1].AsString()

	found := runtime.MakeDynamic(nil)
	if val, ok := dataMap[fieldName]; ok {
		found.Set(val)
	}

	return runtime.MakeOk(found)
}

func makeError(expected, found string, _type checker.Type) *runtime.Object {
	return runtime.MakeStruct(_type,
		map[string]*runtime.Object{
			"expected": runtime.MakeStr(expected),
			"found":    runtime.MakeStr(found),
			"path":     runtime.MakeList(checker.Str),
		},
	)
}

// Helper function to format raw values with smart truncation and previews
func formatRawValueForError(v any) string {
	switch val := v.(type) {
	case string:
		// Truncate very long strings for readability
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
		// Show preview of array contents for small arrays
		if len(val) == 0 {
			return "[]"
		} else if len(val) <= 3 {
			var preview strings.Builder
			preview.WriteString("[")
			for i, item := range val {
				if i > 0 {
					preview.WriteString(", ")
				}
				preview.WriteString(formatRawValueForError(item))
			}
			preview.WriteString("]")
			return preview.String()
		}
		return fmt.Sprintf("[array with %d elements]", len(val))
	case map[string]any:
		// Show preview of object contents for small objects
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
				preview.WriteString(fmt.Sprintf("%s: %s", key, formatRawValueForError(val[key])))
			}
			preview.WriteString("}")
			return preview.String()
		}
		return fmt.Sprintf("{object with keys: %v}", keys[:3])
	case nil:
		return "null"
	default:
		str := fmt.Sprintf("%v", val)
		if len(str) > 50 {
			return str[:47] + "..."
		}
		return str
	}
}
