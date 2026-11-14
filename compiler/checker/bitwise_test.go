package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

// Bitwise operators (ADR 0080) accept Int, every sized integer, Uintptr,
// Byte, and foreign named integers. `&`, `|` and `^` need operands of the
// same type, as arithmetic does; a shift count may be any integer type and
// the result has the left operand's type.
func TestBitwiseOperatorTypes(t *testing.T) {
	errorDiagnostic := func(message string) []checker.Diagnostic {
		return []checker.Diagnostic{{Kind: checker.Error, Message: message}}
	}
	run(t, []test{
		{
			name: "Int operators",
			input: `fn f(a: Int, b: Int) [Int] {
  [a & b, a | b, a ^ b, ~a, a << 2, a >> b]
}`,
		},
		{
			name: "results keep the operand type",
			input: `fn f(a: Uint8, b: Uint8, n: Int) Uint8 {
  let x: Uint8 = a & b
  let y: Uint8 = a | b ^ x
  let z: Uint8 = ~y
  z << n >> 1
}`,
		},
		{
			name: "each sized integer and Byte",
			input: `fn f(i8: Int8, i16: Int16, i32: Int32, i64: Int64, u: Uint, u8: Uint8, u16: Uint16, u32: Uint32, u64: Uint64, p: Uintptr, b: Byte) {
  let _a: Int8 = i8 & 1 | ~i8
  let _b: Int16 = i16 ^ 3
  let _c: Int32 = i32 >> 4
  let _d: Int64 = i64 << 5
  let _e: Uint = u & 6
  let _f: Uint8 = u8 | 7
  let _g: Uint16 = u16 ^ 8
  let _h: Uint32 = ~u32
  let _i: Uint64 = u64 >> 9
  let _j: Uintptr = p & 10
  let _k: Byte = b << 1 | b
}`,
		},
		{
			name: "literals adopt the other operand's type, including through ~",
			input: `fn f(a: Uint8) Uint8 {
  a & ~15 | 128
}`,
		},
		{
			name:  "~ of a literal adopts an annotation",
			input: `let mask: Uint8 = ~0`,
		},
		{
			name: "foreign named integers work without conversion",
			input: `use go:os
fn is_dir(mode: os::FileMode) Bool {
  mode & os::ModeDir != 0
}
fn permissions(mode: os::FileMode) os::FileMode {
  mode & os::ModePerm
}
fn without_dir(mode: os::FileMode) os::FileMode {
  mode & ~os::ModeDir | 420
}
fn shifted(mode: os::FileMode) os::FileMode {
  mode >> 1
}`,
		},
		{
			name: "shift count may be any integer type",
			input: `use go:time
fn f(a: Int, n: Uint8, m: Int64, d: time::Duration) Int {
  a << n >> m << d
}`,
		},
		{
			name:  "a literal shifted by a typed count stays Int",
			input: `fn f(n: Uint8) Int { 1 << n }`,
		},
		{
			name:        "different operand types",
			input:       `fn f(a: Int, b: Uint8) Int { a & b }`,
			diagnostics: errorDiagnostic("Cannot apply '&' to different types"),
		},
		{
			name: "foreign named integer and its underlying type",
			input: `use go:os
fn f(mode: os::FileMode, bits: Uint32) os::FileMode { mode | bits }`,
			diagnostics: errorDiagnostic("Cannot apply '|' to different types"),
		},
		{
			name:        "Bool operands",
			input:       `let x = true & false`,
			diagnostics: errorDiagnostic("The '&' operator can only be used for integers; use 'and' for Bool"),
		},
		{
			name:        "Bool or",
			input:       `let x = true | false`,
			diagnostics: errorDiagnostic("The '|' operator can only be used for integers; use 'or' for Bool"),
		},
		{
			name:        "Bool xor",
			input:       `let x = true ^ false`,
			diagnostics: errorDiagnostic("The '^' operator can only be used for integers; use '!=' for Bool"),
		},
		{
			name:        "Float operands",
			input:       `let x = 1.5 ^ 2.5`,
			diagnostics: errorDiagnostic("The '^' operator can only be used for integers"),
		},
		{
			name:        "Rune operands",
			input:       `let x = 'a' | ' '`,
			diagnostics: errorDiagnostic("The '|' operator can only be used for integers; convert a Rune with Int32::from"),
		},
		{
			name:        "Str operands",
			input:       `let x = "a" & "b"`,
			diagnostics: errorDiagnostic("The '&' operator can only be used for integers"),
		},
		{
			name:        "~ of a Bool",
			input:       `let x = ~true`,
			diagnostics: errorDiagnostic("Only integers can be complemented with '~'; use 'not' for Bool"),
		},
		{
			name:        "~ of a Float64",
			input:       `let x = ~1.5`,
			diagnostics: errorDiagnostic("Only integers can be complemented with '~'"),
		},
		{
			name:        "~ of a Rune",
			input:       `let x = ~'a'`,
			diagnostics: errorDiagnostic("Only integers can be complemented with '~'; convert a Rune with Int32::from"),
		},
		{
			name:        "shifting a Float64",
			input:       `let x = 1.5 << 1`,
			diagnostics: errorDiagnostic("The '<<' operator can only be used for integers"),
		},
		{
			name:        "shifting a Rune",
			input:       `let x = 'a' >> 1`,
			diagnostics: errorDiagnostic("The '>>' operator can only be used for integers; convert a Rune with Int32::from"),
		},
		{
			name:        "non-integer shift count",
			input:       `fn f(a: Int) Int { a << 1.5 }`,
			diagnostics: errorDiagnostic("Shift count must be an integer"),
		},
		{
			name:        "Rune shift count",
			input:       `fn f(a: Int) Int { a >> 'a' }`,
			diagnostics: errorDiagnostic("Shift count must be an integer"),
		},
		{
			name:        "negative literal shift count",
			input:       `fn f(a: Int) Int { a << -1 }`,
			diagnostics: errorDiagnostic("Shift count must not be negative"),
		},
		{
			name:        "constant shift that overflows Int",
			input:       `let x = 1 << 64`,
			diagnostics: errorDiagnostic("Constant 1 << 64 overflows Int"),
		},
		{
			name:        "constant shift into the sign bit",
			input:       `let x = 1 << 63`,
			diagnostics: errorDiagnostic("Constant 1 << 63 overflows Int"),
		},
		{
			name:  "constant shift to the minimum Int",
			input: `let x = -1 << 63`,
		},
		{
			name:  "constant right shift within the width",
			input: `let x = -8 >> 63`,
		},
		{
			name:        "constant right shift past the width",
			input:       `let x = 8 >> 64`,
			diagnostics: errorDiagnostic("Constant shift count 64 is too large for Int"),
		},
		{
			name:        "constant zero shifted past the width",
			input:       `let x = 0 << 2000`,
			diagnostics: errorDiagnostic("Constant shift count 2000 is too large for Int"),
		},
		{
			name:  "a variable shifted by a large literal count",
			input: `fn f(a: Int) Int { a << 2000 }`,
		},
	})
}
