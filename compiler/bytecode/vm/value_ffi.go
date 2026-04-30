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

func vmFFIJsonToDynamic(args []any) any {
	value, err := ffi.JsonToDynamic(args[0].(string))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}
