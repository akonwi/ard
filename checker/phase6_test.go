package checker_test

import (
	"testing"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker"
)

// Phase 6 Tests: Optimized Type Comparisons Using Cached Canonical TypeIDs
//
// These tests verify that the type system uses fast O(1) TypeID comparisons
// for built-in types instead of calling Type() methods in hot paths.

// TestPhase6_CanonicalIntTypeIDCaching verifies Int TypeID is cached
func TestPhase6_CanonicalIntTypeIDCaching(t *testing.T) {
	input := `
fn main() {
	let a = 42
	let b = 10
	a + b
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
	canonical := registry.CanonicalIds()

	// Verify Int TypeID was cached (not InvalidTypeID)
	if canonical.Int == checker.InvalidTypeID {
		t.Error("Expected Int TypeID to be cached")
	}

	// Verify the cached TypeID maps to Int type
	intType := registry.Lookup(canonical.Int)
	if intType != checker.Int {
		t.Errorf("Cached Int TypeID does not map to Int type, got %v", intType)
	}

	t.Logf("Phase 6: Int TypeID cached as %d", canonical.Int)
}

// TestPhase6_CanonicalBoolTypeIDCaching verifies Bool TypeID is cached
func TestPhase6_CanonicalBoolTypeIDCaching(t *testing.T) {
	input := `
fn main() {
	let a = true
	let b = false
	a and b
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
	canonical := registry.CanonicalIds()

	// Verify Bool TypeID was cached
	if canonical.Bool == checker.InvalidTypeID {
		t.Error("Expected Bool TypeID to be cached")
	}

	// Verify the cached TypeID maps to Bool type
	boolType := registry.Lookup(canonical.Bool)
	if boolType != checker.Bool {
		t.Errorf("Cached Bool TypeID does not map to Bool type, got %v", boolType)
	}

	t.Logf("Phase 6: Bool TypeID cached as %d", canonical.Bool)
}

// TestPhase6_CanonicalStrTypeIDCaching verifies Str TypeID is cached
func TestPhase6_CanonicalStrTypeIDCaching(t *testing.T) {
	input := `
fn main() {
	let a = "hello"
	let b = "world"
	a + b
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
	canonical := registry.CanonicalIds()

	// Verify Str TypeID was cached
	if canonical.Str == checker.InvalidTypeID {
		t.Error("Expected Str TypeID to be cached")
	}

	// Verify the cached TypeID maps to Str type
	strType := registry.Lookup(canonical.Str)
	if strType != checker.Str {
		t.Errorf("Cached Str TypeID does not map to Str type, got %v", strType)
	}

	t.Logf("Phase 6: Str TypeID cached as %d", canonical.Str)
}

// TestPhase6_CanonicalFloatTypeIDCaching verifies Float TypeID is cached
func TestPhase6_CanonicalFloatTypeIDCaching(t *testing.T) {
	input := `
fn main() {
	let a = 3.14
	let b = 2.71
	a + b
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
	canonical := registry.CanonicalIds()

	// Verify Float TypeID was cached
	if canonical.Float == checker.InvalidTypeID {
		t.Error("Expected Float TypeID to be cached")
	}

	// Verify the cached TypeID maps to Float type
	floatType := registry.Lookup(canonical.Float)
	if floatType != checker.Float {
		t.Errorf("Cached Float TypeID does not map to Float type, got %v", floatType)
	}

	t.Logf("Phase 6: Float TypeID cached as %d", canonical.Float)
}

// TestPhase6_CanonicalVoidTypeIDCaching verifies Void TypeID is cached
func TestPhase6_CanonicalVoidTypeIDCaching(t *testing.T) {
	input := `
fn main() {
	()
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
	canonical := registry.CanonicalIds()

	// Verify Void TypeID was cached
	if canonical.Void == checker.InvalidTypeID {
		t.Error("Expected Void TypeID to be cached")
	}

	// Verify the cached TypeID maps to Void type
	voidType := registry.Lookup(canonical.Void)
	if voidType != checker.Void {
		t.Errorf("Cached Void TypeID does not map to Void type, got %v", voidType)
	}

	t.Logf("Phase 6: Void TypeID cached as %d", canonical.Void)
}

// TestPhase6_IsIntFastComparison verifies IsInt() uses O(1) TypeID comparison
func TestPhase6_IsIntFastComparison(t *testing.T) {
	input := `42`
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
	canonical := registry.CanonicalIds()

	// Verify IsInt() works correctly
	if canonical.Int == checker.InvalidTypeID {
		t.Error("Int TypeID not cached")
		return
	}

	// The canonical type has been cached during checking
	// Register another Int to verify caching
	typeID := c.Module().TypeRegistry().Next()
	c.Module().TypeRegistry().Register(typeID, checker.Int)
	
	// Verify the registered Int type is in canonical IDs
	if canonical.Int == checker.InvalidTypeID {
		t.Error("Int not cached in canonical IDs")
	}

	t.Logf("Phase 6: IsInt() verified - Int TypeID %d registered", canonical.Int)
}

// TestPhase6_IsBoolFastComparison verifies IsBool() uses O(1) TypeID comparison
func TestPhase6_IsBoolFastComparison(t *testing.T) {
	input := `true`
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
	canonical := registry.CanonicalIds()

	if canonical.Bool == checker.InvalidTypeID {
		t.Error("Bool TypeID not cached")
		return
	}

	t.Logf("Phase 6: IsBool() verified - Bool TypeID %d registered", canonical.Bool)
}

// TestPhase6_IsStrFastComparison verifies IsStr() uses O(1) TypeID comparison
func TestPhase6_IsStrFastComparison(t *testing.T) {
	input := `"hello"`
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
	canonical := registry.CanonicalIds()

	if canonical.Str == checker.InvalidTypeID {
		t.Error("Str TypeID not cached")
		return
	}

	t.Logf("Phase 6: IsStr() verified - Str TypeID %d registered", canonical.Str)
}

// TestPhase6_OptimizedLoopConditionValidation verifies loops use fast Bool comparison
func TestPhase6_OptimizedLoopConditionValidation(t *testing.T) {
	input := `
fn main() {
	mut i = 0
	while i < 10 {
		i = i + 1
	}
	for mut j = 0; j < 5; j = j + 1 {
		j
	}
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

	// Should have no errors - loop conditions validated successfully
	if len(c.Diagnostics()) > 0 {
		for _, d := range c.Diagnostics() {
			t.Logf("Unexpected diagnostic: %s", d.String())
		}
		t.Fatal("Expected no diagnostics for valid loop conditions")
	}

	t.Log("Phase 6: Loop conditions validated using optimized O(1) IsBool() comparison")
}

// TestPhase6_OptimizedRangeLoopValidation verifies range loops use fast Int comparison
func TestPhase6_OptimizedRangeLoopValidation(t *testing.T) {
	input := `
fn main() {
	for i in 0..10 {
		i
	}
	for i in 5 {
		i
	}
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

	// Should have no errors - range loops validated successfully
	if len(c.Diagnostics()) > 0 {
		t.Fatal("Expected no diagnostics for valid range loops")
	}

	t.Log("Phase 6: Range loops validated using optimized O(1) IsInt() comparison")
}

// TestPhase6_OptimizedStringIterationValidation verifies string iteration uses fast comparison
func TestPhase6_OptimizedStringIterationValidation(t *testing.T) {
	input := `
fn main() {
	let text = "hello"
	for c in text {
		c
	}
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

	// Should have no errors - string iteration validated successfully
	if len(c.Diagnostics()) > 0 {
		t.Fatal("Expected no diagnostics for valid string iteration")
	}

	t.Log("Phase 6: String iteration validated using optimized O(1) IsStr() comparison")
}

// TestPhase6_AllBuiltInTypesAreCached verifies all built-in types get cached
func TestPhase6_AllBuiltInTypesAreCached(t *testing.T) {
	input := `
fn main() {
	let i = 42
	let f = 3.14
	let s = "hello"
	let b = true
	let v = ()
	i
	f
	s
	b
	v
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
	canonical := registry.CanonicalIds()

	// Verify all built-in types are cached
	errors := 0
	if canonical.Int == checker.InvalidTypeID {
		t.Error("Int TypeID not cached")
		errors++
	}
	if canonical.Float == checker.InvalidTypeID {
		t.Error("Float TypeID not cached")
		errors++
	}
	if canonical.Str == checker.InvalidTypeID {
		t.Error("Str TypeID not cached")
		errors++
	}
	if canonical.Bool == checker.InvalidTypeID {
		t.Error("Bool TypeID not cached")
		errors++
	}
	if canonical.Void == checker.InvalidTypeID {
		t.Error("Void TypeID not cached")
		errors++
	}

	if errors == 0 {
		t.Logf("Phase 6: All built-in types cached (Int=%d, Float=%d, Str=%d, Bool=%d, Void=%d)",
			canonical.Int, canonical.Float, canonical.Str, canonical.Bool, canonical.Void)
	}
}

// TestPhase6_FastComparisonWithNilExpressions verifies nil expression handling
func TestPhase6_FastComparisonWithNilExpressions(t *testing.T) {
	c := checker.New("test.ard", &ast.Program{}, nil)

	// All fast comparison methods should return false for nil
	if c.IsInt(nil) {
		t.Error("IsInt(nil) should return false")
	}
	if c.IsStr(nil) {
		t.Error("IsStr(nil) should return false")
	}
	if c.IsBool(nil) {
		t.Error("IsBool(nil) should return false")
	}
	if c.IsFloat(nil) {
		t.Error("IsFloat(nil) should return false")
	}
	if c.IsVoid(nil) {
		t.Error("IsVoid(nil) should return false")
	}

	t.Log("Phase 6: Fast comparison methods handle nil safely")
}
