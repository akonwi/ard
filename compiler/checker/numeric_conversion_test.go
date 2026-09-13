package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

// TestConversionTierSelection covers the tier rules in ADR 0072: the lossless
// tier compiles without a Maybe, the checked tier yields one, and the wrong
// spelling is rejected with the right suggestion.
func TestConversionTierSelection(t *testing.T) {
	run(t, []test{
		{
			// #500: Float32 -> Float64 widening.
			name: "lossless widening uses from",
			input: `fn widen(value: Float32) Float64 {
  Float64::from(value)
}`,
		},
		{
			// #500: Int64 -> Int checked narrowing.
			name: "checked narrowing uses try",
			input: `fn narrow(value: Int64) Int? {
  Int::try(value)
}`,
		},
		{
			name: "try on a lossless pair is rejected",
			input: `fn widen(value: Float32) Float64? {
  Float64::try(value)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "every Float32 value is exactly representable as Float64"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Float64?, got Void"},
			},
		},
		{
			name: "fit on a lossless pair is rejected",
			input: `fn widen(value: Float32) Float64 {
  Float64::fit(value)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "every Float32 value is exactly representable as Float64"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Float64, got Void"},
			},
		},
		{
			name: "from on a fallible pair is rejected",
			input: `fn narrow(value: Int64) Int {
  Int::from(value)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Int cannot hold every Int64 value"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Void"},
			},
		},
		{
			name: "fit narrows an Int64",
			input: `fn narrow(value: Int64) Int {
  Int::fit(value)
}`,
		},
		{
			// Integer -> float rounds but can never fail, so fit is the only
			// spelling and try is rejected.
			name: "try into a float target is rejected",
			input: `fn widen(value: Int) Float64? {
  Float64::try(value)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "converting Int to Float64 cannot fail"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Float64?, got Void"},
			},
		},
		{
			name: "fit converts an Int to a Float64",
			input: `fn widen(value: Int) Float64 {
  Float64::fit(value)
}`,
		},
		{
			name: "float to int uses try",
			input: `fn truncate(value: Float64) Int? {
  Int::try(value)
}`,
		},
		{
			name: "float to int uses fit",
			input: `fn truncate(value: Float64) Int {
  Int::fit(value)
}`,
		},
	})
}

// TestRuneConversionInvariant pins ADR 0026's invariant under ADR 0072: no
// conversion may manufacture an invalid Rune, so Rune has no `fit`.
func TestRuneConversionInvariant(t *testing.T) {
	run(t, []test{
		{
			name: "Byte widens losslessly into a Rune",
			input: `fn widen(value: Byte) Rune {
  Rune::from(value)
}`,
		},
		{
			name: "an Int must be checked into a Rune",
			input: `fn scalar(value: Int) Rune? {
  Rune::try(value)
}`,
		},
		{
			name: "Rune has no fit",
			input: `fn scalar(value: Int) Rune {
  Rune::fit(value)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Rune cannot hold every Int value"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Rune, got Void"},
			},
		},
		{
			name: "a Rune widens losslessly because it is a valid scalar",
			input: `fn code(value: Rune) Uint32 {
  Uint32::from(value)
}`,
		},
		{
			name: "an Int32 must be checked into a Rune",
			input: `fn scalar(value: Int32) Rune {
  Rune::from(value)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Rune cannot hold every Int32 value"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Rune, got Void"},
			},
		},
	})
}

// TestIdentityConversion covers same-primitive pairs, which stay lossless so
// foreign named scalars keep working (#284).
func TestIdentityConversion(t *testing.T) {
	run(t, []test{
		{
			name: "Int to Int is lossless",
			input: `fn id(value: Int) Int {
  Int::from(value)
}`,
		},
		{
			name: "Byte and Uint8 share a primitive",
			input: `fn id(value: Uint8) Byte {
  Byte::from(value)
}`,
		},
		{
			name: "try on an identity pair is rejected",
			input: `fn id(value: Int) Int? {
  Int::try(value)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "every Int value is exactly representable as Int"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Int?, got Void"},
			},
		},
		{
			name: "a foreign named scalar converts from its underlying primitive",
			input: `use go:time
fn every(ms: Int64) time::Duration {
  time::Duration::from(ms)
}`,
		},
		{
			name: "a foreign named scalar converts to its underlying primitive",
			input: `use go:time
fn count(d: time::Duration) Int64 {
  Int64::from(d)
}`,
		},
	})
}

// TestConversionLiterals pins the literal rule: a literal adopts the target
// and is range-checked, so `from` is the only spelling.
func TestConversionLiterals(t *testing.T) {
	run(t, []test{
		{
			name:  "a literal adopts the target",
			input: `let value: Uint8 = Uint8::from(200)`,
		},
		{
			name:  "an overflowing literal is rejected",
			input: `let value = Uint8::from(300)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Integer literal 300 overflows Uint8"},
			},
		},
		{
			name:  "try does not take a literal",
			input: `let value = Uint8::try(200)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Uint8::try does not take a literal"},
			},
		},
		{
			name:  "fit does not take a literal",
			input: `let value = Uint8::fit(200)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Uint8::fit does not take a literal"},
			},
		},
		{
			// `isNumericLiteralNode` matches a literal or a negated literal, so
			// a constant expression is an ordinary runtime value.
			name: "a constant expression is not a literal",
			input: `fn f() Uint8 {
  Uint8::fit(1 + 2)
}`,
		},
	})
}
