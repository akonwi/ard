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

// TestGoTargetConversionMatrixCompiles lowers and runs all 225 pairs.
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
				fmt.Fprintf(&body, "  ok = ok and (%s).is_some()\n", expr)
				continue
			}
			fmt.Fprintf(&body, "  ok = ok and (%s).to_str() != \"\"\n", expr)
		}
	}
	body.WriteString("  ok\n}\n")

	program := lowerParitySource(t, body.String())
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("conversion matrix produced %s, want true", got)
	}
}
