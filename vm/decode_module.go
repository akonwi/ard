//go:build goexperiment.jsonv2

package vm

import (
	"encoding/json/v2"
	"fmt"
	"strconv"

	"github.com/akonwi/ard/checker"
)

// DecodeModule handles ard/decode module functions
type DecodeModule struct{}

func (m *DecodeModule) Path() string {
	return "ard/decode"
}

func (m *DecodeModule) Program() *checker.Program {
	return nil
}

func (m *DecodeModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
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
		return &object{
			raw: &Closure{
				vm:        vm,
				expr:      *decoderType,
				builtinFn: decodeAsString,
			},
			_type: decoderType,
		}
	case "int":
		// Return a decoder function as a Closure
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Int, checker.MakeList(checker.DecodeErrorDef)),
		}
		return &object{
			raw: &Closure{
				vm:        vm,
				expr:      *decoderType,
				builtinFn: decodeAsInt,
			},
			_type: decoderType,
		}
	case "float":
		// Return a decoder function as a Closure
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Float, checker.MakeList(checker.DecodeErrorDef)),
		}
		return &object{
			raw: &Closure{
				vm:        vm,
				expr:      *decoderType,
				builtinFn: decodeAsFloat,
			},
			_type: decoderType,
		}
	case "bool":
		// Return a decoder function as a Closure
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Bool, checker.MakeList(checker.DecodeErrorDef)),
		}
		return &object{
			raw: &Closure{
				vm:        vm,
				expr:      *decoderType,
				builtinFn: decodeAsBool,
			},
			_type: decoderType,
		}
	case "run":
		// Apply the decoder function to the data
		data := args[0]    // Data comes first now
		decoder := args[1] // Decoder comes second

		// All decoders are now Closures - use unified approach
		closure := decoder.raw.(*Closure)
		decoderResult := closure.eval(data)

		// Decoder already returns list-based errors, just return the result
		resultWithList := call.Type().(*checker.Result)
		decoderResultValue := decoderResult.raw.(_result)

		if decoderResultValue.ok {
			// Success - return the value
			return makeOk(decoderResultValue.raw, resultWithList)
		} else {
			// Error - decoder already returns error list
			errorList := decoderResultValue.raw
			return makeErr(errorList, resultWithList)
		}
	case "json":
		// Parse external data (JSON string) into Dynamic object
		jsonString := vm.eval(call.Args[0]).raw.(string)
		jsonBytes := []byte(jsonString)

		// Parse JSON into Dynamic object, fallback to nil if parsing fails
		dynamicObj, err := parseJsonToDynamic(jsonBytes)
		if err != nil {
			// Return nil as Dynamic - this is valid and will be caught by decoders
			return &object{
				raw:   nil,
				_type: checker.Dynamic,
			}
		}

		return dynamicObj
	case "nullable":
		// Return a nullable decoder function that wraps the inner decoder
		innerDecoder := args[0]

		// Extract the inner decoder's type to determine the Maybe type
		innerDecoderType := innerDecoder._type.(*checker.FunctionDef)
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
		nullableDecoderFn := func(data *object, resultType *checker.Result) *object {
			return decodeAsNullable(innerDecoder, data, resultType)
		}

		return &object{
			raw: &Closure{
				vm:        vm,
				expr:      *nullableDecoderType,
				builtinFn: nullableDecoderFn,
			},
			_type: nullableDecoderType,
		}
	case "list":
		// Return a list decoder function that wraps the element decoder
		elementDecoder := args[0]

		// Extract the element decoder's type to determine the list type
		elementDecoderType := elementDecoder._type.(*checker.FunctionDef)
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
		listDecoderFn := func(data *object, resultType *checker.Result) *object {
			return decodeAsList(elementDecoder, data, resultType)
		}

		return &object{
			raw: &Closure{
				vm:        vm,
				expr:      *listDecoderType,
				builtinFn: listDecoderFn,
			},
			_type: listDecoderType,
		}
	case "map":
		// Return a map decoder function that wraps both key and value decoders
		keyDecoder := args[0]
		valueDecoder := args[1]

		// Extract the key decoder's type to determine the key type
		keyDecoderType := keyDecoder._type.(*checker.FunctionDef)
		keyReturnType := keyDecoderType.ReturnType.(*checker.Result)
		keyValueType := keyReturnType.Val()

		// Extract the value decoder's type to determine the value type
		valueDecoderType := valueDecoder._type.(*checker.FunctionDef)
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
		mapDecoderFn := func(data *object, resultType *checker.Result) *object {
			return decodeAsMap(keyDecoder, valueDecoder, data, resultType)
		}

		return &object{
			raw: &Closure{
				vm:        vm,
				expr:      *mapDecoderType,
				builtinFn: mapDecoderFn,
			},
			_type: mapDecoderType,
		}
	case "field":
		// Return a field decoder function that extracts a specific field
		fieldKey := vm.eval(call.Args[0]).raw.(string) // The field name to extract
		valueDecoder := args[1]                        // Decoder for the field's value

		// Extract type information
		valueDecoderType := valueDecoder._type.(*checker.FunctionDef)
		valueReturnType := valueDecoderType.ReturnType.(*checker.Result)
		valueValueType := valueReturnType.Val()

		// Create field decoder type
		fieldDecoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(valueValueType, checker.MakeList(checker.DecodeErrorDef)),
		}

		// Create closure that captures field key and value decoder
		fieldDecoderFn := func(data *object, resultType *checker.Result) *object {
			return decodeAsField(fieldKey, valueDecoder, data, resultType)
		}

		return &object{
			raw: &Closure{
				vm:        vm,
				expr:      *fieldDecoderType,
				builtinFn: fieldDecoderFn,
			},
			_type: fieldDecoderType,
		}
	case "one_of":
		// Return a decoder that tries multiple decoders in sequence
		decoderList := args[0] // List of decoders to try

		// Extract the decoder list to get the inner type
		decoderListObj := decoderList.raw.([]*object)
		if len(decoderListObj) == 0 {
			// Return error instead of panicking - empty decoder lists are not allowed
			emptyListError := makeDecodeErrorList("At least one decoder", "Empty decoder list")
			
			// Create a proper decoder type that returns an error
			errorDecoderType := &checker.FunctionDef{
				Name:       "OneOfDecoder",
				Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
				ReturnType: checker.MakeResult(checker.Void, checker.MakeList(checker.DecodeErrorDef)),
			}
			
			return &object{
				raw: &Closure{
					vm:        vm,
					expr:      *errorDecoderType,
					builtinFn: func(data *object, resultType *checker.Result) *object {
						return makeErr(emptyListError, resultType)
					},
				},
				_type: errorDecoderType,
			}
		}

		// All decoders should have the same return type - use the first one
		firstDecoderType := decoderListObj[0]._type.(*checker.FunctionDef)
		firstReturnType := firstDecoderType.ReturnType.(*checker.Result)
		commonValueType := firstReturnType.Val()

		// Create one_of decoder type
		oneOfDecoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(commonValueType, checker.MakeList(checker.DecodeErrorDef)),
		}

		// Create closure that captures the decoder list
		oneOfDecoderFn := func(data *object, resultType *checker.Result) *object {
			return decodeAsOneOf(decoderList, data, resultType)
		}

		return &object{
			raw: &Closure{
				vm:        vm,
				expr:      *oneOfDecoderType,
				builtinFn: oneOfDecoderFn,
			},
			_type: oneOfDecoderType,
		}
	default:
		panic(fmt.Errorf("Unimplemented: decode::%s()", call.Name))
	}
}

