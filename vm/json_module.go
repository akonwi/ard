package vm

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/akonwi/ard/checker"
)

// JSONModule handles ard/json module functions
type JSONModule struct{}

func (m *JSONModule) Path() string {
	return "ard/json"
}

func (m *JSONModule) Program() *checker.Program {
	return nil
}

func (m *JSONModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "encode":
		{
			resultType := call.Type().(*checker.Result)
			bytes, err := json.Marshal(args[0].premarshal())
			if err != nil {
				return makeErr(&object{err.Error(), checker.Str}, resultType)
			}
			return makeOk(&object{string(bytes), checker.Str}, resultType)
		}
	case "decode":
		{
			resultType := call.Type().(*checker.Result)
			toErr := func(msg error) *object {
				return makeErr(&object{msg.Error(), checker.Str}, resultType)
			}
			jsonString := vm.eval(call.Args[0]).raw.(string)
			jsonBytes := []byte(jsonString)

			inner := checker.UnwrapType(resultType.Val())
			val, err := decode(inner, jsonBytes)
			if err != nil {
				return toErr(err)
			}

			return makeOk(&val, resultType)

			// switch subj := inner.(type) {
			// case *checker.StructDef:
			// 	{
			// 		decoder := json.NewDecoder(strings.NewReader(jsonString))
			// 		if t, err := decoder.Token(); err != nil {
			// 			result.raw = _result{
			// 				raw: &object{
			// 					fmt.Errorf("Expected opening brace at %v: [%w]", t, err),
			// 					checker.Str,
			// 				},
			// 			}
			// 			return result
			// 		} else if delim, _ := t.(json.Delim); delim.String() != "{" {
			// 			result.raw = _result{
			// 				raw: &object{
			// 					fmt.Errorf("Expected opening brace at %v: [%w]", t, err),
			// 					checker.Str,
			// 				},
			// 			}
			// 			return result
			// 		}

			// 		return m.decodeAsStruct(result, decoder, subj, vm, toErr, resultType)
			// 	}
			// default:
			// 	panic(fmt.Errorf("unable to decode into %s", subj))
			// }
		}
	default:
		panic(fmt.Errorf("Unimplemented: json::%s()", call.Name))
	}
}

func decode(as checker.Type, data []byte) (object, error) {
	maybeType, isMaybe := as.(*checker.Maybe)
	if isMaybe {
		return json_decodeMaybe(maybeType.Of(), data)
	}

	if as == checker.Str {
		return json_decodeStr(data)
	}

	if as == checker.Int {
		return json_decodeInt(data)
	}

	if as == checker.Bool {
		return json_decodeBool(data)
	}

	switch as := as.(type) {
	case *checker.List:
		return json_decodeList(checker.UnwrapType(as.Of()), data)
	}

	return object{}, nil
}

// this function needs to call decoder.Token() until it passes over the closing delimeter for the provided delim
func skipOver(decoder *json.Decoder, delim string) {
	// Determine the closing delimiter
	var closingDelim string
	switch delim {
	case "{":
		closingDelim = "}"
	case "[":
		closingDelim = "]"
	default:
		// If delim is not a recognized opening delimiter, just return
		return
	}

	// Keep track of nesting level
	nestLevel := 1

	// Read tokens until we've matched all opening delimiters with closing ones
	for nestLevel > 0 {
		token, err := decoder.Token()
		if err != nil {
			// Handle error - in this case we'll just return
			return
		}

		// Check if the token is a delimiter
		if d, ok := token.(json.Delim); ok {
			delimStr := d.String()

			if delimStr == delim {
				nestLevel++
			}

			if delimStr == closingDelim {
				nestLevel--
			}
		}
	}
}

