package ffi

import (
	"encoding/json/v2"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

var (
	decodeErrorType     checker.Type
	decodeErrorTypeOnce sync.Once
)

func getDecodeErrorType() checker.Type {
	decodeErrorTypeOnce.Do(func() {
		mod, ok := checker.FindEmbeddedModule("ard/decode")
		if !ok {
			panic("failed to load ard/decode embedded module")
		}
		sym := mod.Get("Error")
		if sym.Type == nil {
			panic("Error type not found in ard/decode module")
		}
		decodeErrorType = sym.Type
	})
	return decodeErrorType
}

func ListToDynamic(items []any) any {
	raw := make([]any, len(items))
	copy(raw, items)
	return raw
}

func MapToDynamic(m map[string]any) any {
	raw := make(map[string]any, len(m))
	for k, v := range m {
		raw[k] = v
	}
	return raw
}

// Parse external data (JSON text) into Dynamic object
// JsonToDynamic parses a JSON string into a Dynamic value.
func JsonToDynamic(jsonString string) (any, error) {
	var raw any
	err := json.Unmarshal([]byte(jsonString), &raw)
	if err != nil {
		return nil, fmt.Errorf("Error parsing JSON: %s", err.Error())
	}
	return raw, nil
}

// fn (Dynamic) Str!Error
func DecodeString(args []*runtime.Object) *runtime.Object {
	errType := getDecodeErrorType()
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Str", "null", errType))
	}
	if str, ok := data.(string); ok {
		return runtime.MakeOk(runtime.MakeStr(str))
	}

	return runtime.MakeErr(makeError("Str", formatRawValueForError(arg.GoValue()), errType))
}

// fn (Dynamic) Int!Error
func DecodeInt(args []*runtime.Object) *runtime.Object {
	errType := getDecodeErrorType()
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Int", "null", errType))
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

	return runtime.MakeErr(makeError("Int", formatRawValueForError(arg.GoValue()), errType))
}

// fn (Dynamic) Float!Error
func DecodeFloat(args []*runtime.Object) *runtime.Object {
	errType := getDecodeErrorType()
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Float", "null", errType))
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

	return runtime.MakeErr(makeError("Float", formatRawValueForError(arg.GoValue()), errType))
}

// fn (Dynamic) Bool!Error
func DecodeBool(args []*runtime.Object) *runtime.Object {
	errType := getDecodeErrorType()
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Bool", "null", errType))
	}
	if val, ok := data.(bool); ok {
		return runtime.MakeOk(runtime.MakeBool(val))
	}

	return runtime.MakeErr(makeError("Bool", formatRawValueForError(arg.GoValue()), errType))
}

// fn (Dynamic) Bool
// IsNil checks if a Dynamic value is nil.
func IsNil(value any) bool {
	return value == nil
}

// fn (Dyanmic) [Dynamic]!Str
func DynamicToList(args []*runtime.Object) *runtime.Object {
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
func DynamicToMap(args []*runtime.Object) *runtime.Object {
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
func ExtractField(args []*runtime.Object) *runtime.Object {
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

func MakeDecodeError(expected, found string) *runtime.Object {
	return makeError(expected, found, getDecodeErrorType())
}

func FormatRawValueForError(v any) string {
	return formatRawValueForError(v)
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
