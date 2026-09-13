package runtime

import (
	"math"
	"testing"
)

// Exact float boundaries; the corresponding integer maxima are not exact.
const (
	twoPow63 = 9223372036854775808.0  // 2^63
	twoPow64 = 18446744073709551616.0 // 2^64
	twoPow31 = 2147483648.0           // 2^31
)

func TestTryFloatToIntegerEndpoints(t *testing.T) {
	if got := TryFloatToSigned[int64](twoPow63, 64); got.IsSome() {
		t.Errorf("2^63 must not convert to int64, got %v", got)
	}
	if got := TryFloatToSigned[int64](-twoPow63, 64); !got.IsSome() {
		t.Error("-2^63 should convert to int64")
	} else if v := got.Value(); v != math.MinInt64 {
		t.Errorf("-2^63 should be MinInt64, got %d", v)
	}
	if got := TryFloatToUnsigned[uint64](twoPow64, 64); got.IsSome() {
		t.Error("2^64 must not convert to uint64")
	}
	if got := TryFloatToSigned[int32](twoPow31, 32); got.IsSome() {
		t.Error("2^31 must not convert to int32")
	}
}

func TestTryFloatToIntegerRejectsNonIntegral(t *testing.T) {
	for _, value := range []float64{1.5, -0.5, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := TryFloatToSigned[int64](value, 64); got.IsSome() {
			t.Errorf("%v must not convert to int64", value)
		}
	}
	if got := TryFloatToSigned[int64](math.Copysign(0, -1), 64); !got.IsSome() {
		t.Error("-0.0 should convert to 0")
	}
	if got := TryFloatToUnsigned[uint64](-1, 64); got.IsSome() {
		t.Error("-1 must not convert to uint64")
	}
}

func TestTrySignedAcrossSignedness(t *testing.T) {
	if got := TrySignedToUnsigned[uint8](-1, 8); got.IsSome() {
		t.Error("-1 must not convert to uint8")
	}
	if got := TrySignedToSigned[int8](127, 8); !got.IsSome() {
		t.Error("127 should convert to int8")
	}
	if got := TrySignedToSigned[int8](128, 8); got.IsSome() {
		t.Error("128 must not convert to int8")
	}
	if got := TryUnsignedToSigned[int64](math.MaxUint64, 64); got.IsSome() {
		t.Error("MaxUint64 must not convert to int64")
	}
	if got := TryUnsignedToSigned[int64](math.MaxInt64, 64); !got.IsSome() {
		t.Error("MaxInt64 should convert to int64")
	}
}

func TestTryFloat64ToFloat32(t *testing.T) {
	if got := TryFloat64ToFloat32[float32](0.1); !got.IsSome() {
		t.Error("0.1 should convert to float32 by rounding")
	}
	if got := TryFloat64ToFloat32[float32](1e39); got.IsSome() {
		t.Error("1e39 overflows float32 and must not convert")
	}
	if got := TryFloat64ToFloat32[float32](1e-50); !got.IsSome() {
		t.Error("underflow to zero is still a conversion")
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := TryFloat64ToFloat32[float32](value); !got.IsSome() {
			t.Errorf("%v should be preserved", value)
		}
	}
}

func TestTryRune(t *testing.T) {
	valid := []int64{0, 'a', 0xD7FF, 0xE000, 0x10FFFF}
	for _, value := range valid {
		if got := TryRuneFromSigned(value); !got.IsSome() {
			t.Errorf("%#x should be a valid scalar", value)
		}
	}
	invalid := []int64{-1, 0xD800, 0xDFFF, 0x110000}
	for _, value := range invalid {
		if got := TryRuneFromSigned(value); got.IsSome() {
			t.Errorf("%#x must not be a valid scalar", value)
		}
	}
	if got := TryRuneFromUnsigned(math.MaxUint64); got.IsSome() {
		t.Error("MaxUint64 must not be a valid scalar")
	}
	if got := TryRuneFromFloat(65.5); got.IsSome() {
		t.Error("a fractional value must not be a valid scalar")
	}
	if got := TryRuneFromFloat(65); !got.IsSome() {
		t.Error("65.0 should be a valid scalar")
	}
}

func TestFitFloatToIntegerSaturates(t *testing.T) {
	cases := []struct {
		value float64
		want  int64
	}{
		{twoPow63, math.MaxInt64},     // 2^63 saturates
		{1e300, math.MaxInt64},        // far beyond saturates
		{-1e300, math.MinInt64},       // far below saturates
		{-twoPow63, math.MinInt64},    // exactly the minimum
		{math.NaN(), 0},               // NaN is zero
		{math.Inf(1), math.MaxInt64},  // +Inf saturates
		{math.Inf(-1), math.MinInt64}, // -Inf saturates
		{1.9, 1},                      // truncates toward zero
		{-1.9, -1},                    // truncates toward zero
		{math.Copysign(0, -1), 0},     // -0.0 is zero
	}
	for _, tt := range cases {
		got := FitFloatToSigned[int64](tt.value, 64)
		if got != tt.want {
			t.Errorf("FitFloatToSigned(%v) = %d, want %d", tt.value, got, tt.want)
		}
	}
	if got := FitFloatToUnsigned[uint8](-5, 8); got != 0 {
		t.Errorf("negative should clamp to 0, got %d", got)
	}
	if got := FitFloatToUnsigned[uint8](300, 8); got != math.MaxUint8 {
		t.Errorf("300 should clamp to 255, got %d", got)
	}
}
