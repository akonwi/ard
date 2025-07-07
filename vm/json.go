package vm

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strconv"

	"github.com/akonwi/ard/checker"
)

func enforceSchema(vm *VM, val any, as checker.Type) (*object, error) {
	if as == checker.Int {
		num, ok := val.(float64)
		if !ok {
			return nil, fmt.Errorf("expected Int, got %s in JSON", val)
		}
		return &object{int(num), as}, nil
	}
	if as == checker.Float {
		num, ok := val.(float64)
		if !ok {
			return nil, fmt.Errorf("expected Float, got %s in JSON", val)
		}
		return &object{num, as}, nil
	}
	if as == checker.Str {
		str, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("expected String, got %s in JSON", val)
		}
		return &object{str, as}, nil
	}
	if as == checker.Bool {
		is_ok, ok := val.(bool)
		if !ok {
			return nil, fmt.Errorf("expected Bool, got %s in JSON", val)
		}
		return &object{is_ok, as}, nil
	}

	switch as := as.(type) {
	case *checker.StructDef:
		{
			jMap, ok := val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("expected Struct, got %v in JSON", val)
			}

			fields := make(map[string]*object)
			for name, fType := range as.Fields {
				val, err := enforceSchema(vm, jMap[name], fType)
				if err != nil {
					return nil, err
				}
				fields[name] = val
			}

			return &object{fields, as}, nil
		}
	case *checker.Maybe:
		if val == nil {
			return &object{nil, as}, nil
		}
		return enforceSchema(vm, val, as.Of())
	default:
		return nil, fmt.Errorf("Unexpected ard Type for JSON decoding: %s", as)
	}
}

func json_decode(as checker.Type, data []byte) (object, error) {
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
	case *checker.StructDef:
		return json_decodeStruct(as, data)
	default:
		panic(fmt.Errorf("unable to decode into %s: \"%s\"", as, data))
	}
}

func json_decodeStr(bytes []byte) (object, error) {
	str := object{nil, checker.Str}
	if err := json.Unmarshal(bytes, &str.raw); err != nil {
		return str, err
	}
	return str, nil
}

var intUnmarshaler = json.UnmarshalFunc(
	func(data []byte, val *object) error {
		num, err := strconv.Atoi(string(data))
		if err != nil {
			return fmt.Errorf("Unable to decode \"%s\" as Int: %w", data, err)
		}
		*val = object{num, checker.Int}
		return nil
	},
)

func json_decodeInt(bytes []byte) (object, error) {
	int := object{nil, checker.Int}
	err := json.Unmarshal(bytes, &int, json.WithUnmarshalers(intUnmarshaler))
	return int, err
}

func json_decodeBool(bytes []byte) (object, error) {
	bool := object{nil, checker.Bool}
	if err := json.Unmarshal(bytes, &bool.raw); err != nil {
		return bool, fmt.Errorf("Unable to decode \"%s\" as Bool: %w", bytes, err)
	}

	return bool, nil
}

func json_decodeMaybe(of checker.Type, data []byte) (object, error) {
	if string(data) == "null" {
		return object{nil, checker.MakeMaybe(of)}, nil
	}

	val, err := json_decode(of, data)
	val._type = checker.MakeMaybe(val._type)
	return val, err
}

func json_decodeList(of checker.Type, data []byte) (object, error) {
	var items []*object
	err := json.Unmarshal(data, &items, json.WithUnmarshalers(
		json.UnmarshalFromFunc(func(decoder *jsontext.Decoder, out *object) error {
			// decode one value at a time
			v, err := decoder.ReadValue()
			if err != nil {
				return err
			}
			val, err := json_decode(of, v)
			if err != nil {
				return err
			}
			*out = val
			return nil
		})),
	)
	return object{items, checker.MakeList(of)}, err
}

func json_decodeStruct(as *checker.StructDef, data []byte) (object, error) {
	var fields map[string]*object

	var valType checker.Type
	err := json.Unmarshal(data, &fields, json.WithUnmarshalers(
		json.JoinUnmarshalers(
			json.UnmarshalFunc(func(data []byte, out *string) error {
				// decode keys
				str, err := json_decodeStr(data)
				if err != nil {
					return err
				}
				key := str.raw.(string)
				*out = key
				valType = as.Fields[key]
				return nil
			}),
			json.UnmarshalFromFunc(func(decoder *jsontext.Decoder, out *object) error {
				// decode value at a time
				v, err := decoder.ReadValue()
				if err != nil {
					return err
				}

				// skip unexpected values
				if valType == nil {
					return nil
				}

				val, err := json_decode(valType, v)
				if err != nil {
					return err
				}
				*out = val
				return nil
			}))),
	)

	// delete unexpected keys
	for key := range fields {
		if _, ok := as.Fields[key]; !ok {
			delete(fields, key)
		}
	}

	// check for required keys
	for key := range as.Fields {
		if _, ok := fields[key]; !ok {
			return object{}, fmt.Errorf("Missing field in input JSON: %s", key)
		}
	}

	return object{fields, as}, err
}
