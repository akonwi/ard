package ffi

import (
	"encoding/base64"
	"encoding/hex"
	"math"
	"strconv"
)

var HostFunctions = Host{
	Base64Decode:    Base64Decode,
	Base64DecodeURL: Base64DecodeURL,
	Base64Encode:    Base64Encode,
	Base64EncodeURL: Base64EncodeURL,
	FloatFloor:      FloatFloor,
	FloatFromInt:    FloatFromInt,
	FloatFromStr:    FloatFromStr,
	HexDecode:       HexDecode,
	HexEncode:       HexEncode,
	IntFromStr:      IntFromStr,
}.Functions()

func Base64Encode(input string, noPad Maybe[bool]) string {
	if noPad.Some && noPad.Value {
		return base64.RawStdEncoding.EncodeToString([]byte(input))
	}
	return base64.StdEncoding.EncodeToString([]byte(input))
}

func Base64Decode(input string, noPad Maybe[bool]) (string, error) {
	enc := base64.StdEncoding
	if noPad.Some && noPad.Value {
		enc = base64.RawStdEncoding
	}
	decoded, err := enc.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func Base64EncodeURL(input string, noPad Maybe[bool]) string {
	if noPad.Some && noPad.Value {
		return base64.RawURLEncoding.EncodeToString([]byte(input))
	}
	return base64.URLEncoding.EncodeToString([]byte(input))
}

func Base64DecodeURL(input string, noPad Maybe[bool]) (string, error) {
	enc := base64.URLEncoding
	if noPad.Some && noPad.Value {
		enc = base64.RawURLEncoding
	}
	decoded, err := enc.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func HexEncode(input string) string {
	return hex.EncodeToString([]byte(input))
}

func HexDecode(input string) (string, error) {
	decoded, err := hex.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func FloatFromStr(str string) Maybe[float64] {
	value, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return None[float64]()
	}
	return Some(value)
}

func FloatFromInt(value int) float64 {
	return float64(value)
}

func FloatFloor(value float64) float64 {
	return math.Floor(value)
}

func IntFromStr(str string) Maybe[int] {
	value, err := strconv.Atoi(str)
	if err != nil {
		return None[int]()
	}
	return Some(value)
}
