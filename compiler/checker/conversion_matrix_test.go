package checker_test

import (
	"fmt"
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

var conversionTypes = []string{
	"Int", "Int8", "Int16", "Int32", "Int64",
	"Uint", "Uint8", "Uint16", "Uint32", "Uint64", "Uintptr",
	"Byte", "Rune", "Float32", "Float64",
}

func conversionCompiles(t *testing.T, source string, target string, tier string) bool {
	t.Helper()
	src := fmt.Sprintf("fn probe(value: %s) {\n  let result = %s::%s(value)\n}\n", source, target, tier)
	result := parse.Parse([]byte(src), "matrix.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors for %s: %v", src, result.Errors)
	}
	c := checker.New("matrix.ard", result.Program, nil)
	c.Check()
	return !c.HasErrors()
}

func validTiers(t *testing.T, source string, target string) []string {
	t.Helper()
	var valid []string
	for _, tier := range []string{"from", "try", "fit"} {
		if conversionCompiles(t, source, target, tier) {
			valid = append(valid, tier)
		}
	}
	return valid
}

// TestEveryPairIsExpressible ensures every pair has a tier and that lossless
// `from` never coexists with a lossy tier.
func TestEveryPairIsExpressible(t *testing.T) {
	for _, source := range conversionTypes {
		for _, target := range conversionTypes {
			t.Run(source+"_to_"+target, func(t *testing.T) {
				valid := validTiers(t, source, target)
				if len(valid) == 0 {
					t.Fatalf("%s -> %s has no valid spelling", source, target)
				}
				if valid[0] == "from" && len(valid) > 1 {
					t.Errorf("%s -> %s accepts %v; `from` must exclude the lossy tiers",
						source, target, valid)
				}
			})
		}
	}
}

// TestMatrixTierAssignments spot-checks policy-sensitive pairs.
func TestMatrixTierAssignments(t *testing.T) {
	expected := map[string]string{
		// Identity.
		"Int->Int":         "from",
		"Byte->Uint8":      "from",
		"Uint8->Byte":      "from",
		"Float32->Float64": "from",
		"Int64->Int":       "try fit",
		// Platform-sized types are 64-bit as a source, 32-bit as a target.
		"Int->Int64":   "from",
		"Int->Int32":   "try fit",
		"Uint32->Uint": "from",
		"Uint32->Int":  "try fit",
		// A Rune is a valid scalar, so it widens but can never be forged: a
		// Rune target is the one case with `try` and no `fit`.
		"Byte->Rune":    "from",
		"Rune->Int32":   "from",
		"Rune->Uint32":  "from",
		"Rune->Float32": "from",
		"Int32->Rune":   "try",
		"Int->Rune":     "try",
		"Float64->Rune": "try",
		// Integer to float rounds but never fails, so it has no `try`.
		"Int->Float64":   "fit",
		"Int64->Float64": "fit",
		"Int8->Float32":  "from",
		"Int32->Float32": "fit",
		// Float narrowing can overflow; float to int can be fractional.
		"Float64->Float32": "try fit",
		"Float64->Int":     "try fit",
		"Float32->Int":     "try fit",
	}
	for pair, want := range expected {
		parts := strings.Split(pair, "->")
		source, target := parts[0], parts[1]
		t.Run(pair, func(t *testing.T) {
			valid := validTiers(t, source, target)
			if strings.Join(valid, " ") != want {
				t.Errorf("%s accepts %q, want %q", pair, strings.Join(valid, " "), want)
			}
		})
	}
}
