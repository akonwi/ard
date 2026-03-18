package ffi

import (
	"math"
	"strconv"
)

// FloatFromStr parses a string to a float, returning Float? (Maybe<Float>)
func FloatFromStr(str string) *float64 {
	if value, err := strconv.ParseFloat(str, 64); err == nil {
		return &value
	}
	return nil
}

// FloatFromInt converts an integer to a float
func FloatFromInt(intVal int) float64 {
	return float64(intVal)
}

// FloatFloor returns the floor of a float
func FloatFloor(floatVal float64) float64 {
	return math.Floor(floatVal)
}

// IntFromStr parses a string to an int, returning Int? (Maybe<Int>)
func IntFromStr(str string) *int {
	if value, err := strconv.Atoi(str); err == nil {
		return &value
	}
	return nil
}
