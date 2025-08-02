package vm

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"

	"github.com/akonwi/ard/checker"
)

// go:build goexperiment.jsonv2

func json_encode(data any, t checker.Type) ([]byte, error) {
	if t == checker.Str || t == checker.Int || t == checker.Float || t == checker.Bool || checker.IsMaybe(t) {
		str, err := json.Marshal(data)
		return str, err
	}

	switch t.(type) {
	case *checker.Enum:
		return json.Marshal(data)
	case *checker.List:
		raw := data.([]*object)
		_array := make([]any, len(raw))
		for i, item := range raw {
			if item._type == checker.Str || item._type == checker.Int || item._type == checker.Float || item._type == checker.Bool || checker.IsMaybe(item._type) {
				_array[i] = item.raw
			} else {
				e, err := json_encode(item.raw, item._type)
				if err != nil {
					return nil, err
				}
				_array[i] = jsontext.Value(e)
			}
		}
		return json.Marshal(_array)
	case *checker.StructDef:
		raw := data.(map[string]*object)
		_struct := make(map[string]any)
		for field, val := range raw {
			if val._type == checker.Str || val._type == checker.Int || val._type == checker.Float || val._type == checker.Bool || checker.IsMaybe(val._type) {
				_struct[field] = val.raw
			} else {
				e, err := json_encode(val.raw, val._type)
				if err != nil {
					return nil, err
				}
				_struct[field] = jsontext.Value(e)
			}
		}
		return json.Marshal(_struct)
	case *checker.Map:
		raw := data.(map[string]*object)
		_map := make(map[string]any)
		for key, val := range raw {
			if val._type == checker.Str || val._type == checker.Int || val._type == checker.Float || val._type == checker.Bool || checker.IsMaybe(val._type) {
				_map[key] = val.raw
			} else {
				e, err := json_encode(val.raw, val._type)
				if err != nil {
					return nil, err
				}
				_map[key] = jsontext.Value(e)
			}
		}
		return json.Marshal(_map)
	case *checker.Result:
		inner := data.(_result).raw
		return json_encode(inner.raw, inner._type)
	default:
		panic(fmt.Sprintf("Encoding error: Unhandled type %s", t))
	}
}
