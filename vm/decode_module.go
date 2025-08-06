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
			// For primitive decoders, use the call's result type
			resultType := call.Type().(*checker.Result)
			return fn(data, resultType)
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
			expected := "String"
			found := "null"
			decodeErrList := makeDecodeErrorList(expected, found)
			return makeErr(decodeErrList, resultType)
		}
		if str, ok := data.raw.(string); ok {
			return makeOk(&object{raw: str, _type: checker.Str}, resultType)
		}
	} else if data._type == checker.Str {
		return makeOk(data, resultType)
	}
	
	expected := "String"
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
			found := "null"
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
			found := "null"
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
			found := "null"
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