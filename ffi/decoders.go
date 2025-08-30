package ffi

import (
	"fmt"
	"strconv"

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

// fn (Dynamic) Str!Error
func DecodeString(args []*runtime.Object, _ checker.Type) *runtime.Object {
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Str", "null"))
	}
	if str, ok := data.(string); ok {
		return runtime.MakeOk(runtime.MakeStr(str))
	}

	return runtime.MakeErr(makeError("Str", formatRawValueForError(arg.GoValue())))
}

// fn (Dynamic) Int!Error
func DecodeInt(args []*runtime.Object, _ checker.Type) *runtime.Object {
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Int", "null"))
	}
	if int, ok := data.(int); ok {
		return runtime.MakeOk(runtime.MakeInt(int))
	}

	return runtime.MakeErr(makeError("Int", formatRawValueForError(arg.GoValue())))
}

// fn (Dynamic) Float!Error
func DecodeFloat(args []*runtime.Object, _ checker.Type) *runtime.Object {
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Float", "null"))
	}
	if float, ok := data.(float64); ok {
		return runtime.MakeOk(runtime.MakeFloat(float))
	}

	return runtime.MakeErr(makeError("Float", formatRawValueForError(arg.GoValue())))
}

// fn (Dynamic) Bool!Error
func DecodeBool(args []*runtime.Object, _ checker.Type) *runtime.Object {
	arg := args[0]
	data := arg.Raw()
	if data == nil {
		return runtime.MakeErr(makeError("Bool", "null"))
	}
	if val, ok := data.(bool); ok {
		return runtime.MakeOk(runtime.MakeBool(val))
	}

	return runtime.MakeErr(makeError("Bool", formatRawValueForError(arg.GoValue())))
}

func makeError(expected, found string) *runtime.Object {
	return runtime.MakeStruct(checker.DecodeErrorDef,
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
			preview := "["
			for i, item := range val {
				if i > 0 {
					preview += ", "
				}
				preview += formatRawValueForError(item)
			}
			preview += "]"
			return preview
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
			preview := "{"
			for i, key := range keys {
				if i > 0 {
					preview += ", "
				}
				preview += fmt.Sprintf("%s: %s", key, formatRawValueForError(val[key]))
			}
			preview += "}"
			return preview
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
