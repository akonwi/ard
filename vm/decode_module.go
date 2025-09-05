//go:build goexperiment.jsonv2

package vm

import (
	"fmt"
	"strconv"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

// DecodeModule handles ard/decode module functions
type DecodeModule struct {
	vm *VM
}

func (m *DecodeModule) Path() string {
	return "ard/decode"
}

func (m *DecodeModule) Program() *checker.Program {
	return nil
}

func (m *DecodeModule) get(name string) *runtime.Object {
	return nil
}

func (m *DecodeModule) Handle(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "from_list":
		// Create Dynamic from List primitive
		listValue := args[0].AsList()
		return runtime.MakeDynamic(listValue)
	case "one_of":
		// Return a decoder that tries multiple decoders in sequence
		decoderList := args[0] // List of decoders to try

		// Extract the decoder list to get the inner type
		decoderListObj := decoderList.AsList()
		if len(decoderListObj) == 0 {
			// Return error instead of panicking - empty decoder lists are not allowed
			emptyListError := makeDecodeErrorList("At least one decoder", "Empty decoder list")

			// Create a proper decoder type that returns an error
			errorDecoderType := &checker.FunctionDef{
				Name:       "OneOfDecoder",
				Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
				ReturnType: checker.MakeResult(checker.Void, checker.MakeList(checker.DecodeErrorDef)),
			}

			return runtime.Make(
				&VMClosure{
					vm:   m.vm,
					expr: errorDecoderType,
					builtinFn: func(data *runtime.Object, resultType *checker.Result) *runtime.Object {
						return runtime.MakeErr(emptyListError)
					},
				},
				errorDecoderType,
			)
		}

		// All decoders should have the same return type - use the first one
		firstDecoderType := decoderListObj[0].Type().(*checker.FunctionDef)
		firstReturnType := firstDecoderType.ReturnType.(*checker.Result)
		commonValueType := firstReturnType.Val()

		// Create one_of decoder type
		oneOfDecoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(commonValueType, checker.MakeList(checker.DecodeErrorDef)),
		}

		// Create closure that captures the decoder list
		oneOfDecoderFn := func(data *runtime.Object, resultType *checker.Result) *runtime.Object {
			return decodeAsOneOf(decoderList, data, resultType)
		}

		return runtime.Make(
			&VMClosure{
				vm:        m.vm,
				expr:      oneOfDecoderType,
				builtinFn: oneOfDecoderFn,
			},
			oneOfDecoderType,
		)
	default:
		panic(fmt.Errorf("Unimplemented: decode::%s()", call.Name))
	}
}

func (m *DecodeModule) HandleStatic(structName string, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	panic(fmt.Errorf("Unimplemented: decode::%s::%s()", structName, call.Name))
}

// Helper function to create DecodeError
func makeDecodeError(expected, found string) *runtime.Object {
	return runtime.MakeStruct(checker.DecodeErrorDef,
		map[string]*runtime.Object{
			"expected": runtime.MakeStr(expected),
			"found":    runtime.MakeStr(found),
			"path":     runtime.MakeList(checker.Str),
		},
	)
}

// Helper function to create [DecodeError] with one error
func makeDecodeErrorList(expected string, found any) *runtime.Object {
	decodeErr := makeDecodeError(expected, formatRawValueForError(found))
	return runtime.MakeList(checker.DecodeErrorDef, decodeErr)
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

// decodeAsOneOf tries multiple decoders in sequence until one succeeds
func decodeAsOneOf(decoderList *runtime.Object, data *runtime.Object, resultType *checker.Result) *runtime.Object {
	decoders := decoderList.AsList()

	if len(decoders) == 0 {
		emptyListError := makeDecodeErrorList("At least one decoder", "Empty decoder list")
		return runtime.MakeErr(emptyListError)
	}

	var firstError *runtime.Object

	// Try each decoder in sequence
	for i, decoder := range decoders {
		closure := decoder.Raw().(*VMClosure)
		result := closure.eval(data)

		if result.IsOk() {
			return result
		}

		// Store the first error (following Gleam's pattern)
		if i == 0 {
			firstError = result
		}
	}

	// All decoders failed - return the first error
	return firstError
}
