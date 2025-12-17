package checker_test

import (
	"testing"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker"
)

// TestPhase3_RegistryContainsTypes verifies that the type registry contains type
// registrations from checking a program. This test validates that Phase 2
// (wiring up type registration) is working correctly.
func TestPhase3_RegistryContainsTypes(t *testing.T) {
	input := `
fn main() {
	let x: Int = 42
	let y: Str = "hello"
	let z: Bool = true
}
`
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Checker errors: %v", c.Diagnostics())
	}

	registry := c.Module().TypeRegistry()
	allTypes := registry.All()

	if len(allTypes) == 0 {
		t.Fatal("Registry should contain registered types after checking")
	}

	t.Logf("Phase 3: Registry contains %d registered types", len(allTypes))

	// Verify registry has basic types
	foundInt := false
	foundStr := false
	foundBool := false

	for _, typ := range allTypes {
		if typ == checker.Int {
			foundInt = true
		}
		if typ == checker.Str {
			foundStr = true
		}
		if typ == checker.Bool {
			foundBool = true
		}
	}

	if !foundInt {
		t.Error("Expected Int type in registry")
	}
	if !foundStr {
		t.Error("Expected Str type in registry")
	}
	if !foundBool {
		t.Error("Expected Bool type in registry")
	}
}

// TestPhase3_ComplexProgramRegistration verifies registry registration for more complex programs
func TestPhase3_ComplexProgramRegistration(t *testing.T) {
	input := `
struct Point {
	x: Int
	y: Int
}

fn distance(p: Point) {
	let dx = p.x
	let dy = p.y
	dx + dy
}

fn main() {
	let p = Point { x: 10, y: 20 }
	distance(p)
}
`
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		for _, d := range c.Diagnostics() {
			t.Logf("Checker error: %s", d.String())
		}
		t.Fatal("Checker errors")
	}

	registry := c.Module().TypeRegistry()
	allTypes := registry.All()

	if len(allTypes) == 0 {
		t.Fatal("Registry should contain types from complex program")
	}

	t.Logf("Phase 3: Complex program registered %d types", len(allTypes))

	// Verify we have Int type (used in arithmetic)
	foundInt := false
	for _, typ := range allTypes {
		if typ == checker.Int {
			foundInt = true
			break
		}
	}

	if !foundInt {
		t.Error("Expected Int type in registry for complex program")
	}
}

// TestPhase3_RegistryLookup verifies that types can be looked up from the registry
func TestPhase3_RegistryLookup(t *testing.T) {
	input := `
fn main() {
	let vals = [1, 2, 3]
}
`
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Checker errors: %v", c.Diagnostics())
	}

	registry := c.Module().TypeRegistry()
	allTypes := registry.All()

	// Each TypeID in the registry should return a non-nil type on Lookup
	for id := range allTypes {
		lookedUp := registry.Lookup(id)
		if lookedUp == nil {
			t.Errorf("TypeID %d registered but Lookup returned nil", id)
		}
	}

	t.Logf("Phase 3: Successfully looked up all %d types from registry", len(allTypes))
}
