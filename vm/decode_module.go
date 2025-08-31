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
	switch name {
	case "string":
		// Return the decoder function as a Closure
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Str, checker.MakeList(checker.DecodeErrorDef)),
		}
		return runtime.Make(
			&VMClosure{
				vm:        m.vm,
				expr:      decoderType,
				builtinFn: decodeAsString,
			},
			decoderType,
		)
	}

	return nil
}

func (m *DecodeModule) Handle(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "as_string":
		return decodeAsString(args[0], call.Type().(*checker.Result))
	case "as_int":
		return decodeAsInt(args[0], call.Type().(*checker.Result))
	case "as_float":
		return decodeAsFloat(args[0], call.Type().(*checker.Result))
	case "as_bool":
		return decodeAsBool(args[0], call.Type().(*checker.Result))
	case "string":
		// Return the decoder function as a Closure
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Str, checker.MakeList(checker.DecodeErrorDef)),
		}
		return runtime.Make(
			&VMClosure{
				vm:        m.vm,
				expr:      decoderType,
				builtinFn: decodeAsString,
			},
			decoderType,
		)
	case "int":
		// Return a decoder function as a Closure
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Int, checker.MakeList(checker.DecodeErrorDef)),
		}
		return runtime.Make(
			&VMClosure{
				vm:        m.vm,
				expr:      decoderType,
				builtinFn: decodeAsInt,
			},
			decoderType,
		)
	case "float":
		// Return a decoder function as a Closure
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Float, checker.MakeList(checker.DecodeErrorDef)),
		}
		return runtime.Make(
			&VMClosure{
				vm:        m.vm,
				expr:      decoderType,
				builtinFn: decodeAsFloat,
			},
			decoderType,
		)
	case "bool":
		// Return a decoder function as a Closure
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Bool, checker.MakeList(checker.DecodeErrorDef)),
		}
		return runtime.Make(
			&VMClosure{
				vm:        m.vm,
				expr:      decoderType,
				builtinFn: decodeAsBool,
			},
			decoderType,
		)
	case "from_list":
		// Create Dynamic from List primitive
		listValue := args[0].AsList()
		return runtime.MakeDynamic(listValue)
	case "nullable":
		// Return a nullable decoder function that wraps the inner decoder
		innerDecoder := args[0]

		// Extract the inner decoder's type to determine the Maybe type
		innerDecoderType := innerDecoder.Type().(*checker.FunctionDef)
		innerReturnType := innerDecoderType.ReturnType.(*checker.Result)
		innerValueType := innerReturnType.Val()

		// Create Maybe type of the inner value type
		maybeType := checker.MakeMaybe(innerValueType)

		nullableDecoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(maybeType, checker.MakeList(checker.DecodeErrorDef)),
		}

		// Create a closure that captures the inner decoder
		nullableDecoderFn := func(data *runtime.Object, resultType *checker.Result) *runtime.Object {
			return decodeAsNullable(innerDecoder, data, resultType)
		}

		return runtime.Make(
			&VMClosure{
				vm:        m.vm,
				expr:      nullableDecoderType,
				builtinFn: nullableDecoderFn,
			},
			nullableDecoderType,
		)
	case "list":
		// Return a list decoder function that wraps the element decoder
		elementDecoder := args[0]

		// Extract the element decoder's type to determine the list type
		elementDecoderType := elementDecoder.Type().(*checker.FunctionDef)
		elementReturnType := elementDecoderType.ReturnType.(*checker.Result)
		elementValueType := elementReturnType.Val()

		// Create List type of the element value type
		listType := checker.MakeList(elementValueType)

		listDecoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(listType, checker.MakeList(checker.DecodeErrorDef)),
		}

		// Create a closure that captures the element decoder
		listDecoderFn := func(data *runtime.Object, resultType *checker.Result) *runtime.Object {
			return decodeAsList(elementDecoder, data, resultType)
		}

		return runtime.Make(
			&VMClosure{
				vm:        m.vm,
				expr:      listDecoderType,
				builtinFn: listDecoderFn,
			},
			listDecoderType,
		)
	case "map":
		// Return a map decoder function that wraps both key and value decoders
		keyDecoder := args[0]
		valueDecoder := args[1]

		// Extract the key decoder's type to determine the key type
		keyDecoderType := keyDecoder.Type().(*checker.FunctionDef)
		keyReturnType := keyDecoderType.ReturnType.(*checker.Result)
		keyValueType := keyReturnType.Val()

		// Extract the value decoder's type to determine the value type
		valueDecoderType := valueDecoder.Type().(*checker.FunctionDef)
		valueReturnType := valueDecoderType.ReturnType.(*checker.Result)
		valueValueType := valueReturnType.Val()

		// Create Map type with K keys and V values
		mapType := checker.MakeMap(keyValueType, valueValueType)

		mapDecoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(mapType, checker.MakeList(checker.DecodeErrorDef)),
		}

		// Create a closure that captures both decoders
		mapDecoderFn := func(data *runtime.Object, resultType *checker.Result) *runtime.Object {
			return decodeAsMap(keyDecoder, valueDecoder, data, resultType)
		}

		return runtime.Make(
			&VMClosure{
				vm:        m.vm,
				expr:      mapDecoderType,
				builtinFn: mapDecoderFn,
			},
			mapDecoderType,
		)
	case "field":
		// Return a field decoder function that extracts a specific field
		fieldKey := m.vm.eval(call.Args[0]).AsString()
		valueDecoder := args[1] // Decoder for the field's value

		// Extract type information
		valueDecoderType := valueDecoder.Type().(*checker.FunctionDef)
		valueReturnType := valueDecoderType.ReturnType.(*checker.Result)
		valueValueType := valueReturnType.Val()

		// Create field decoder type
		fieldDecoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(valueValueType, checker.MakeList(checker.DecodeErrorDef)),
		}

		// Create closure that captures field key and value decoder
		fieldDecoderFn := func(data *runtime.Object, resultType *checker.Result) *runtime.Object {
			return decodeAsField(fieldKey, valueDecoder, data, resultType)
		}

		return runtime.Make(
			&VMClosure{
				vm:        m.vm,
				expr:      fieldDecoderType,
				builtinFn: fieldDecoderFn,
			},
			fieldDecoderType,
		)
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

