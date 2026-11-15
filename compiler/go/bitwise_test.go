package gotarget

import (
	"strings"
	"testing"
)

// Bitwise operators (ADR 0080) lower to Go's operators with Go's semantics:
// values wrap at the type's width, `>>` is arithmetic on signed values, and a
// count past the width shifts every bit out.
func TestGoTargetBitwiseOperators(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

fn main() {
  let a = 12
  let b = 10
  fmt::Println("int", a & b, a | b, a ^ b, ~a, a << 2, a >> 2)
  let n = -6
  fmt::Println("negative", n & 3, n | 3, n ^ 3, ~n, n << 1, n >> 1, -16 >> 2)
  fmt::Println("precedence", a & b == 8, a + b << 1, a | b ^ 3, a & b + 1, 1 << 3 - 1)
  fmt::Println("lowbit", a & -a, 40 & -40)

  let wide = 64
  let wider = 70
  fmt::Println("oversized", 5 << wide, 5 >> wide, n >> wider, n << wider, n >> 2000)

  let i8: Int8 = -128
  let i16: Int16 = 300
  let i32: Int32 = -1
  let i64: Int64 = 1
  fmt::Println("int8", i8 >> 7, i8 << 1, ~i8, i8 | 1, i8 ^ -1)
  fmt::Println("int16", i16 & 255, i16 << 8, i16 >> 2)
  fmt::Println("int32", i32 >> 31, i32 << 31, ~i32)
  fmt::Println("int64", i64 << 62, i64 << 63, i64 << 64)

  let u: Uint = 6
  let u8: Uint8 = 200
  let u16: Uint16 = 65535
  let u32: Uint32 = 1
  let u64: Uint64 = 0
  let p: Uintptr = 255
  let byte: Byte = 15
  fmt::Println("uint", u & 3, u | 1, u ^ 7, u >> 1)
  fmt::Println("uint8", ~u8, u8 >> 4, u8 << 1, u8 >> 8, u8 & ~15, u8 | 1)
  fmt::Println("uint16", u16 << 4, ~u16, u16 ^ 255)
  fmt::Println("uint32", u32 << 31, u32 << 32, ~u32)
  fmt::Println("uint64", ~u64, ~u64 >> 63, u64 | 9)
  fmt::Println("uintptr", p & 15, p >> 4)
  fmt::Println("byte", ~byte, byte << 4, byte | 240, byte ^ 255)

  let mask: Uint8 = ~0
  let count: Uint8 = 3
  fmt::Println("counts", a << count, u8 >> b, i64 << u32)
  fmt::Println("literal left", 1 << count, Float64::fit(1 << b), mask)
}
`)
	want := strings.Join([]string{
		"int 8 14 6 -13 48 3",
		"negative 2 -5 -7 5 -12 -3 -4",
		"precedence true 32 13 9 7",
		"lowbit 4 8",
		"oversized 0 0 -1 0 -1",
		"int8 -1 0 127 -127 127",
		"int16 44 11264 75",
		"int32 -1 -2147483648 0",
		"int64 4611686018427387904 -9223372036854775808 0",
		"uint 2 7 1 3",
		"uint8 55 12 144 0 192 201",
		"uint16 65520 0 65280",
		"uint32 2147483648 0 4294967294",
		"uint64 18446744073709551615 1 9",
		"uintptr 15 15",
		"byte 240 240 255 240",
		"counts 96 0 2",
		"literal left 8 1024 255",
	}, "\n") + "\n"
	if got != want {
		t.Fatalf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// Foreign named integers work without conversion, and their Go constants are
// typed with the named type (ADR 0051).
func TestGoTargetBitwiseForeignNamedIntegers(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
use go:os
use go:time

fn has(mode: os::FileMode, flag: os::FileMode) Bool {
  mode & flag != 0
}

fn main() {
  let mode = os::ModeDir | 493
  fmt::Println(has(mode, os::ModeDir), has(mode, os::ModeSymlink))
  fmt::Println(mode & os::ModePerm, mode & ~os::ModeDir)
  fmt::Println(time::Second >> 1, time::Millisecond << 2)
}
`)
	want := "true false\n-rwxr-xr-x -rwxr-xr-x\n500ms 4ms\n"
	if got != want {
		t.Fatalf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// Packing and unpacking bytes, as encoding code does.
func TestGoTargetBitwisePackBytes(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

fn pack(hi: Byte, lo: Byte) Uint16 {
  Uint16::from(hi) << 8 | Uint16::from(lo)
}

fn main() {
  let packed = pack(Byte::from(18), Byte::from(52))
  fmt::Println(packed, Byte::fit(packed >> 8), Byte::fit(packed & 255))
}
`)
	if got != "4660 18 52\n" {
		t.Fatalf("got %q", got)
	}
}

// A negative count is a run-time panic, as in Go.
func TestGoTargetNegativeShiftCountPanics(t *testing.T) {
	_, stderr, err := runGoTargetSourceOutput(t, `fn main() {
  let n = -1
  let x = 1 << n
}
`)
	if err == nil {
		t.Fatal("expected the program to panic")
	}
	if !strings.Contains(stderr, "negative shift amount") {
		t.Fatalf("stderr = %q, want a negative shift amount panic", stderr)
	}
}

// Operands are evaluated left to right (ADR 0078): a read on the left sees
// its value before a call on the right changes it.
func TestGoTargetBitwiseOperandOrder(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

fn set(n: &mut Int, value: Int) Int {
  n.* = value
  value
}

fn tick(label: Str, value: Int) Int {
  fmt::Println(label)
  value
}

fn main() {
  mut x = 12
  let anded = x & set(&mut x, 10)
  x = 12
  let ored = x | set(&mut x, 1)
  x = 12
  let shifted = x << set(&mut x, 1)
  x = 12
  let notted = ~x + set(&mut x, 1)
  fmt::Println(anded, ored, shifted, notted)
  fmt::Println(tick("left", 6) ^ tick("right", 3))
}
`)
	want := "8 13 24 -12\nleft\nright\n5\n"
	if got != want {
		t.Fatalf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// Go types an untyped constant shifted by a variable count from its context,
// and a float context does not compile. Every constant left operand is
// emitted with its Ard type: a literal, an untyped Go constant, and a
// constant expression.
func TestGoTargetConstantShiftedByVariableCount(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
use go:math

fn main() {
  let n: Uint8 = 1
  fmt::Println(Float64::fit(1 << n), Float64::fit(math::MaxInt << n), Float64::fit((1 + 2) << n), Float64::fit(~1 << n), Float64::fit(-8 >> n))
}
`)
	if got != "2 -2 6 -4 -4\n" {
		t.Fatalf("got %q", got)
	}
}
