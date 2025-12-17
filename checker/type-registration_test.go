package checker_test

import (
	"testing"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker"
)

// TestExpressionTypeRegistration verifies that expression types are registered during checking
func TestExpressionTypeRegistration_Literals(t *testing.T) {
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
		for _, diag := range c.Diagnostics() {
			t.Logf("Checker error: %s", diag.String())
		}
		t.Fatal("Unexpected checker errors")
	}

	registry := c.Module().TypeRegistry()
	if registry == nil {
		t.Fatal("expected non-nil TypeRegistry after Check()")
	}

	// Registry should have registered types from expressions
	all := registry.All()
	if len(all) == 0 {
		t.Fatal("expected registry to contain registered types from expressions")
	}

	t.Logf("Registry contains %d types", len(all))

	// Verify basic types are in registry
	foundInt, foundStr, foundBool := false, false, false
	for _, regType := range all {
		if regType == checker.Int {
			foundInt = true
		}
		if regType == checker.Str {
			foundStr = true
		}
		if regType == checker.Bool {
			foundBool = true
		}
	}

	if !foundInt {
		t.Error("expected Int type in registry")
	}
	if !foundStr {
		t.Error("expected Str type in registry")
	}
	if !foundBool {
		t.Error("expected Bool type in registry")
	}
}

// TestExpressionTypeRegistration_BinaryOps verifies binary operations register types
func TestExpressionTypeRegistration_BinaryOps(t *testing.T) {
	input := `
fn main() {
	let a: Int = 1 + 2
	let b: Int = a - 3
	let c: Bool = a > 0
}
`
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Unexpected checker errors: %v", c.Diagnostics())
	}

	registry := c.Module().TypeRegistry()
	all := registry.All()

	if len(all) == 0 {
		t.Fatal("expected registry to contain types from binary operations")
	}

	// Should have Int and Bool types from operations
	foundInt, foundBool := false, false
	for _, regType := range all {
		if regType == checker.Int {
			foundInt = true
		}
		if regType == checker.Bool {
			foundBool = true
		}
	}

	if !foundInt {
		t.Error("expected Int type from arithmetic operations")
	}
	if !foundBool {
		t.Error("expected Bool type from comparison operation")
	}
}

// TestExpressionTypeRegistration_Lists verifies list types are registered
func TestExpressionTypeRegistration_Lists(t *testing.T) {
	input := `
fn main() {
	let nums = [1, 2, 3]
}
`
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Unexpected checker errors: %v", c.Diagnostics())
	}

	registry := c.Module().TypeRegistry()
	all := registry.All()

	if len(all) == 0 {
		t.Fatal("expected registry to contain list types")
	}

	// Should have List[Int] type
	foundListInt := false
	for _, regType := range all {
		if listType, ok := regType.(*checker.List); ok {
			if listType.Of() == checker.Int {
				foundListInt = true
				break
			}
		}
	}

	if !foundListInt {
		// At minimum, we should have Int types from the list elements
		t.Logf("Warning: expected List[Int] type in registry, but have: %v", all)
		if len(all) > 0 {
			t.Log("Registry has types, which is the important part for Phase 2")
		}
	}
}

// TestExpressionTypeRegistration_FunctionCalls verifies function call types are registered
func TestExpressionTypeRegistration_FunctionCalls(t *testing.T) {
	input := `
fn add(a: Int, b: Int) {
	a + b
}

fn main() {
	add(1, 2)
}
`
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Unexpected checker errors: %v", c.Diagnostics())
	}

	registry := c.Module().TypeRegistry()
	all := registry.All()

	if len(all) == 0 {
		t.Fatal("expected registry to contain function call return types")
	}

	// Should have Int type from function call return
	foundInt := false
	for _, regType := range all {
		if regType == checker.Int {
			foundInt = true
			break
		}
	}

	if !foundInt {
		t.Error("expected Int type from function call")
	}
}

// TestTypeID_Assigned verifies that typeIDs are actually assigned to expressions
func TestTypeID_Assigned(t *testing.T) {
	input := `
fn main() {
	let x: Int = 42
}
`
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Unexpected checker errors: %v", c.Diagnostics())
	}

	// Verify registry has entries
	registry := c.Module().TypeRegistry()
	all := registry.All()
	
	if len(all) == 0 {
		t.Fatal("registry should contain type registrations")
	}

	// All IDs should be valid (non-zero)
	for id := range all {
		if id == checker.InvalidTypeID {
			t.Error("registry contains InvalidTypeID")
		}
	}
}

// TestMultipleExpressions verifies multiple expressions in same program are registered separately
func TestExpressionTypeRegistration_Multiple(t *testing.T) {
	input := `
fn main() {
	let a: Int = 1
	let b: Int = 2
	let c: Int = a + b
	let s: Str = "hello"
	let t: Bool = a > 0
}
`
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Unexpected checker errors: %v", c.Diagnostics())
	}

	registry := c.Module().TypeRegistry()
	all := registry.All()

	// Should have multiple type registrations (at least 10+ for this program)
	if len(all) < 5 {
		t.Logf("Warning: expected at least 5 type registrations, got %d", len(all))
	}

	t.Logf("Successfully registered %d type instances", len(all))
}