// as_string decoder implementation
// fn (Dynamic) Str![Error]
func decodeAsString(data *runtime.Object, resultType *checker.Result) *runtime.Object {
	// For Dynamic objects, check the raw value type
	if data.Raw() == nil {
		decodeErrList := makeDecodeErrorList("Str", "null")
		return runtime.MakeErr(decodeErrList)
	}
	if str, ok := data.Raw().(string); ok {
		return runtime.MakeOk(runtime.MakeStr(str))
	}

	decodeErrList := makeDecodeErrorList("Str", data.GoValue())
	return runtime.MakeErr(decodeErrList)
}

// as_int decoder implementation
// fn (Dynamic) Int![Error]
func decodeAsInt(data *runtime.Object, resultType *checker.Result) *runtime.Object {
	// For Dynamic objects, check the raw value type
	if data.Raw() == nil {
		decodeErrList := makeDecodeErrorList("Int", "null")
		return runtime.MakeErr(decodeErrList)
	}
	if intVal, ok := data.Raw().(int); ok {
		return runtime.MakeOk(runtime.MakeInt(intVal))
	}
	// SQLite integers come as int64
	if int64Val, ok := data.Raw().(int64); ok {
		return runtime.MakeOk(runtime.MakeInt(int(int64Val)))
	}
	// JSON numbers might come as float64
	if floatVal, ok := data.Raw().(float64); ok {
		int := int(floatVal)
		if floatVal == float64(int) { // Check if it's actually an integer without losing precision
			return runtime.MakeOk(runtime.MakeInt(int))
		}
	}

	decodeErrList := makeDecodeErrorList("Int", data.GoValue())
	return runtime.MakeErr(decodeErrList)
}

// as_float decoder implementation
// fn (Dynamic) Float![Error]
func decodeAsFloat(data *runtime.Object, resultType *checker.Result) *runtime.Object {
	// For Dynamic objects, check the raw value type
	if floatVal, ok := data.Raw().(float64); ok {
		return runtime.MakeOk(runtime.MakeFloat(floatVal))
	}
	if data.Raw() == nil {
		decodeErrList := makeDecodeErrorList("Float", "null")
		return runtime.MakeErr(decodeErrList)
	}
	// Allow int to float conversion
	if intVal, ok := data.Raw().(int); ok {
		return runtime.MakeOk(runtime.MakeFloat(float64(intVal)))
	}
	if intVal, ok := data.Raw().(int64); ok {
		return runtime.MakeOk(runtime.MakeFloat(float64(intVal)))
	}

	decodeErrList := makeDecodeErrorList("Float", data.GoValue())
	return runtime.MakeErr(decodeErrList)
}

// as_bool decoder implementation
func decodeAsBool(data *runtime.Object, resultType *checker.Result) *runtime.Object {
	if boolVal, ok := data.Raw().(bool); ok {
		return runtime.MakeOk(runtime.MakeBool(boolVal))
	}
	// For Dynamic objects, check the raw value type
	if data.Type() == checker.Dynamic {
		if data.Raw() == nil {
			decodeErrList := makeDecodeErrorList("Bool", "null")
			return runtime.MakeErr(decodeErrList)
		}
	}

	decodeErrList := makeDecodeErrorList("Bool", data.GoValue())
	return runtime.MakeErr(decodeErrList)
}

