package checker

import (
	"sort"
	"strings"
	"testing"
)

var classNames = map[numericClass]string{
	classInt8: "Int8", classInt16: "Int16", classInt32: "Int32",
	classInt64: "Int64", classInt: "Int", classUint8: "Uint8",
	classUint16: "Uint16", classUint32: "Uint32", classUint64: "Uint64",
	classUint: "Uint", classUintptr: "Uintptr", classRune: "Rune",
	classFloat32: "Float32", classFloat64: "Float64",
}

var allClasses = []numericClass{
	classInt8, classInt16, classInt32, classInt64, classInt,
	classUint8, classUint16, classUint32, classUint64, classUint, classUintptr,
	classRune, classFloat32, classFloat64,
}

// TestLosslessMatrixMatchesADR pins the lossless matrix in ADR 0072. Identity
// pairs are lossless and omitted from the table there, so they are omitted
// here too.
func TestLosslessMatrixMatchesADR(t *testing.T) {
	want := map[numericClass]string{
		classInt8:    "Int16 Int32 Int64 Int Float32 Float64",
		classInt16:   "Int32 Int64 Int Float32 Float64",
		classInt32:   "Int64 Int Float64",
		classInt64:   "",
		classInt:     "Int64",
		classUint8:   "Int16 Int32 Int64 Int Uint16 Uint32 Uint64 Uint Uintptr Rune Float32 Float64",
		classUint16:  "Int32 Int64 Int Uint32 Uint64 Uint Uintptr Float32 Float64",
		classUint32:  "Int64 Uint64 Uint Uintptr Float64",
		classUint64:  "",
		classUint:    "Uint64",
		classUintptr: "Uint64",
		classRune:    "Int32 Int64 Int Uint32 Uint64 Uint Uintptr Float32 Float64",
		classFloat32: "Float64",
		classFloat64: "",
	}

	for _, src := range allClasses {
		var got []string
		for _, dst := range allClasses {
			if src == dst {
				continue // identity, covered separately
			}
			if losslessConversion(src, dst) {
				got = append(got, classNames[dst])
			}
		}
		expected := strings.Fields(want[src])
		sort.Strings(expected)
		sorted := append([]string(nil), got...)
		sort.Strings(sorted)
		if strings.Join(sorted, " ") != strings.Join(expected, " ") {
			t.Errorf("lossless targets for %s:\n  got  %v\n  want %v",
				classNames[src], got, strings.Fields(want[src]))
		}
	}
}

func TestIdentityIsLossless(t *testing.T) {
	for _, class := range allClasses {
		if !losslessConversion(class, class) {
			t.Errorf("%s -> %s should be lossless (identity)", classNames[class], classNames[class])
		}
	}
	// Byte and Uint8 share a class, so both directions are identity.
	if numericClassOf(Byte) != numericClassOf(Uint8) {
		t.Error("Byte and Uint8 should share a conversion class")
	}
	// Rune and Int32 must not, because Rune carries a validity invariant.
	if numericClassOf(Rune) == numericClassOf(Int32) {
		t.Error("Rune and Int32 must not share a conversion class")
	}
}

func TestConversionTiers(t *testing.T) {
	tests := []struct {
		src, dst numericClass
		want     string
	}{
		{classFloat32, classFloat64, "from"}, // #500 widening
		{classInt64, classInt, "try fit"},    // #500 checked narrowing
		{classInt, classInt, "from"},         // identity
		{classInt64, classInt64, "from"},     // identity
		{classInt, classByteAlias(), "try fit"},
		{classInt, classFloat64, "fit"},   // rounds, never fails
		{classInt64, classFloat64, "fit"}, // rounds, never fails
		{classInt8, classFloat32, "from"}, // fits in the mantissa
		{classFloat64, classFloat32, "try fit"},
		{classFloat64, classInt, "try fit"}, // fractional or out of range
		{classUint8, classRune, "from"},     // whole range is valid Unicode
		{classInt, classRune, "try"},        // no fit: cannot forge a Rune
		{classFloat64, classRune, "try"},
		{classRune, classInt32, "from"},
		{classRune, classUint32, "from"},
		{classInt32, classRune, "try"},
		{classUint32, classInt, "try fit"}, // Int may be 32 bits
		{classInt, classInt32, "try fit"},  // Int may be 64 bits
	}
	for _, tt := range tests {
		tiers, ok := conversionTierFor(tt.src, tt.dst)
		if !ok {
			t.Errorf("%s -> %s: not a conversion", classNames[tt.src], classNames[tt.dst])
			continue
		}
		var got []string
		for _, tier := range tiers {
			got = append(got, tier.String())
		}
		if strings.Join(got, " ") != tt.want {
			t.Errorf("%s -> %s: got %q, want %q",
				classNames[tt.src], classNames[tt.dst], strings.Join(got, " "), tt.want)
		}
	}
}

func classByteAlias() numericClass { return classUint8 }
