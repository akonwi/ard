package vm

import (
	"github.com/akonwi/ard/ffi"
	"github.com/akonwi/ard/runtime"
)

func vmFFIFloatFromInt(args []any) any {
	return ffi.FloatFromInt(args[0].(int))
}

func vmFFIIntFromStr(args []any) any {
	value := ffi.IntFromStr(args[0].(string))
	if value == nil {
		return runtime.NoneValue()
	}
	return runtime.SomeValue(*value)
}

func vmFFIFloatFromStr(args []any) any {
	value := ffi.FloatFromStr(args[0].(string))
	if value == nil {
		return runtime.NoneValue()
	}
	return runtime.SomeValue(*value)
}

func vmFFIFloatFloor(args []any) any {
	return ffi.FloatFloor(args[0].(float64))
}

func vmFFIEnvGet(args []any) any {
	value := ffi.EnvGet(args[0].(string))
	if value == nil {
		return runtime.NoneValue()
	}
	return runtime.SomeValue(*value)
}

func vmFFIIsNil(args []any) any {
	return args[0] == nil
}

func maybeBoolArg(value any) *bool {
	maybe, ok := value.(runtime.MaybeValue)
	if !ok {
		panic("expected Bool? argument")
	}
	if maybe.None {
		return nil
	}
	boolValue := maybe.Value.(bool)
	return &boolValue
}

func vmFFIJsonToDynamic(args []any) any {
	value, err := ffi.JsonToDynamic(args[0].(string))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFIBase64Encode(args []any) any {
	return ffi.Base64Encode(args[0].(string), maybeBoolArg(args[1]))
}

func vmFFIBase64Decode(args []any) any {
	value, err := ffi.Base64Decode(args[0].(string), maybeBoolArg(args[1]))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFIBase64EncodeURL(args []any) any {
	return ffi.Base64EncodeURL(args[0].(string), maybeBoolArg(args[1]))
}

func vmFFIBase64DecodeURL(args []any) any {
	value, err := ffi.Base64DecodeURL(args[0].(string), maybeBoolArg(args[1]))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFIHexEncode(args []any) any {
	return ffi.HexEncode(args[0].(string))
}

func vmFFIHexDecode(args []any) any {
	value, err := ffi.HexDecode(args[0].(string))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}
