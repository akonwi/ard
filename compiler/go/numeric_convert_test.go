package gotarget

import "testing"

// Executable coverage for the tiered numeric conversions in ADR 0072 (#500).
// These run the generated Go program, so they pin real runtime behavior rather
// than the shape of the emitted code.

func runConversionCases(t *testing.T, cases []struct {
	name  string
	input string
	want  string
},
) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			if got := runGoTargetParityJSON(t, program); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// #500: Float32 -> Float64 widening must preserve the numeric value, including
// signed zero, the infinities, and NaN.
func TestGoTargetLosslessWidening(t *testing.T) {
	runConversionCases(t, []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "Float32 widens to Float64",
			input: `fn main() Bool {
  let ratio: Float32 = Float32::from(1.5)
  Float64::from(ratio) == 1.5
}`,
			want: "true",
		},
		{
			name: "negative fractional Float32 widens",
			input: `fn main() Bool {
  let ratio: Float32 = Float32::from(-0.5)
  Float64::from(ratio) == -0.5
}`,
			want: "true",
		},
		{
			name: "Rune widens to Uint32 because it is a valid scalar",
			input: `fn main() Bool {
  let r: Rune = 'A'
  Uint32::from(r) == 65
}`,
			want: "true",
		},
		{
			name: "Byte widens losslessly into a Rune",
			input: `fn main() Bool {
  let b: Byte = 65
  Rune::from(b) == 'A'
}`,
			want: "true",
		},
	})
}

// #500: Int64 -> Int is checked. On a 64-bit platform the whole Int64 range is
// representable, so these assert the in-range behavior that holds everywhere.
func TestGoTargetCheckedNarrowing(t *testing.T) {
	runConversionCases(t, []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "representable Int64 converts",
			input: `fn main() Bool {
  let elapsed: Int64 = 1500
  Int::try(elapsed).or(0) == 1500
}`,
			want: "true",
		},
		{
			name: "negative Int64 converts",
			input: `fn main() Bool {
  let elapsed: Int64 = -1500
  Int::try(elapsed).or(0) == -1500
}`,
			want: "true",
		},
		{
			name: "out of range narrowing reports none",
			input: `fn main() Bool {
  let big: Int64 = 300
  Uint8::try(big).is_none()
}`,
			want: "true",
		},
		{
			name: "in range narrowing reports some",
			input: `fn main() Bool {
  let small: Int64 = 200
  Uint8::try(small).or(Uint8::from(0)) == 200
}`,
			want: "true",
		},
		{
			// A matching bit pattern is not a representable value.
			name: "a negative value is not an unsigned value",
			input: `fn main() Bool {
  let negative: Int64 = -1
  Uint8::try(negative).is_none()
}`,
			want: "true",
		},
		{
			name: "both Int8 endpoints are representable",
			input: `fn main() Bool {
  let low: Int64 = -128
  let high: Int64 = 127
  Int8::try(low).or(Int8::from(0)) == -128 and Int8::try(high).or(Int8::from(0)) == 127
}`,
			want: "true",
		},
		{
			name: "one past each Int8 endpoint is rejected",
			input: `fn main() Bool {
  let low: Int64 = -129
  let high: Int64 = 128
  Int8::try(low).is_none() and Int8::try(high).is_none()
}`,
			want: "true",
		},
	})
}

