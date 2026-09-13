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

// conversionCompiles reports whether `target::tier(value)` checks cleanly for a
// runtime value of the source type.
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

// validTiers reports which of from/try/fit compile for a source/target pair.
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

// TestEveryPairIsExpressible checks the two structural invariants of ADR 0072
// across the whole matrix:
//
//   - Every pair has at least one spelling, so no conversion is unexpressible.
//   - `from` never coexists with `try` or `fit`. A pair is either lossless,
//     and then `from` is the only spelling, or it is lossy, and then the
//     lossless spelling is unavailable. That mutual exclusion is what makes a
//     lossy conversion announce itself.
//
// A lossy pair may offer both `try` and `fit`: checked versus forced is the
// caller's choice, not the compiler's.
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

// TestMatrixTierAssignments spot-checks the exact spellings for pairs whose
// classification carries a design decision.
func TestMatrixTierAssignments(t *testing.T) {
	expected := map[string]string{
		// Identity keeps foreign named scalars working (#284).
		"Int->Int":    "from",
		"Byte->Uint8": "from",
		"Uint8->Byte": "from",
		// #500.
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
