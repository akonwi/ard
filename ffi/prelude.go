package ffi

import (
	"math"
	"strconv"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// FloatFromStr parses a string to a float, returning Float? (Maybe<Float>)
func FloatFromStr(vm runtime.VM, args []*runtime.Object) *runtime.Object {
	if len(args) != 1 {
		panic("FloatFromStr expects 1 argument")
	}

	str := args[0].AsString()
	if value, err := strconv.ParseFloat(str, 64); err == nil {
		floatObj := runtime.MakeFloat(value)
		return floatObj.ToSome()
	}
	return runtime.MakeMaybe(nil, checker.Float)
}

// FloatFromInt converts an integer to a float
func FloatFromInt(vm runtime.VM, args []*runtime.Object) *runtime.Object {
	if len(args) != 1 {
		panic("FloatFromInt expects 1 argument")
	}

	intVal := args[0].AsInt()
	return runtime.MakeFloat(float64(intVal))
}

// FloatFloor returns the floor of a float
func FloatFloor(vm runtime.VM, args []*runtime.Object) *runtime.Object {
	if len(args) != 1 {
		panic("FloatFloor expects 1 argument")
	}

	floatVal := args[0].AsFloat()
	return runtime.MakeFloat(math.Floor(floatVal))
}

func IntFromStr(vm runtime.VM, args []*runtime.Object) *runtime.Object {
	if len(args) != 1 {
		panic("IntFromStr expects 1 argument")
	}

	str := args[0].AsString()
	out := runtime.MakeMaybe(nil, checker.Int)
	if value, err := strconv.Atoi(str); err == nil {
		out.Set(value)
	}
	return out
}
