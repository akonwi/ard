//go:build goexperiment.jsonv2

package vm

import (
	"encoding/json/v2"
	"fmt"

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
		// Return a decoder function (as_string)
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Str, checker.MakeList(checker.DecodeErrorDef)),
		}
		return &object{
			raw:   decodeAsString,
			_type: decoderType,
		}
	case "int":
		// Return a decoder function (as_int) 
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Int, checker.MakeList(checker.DecodeErrorDef)),
		}
		return &object{
			raw:   decodeAsInt,
			_type: decoderType,
		}
	case "float":
		// Return a decoder function (as_float)
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Float, checker.MakeList(checker.DecodeErrorDef)),
		}
		return &object{
			raw:   decodeAsFloat,
			_type: decoderType,
		}
	case "bool":
		// Return a decoder function (as_bool)
		decoderType := &checker.FunctionDef{
			Name:       "Decoder",
			Parameters: []checker.Parameter{{Name: "data", Type: checker.Dynamic}},
			ReturnType: checker.MakeResult(checker.Bool, checker.MakeList(checker.DecodeErrorDef)),
		}
		return &object{
			raw:   decodeAsBool,
			_type: decoderType,
		}
	case "decode":
		// Apply the decoder function to the data
		decoder := args[0] 
		data := args[1]
		
		// The decoder should be a function, call it with data
		if fn, ok := decoder.raw.(func(*object, *checker.Result) *object); ok {
			// Get the result type for the decoder call (list-based errors)
			decoderFnType := decoder._type.(*checker.FunctionDef)
			listErrorResultType := decoderFnType.ReturnType.(*checker.Result)
			
			// Call the decoder function
			decoderResult := fn(data, listErrorResultType)
			
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
		} else {
			// Handle Ard function calls
			panic(fmt.Errorf("Complex decoder functions not yet supported: got %T", decoder.raw))
		}
	case "any":
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
			raw:   nullableDecoderFn,
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
			raw:   listDecoderFn,
			_type: listDecoderType,
		}
	default:
		panic(fmt.Errorf("Unimplemented: decode::%s()", call.Name))
	}
}

func (m *DecodeModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: decode::%s::%s()", structName, call.Name))
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


// as_string decoder implementation
func decodeAsString(data *object, resultType *checker.Result) *object {
	// For Dynamic objects, check the raw value type
	if data._type == checker.Dynamic {
		if data.raw == nil {
			expected := "Str"
			found := "Void"
			decodeErrList := makeDecodeErrorList(expected, found)
			return makeErr(decodeErrList, resultType)
		}
		if str, ok := data.raw.(string); ok {
			return makeOk(&object{raw: str, _type: checker.Str}, resultType)
		}
	} else if data._type == checker.Str {
		return makeOk(data, resultType)
	}
	
	expected := "Str"
	found := data._type.String()
	decodeErrList := makeDecodeErrorList(expected, found)
	return makeErr(decodeErrList, resultType)
}

// as_int decoder implementation  
func decodeAsInt(data *object, resultType *checker.Result) *object {
	// For Dynamic objects, check the raw value type
	if data._type == checker.Dynamic {
		if data.raw == nil {
			expected := "Int"
			found := "Void"
			decodeErrList := makeDecodeErrorList(expected, found)
			return makeErr(decodeErrList, resultType)
		}
		if intVal, ok := data.raw.(int); ok {
			return makeOk(&object{raw: intVal, _type: checker.Int}, resultType)
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
	
	expected := "Int"
	found := data._type.String()
	decodeErrList := makeDecodeErrorList(expected, found)
	return makeErr(decodeErrList, resultType)
}

// as_float decoder implementation
func decodeAsFloat(data *object, resultType *checker.Result) *object {
	// For Dynamic objects, check the raw value type
	if data._type == checker.Dynamic {
		if data.raw == nil {
			expected := "Float"
			found := "Void"
			decodeErrList := makeDecodeErrorList(expected, found)
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
	
	expected := "Float"
	found := data._type.String()
	decodeErrList := makeDecodeErrorList(expected, found)
	return makeErr(decodeErrList, resultType)
}

// as_bool decoder implementation
func decodeAsBool(data *object, resultType *checker.Result) *object {
	// For Dynamic objects, check the raw value type
	if data._type == checker.Dynamic {
		if data.raw == nil {
			expected := "Bool"
			found := "Void"
			decodeErrList := makeDecodeErrorList(expected, found)
			return makeErr(decodeErrList, resultType)
		}
		if boolVal, ok := data.raw.(bool); ok {
			return makeOk(&object{raw: boolVal, _type: checker.Bool}, resultType)
		}
	} else if data._type == checker.Bool {
		return makeOk(data, resultType)
	}
	
	expected := "Bool"
	found := data._type.String()
	decodeErrList := makeDecodeErrorList(expected, found)
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
	if fn, ok := innerDecoder.raw.(func(*object, *checker.Result) *object); ok {
		// Get the inner decoder's result type
		innerDecoderType := innerDecoder._type.(*checker.FunctionDef)
		innerResultType := innerDecoderType.ReturnType.(*checker.Result)
		
		// Call the inner decoder
		innerResult := fn(data, innerResultType)
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
	} else {
		panic(fmt.Errorf("Inner decoder is not a function: got %T", innerDecoder.raw))
	}
}

// decodeAsList handles list decoding by checking for array data and delegating to element decoder
func decodeAsList(elementDecoder *object, data *object, resultType *checker.Result) *object {
	// Check if data is Dynamic and contains array-like structure
	if data._type == checker.Dynamic {
		if data.raw == nil {
			// Null data - return error (use nullable(list(...)) for nullable lists)
			expected := "Array"
			found := "Void"
			decodeErrList := makeDecodeErrorList(expected, found)
			return makeErr(decodeErrList, resultType)
		}
		
		// Check if raw data is a slice (JSON array becomes []interface{})
		if rawSlice, ok := data.raw.([]interface{}); ok {
			return decodeArrayElements(elementDecoder, rawSlice, resultType)
		}
	}
	
	// Not array-like data
	expected := "Array"
	found := data._type.String()
	decodeErrList := makeDecodeErrorList(expected, found)
	return makeErr(decodeErrList, resultType)
}

// decodeArrayElements decodes each element in the array using the element decoder
func decodeArrayElements(elementDecoder *object, rawSlice []interface{}, resultType *checker.Result) *object {
	// Get element decoder function
	elementDecoderFn := elementDecoder.raw.(func(*object, *checker.Result) *object)
	elementDecoderType := elementDecoder._type.(*checker.FunctionDef)
	elementResultType := elementDecoderType.ReturnType.(*checker.Result)
	
	var decodedElements []*object
	var errors []*object
	
	// Decode each element
	for i, rawElement := range rawSlice {
		elementData := &object{raw: rawElement, _type: checker.Dynamic}
		elementResult := elementDecoderFn(elementData, elementResultType)
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