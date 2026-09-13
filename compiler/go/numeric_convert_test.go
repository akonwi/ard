package gotarget

import "testing"

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

// Int64 -> Int succeeds for these values on every platform.
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
