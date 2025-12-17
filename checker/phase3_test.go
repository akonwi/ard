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

// Phase 4 Tests: Registry-Based Type Lookup

// TestPhase4_GetTypeIDMethod verifies that all expressions have GetTypeID() method
func TestPhase4_GetTypeIDMethod(t *testing.T) {
	input := `
fn main() {
	let x: Int = 42
	let y: Str = "hello"
	let z: Bool = true
	let vals = [1, 2, 3]
	let dict = ["key": 10]
	not true
	42 + 10
	"hello" + " world"
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

	// Walk through statements and verify all expressions have valid TypeIDs
	for _, stmt := range c.Module().Program().Statements {
		if stmt.Expr != nil {
			// Skip statement-like nodes that are returned as expressions
			// (FunctionDef, ExternalFunctionDef, etc)
			switch stmt.Expr.(type) {
			case *checker.FunctionDef, *checker.ExternalFunctionDef, *checker.If:
				continue
			}
			
			typeID := stmt.Expr.GetTypeID()
			if typeID == checker.InvalidTypeID {
				t.Errorf("Expression %T has InvalidTypeID", stmt.Expr)
			}
			// Verify the TypeID maps to a registered type
			registry := c.Module().TypeRegistry()
			if registeredType := registry.Lookup(typeID); registeredType == nil {
				t.Errorf("TypeID %d not found in registry for expression %T", typeID, stmt.Expr)
			}
		}
	}

	t.Log("Phase 4: All expressions have valid GetTypeID() implementations")
}

// TestPhase4_LookupTypeHelper verifies the LookupType() helper function
func TestPhase4_LookupTypeHelper(t *testing.T) {
	input := `
fn main() {
	let x: Int = 42
	let y: Str = "hello"
	let z: [Int] = [1, 2, 3]
	x
	y
	z
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

	// Test the LookupType helper on actual expressions
	found := 0
	for _, stmt := range c.Module().Program().Statements {
		if stmt.Expr != nil {
			// LookupType should return the same type as Type()
			lookupResult := c.LookupType(stmt.Expr)
			computedResult := stmt.Expr.Type()
			
			if lookupResult == nil || computedResult == nil {
				continue
			}
			
			// Types should match by string representation (since equal is unexported)
			if lookupResult.String() != computedResult.String() {
				t.Errorf("LookupType mismatch for %T: got %v, expected %v", 
					stmt.Expr, lookupResult.String(), computedResult.String())
			}
			found++
		}
	}

	if found == 0 {
		t.Error("No expressions found to test LookupType")
	}

	t.Logf("Phase 4: LookupType verified for %d expressions", found)
}

// TestPhase4_RegistryHybridiTransition verifies the strangler fig pattern:
// types are registered in parallel and LookupType() can retrieve them
func TestPhase4_RegistryHybridTransition(t *testing.T) {
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
		t.Fatal("Registry should have types from this program")
	}

	// Verify we can look up types for complex expressions
	for _, stmt := range c.Module().Program().Statements {
		if stmt.Expr != nil {
			typeID := stmt.Expr.GetTypeID()
			if typeID != checker.InvalidTypeID {
				lookedUp := registry.Lookup(typeID)
				if lookedUp == nil {
					t.Errorf("TypeID %d cannot be looked up in registry for expr %T", typeID, stmt.Expr)
				}
			}
		}
	}

	t.Logf("Phase 4: Hybrid transition verified with %d types registered", len(allTypes))
}

// TestPhase4_TypeIDPersistence verifies typeIDs are consistent across the program
func TestPhase4_TypeIDPersistence(t *testing.T) {
	input := `
fn main() {
	let a = 42
	let b = a
	let c = a + 1
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

	// All Int expressions should have TypeIDs that point to the Int type in the registry
	registry := c.Module().TypeRegistry()
	intTypeID := checker.InvalidTypeID

	for _, stmt := range c.Module().Program().Statements {
		if stmt.Expr != nil {
			exprType := stmt.Expr.Type()
			if exprType == checker.Int {
				typeID := stmt.Expr.GetTypeID()
				if intTypeID == checker.InvalidTypeID {
					intTypeID = typeID
				} else if typeID != intTypeID {
					// Same types may have different TypeIDs (each expression gets its own)
					// But they should both map to the same Type in the registry
					lookedUp1 := registry.Lookup(intTypeID)
					lookedUp2 := registry.Lookup(typeID)
					if lookedUp1 != lookedUp2 {
						t.Errorf("Different Int expressions have inconsistent registry mappings")
					}
				}
			}
		}
	}

	t.Log("Phase 4: Type ID persistence verified")
}