func (m *DecodeModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: decode::%s::%s()", structName, call.Name))
}

// Helper function to format a value for error messages using premarshal
func formatValueForError(data *object) string {
	// Use premarshal to get the raw representation consistently
	rawValue := data.premarshal()
	return formatRawValueForError(rawValue)
}

// Helper function to format raw values with smart truncation and previews
func formatRawValueForError(v interface{}) string {
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
	case []interface{}:
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
	case map[string]interface{}:
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

// Helper function to create DecodeError
func makeDecodeError(expected, found string) *object {
	errorStruct := &object{
		raw: map[string]*object{
			"expected": {raw: expected, _type: checker.Str},
			"found":    {raw: found, _type: checker.Str},
			"path":     {raw: []*object{}, _type: checker.MakeList(checker.Str)},
		},
		_type: checker.DecodeErrorDef,
	}
	return errorStruct
}

// Helper function to create a list with single DecodeError
func makeDecodeErrorList(expected, found string) *object {
	decodeErr := makeDecodeError(expected, found)
	return &object{
		raw:   []*object{decodeErr},
		_type: checker.MakeList(checker.DecodeErrorDef),
	}
}

// Helper function to create a decode error list from data object (shows actual value)
func makeDecodeErrorListFromData(expected string, data *object) *object {
	found := formatValueForError(data)
	return makeDecodeErrorList(expected, found)
}

// as_string decoder implementation
func decodeAsString(data *object, resultType *checker.Result) *object {
	// For Dynamic objects, check the raw value type
	if data._type == checker.Dynamic {
		if data.raw == nil {
			decodeErrList := makeDecodeErrorListFromData("Str", data)
			return makeErr(decodeErrList, resultType)
		}
		if str, ok := data.raw.(string); ok {
			return makeOk(&object{raw: str, _type: checker.Str}, resultType)
		}
	} else if data._type == checker.Str {
		return makeOk(data, resultType)
	}

	decodeErrList := makeDecodeErrorListFromData("Str", data)
	return makeErr(decodeErrList, resultType)
}

// as_int decoder implementation
func decodeAsInt(data *object, resultType *checker.Result) *object {
	// For Dynamic objects, check the raw value type
	if data._type == checker.Dynamic {
		if data.raw == nil {
			decodeErrList := makeDecodeErrorListFromData("Int", data)
			return makeErr(decodeErrList, resultType)
		}
		if intVal, ok := data.raw.(int); ok {
			return makeOk(&object{raw: intVal, _type: checker.Int}, resultType)
		}
		// SQLite integers come as int64
		if int64Val, ok := data.raw.(int64); ok {
			return makeOk(&object{raw: int(int64Val), _type: checker.Int}, resultType)
		}
		// JSON numbers might come as float64
		if floatVal, ok := data.raw.(float64); ok {
			if floatVal == float64(int(floatVal)) { // Check if it's actually an integer
				return makeOk(&object{raw: int(floatVal), _type: checker.Int}, resultType)
			}
		}
	} else if data._type == checker.Int {
		return makeOk(data, resultType)
	}

	decodeErrList := makeDecodeErrorListFromData("Int", data)
	return makeErr(decodeErrList, resultType)
}

// as_float decoder implementation
func decodeAsFloat(data *object, resultType *checker.Result) *object {
	// For Dynamic objects, check the raw value type
	if data._type == checker.Dynamic {
		if data.raw == nil {
			decodeErrList := makeDecodeErrorListFromData("Float", data)
			return makeErr(decodeErrList, resultType)
		}
		if floatVal, ok := data.raw.(float64); ok {
			return makeOk(&object{raw: floatVal, _type: checker.Float}, resultType)
		}
		// Allow int to float conversion
		if intVal, ok := data.raw.(int); ok {
			return makeOk(&object{raw: float64(intVal), _type: checker.Float}, resultType)
		}
	} else if data._type == checker.Float {
		return makeOk(data, resultType)
	}

	decodeErrList := makeDecodeErrorListFromData("Float", data)
	return makeErr(decodeErrList, resultType)
}

// as_bool decoder implementation
func decodeAsBool(data *object, resultType *checker.Result) *object {
	// For Dynamic objects, check the raw value type
	if data._type == checker.Dynamic {
		if data.raw == nil {
			decodeErrList := makeDecodeErrorListFromData("Bool", data)
			return makeErr(decodeErrList, resultType)
		}
		if boolVal, ok := data.raw.(bool); ok {
			return makeOk(&object{raw: boolVal, _type: checker.Bool}, resultType)
		}
	} else if data._type == checker.Bool {
		return makeOk(data, resultType)
	}

	decodeErrList := makeDecodeErrorListFromData("Bool", data)
	return makeErr(decodeErrList, resultType)
}

// decodeAsNullable handles nullable decoding by checking for null and delegating to inner decoder
func decodeAsNullable(innerDecoder *object, data *object, resultType *checker.Result) *object {
	// If data is null (nil raw value in Dynamic), return maybe::none()
	if data._type == checker.Dynamic && data.raw == nil {
		maybeType := resultType.Val().(*checker.Maybe)
		noneValue := &object{raw: nil, _type: maybeType}
		return makeOk(noneValue, resultType)
	}

	// Otherwise, call the inner decoder
	closure := innerDecoder.raw.(*Closure)
	innerResult := closure.eval(data)
	innerResultValue := innerResult.raw.(_result)

	if innerResultValue.ok {
		// Success - wrap the decoded value in maybe::some()
		maybeType := resultType.Val().(*checker.Maybe)
		decodedValue := innerResultValue.raw
		someValue := &object{raw: decodedValue.raw, _type: maybeType}
		return makeOk(someValue, resultType)
	} else {
		// Error - propagate the error list as-is
		errorList := innerResultValue.raw
		return makeErr(errorList, resultType)
	}
}

// decodeAsList handles list decoding by checking for array data and delegating to element decoder
func decodeAsList(elementDecoder *object, data *object, resultType *checker.Result) *object {
	// Check if data is Dynamic and contains array-like structure
	if data._type == checker.Dynamic {
		if data.raw == nil {
			// Null data - return error (use nullable(list(...)) for nullable lists)
			decodeErrList := makeDecodeErrorListFromData("Array", data)
			return makeErr(decodeErrList, resultType)
		}

		// Check if raw data is a slice (JSON array becomes []interface{})
		if rawSlice, ok := data.raw.([]interface{}); ok {
			return decodeArrayElements(elementDecoder, rawSlice, resultType)
		}
	}

	// Not array-like data
	decodeErrList := makeDecodeErrorListFromData("Array", data)
	return makeErr(decodeErrList, resultType)
}

// decodeArrayElements decodes each element in the array using the element decoder
func decodeArrayElements(elementDecoder *object, rawSlice []interface{}, resultType *checker.Result) *object {
	// Get element decoder closure
	closure := elementDecoder.raw.(*Closure)

	var decodedElements []*object
	var errors []*object

	// Decode each element
	for i, rawElement := range rawSlice {
		elementData := &object{raw: rawElement, _type: checker.Dynamic}
		elementResult := closure.eval(elementData)
		elementResultValue := elementResult.raw.(_result)

		if elementResultValue.ok {
			decodedElements = append(decodedElements, elementResultValue.raw)
		} else {
			// Add element errors with path information
			elementErrors := elementResultValue.raw.raw.([]*object)
			for _, err := range elementErrors {
				// Add index to error path
				errStruct := err.raw.(map[string]*object)
				path := errStruct["path"].raw.([]*object)
				indexStr := &object{raw: fmt.Sprintf("[%d]", i), _type: checker.Str}
				newPath := append([]*object{indexStr}, path...)
				errStruct["path"] = &object{raw: newPath, _type: checker.MakeList(checker.Str)}
				errors = append(errors, err)
			}
		}
	}

	if len(errors) > 0 {
		// Return accumulated errors
		errorList := &object{raw: errors, _type: checker.MakeList(checker.DecodeErrorDef)}
		return makeErr(errorList, resultType)
	}

	// Success - create list object
	listType := resultType.Val().(*checker.List)
	listObject := &object{raw: decodedElements, _type: listType}
	return makeOk(listObject, resultType)
}

// decodeAsMap handles map decoding by checking for object data and delegating to key/value decoders
func decodeAsMap(keyDecoder *object, valueDecoder *object, data *object, resultType *checker.Result) *object {
	// Check if data is Dynamic and contains object-like structure
	if data._type == checker.Dynamic {
		if data.raw == nil {
			// Null data - return error (use nullable(map(...)) for nullable maps)
			decodeErrList := makeDecodeErrorListFromData("Object", data)
			return makeErr(decodeErrList, resultType)
		}

		// Check if raw data is a map (JSON object becomes map[string]interface{})
		if rawMap, ok := data.raw.(map[string]interface{}); ok {
			return decodeMapValues(keyDecoder, valueDecoder, rawMap, resultType)
		}
	}

	// Not object-like data
	decodeErrList := makeDecodeErrorListFromData("Object", data)
	return makeErr(decodeErrList, resultType)
}

// decodeMapValues decodes each key and value in the object using their respective decoders
func decodeMapValues(keyDecoder *object, valueDecoder *object, rawMap map[string]interface{}, resultType *checker.Result) *object {
	// Get decoder closures
	keyClosure := keyDecoder.raw.(*Closure)
	valueClosure := valueDecoder.raw.(*Closure)

	// Create a new map to store decoded keys and values
	decodedMap := make(map[string]*object)
	var errors []*object

	// Decode each key-value pair
	for rawKey, rawValue := range rawMap {
		// Decode key
		keyData := &object{raw: rawKey, _type: checker.Dynamic}
		keyResult := keyClosure.eval(keyData)
		keyResultValue := keyResult.raw.(_result)

		// Decode value
		valueData := &object{raw: rawValue, _type: checker.Dynamic}
		valueResult := valueClosure.eval(valueData)
		valueResultValue := valueResult.raw.(_result)

		if keyResultValue.ok && valueResultValue.ok {
			// Both key and value decoded successfully
			// Convert decoded key to string for map storage
			decodedKey := convertToMapKey(keyResultValue.raw)
			decodedMap[decodedKey] = valueResultValue.raw
		} else {
			// Add errors with path information
			if !keyResultValue.ok {
				keyErrors := keyResultValue.raw.raw.([]*object)
				for _, err := range keyErrors {
					// Add key path information
					errStruct := err.raw.(map[string]*object)
					path := errStruct["path"].raw.([]*object)
					keyStr := &object{raw: fmt.Sprintf("key(%s)", rawKey), _type: checker.Str}
					newPath := append([]*object{keyStr}, path...)
					errStruct["path"] = &object{raw: newPath, _type: checker.MakeList(checker.Str)}
					errors = append(errors, err)
				}
			}
			if !valueResultValue.ok {
				valueErrors := valueResultValue.raw.raw.([]*object)
				for _, err := range valueErrors {
					// Add value path information
					errStruct := err.raw.(map[string]*object)
					path := errStruct["path"].raw.([]*object)
					keyStr := &object{raw: fmt.Sprintf("value(%s)", rawKey), _type: checker.Str}
					newPath := append([]*object{keyStr}, path...)
					errStruct["path"] = &object{raw: newPath, _type: checker.MakeList(checker.Str)}
					errors = append(errors, err)
				}
			}
		}
	}

	if len(errors) > 0 {
		// Return accumulated errors
		errorList := &object{raw: errors, _type: checker.MakeList(checker.DecodeErrorDef)}
		return makeErr(errorList, resultType)
	}

	// Success - create map object
	mapType := resultType.Val().(*checker.Map)
	mapObject := &object{raw: decodedMap, _type: mapType}
	return makeOk(mapObject, resultType)
}

// convertToMapKey converts a decoded key object to a string for map storage
func convertToMapKey(keyObj *object) string {
	switch keyObj._type {
	case checker.Str:
		return keyObj.raw.(string)
	case checker.Int:
		return fmt.Sprintf("%d", keyObj.raw.(int))
	case checker.Float:
		return fmt.Sprintf("%g", keyObj.raw.(float64))
	case checker.Bool:
		if keyObj.raw.(bool) {
			return "true"
		}
		return "false"
	default:
		// For other types, use a string representation
		return fmt.Sprintf("%v", keyObj.raw)
	}
}

// decodeAsField extracts a specific field from an object and decodes it
func decodeAsField(fieldKey string, valueDecoder *object, data *object, resultType *checker.Result) *object {
	// Check if data is Dynamic and contains object-like structure
	if data._type == checker.Dynamic {
		if data.raw == nil {
			// Null data - return error
			decodeErrList := makeDecodeErrorListFromData("Object", data)
			return makeErr(decodeErrList, resultType)
		}

		// Check if raw data is a map (JSON object becomes map[string]interface{})
		if rawMap, ok := data.raw.(map[string]interface{}); ok {
			return extractField(fieldKey, valueDecoder, rawMap, resultType)
		}
	}

	// Not object-like data
	decodeErrList := makeDecodeErrorListFromData("Object", data)
	return makeErr(decodeErrList, resultType)
}

// extractField handles the actual field extraction and value decoding
func extractField(fieldKey string, valueDecoder *object, rawMap map[string]interface{}, resultType *checker.Result) *object {
	// Get value decoder closure
	valueClosure := valueDecoder.raw.(*Closure)

	// Check if field exists
	rawValue, exists := rawMap[fieldKey]
	if !exists {
		// Missing field error with path
		decodeErr := &object{
			raw: map[string]*object{
				"expected": {raw: "field '" + fieldKey + "'", _type: checker.Str},
				"found":    {raw: "missing", _type: checker.Str},
				"path":     {raw: []*object{{raw: fieldKey, _type: checker.Str}}, _type: checker.MakeList(checker.Str)},
			},
			_type: checker.DecodeErrorDef,
		}
		errorList := &object{raw: []*object{decodeErr}, _type: checker.MakeList(checker.DecodeErrorDef)}
		return makeErr(errorList, resultType)
	}

	// Field exists, decode its value
	valueData := &object{raw: rawValue, _type: checker.Dynamic}
	valueResult := valueClosure.eval(valueData)
	valueResultValue := valueResult.raw.(_result)

	if valueResultValue.ok {
		return makeOk(valueResultValue.raw, resultType)
	} else {
		// Propagate errors with field name in path
		valueErrors := valueResultValue.raw.raw.([]*object)
		for _, err := range valueErrors {
			errStruct := err.raw.(map[string]*object)
			path := errStruct["path"].raw.([]*object)
			fieldStr := &object{raw: fieldKey, _type: checker.Str}
			newPath := append([]*object{fieldStr}, path...)
			errStruct["path"] = &object{raw: newPath, _type: checker.MakeList(checker.Str)}
		}

		errorList := &object{raw: valueErrors, _type: checker.MakeList(checker.DecodeErrorDef)}
		return makeErr(errorList, resultType)
	}
}

// decodeAsOneOf tries multiple decoders in sequence until one succeeds
func decodeAsOneOf(decoderList *object, data *object, resultType *checker.Result) *object {
	decoders := decoderList.raw.([]*object)
	
	if len(decoders) == 0 {
		// Return error instead of panicking
		emptyListError := makeDecodeErrorList("At least one decoder", "Empty decoder list")
		return makeErr(emptyListError, resultType)
	}

	var firstError *object

	// Try each decoder in sequence
	for i, decoder := range decoders {
		closure := decoder.raw.(*Closure)
		result := closure.eval(data)
		resultValue := result.raw.(_result)

		if resultValue.ok {
			// Success! Return the result
			return makeOk(resultValue.raw, resultType)
		}

		// Store the first error (following Gleam's pattern)
		if i == 0 {
			firstError = result
		}
	}

	// All decoders failed - return the first error
	firstErrorValue := firstError.raw.(_result)
	return makeErr(firstErrorValue.raw, resultType)
}

// parseJsonToDynamic parses JSON into a Dynamic object
func parseJsonToDynamic(jsonBytes []byte) (*object, error) {
	var rawValue interface{}

	// Parse JSON into Go interface{}
	err := json.Unmarshal(jsonBytes, &rawValue)
	if err != nil {
		return nil, err
	}

	// Wrap the raw value as a Dynamic object
	return &object{
		raw:   rawValue,
		_type: checker.Dynamic,
	}, nil
}
