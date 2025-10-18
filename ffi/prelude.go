package ffi

import (
	"fmt"
	"math"
	"strconv"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

// FloatFromStr parses a string to a float, returning Float? (Maybe<Float>)
func FloatFromStr(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic("FloatFromStr expects 1 argument")
	}

	str := args[0].AsString()
	out := runtime.MakeNone(checker.Float)
	if value, err := strconv.ParseFloat(str, 64); err == nil {
		out = out.ToSome(value)
	}
	return out
}

// FloatFromInt converts an integer to a float
func FloatFromInt(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic("FloatFromInt expects 1 argument")
	}

	intVal := args[0].AsInt()
	return runtime.MakeFloat(float64(intVal))
}

// FloatFloor returns the floor of a float
func FloatFloor(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic("FloatFloor expects 1 argument")
	}

	floatVal := args[0].AsFloat()
	return runtime.MakeFloat(math.Floor(floatVal))
}

func IntFromStr(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic("IntFromStr expects 1 argument")
	}

	str := args[0].AsString()
	out := runtime.MakeNone(checker.Int)
	if value, err := strconv.Atoi(str); err == nil {
		return out.ToSome(value)
	}
	return out
}

func NewList(args []*runtime.Object, ret checker.Type) *runtime.Object {
	retList, ok := ret.(*checker.List)
	if !ok {
		panic(fmt.Errorf("expected *checker.List, got %T", ret))
	}
	return runtime.MakeList(retList.Of())
}
