package vm

import (
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
