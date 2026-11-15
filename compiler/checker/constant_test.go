package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

// An integer expression built only from literals and Go constants is a Go
// constant expression: Go evaluates it while compiling and rejects a value
// that does not fit its type. The checker evaluates it first, so the error is
// reported in Ard rather than as a Go build failure. Every intermediate value
// must fit its type, as for Go's typed constants.
func TestConstantIntegerExpressions(t *testing.T) {
	errorDiagnostic := func(message string) []checker.Diagnostic {
		return []checker.Diagnostic{{Kind: checker.Error, Message: message}}
	}
	run(t, []test{
		{
			name:        "addition past the maximum Int",
			input:       `let x = 9223372036854775807 + 1`,
			diagnostics: errorDiagnostic("Constant value 9223372036854775808 overflows Int"),
		},
		{
			name:        "an intermediate value must fit",
			input:       `let x = 9223372036854775807 + 1 - 1`,
			diagnostics: errorDiagnostic("Constant value 9223372036854775808 overflows Int"),
		},
		{
			name:  "values that stay in range",
			input: `let x = 9223372036854775807 - 1 + 1 - 3 * 4 + 12 / 5 % 2`,
		},
		{
			name:        "subtraction past the minimum Int",
			input:       `let x = -9223372036854775807 - 2`,
			diagnostics: errorDiagnostic("Constant value -9223372036854775809 overflows Int"),
		},
		{
			name:        "multiplication",
			input:       `let x = 4611686018427387904 * 2`,
			diagnostics: errorDiagnostic("Constant value 9223372036854775808 overflows Int"),
		},
		{
			name:        "nested shifts",
			input:       `let x = (1 << 62) << 2`,
			diagnostics: errorDiagnostic("Constant value 18446744073709551616 overflows Int"),
		},
		{
			name:        "a sized literal",
			input:       `let x = Uint8::from(200) + 100`,
			diagnostics: errorDiagnostic("Constant value 300 overflows Uint8"),
		},
		{
			name:        "negating the minimum Int8",
			input:       `let x = -Int8::from(-128)`,
			diagnostics: errorDiagnostic("Constant value 128 overflows Int8"),
		},
		{
			name:        "dividing the minimum Int8 by -1",
			input:       `let x = Int8::from(-128) / -1`,
			diagnostics: errorDiagnostic("Constant value 128 overflows Int8"),
		},
		{
			name:        "complementing an unsigned value stays in range",
			input:       `let x = ~Uint8::from(5) + 6`,
			diagnostics: errorDiagnostic("Constant value 256 overflows Uint8"),
		},
		{
			name:        "Rune arithmetic",
			input:       `let x = 'a' * 100000000`,
			diagnostics: errorDiagnostic("Constant value 9700000000 overflows Rune"),
		},
		{
			name: "a Go constant",
			input: `use go:time
let x = time::Second * 10000000000000`,
			diagnostics: errorDiagnostic("Constant value 10000000000000000000000 overflows time::Duration"),
		},
		{
			name: "an unsigned Go constant",
			input: `use go:os
let x = os::ModeDir << 1`,
			diagnostics: errorDiagnostic("Constant value 4294967296 overflows fs::FileMode"),
		},
		{
			name: "Go constants that stay in range",
			input: `use go:os
use go:time
let x = time::Second * 60 / 2
let y = os::ModeDir | os::ModePerm`,
		},
		{
			name:  "a variable operand is not constant",
			input: `fn f(a: Int) Int { a + 9223372036854775807 }`,
		},
		{
			name:        "division by a literal zero",
			input:       `fn f(a: Int) Int { a / 0 }`,
			diagnostics: errorDiagnostic("Division by zero"),
		},
		{
			name:        "modulo by a literal zero",
			input:       `fn f(a: Int) Int { a % 0 }`,
			diagnostics: errorDiagnostic("Division by zero"),
		},
		{
			name:        "division by a constant expression equal to zero",
			input:       `fn f(a: Uint8) Uint8 { a / (Uint8::from(3) - 3) }`,
			diagnostics: errorDiagnostic("Division by zero"),
		},
		{
			name:  "float division by zero is a run-time value",
			input: `fn f(a: Float64) Float64 { a / 0.0 }`,
		},
		{
			name:        "a fit that keeps its value is still constant",
			input:       `let x = Int8::fit(100 + 0) * 2`,
			diagnostics: errorDiagnostic("Constant value 200 overflows Int8"),
		},
		{
			name: "fit wraps a constant argument",
			input: `let a: Uint8 = Uint8::fit(0 - 1)
let b: Int8 = Int8::fit(200 + 100)
let c: Uint8 = Uint8::fit(~0)
let d: Uint64 = Uint64::fit(0 - 1)
let e: Int = Int::fit(Int64::from(1) << 40)`,
		},
	})
}
