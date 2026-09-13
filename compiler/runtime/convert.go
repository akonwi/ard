package runtime

import "math"

// Float-to-integer helpers compare against exact powers of two before using a
// Go conversion. Go's result is implementation-defined outside that range.

// Signed admits foreign named scalar types through its ~ constraints.
type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

// Unsigned admits foreign named scalar types through its ~ constraints.
type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

func signedMin(bits int) int64    { return int64(-1) << (bits - 1) }
func signedMax(bits int) int64    { return int64(uint64(1)<<(bits-1) - 1) }
func unsignedMax(bits int) uint64 { return ^uint64(0) >> (64 - bits) }

// TrySignedToSigned reports value when it fits the target's signed range.
func TrySignedToSigned[T Signed](value int64, bits int) Maybe[T] {
	if value < signedMin(bits) || value > signedMax(bits) {
		return None[T]()
	}
	return Some(T(value))
}

// TrySignedToUnsigned reports value when it fits the target's unsigned range.
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

// TryFloatToSigned reports value when it is finite, integral, and in range.
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

// Rune conversions preserve the Unicode scalar-value invariant from ADR 0026.
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

// FitFloatToSigned saturates at the target bounds and maps NaN to zero.
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
