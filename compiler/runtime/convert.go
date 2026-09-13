package runtime

import "math"

// Tiered numeric conversion helpers (ADR 0072).
//
// Every helper takes the target's bit width rather than its bounds, because a
// platform-sized target (Int, Uint, Uintptr) has no constant bounds the
// backend could emit: it passes math/bits.UintSize instead. Bounds are then
// derived with integer arithmetic.
//
// Float bounds are exact powers of two and the upper bound is exclusive.
// float64(math.MaxInt64) rounds *up* to 2^63, so comparing a float against the
// type's maximum would wrongly accept an out-of-range value. Nothing here
// hands an unguarded float to a Go integer conversion, because Go leaves that
// result implementation-defined (amd64 wraps, arm64 saturates).

// Signed, Unsigned, and Float admit named Go types (a foreign
// `type Duration int64`) through the ~ approximation.
type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

// signedMin is the most negative value of a signed integer of the given width.
func signedMin(bits int) int64 { return int64(-1) << (bits - 1) }

// signedMax is the largest value of a signed integer of the given width.
func signedMax(bits int) int64 { return int64(uint64(1)<<(bits-1) - 1) }

// unsignedMax is the largest value of an unsigned integer of the given width.
func unsignedMax(bits int) uint64 { return ^uint64(0) >> (64 - bits) }

// TrySignedToSigned reports value when it fits the target's signed range.
func TrySignedToSigned[T Signed](value int64, bits int) Maybe[T] {
	if value < signedMin(bits) || value > signedMax(bits) {
		return None[T]()
	}
	return Some(T(value))
}

// TrySignedToUnsigned rejects negatives outright: a matching bit pattern is
// not a representable value, so int8(-1) is not a uint8.
func TrySignedToUnsigned[T Unsigned](value int64, bits int) Maybe[T] {
	if value < 0 || uint64(value) > unsignedMax(bits) {
		return None[T]()
	}
	return Some(T(value))
}

// TryUnsignedToSigned reports value when it fits the target's signed maximum.
func TryUnsignedToSigned[T Signed](value uint64, bits int) Maybe[T] {
	if value > uint64(signedMax(bits)) {
		return None[T]()
	}
	return Some(T(value))
}

// TryUnsignedToUnsigned reports value when it fits the target's width.
func TryUnsignedToUnsigned[T Unsigned](value uint64, bits int) Maybe[T] {
	if value > unsignedMax(bits) {
		return None[T]()
	}
	return Some(T(value))
}

// TryFloatToSigned reports value when it is finite, integral, and within the
// target's range. NaN fails every comparison, so the range test rejects it.
func TryFloatToSigned[T Signed](value float64, bits int) Maybe[T] {
	hi := math.Ldexp(1, bits-1) // 2^(bits-1), exclusive
	if !(value >= -hi && value < hi) || value != math.Trunc(value) {
		return None[T]()
	}
	return Some(T(value))
}

// TryFloatToUnsigned reports value when it is finite, integral, and within the
// target's range.
func TryFloatToUnsigned[T Unsigned](value float64, bits int) Maybe[T] {
	hi := math.Ldexp(1, bits) // 2^bits, exclusive
	if !(value >= 0 && value < hi) || value != math.Trunc(value) {
		return None[T]()
	}
	return Some(T(value))
}

// TryFloat64ToFloat32 rounds, and fails only when a finite value overflows to
// an infinity. NaN and infinite inputs are preserved (ADR 0072).
func TryFloat64ToFloat32[T ~float32](value float64) Maybe[T] {
	narrowed := float32(value)
	if math.IsInf(float64(narrowed), 0) && !math.IsInf(value, 0) {
		return None[T]()
	}
	return Some(T(narrowed))
}

// A Rune is always a valid Unicode scalar value, so every conversion into one
// is checked (ADR 0026, ADR 0072). The target is always Go's rune, because a
// foreign named type over int32 converts as an Int32.
func validScalarValue(value int64) bool {
	if value < 0 || value > 0x10FFFF {
		return false
	}
	return value < 0xD800 || value > 0xDFFF
}

func TryRuneFromSigned(value int64) Maybe[rune] {
	if !validScalarValue(value) {
		return None[rune]()
	}
	return Some(rune(value))
}

func TryRuneFromUnsigned(value uint64) Maybe[rune] {
	if value > 0x10FFFF {
		return None[rune]()
	}
	return TryRuneFromSigned(int64(value))
}

func TryRuneFromFloat(value float64) Maybe[rune] {
	if !(value >= 0 && value <= 0x10FFFF) || value != math.Trunc(value) {
		return None[rune]()
	}
	return TryRuneFromSigned(int64(value))
}

// FitFloatToSigned saturates: values at or beyond the bounds clamp and NaN
// becomes zero, replacing Go's implementation-defined float-to-integer
// overflow with defined behavior (ADR 0072). Bounds are derived with integer
// arithmetic because the maximum is not exactly representable as a float.
func FitFloatToSigned[T Signed](value float64, bits int) T {
	hi := math.Ldexp(1, bits-1)
	switch {
	case math.IsNaN(value):
		return 0
	case value <= -hi:
		return T(signedMin(bits))
	case value >= hi:
		return T(signedMax(bits))
	default:
		return T(value)
	}
}

// FitFloatToUnsigned saturates, clamping negatives to zero.
func FitFloatToUnsigned[T Unsigned](value float64, bits int) T {
	hi := math.Ldexp(1, bits)
	switch {
	case math.IsNaN(value):
		return 0
	case value <= 0:
		return 0
	case value >= hi:
		return T(unsignedMax(bits))
	default:
		return T(value)
	}
}
