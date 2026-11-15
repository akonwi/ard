package gotarget

import (
	"strings"
	"testing"
)

// Constant integer expressions compile (the checker rejects the ones Go
// would), and T::fit of a constant wraps like T::fit of a variable.
func TestGoTargetConstantIntegerExpressions(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
use go:os
use go:time

fn main() {
  let n = 0
  fmt::Println("fit", Uint8::fit(0 - 1), Int8::fit(200 + 100), Uint8::fit(~0), Uint64::fit(0 - 1), Int::fit(Int64::from(1) << 40))
  fmt::Println("fit variable", Uint8::fit(n - 1), Int8::fit(n + 300))
  fmt::Println("fit Go constant", Uint8::fit(os::ModeDir >> 24), Int32::fit(time::Hour * 1000))
  fmt::Println("in range", 9223372036854775807 - 1 + 1, -9223372036854775807 - 1, 7 / -2, -7 % 3, ~Uint8::from(5))
  fmt::Println("go constants", time::Second * 60 / 2, os::ModeDir | os::ModePerm)
}
`)
	want := strings.Join([]string{
		"fit 255 44 255 18446744073709551615 1099511627776",
		"fit variable 255 44",
		"fit Go constant 128 1362165760",
		"in range 9223372036854775807 -9223372036854775808 -3 -1 250",
		"go constants 30s drwxrwxrwx",
	}, "\n") + "\n"
	if got != want {
		t.Fatalf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}
