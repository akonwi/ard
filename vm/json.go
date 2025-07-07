package vm

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strconv"

	"github.com/akonwi/ard/checker"
)

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

	if as == checker.Float {
		return json_decodeFloat(data)
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

func json_decodeFloat(bytes []byte) (object, error) {
	float := object{nil, checker.Float}
	err := json.Unmarshal(bytes, &float.raw, json.WithUnmarshalers(json.UnmarshalFunc(func(data []byte, val *float64) error {
		num, err := strconv.ParseFloat(string(data), 64)
		if err != nil {
			return fmt.Errorf("Unable to decode \"%s\" as Float: %w", data, err)
		}
		*val = num
		return nil
	})))
	return float, err
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
	for key, t := range as.Fields {
		if _, ok := fields[key]; !ok {
			if checker.IsMaybe(t) {
				// missing Maybe fields default to maybe::none()
				fields[key] = &object{nil, t}
			} else {
				return object{}, fmt.Errorf("Missing field in input JSON: %s", key)
			}
		}
	}

	return object{fields, as}, err
}