// decodeAsNullable handles nullable decoding by checking for null and delegating to inner decoder
func decodeAsNullable(innerDecoder *runtime.Object, data *runtime.Object, resultType *checker.Result) *runtime.Object {
	maybeType := resultType.Val().(*checker.Maybe)

	// If data is null (nil raw value in Dynamic), return maybe::none()
	if data.Type() == checker.Dynamic && data.Raw() == nil {
		noneValue := runtime.MakeMaybe(nil, maybeType.Of())
		return runtime.MakeOk(noneValue)
	}

	// Otherwise, call the inner decoder
	closure := innerDecoder.Raw().(*VMClosure)
	innerResult := closure.eval(data)

	if innerResult.IsOk() {
		// Success - turn the decoded value into maybe::some()
		return runtime.MakeOk(innerResult.ToSome())
	}
	return innerResult
}

// decodeAsList handles list decoding by checking for array data and delegating to element decoder
func decodeAsList(elementDecoder *runtime.Object, data *runtime.Object, resultType *checker.Result) *runtime.Object {
	// Check if data is Dynamic and contains array-like structure
	if data.Type() == checker.Dynamic {
		if data.Raw() == nil {
			// Null data - return error (use nullable(list(...)) for nullable lists)
			decodeErrList := makeDecodeErrorList("Array", "null")
			return runtime.MakeErr(decodeErrList)
		}

		// Check if raw data is a slice (JSON array becomes []interface{})
		if rawSlice, ok := data.Raw().([]any); ok {
			return decodeArrayElements(elementDecoder, rawSlice, resultType)
		}
	}

	// Not array-like data
	decodeErrList := makeDecodeErrorList("Array", data.Raw())
	return runtime.MakeErr(decodeErrList)
}

// decodeArrayElements decodes each element in the array using the element decoder
func decodeArrayElements(elementDecoder *runtime.Object, rawSlice []any, resultType *checker.Result) *runtime.Object {
	// Get element decoder closure
	closure := elementDecoder.Raw().(*VMClosure)

	var decodedElements []*runtime.Object
	var errors []*runtime.Object

	// Decode each element
	for i, rawElement := range rawSlice {
		elementData := runtime.Make(rawElement, checker.Dynamic)
		elementResult := closure.eval(elementData)

		if elementResult.IsOk() {
			decodedElements = append(decodedElements, runtime.Make(elementResult.Raw(), elementResult.Type()))
		} else {
			// Add element errors with path information
			elementErrors := elementResult.AsList()
			for _, err := range elementErrors {
				// Add index to error path
				errStruct := err.Raw().(map[string]*runtime.Object)
				path := errStruct["path"].Raw().([]*runtime.Object)
				indexStr := runtime.MakeStr(fmt.Sprintf("[%d]", i))
				newPath := append([]*runtime.Object{indexStr}, path...)
				errStruct["path"] = runtime.Make(newPath, checker.MakeList(checker.Str))
				errors = append(errors, err)
			}
		}
	}

	if len(errors) > 0 {
		// Return accumulated errors
		errorList := runtime.Make(errors, checker.MakeList(checker.DecodeErrorDef))
		return runtime.MakeErr(errorList)
	}

	// Success - create list object
	listType := resultType.Val().(*checker.List)
	listObject := runtime.Make(decodedElements, listType)
	return runtime.MakeOk(listObject)
}

// decodeAsMap handles map decoding by checking for object data and delegating to key/value decoders
func decodeAsMap(keyDecoder *runtime.Object, valueDecoder *runtime.Object, data *runtime.Object, resultType *checker.Result) *runtime.Object {
	// Check if data is Dynamic and contains object-like structure
	if data.Type() == checker.Dynamic {
		if data.Raw() == nil {
			// Null data - return error (use nullable(map(...)) for nullable maps)
			decodeErrList := makeDecodeErrorList("Object", "null")
			return runtime.MakeErr(decodeErrList)
		}

		// Check if raw data is a map (JSON object becomes map[string]interface{})
		if rawMap, ok := data.Raw().(map[string]interface{}); ok {
			return decodeMapValues(keyDecoder, valueDecoder, rawMap, resultType)
		}
	}

	// Not object-like data
	decodeErrList := makeDecodeErrorList("Object", data.String())
	return runtime.MakeErr(decodeErrList)
}

