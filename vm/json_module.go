package vm

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/akonwi/ard/checker"
)

// JSONModule handles ard/json module functions
type JSONModule struct{}

func (m *JSONModule) Path() string {
	return "ard/json"
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
			errorResult := makeErr(&object{"Parsing Error", checker.Str}, resultType)
			result := makeOk(nil, resultType)
			jsonString := vm.eval(call.Args[0]).raw.(string)
			jsonBytes := []byte(jsonString)

			inner := resultType.Val()
			anyType, isAny := inner.(*checker.Any)
			maybeType, isMaybe := inner.(*checker.Maybe)
			// if inner is a generic, reach all the way to the core
			for (isAny && anyType.Actual() != nil) || (isMaybe) {
				if isAny && anyType.Actual() != nil {
					inner = anyType.Actual()
				} else {
					inner = maybeType.Of()
				}
				anyType, isAny = inner.(*checker.Any)
				maybeType, isMaybe = inner.(*checker.Maybe)
			}

			if inner == checker.Str {
				res := &_result{ok: true, raw: &object{jsonString, checker.Str}}
				if isMaybe {
					res.raw._type = maybeType
				}
				result.raw = *res
				return result
			}

			switch subj := inner.(type) {
			case *checker.StructDef:
				{
					decoder := json.NewDecoder(strings.NewReader(jsonString))
					if t, err := decoder.Token(); err != nil {
						result.raw = _result{
							raw: &object{
								fmt.Errorf("Expected opening brace at %v: [%w]", t, err),
								checker.Str,
							},
						}
						return result
					} else if delim, _ := t.(json.Delim); delim.String() != "{" {
						result.raw = _result{
							raw: &object{
								fmt.Errorf("Expected opening brace at %v: [%w]", t, err),
								checker.Str,
							},
						}
						return result
					}

					return m.decodeAsStruct(result, decoder, subj, vm, errorResult, resultType)
				}
			case *checker.List:
				{
					array := []any{}
					err := json.Unmarshal([]byte(jsonBytes), &array)
					if err != nil {
						result.raw = &object{err.Error(), checker.Str}
						return result
					}

					raw := make([]*object, len(array))
					for i := range array {
						raw[i] = &object{array[i], subj.Of()}
					}

					rawObj := &object{raw, subj}
					result.raw = _result{ok: true, raw: rawObj}
					return result
				}
			default:
				panic(fmt.Errorf("unable to decode into %s", subj))
			}
		}
	default:
		panic(fmt.Errorf("Unimplemented: json::%s()", call.Name))
	}
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

func (m *JSONModule) decodeAsStruct(result *object, decoder *json.Decoder, subj *checker.StructDef, vm *VM, errorResult *object, resultType *checker.Result) *object {
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

			// For recursive decode calls, we need to handle the module name properly
			// Since we're inside the JSON module, we need to reference ourselves
			moduleName := "ard/json"
			if vm.imports != nil {
				// Find the module name that resolves to ard/json
				for importName, module := range vm.imports {
					if module.Path() == "ard/json" {
						moduleName = importName
						break
					}
				}
			}

			decoded := vm.eval(&checker.ModuleFunctionCall{
				Module: moduleName,
				Call: checker.CreateCall("decode",
					[]checker.Expression{&checker.StrLiteral{Value: val}},
					checker.FunctionDef{
						ReturnType: checker.MakeResult(decodeAs, checker.Str),
					},
				),
			})
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
				return errorResult
			}
		case bool:
			if subj.Fields[key] != checker.Bool {
				return errorResult
			}
			fields[key] = &object{val, checker.Bool}
		case nil:
			if maybe, isMaybe := subj.Fields[key].(*checker.Maybe); !isMaybe {
				return errorResult
			} else {
				fields[key] = &object{val, maybe}
			}
		case json.Delim:
			if val.String() == "[" {
				listType, ok := subj.Fields[key].(*checker.List)
				if !ok {
					return errorResult
				}
				list := []*object{}
				for decoder.More() {
					var v any
					if err := decoder.Decode(&v); err != nil {
						return errorResult
					}
					obj := enforceSchema(vm, v, listType.Of())
					if obj == nil {
						return errorResult
					}
					list = append(list, obj)
				}
				if t, err := decoder.Token(); err != nil {
					log.Fatal(fmt.Errorf("Error taking closing ]: [%w] %T - %v\n", err, t, t))
					return errorResult
				}

				fields[key] = &object{list, listType}
			} else {
				// otherwise it's an object
				nestedTarget, ok := subj.Fields[key].(*checker.StructDef)
				if !ok {
					errorResult.raw = fmt.Sprintf("%s cannot be decoded into %s", key, subj.Fields[key])
					return errorResult
				}
				decoded := m.decodeAsStruct(result, decoder, nestedTarget, vm, errorResult, resultType)

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
