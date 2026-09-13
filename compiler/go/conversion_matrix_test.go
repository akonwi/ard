package gotarget

import (
	"fmt"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

var matrixTypes = []string{
	"Int", "Int8", "Int16", "Int32", "Int64",
	"Uint", "Uint8", "Uint16", "Uint32", "Uint64", "Uintptr",
	"Byte", "Rune", "Float32", "Float64",
}

// tierFor reports the spelling to use for a source/target pair, preferring the
// lossless one and falling back to the checked one.
func tierFor(t *testing.T, source string, target string) string {
	t.Helper()
	for _, tier := range []string{"from", "try", "fit"} {
		src := fmt.Sprintf("fn probe(value: %s) {\n  let result = %s::%s(value)\n}\n", source, target, tier)
		result := parse.Parse([]byte(src), "probe.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("parse errors: %v", result.Errors)
		}
		c := checker.New("probe.ard", result.Program, nil)
		c.Check()
		if !c.HasErrors() {
			return tier
		}
	}
	t.Fatalf("%s -> %s has no valid tier", source, target)
	return ""
}

// seedFor produces a runtime value of the given type. Values stay small so
// every conversion in the matrix succeeds at run time.
func seedFor(typeName string) string {
	switch typeName {
	case "Rune":
		return "'A'"
	case "Float32", "Float64":
		return fmt.Sprintf("%s::from(65.0)", typeName)
	default:
		return fmt.Sprintf("%s::from(65)", typeName)
	}
}

// TestGoTargetConversionMatrixCompiles lowers and runs every source/target
// pair in one program. It is the end-to-end guard that the backend emits
// compiling Go for every combination the checker accepts, including the
// platform-sized targets and the Rune invariant.
func TestGoTargetConversionMatrixCompiles(t *testing.T) {
	var body strings.Builder
	body.WriteString("fn main() Bool {\n")
	for _, typeName := range matrixTypes {
		fmt.Fprintf(&body, "  let seed_%s: %s = %s\n", typeName, typeName, seedFor(typeName))
	}
	body.WriteString("  mut ok = true\n")
	for _, source := range matrixTypes {
		for _, target := range matrixTypes {
			tier := tierFor(t, source, target)
			expr := fmt.Sprintf("%s::%s(seed_%s)", target, tier, source)
			if tier == "try" {
				// Every seed is 65, which is representable in every target,
				// so a checked conversion must report some.
				fmt.Fprintf(&body, "  ok = ok and (%s).is_some()\n", expr)
				continue
			}
			// Reading the value is enough to force the conversion to be
			// emitted and type-checked in the generated Go.
			fmt.Fprintf(&body, "  ok = ok and (%s).to_str() != \"\"\n", expr)
		}
	}
	body.WriteString("  ok\n}\n")

	program := lowerParitySource(t, body.String())
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("conversion matrix produced %s, want true", got)
	}
}