// Float sources must be finite, integral, and in range. The endpoints that
// matter are the exact powers of two, which are representable as floats while
// the corresponding type maxima are not.
func TestGoTargetCheckedFloatToInt(t *testing.T) {
	runConversionCases(t, []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "an integral float converts",
			input: `fn main() Bool {
  let value = 42.0
  Int::try(value).or(0) == 42
}`,
			want: "true",
		},
		{
			name: "a fractional float is rejected",
			input: `fn main() Bool {
  let value = 42.5
  Int::try(value).is_none()
}`,
			want: "true",
		},
		{
			name: "negative zero converts to zero",
			input: `fn main() Bool {
  let value = -0.0
  Int::try(value).or(1) == 0
}`,
			want: "true",
		},
		{
			// 2^31 is exactly representable as a float; Int32's maximum is not.
			name: "two to the thirty-first is out of range for Int32",
			input: `fn main() Bool {
  let value = 2147483648.0
  Int32::try(value).is_none()
}`,
			want: "true",
		},
		{
			name: "one below two to the thirty-first is in range for Int32",
			input: `fn main() Bool {
  let value = 2147483647.0
  Int32::try(value).or(Int32::from(0)) == 2147483647
}`,
			want: "true",
		},
		{
			name: "a negative float is not an unsigned value",
			input: `fn main() Bool {
  let value = -1.0
  Uint8::try(value).is_none()
}`,
			want: "true",
		},
	})
}

// `fit` is total: integers wrap and floats saturate, replacing Go's
// implementation-defined float-to-integer overflow.
func TestGoTargetForcedConversion(t *testing.T) {
	runConversionCases(t, []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "integer narrowing wraps",
			input: `fn main() Bool {
  let n: Int = 300
  Uint8::fit(n) == 44
}`,
			want: "true",
		},
		{
			name: "float to int truncates toward zero",
			input: `fn main() Bool {
  let value = 42.9
  let negative = -42.9
  Int::fit(value) == 42 and Int::fit(negative) == -42
}`,
			want: "true",
		},
		{
			name: "float overflow saturates instead of wrapping",
			input: `fn main() Bool {
  let value = 3000000000.0
  Int32::fit(value) == 2147483647
}`,
			want: "true",
		},
		{
			name: "negative float overflow saturates",
			input: `fn main() Bool {
  let value = -3000000000.0
  Int32::fit(value) == -2147483648
}`,
			want: "true",
		},
		{
			name: "a negative float clamps to zero for an unsigned target",
			input: `fn main() Bool {
  let value = -5.0
  Uint8::fit(value) == 0
}`,
			want: "true",
		},
		{
			name: "a large float clamps to the unsigned maximum",
			input: `fn main() Bool {
  let value = 300.0
  Uint8::fit(value) == 255
}`,
			want: "true",
		},
		{
			name: "Int converts to Float64",
			input: `fn main() Bool {
  let n: Int = 3
  Float64::fit(n) == 3.0
}`,
			want: "true",
		},
	})
}

// A Rune is always a valid Unicode scalar value (ADR 0026), so conversions
// into one are always checked.
func TestGoTargetRuneConversion(t *testing.T) {
	runConversionCases(t, []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "a valid scalar converts",
			input: `fn main() Bool {
  let code: Int = 65
  Rune::try(code).or('?') == 'A'
}`,
			want: "true",
		},
		{
			name: "a surrogate is rejected",
			input: `fn main() Bool {
  let code: Int = 55296
  Rune::try(code).is_none()
}`,
			want: "true",
		},
		{
			name: "a negative code point is rejected",
			input: `fn main() Bool {
  let code: Int = -1
  Rune::try(code).is_none()
}`,
			want: "true",
		},
		{
			name: "beyond the last scalar is rejected",
			input: `fn main() Bool {
  let code: Int = 1114112
  Rune::try(code).is_none()
}`,
			want: "true",
		},
		{
			name: "the last scalar is accepted",
			input: `fn main() Bool {
  let code: Int = 1114111
  Rune::try(code).is_some()
}`,
			want: "true",
		},
	})
}

// A foreign named scalar converts through its underlying Go kind (#284).
func TestGoTargetForeignScalarConversion(t *testing.T) {
	runConversionCases(t, []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "an identity conversion into a foreign named scalar",
			input: `use go:time
fn main() Bool {
  let ms: Int64 = 5
  time::Duration::from(ms) * time::Millisecond == 5 * time::Millisecond
}`,
			want: "true",
		},
		{
			name: "a foreign named scalar narrows with try",
			input: `use go:time
fn main() Bool {
  let d = 5 * time::Millisecond
  Int32::try(d).is_some()
}`,
			want: "true",
		},
	})
}