func (m *JSONModule) decodeAsStruct(result *object, decoder *json.Decoder, subj *checker.StructDef, vm *VM, toErr func(msg error) *object, resultType *checker.Result) *object {
	fields := make(map[string]*object)

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			result.raw = _result{
				raw: &object{
					fmt.Errorf("Error decoding key: [%w] %T - %v\n", err, keyToken, keyToken),
					checker.Str,
				},
			}
			return result
		}
		valToken, err := decoder.Token()
		if err != nil {
			result.raw = _result{
				raw: &object{
					fmt.Errorf("Error decoding value: [%w] %T - %v\n", err, valToken, valToken),
					checker.Str,
				},
			}
			return result
		}

		key := keyToken.(string)

		if _, ok := subj.Fields[key]; !ok {
			delim, isDelim := valToken.(json.Delim)
			if isDelim {
				skipOver(decoder, delim.String())
			}
			continue
		}

		switch val := valToken.(type) {
		case string:
			valType := subj.Fields[key]
			var decodeAs checker.Type = valType
			// maybe, isMaybe := valType.(*checker.Maybe)
			// if isMaybe {
			// 	decodeAs = maybe
			// } else {
			// 	decodeAs = valType
			// }

			decoded := m.Handle(vm, checker.CreateCall("decode",
				[]checker.Expression{&checker.StrLiteral{Value: val}},
				checker.FunctionDef{
					ReturnType: checker.MakeResult(decodeAs, checker.Str),
				},
			), []*object{{val, checker.Str}})

			// if err
			if !decoded.raw.(_result).ok {
				return decoded
			}
			raw := decoded.raw.(_result).raw
			if maybe, isMaybe := valType.(*checker.Maybe); isMaybe {
				raw._type = maybe
			}
			fields[key] = raw
		case float64:
			if subj.Fields[key] == checker.Float {
				fields[key] = &object{val, checker.Float}
			} else if subj.Fields[key] == checker.Int {
				fields[key] = &object{int(val), checker.Int}
			} else {
				return toErr(fmt.Errorf("Parsing error: Invalid type - Encountered a number instead of %s", subj.Fields[key]))
			}
		case bool:
			if subj.Fields[key] != checker.Bool {
				return toErr(fmt.Errorf("Parsing error: Invalid type - Encountered a boolean instead of %s", subj.Fields[key]))
			}
			fields[key] = &object{val, checker.Bool}
		case nil:
			if maybe, isMaybe := subj.Fields[key].(*checker.Maybe); !isMaybe {
				return toErr(fmt.Errorf("Parsing error: Invalid type - Encountered a nil instead of %s", subj.Fields[key]))
			} else {
				fields[key] = &object{val, maybe}
			}
		case json.Delim:
			if val.String() == "[" {
				listType, ok := subj.Fields[key].(*checker.List)
				if !ok {
					return toErr(fmt.Errorf("Parsing error: Invalid type - Encountered a list instead of %s", subj.Fields[key]))
				}
				list := []*object{}
				for decoder.More() {
					var v any
					if err := decoder.Decode(&v); err != nil {
						return toErr(fmt.Errorf("Parsing error: Failed to decode list element: %v", err))
					}
					obj, err := enforceSchema(vm, v, listType.Of())
					if err != nil {
						return toErr(fmt.Errorf("Parsing error: Failed to decode schema list element: %w", err))
					}
					list = append(list, obj)
				}
				if t, err := decoder.Token(); err != nil {
					log.Fatal(fmt.Errorf("Error taking closing ]: [%w] %T - %v\n", err, t, t))
					return toErr(errors.New("Parsing error: Failed to take closing bracket"))
				}

				fields[key] = &object{list, listType}
			} else {
				// otherwise it's an object
				nestedTarget, ok := subj.Fields[key].(*checker.StructDef)
				if !ok {
					return toErr(fmt.Errorf("%s cannot be decoded into %s", key, subj.Fields[key]))
				}
				decoded := m.decodeAsStruct(result, decoder, nestedTarget, vm, toErr, resultType)

				value, ok := decoded.raw.(_result)
				if !ok {
					panic(fmt.Errorf("should have gotten a result: %v", decoded))
				}
				if !value.ok {
					return decoded
				}
				fields[key] = value.raw
			}
		default:
			panic(fmt.Errorf("unexpected: %v", val))
		}
	}

	for name, fType := range subj.Fields {
		if _, ok := fields[name]; !ok {
			maybe, isMaybe := fType.(*checker.Maybe)
			if !isMaybe {
				return makeErr(&object{"Missing required property: " + name, checker.Str}, resultType)
			}
			fields[name] = &object{nil, maybe}
		}
	}

	result.raw = _result{ok: true, raw: &object{fields, subj}}
	return result
}

func (m *JSONModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: json::%s::%s()", structName, call.Name))
}