// decodeMapValues decodes each key and value in the object using their respective decoders
func decodeMapValues(keyDecoder *runtime.Object, valueDecoder *runtime.Object, rawMap map[string]any, resultType *checker.Result) *runtime.Object {
	// Get decoder closures
	keyClosure := keyDecoder.Raw().(*VMClosure)
	valueClosure := valueDecoder.Raw().(*VMClosure)

	// Create a new map to store decoded keys and values
	decodedMap := make(map[string]*runtime.Object)
	var errors []*runtime.Object

	// Decode each key-value pair
	for rawKey, rawValue := range rawMap {
		// Decode key
		keyData := runtime.Make(rawKey, checker.Dynamic)
		keyResult := keyClosure.eval(keyData)

		// Decode value
		valueData := runtime.Make(rawValue, checker.Dynamic)
		valueResult := valueClosure.eval(valueData)

		if keyResult.IsOk() && valueResult.IsOk() {
			// Both key and value decoded successfully
			// Convert decoded key to string for map storage
			decodedKey := runtime.ToMapKey(keyResult)
			decodedMap[decodedKey] = valueResult.Unwrap()
		} else {
			// Add errors with path information
			if keyResult.IsErr() {
				keyErrors := keyResult.AsList()
				for _, err := range keyErrors {
					// Add key path information
					errStruct := err.Raw().(map[string]*runtime.Object)
					path := errStruct["path"].Raw().([]*runtime.Object)
					keyStr := runtime.MakeStr(fmt.Sprintf("key(%s)", rawKey))
					newPath := append([]*runtime.Object{keyStr}, path...)
					errStruct["path"] = runtime.Make(newPath, checker.MakeList(checker.Str))
					errors = append(errors, err)
				}
			}
			if valueResult.IsErr() {
				valueErrors := valueResult.AsList()
				for _, err := range valueErrors {
					// Add value path information
					errStruct := err.Raw().(map[string]*runtime.Object)
					path := errStruct["path"].Raw().([]*runtime.Object)
					keyStr := runtime.MakeStr(fmt.Sprintf("value(%s)", rawKey))
					newPath := append([]*runtime.Object{keyStr}, path...)
					errStruct["path"] = runtime.Make(newPath, checker.MakeList(checker.Str))
					errors = append(errors, err)
				}
			}
		}
	}

	if len(errors) > 0 {
		// Return accumulated errors
		errorList := runtime.Make(errors, checker.MakeList(checker.DecodeErrorDef))
		return runtime.MakeErr(errorList)
	}

	// Success - create map object
	mapType := resultType.Val().(*checker.Map)
	mapObject := runtime.Make(decodedMap, mapType)
	return runtime.MakeOk(mapObject)
}

// decodeAsField extracts a specific field from an object and decodes it
func decodeAsField(fieldKey string, valueDecoder *runtime.Object, data *runtime.Object, resultType *checker.Result) *runtime.Object {
	// Check if data is Dynamic and contains object-like structure
	if data.Raw() == nil {
		// Null data - return error
		decodeErrList := makeDecodeErrorList("Object", "null")
		return runtime.MakeErr(decodeErrList)
	}

	// Check if raw data is a map (JSON object becomes map[string]any)
	if rawMap, ok := data.Raw().(map[string]any); ok {
		return extractField(fieldKey, valueDecoder, rawMap, resultType)
	}

	// Not object-like data
	decodeErrList := makeDecodeErrorList("Object", data.String())
	return runtime.MakeErr(decodeErrList)
}

// extractField handles the actual field extraction and value decoding
func extractField(fieldKey string, valueDecoder *runtime.Object, rawMap map[string]any, resultType *checker.Result) *runtime.Object {
	// Get value decoder closure
	valueClosure := valueDecoder.Raw().(*VMClosure)

	// Check if field exists
	rawValue, exists := rawMap[fieldKey]
	if !exists {
		// Missing field error with path
		decodeErr := runtime.MakeStruct(
			checker.DecodeErrorDef,
			map[string]*runtime.Object{
				"expected": runtime.MakeStr("field '" + fieldKey + "'"),
				"found":    runtime.MakeStr("missing"),
				"path":     runtime.MakeList(checker.Str, runtime.MakeStr(fieldKey)),
			},
		)
		errorList := runtime.MakeList(checker.DecodeErrorDef, decodeErr)
		return runtime.MakeErr(errorList)
	}

	// Field exists, decode its value
	valueData := runtime.MakeDynamic(rawValue)
	valueResult := valueClosure.eval(valueData)

	if valueResult.IsOk() {
		return valueResult
	}

	// Propagate errors with field name in path
	errors := valueResult.AsList()
	for _, err := range errors {
		_path := err.Struct_Get("path")
		path := _path.AsList()
		fieldStr := runtime.MakeStr(fieldKey)
		// shouldn't this go on the end?
		_path.Set(append([]*runtime.Object{fieldStr}, path...))
	}

	// question: maybe should copy instead of mutate
	return valueResult
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
