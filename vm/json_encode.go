//go:build goexperiment.jsonv2

package vm

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"

	"github.com/akonwi/ard/checker"
)

func json_encode(data any, t checker.Type) ([]byte, error) {
	if t == checker.Str || t == checker.Int || t == checker.Float || t == checker.Bool {
		str, err := json.Marshal(data)
		return str, err
	}

	// Handle Maybe types specially
	if checker.IsMaybe(t) {
		maybeType := t.(*checker.Maybe)
		innerType := maybeType.Of()

		// If data is nil, it's a None value
		if data == nil {
			return json.Marshal(nil)
		}

		// If it's a Maybe of a primitive type, marshal directly
		if innerType == checker.Str || innerType == checker.Int || innerType == checker.Float || innerType == checker.Bool {
			return json.Marshal(data)
		}

		// For complex types wrapped in Maybe, recursively encode with the inner type
		return json_encode(data, innerType)
	}

	switch t.(type) {
	case *checker.Any:
		if o, ok := data.(*object); ok {
			return json_encode(o, o._type)
		}
		if o, ok := data.([]*object); ok {
			if len(o) > 0 {
				return json_encode(o, checker.MakeList(o[0]._type))
			}
		}
		return json.Marshal(data)
	case *checker.Enum:
		return json.Marshal(data)
	case *checker.List:
		raw := data.([]*object)
		_array := make([]any, len(raw))
		for i, item := range raw {
			if item._type == checker.Str || item._type == checker.Int || item._type == checker.Float || item._type == checker.Bool {
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
			if val._type == checker.Str || val._type == checker.Int || val._type == checker.Float || val._type == checker.Bool {
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
			if val._type == checker.Str || val._type == checker.Int || val._type == checker.Float || val._type == checker.Bool {
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
