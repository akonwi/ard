package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

func TestExplicitScalarLiteralTyping(t *testing.T) {
	run(t, []test{
		{
			name: "integer literal contextually types as explicit scalar",
			input: `let a: Int8 = 127
let b: Uint16 = 65535`,
		},
		{
			name:        "integer literal range checked",
			input:       `let a: Int8 = 128`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Integer literal 128 overflows Int8"}},
		},
		{
			name:  "negative literal contextually types as signed scalar",
			input: `let a: Int8 = -1`,
		},
		{
			name:        "negative literal rejected for unsigned scalar",
			input:       `let a: Uint8 = -1`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Integer literal -1 overflows Uint8"}},
		},
		{
			name:  "float literal contextually types as Float32",
			input: `let a: Float32 = 1.5`,
		},
		{
			name:        "float32 literal overflow rejected",
			input:       `let a: Float32 = 3402823466385288598117041834845169254400.0`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Float literal 3402823466385288598117041834845169254400.0 overflows Float32"}},
		},
	})
}

func TestExplicitScalarComparisons(t *testing.T) {
	run(t, []test{
		{
			name: "integer scalar equality and relational comparisons",
			input: `let a: Uint32 = 1
let b: Uint32 = 2
let same = a == b
let less = a < b`,
		},
		{
			name: "float32 equality and relational comparisons",
			input: `let a: Float32 = 1.5
let b: Float32 = 2.5
let same = a == b
let less = a < b`,
		},
	})
}

func TestExplicitScalarArithmetic(t *testing.T) {
	run(t, []test{
		{
			name: "integer scalar arithmetic preserves scalar type",
			input: `fn f() Int16 {
  let a: Int16 = 10
  let b: Int16 = 3
  a + b
}`,
		},
		{
			name: "integer scalar arithmetic",
			input: `let a: Int16 = 10
let b: Int16 = 3
let sum = a + b
let diff = a - b
let product = a * b
let quotient = a / b
let rem = a % b
let neg = -a`,
		},
		{
			name: "unsigned scalar arithmetic but not unary minus",
			input: `let a: Uint32 = 10
let b: Uint32 = 3
let sum = a + b
let rem = a % b`,
		},
		{
			name: "unsigned scalar unary minus rejected",
			input: `let a: Uint32 = 10
let neg = -a`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Only signed numbers can be negated with '-'"}},
		},
		{
			name: "float32 arithmetic preserves scalar type",
			input: `fn f() Float32 {
  let a: Float32 = 10.5
  let b: Float32 = 3.5
  a + b
}`,
		},
		{
			name: "float32 arithmetic and unary minus",
			input: `let a: Float32 = 10.5
let b: Float32 = 3.5
let sum = a + b
let diff = a - b
let product = a * b
let quotient = a / b
let neg = -a`,
		},
	})
}

func TestTargetWidthScalarLiteralChecks(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		target      checker.TargetInfo
		diagnostics []checker.Diagnostic
	}{
		{name: "int32 max accepted", input: `let x: Int = 2147483647`, target: checker.TargetInfo{IntBits: 32}},
		{name: "int32 overflow rejected", input: `let x: Int = 2147483648`, target: checker.TargetInfo{IntBits: 32}, diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Integer literal 2147483648 overflows Int"}}},
		{name: "inferred int32 overflow rejected", input: `let x = 2147483648`, target: checker.TargetInfo{IntBits: 32}, diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Integer literal 2147483648 overflows Int"}}},
		{name: "uint32 max accepted", input: `let x: Uint = 4294967295`, target: checker.TargetInfo{IntBits: 32, UintBits: 32}},
		{name: "uint32 overflow rejected", input: `let x: Uint = 4294967296`, target: checker.TargetInfo{IntBits: 32, UintBits: 32}, diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Integer literal 4294967296 overflows Uint"}}},
		{name: "uintptr32 overflow rejected", input: `let x: Uintptr = 4294967296`, target: checker.TargetInfo{IntBits: 64, UintptrBits: 32}, diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Integer literal 4294967296 overflows Uintptr"}}},
		{name: "uint64 max accepted", input: `let x: Uint = 18446744073709551615`, target: checker.TargetInfo{IntBits: 64, UintBits: 64}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse error: %s", result.Errors[0].Message)
			}
			c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{Target: tt.target})
			c.Check()
			if len(tt.diagnostics) == 0 && len(c.Diagnostics()) == 0 {
				return
			}
			if diff := cmp.Diff(tt.diagnostics, c.Diagnostics(), compareOptions); diff != "" {
				t.Fatalf("Diagnostics mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGoPrimitiveScalarFunctionSignaturesUseExactArdScalars(t *testing.T) {
	run(t, []test{
		{
			name: "float32 parameter and uint32 return",
			input: `use go:math
fn bits() Uint32 {
  math::Float32bits(1.5)
}`,
		},
	})
}
