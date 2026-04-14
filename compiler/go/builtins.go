package ardgo

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/akonwi/ard/ffi"
)

const osArgsEnvVar = "ARDGO_OS_ARGS_JSON"

var registerBuiltinExternsOnce sync.Once

func builtinOSArgs() []string {
	raw, ok := os.LookupEnv(osArgsEnvVar)
	if ok && raw != "" {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err == nil {
			return args
		}
	}
	return ffi.OsArgs()
}

func maybeBoolPointer(value Maybe[bool]) *bool {
	if value.IsNone() {
		return nil
	}
	boolValue := value.Or(false)
	return &boolValue
}

func builtinDynamicValue(value any) any {
	if encodable, ok := value.(Encodable); ok {
		return encodable.ToDyn()
	}
	return value
}

func RegisterBuiltinExterns() {
	registerBuiltinExternsOnce.Do(func() {
		RegisterExtern("Print", func(args ...any) (any, error) {
			ffi.Print(args[0].(string))
			return nil, nil
		})
		RegisterExtern("FloatFromInt", func(args ...any) (any, error) {
			return ffi.FloatFromInt(args[0].(int)), nil
		})
		RegisterExtern("IntFromStr", func(args ...any) (any, error) {
			value := ffi.IntFromStr(args[0].(string))
			if value == nil {
				return None[int](), nil
			}
			return Some(*value), nil
		})
		RegisterExtern("FloatFromStr", func(args ...any) (any, error) {
			value := ffi.FloatFromStr(args[0].(string))
			if value == nil {
				return None[float64](), nil
			}
			return Some(*value), nil
		})
		RegisterExtern("FloatFloor", func(args ...any) (any, error) {
			return ffi.FloatFloor(args[0].(float64)), nil
		})
		RegisterExtern("OsArgs", func(args ...any) (any, error) {
			return builtinOSArgs(), nil
		})
		RegisterExtern("EnvGet", func(args ...any) (any, error) {
			value := ffi.EnvGet(args[0].(string))
			if value == nil {
				return None[string](), nil
			}
			return Some(*value), nil
		})
		RegisterExtern("ReadLine", func(args ...any) (any, error) {
			value, err := ffi.ReadLine()
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("HexEncode", func(args ...any) (any, error) {
			return ffi.HexEncode(args[0].(string)), nil
		})
		RegisterExtern("HexDecode", func(args ...any) (any, error) {
			value, err := ffi.HexDecode(args[0].(string))
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("Base64Encode", func(args ...any) (any, error) {
			return ffi.Base64Encode(args[0].(string), maybeBoolPointer(args[1].(Maybe[bool]))), nil
		})
		RegisterExtern("Base64Decode", func(args ...any) (any, error) {
			value, err := ffi.Base64Decode(args[0].(string), maybeBoolPointer(args[1].(Maybe[bool])))
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("Base64EncodeURL", func(args ...any) (any, error) {
			return ffi.Base64EncodeURL(args[0].(string), maybeBoolPointer(args[1].(Maybe[bool]))), nil
		})
		RegisterExtern("Base64DecodeURL", func(args ...any) (any, error) {
			value, err := ffi.Base64DecodeURL(args[0].(string), maybeBoolPointer(args[1].(Maybe[bool])))
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("JsonEncode", func(args ...any) (any, error) {
			value, err := ffi.JsonEncode(builtinDynamicValue(args[0]))
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
	})
}
