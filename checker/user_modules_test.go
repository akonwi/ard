package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker"
)

func TestUserModulePathResolution(t *testing.T) {
	// Create a temporary project for testing
	tempDir, err := os.MkdirTemp("", "ard_user_imports_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "test_project"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a simple module file
	moduleContent := `pub fn helper() Int { 42 }`
	err = os.WriteFile(filepath.Join(tempDir, "utils.ard"), []byte(moduleContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create nested module
	mathDir := filepath.Join(tempDir, "math")
	err = os.MkdirAll(mathDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	nestedContent := `pub fn add(a: Int, b: Int) Int { a + b }`
	err = os.WriteFile(filepath.Join(mathDir, "operations.ard"), []byte(nestedContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test project info
	project := resolver.GetProjectInfo()
	if project.ProjectName != "test_project" {
		t.Errorf("Expected project name 'test_project', got '%s'", project.ProjectName)
	}

	// Test simple import resolution
	filePath, err := resolver.ResolveImportPath("test_project/utils")
	if err != nil {
		t.Fatalf("Failed to resolve import: %v", err)
	}

	expectedPath := filepath.Join(tempDir, "utils.ard")
	if filePath != expectedPath {
		t.Errorf("Expected path '%s', got '%s'", expectedPath, filePath)
	}

	// Test nested import resolution
	filePath, err = resolver.ResolveImportPath("test_project/math/operations")
	if err != nil {
		t.Fatalf("Failed to resolve nested import: %v", err)
	}

	expectedPath = filepath.Join(tempDir, "math", "operations.ard")
	if filePath != expectedPath {
		t.Errorf("Expected nested path '%s', got '%s'", expectedPath, filePath)
	}
}

func TestUserModuleImports(t *testing.T) {
	// Create a temporary project for testing
	tempDir, err := os.MkdirTemp("", "ard_checker_integration_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "my_calculator"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a simple module file
	moduleContent := `pub fn helper() Int { 42 }`
	err = os.WriteFile(filepath.Join(tempDir, "utils.ard"), []byte(moduleContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test checker integration with user module import
	input := `use my_calculator/utils
fn main() Int {
    utils::helper()
}`
	astTree, err := ast.Parse([]byte(input), "main.ard")
	if err != nil {
		t.Fatal(err)
	}

	module, diagnostics := checker.Check(astTree, resolver, "main.ard")
	if len(diagnostics) > 0 {
		t.Fatalf("Unexpected diagnostics: %v", diagnostics)
	}
	program := module.Program()

	// Should have imported the utils module
	if len(program.Imports) != 1 {
		t.Errorf("Expected 1 import, got %d", len(program.Imports))
	}

	// Should be able to access the utils module
	utilsModule, ok := program.Imports["my_calculator/utils"]
	if !ok {
		t.Fatal("Expected 'utils' module to be imported")
	}

	// Test that the module provides the public function
	if userMod, ok := utilsModule.(*checker.UserModule); ok {
		helperFunc := userMod.Get("helper")
		if helperFunc == nil {
			t.Error("Expected to find 'helper' function in utils module")
		}
	} else {
		t.Error("Expected utils module to be a UserModule")
	}
}

func TestUserModuleSymbolResolution(t *testing.T) {
	// Create temporary directory structure
	tempDir, err := os.MkdirTemp("", "ard_symbol_resolution_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create project structure
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"test_project\""), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create the math module with public and private functions
	mathContent := `pub fn add(a: Int, b: Int) Int {
    a + b
}

pub fn multiply(x: Int, y: Int) Int {
    x * y
}

fn private_divide(a: Int, b: Int) Int {
    a / b
}`
	err = os.WriteFile(filepath.Join(tempDir, "math.ard"), []byte(mathContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a source file that uses :: syntax to call functions from the imported module
	mainContent := `use test_project/math
fn main() Int {
    let sum: Int = math::add(5, 3)
    let product: Int = math::multiply(2, 4)
    sum + product
}`

	astTree, err := ast.Parse([]byte(mainContent), "main.ard")
	if err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	module, diagnostics := checker.Check(astTree, resolver, "main.ard")
	if len(diagnostics) > 0 {
		t.Fatalf("Unexpected diagnostics: %v", diagnostics)
	}
	program := module.Program()

	// Should have imported the math module
	if len(program.Imports) != 1 {
		t.Errorf("Expected 1 import, got %d", len(program.Imports))
	}

	// Should be able to access the math module
	mathModule, ok := program.Imports["test_project/math"]
	if !ok {
		t.Fatal("Expected 'math' module to be imported")
	}

	// Test that public functions are accessible
	if userMod, ok := mathModule.(*checker.UserModule); ok {
		addFunc := userMod.Get("add")
		if addFunc == nil {
			t.Error("Expected to find 'add' function in math module")
		}

		multiplyFunc := userMod.Get("multiply")
		if multiplyFunc == nil {
			t.Error("Expected to find 'multiply' function in math module")
		}

		// Test that private functions are not accessible
		privateFunc := userMod.Get("private_divide")
		if privateFunc != nil {
			t.Error("Expected private function to not be accessible")
		}
	} else {
		t.Error("Expected math module to be a UserModule")
	}
}

func TestUserModulePrivateAccessError(t *testing.T) {
	// Create temporary directory structure
	tempDir, err := os.MkdirTemp("", "ard_private_access_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create project structure
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"test_project\""), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a module with only private functions
	utilsContent := `fn private_helper() Int {
    42
}`
	err = os.WriteFile(filepath.Join(tempDir, "utils.ard"), []byte(utilsContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Try to access the private function - should fail
	mainContent := `use test_project/utils
fn main() Int {
    utils::private_helper()
}`

	astTree, err := ast.Parse([]byte(mainContent), "main.ard")
	if err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	_, diagnostics := checker.Check(astTree, resolver, "main.ard")
	if len(diagnostics) == 0 {
		t.Error("Expected error when accessing private function")
	}

	// Should contain "Undefined" error for the private function
	found := false
	for _, diag := range diagnostics {
		if strings.Contains(diag.Message, "Undefined: utils::private_helper") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'Undefined: utils::private_helper' error, got: %v", diagnostics)
	}
}

func TestUserModuleCaching(t *testing.T) {
	// Create temporary directory structure
	tempDir, err := os.MkdirTemp("", "ard_caching_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create project structure
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"test_project\""), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create the shared module
	sharedContent := `pub fn shared_function() Int {
    100
}`
	err = os.WriteFile(filepath.Join(tempDir, "shared.ard"), []byte(sharedContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// First import of the shared module
	content1 := `use test_project/shared
fn func1() Int {
    shared::shared_function()
}`

	astTree1, err := ast.Parse([]byte(content1), "main1.ard")
	if err != nil {
		t.Fatal(err)
	}

	module1, diagnostics1 := checker.Check(astTree1, resolver, "main1.ard")
	if len(diagnostics1) > 0 {
		t.Fatalf("Unexpected diagnostics in first check: %v", diagnostics1)
	}
	program1 := module1.Program()

	// Second import of the same module - should use cache
	content2 := `use test_project/shared
fn func2() Int {
    shared::shared_function() + 50
}`

	astTree2, err := ast.Parse([]byte(content2), "main2.ard")
	if err != nil {
		t.Fatal(err)
	}

	module2, diagnostics2 := checker.Check(astTree2, resolver, "main2.ard")
	if len(diagnostics2) > 0 {
		t.Fatalf("Unexpected diagnostics in second check: %v", diagnostics2)
	}
	program2 := module2.Program()

	// Both should have the shared module imported
	if len(program1.Imports) != 1 || len(program2.Imports) != 1 {
		t.Error("Expected both programs to have 1 import")
	}

	// The module instances should be the same (cached)
	importedModule1 := program1.Imports["shared"]
	importedModule2 := program2.Imports["shared"]

	if importedModule1 != importedModule2 {
		t.Error("Expected modules to be the same instance (cached)")
	}
}

func TestUserModuleErrors(t *testing.T) {
	// Create a temporary project for testing
	tempDir, err := os.MkdirTemp("", "ard_error_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "error_project"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		input       string
		expectError string
	}{
		{
			name:        "wrong project name",
			input:       `use other_project/utils`,
			expectError: "does not match project name",
		},
		{
			name:        "missing module file",
			input:       `use error_project/nonexistent`,
			expectError: "module file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			astTree, err := ast.Parse([]byte(tt.input), "main.ard")
			if err != nil {
				t.Fatal(err)
			}

			_, diagnostics := checker.Check(astTree, resolver, "main.ard")
			if len(diagnostics) == 0 {
				t.Error("Expected error but got none")
				return
			}

			found := false
			for _, diag := range diagnostics {
				if strings.Contains(diag.Message, tt.expectError) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected error containing '%s', got: %v", tt.expectError, diagnostics)
			}
		})
	}
}

func TestModuleResolverWithoutArdToml(t *testing.T) {
	// Create a temporary directory without ard.toml
	tempDir, err := os.MkdirTemp("", "fallback_project_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a module file
	moduleContent := `pub fn helper() -> Int { 42 }`
	err = os.WriteFile(filepath.Join(tempDir, "utils.ard"), []byte(moduleContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatalf("Failed to create module resolver: %v", err)
	}

	project := resolver.GetProjectInfo()
	expectedName := filepath.Base(tempDir)
	if project.ProjectName != expectedName {
		t.Errorf("Expected project name '%s', got '%s'", expectedName, project.ProjectName)
	}

	// Test import with fallback project name
	importPath := expectedName + "/utils"
	filePath, err := resolver.ResolveImportPath(importPath)
	if err != nil {
		t.Fatalf("Failed to resolve import with fallback name: %v", err)
	}

	expectedPath := filepath.Join(tempDir, "utils.ard")
	if filePath != expectedPath {
		t.Errorf("Expected path '%s', got '%s'", expectedPath, filePath)
	}
}

func TestLoadModule(t *testing.T) {
	// Create a temporary project for testing
	tempDir, err := os.MkdirTemp("", "ard_load_module_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "load_test"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a valid module file
	moduleContent := `pub fn add(a: Int, b: Int) Int {
	a + b
}

fn private_helper() Str {
	"helper"
}`
	err = os.WriteFile(filepath.Join(tempDir, "math.ard"), []byte(moduleContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test loading the module
	program, err := resolver.LoadModule("load_test/math")
	if err != nil {
		t.Fatalf("Failed to load module: %v", err)
	}

	// Verify the program was parsed correctly
	if program == nil {
		t.Fatal("Expected parsed program, got nil")
	}

	// Should have 2 statements (pub function and private function)
	if len(program.Statements) != 2 {
		t.Errorf("Expected 2 statements, got %d", len(program.Statements))
	}

	// No imports in this simple module
	if len(program.Imports) != 0 {
		t.Errorf("Expected 0 imports, got %d", len(program.Imports))
	}
}

func TestLoadModuleErrors(t *testing.T) {
	// Create a temporary project for testing
	tempDir, err := os.MkdirTemp("", "ard_load_error_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "error_test"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a module with invalid syntax
	invalidContent := `pub fn broken( Int {  // missing parameter name
	42
}`
	err = os.WriteFile(filepath.Join(tempDir, "broken.ard"), []byte(invalidContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		importPath  string
		expectError string
	}{
		{
			name:        "non-existent module",
			importPath:  "error_test/nonexistent",
			expectError: "module file not found",
		},
		{
			name:        "invalid syntax",
			importPath:  "error_test/broken",
			expectError: "failed to parse module",
		},
		{
			name:        "wrong project name",
			importPath:  "wrong_project/math",
			expectError: "does not match project name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.LoadModule(tt.importPath)
			if err == nil {
				t.Error("Expected error but got none")
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("Expected error containing '%s', got: %v", tt.expectError, err)
			}
		})
	}
}

func TestModuleAST_Caching(t *testing.T) {
	// Create a temporary project for testing
	tempDir, err := os.MkdirTemp("", "ard_caching_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "cache_test"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a module file
	moduleContent := `pub fn cached_function() Int {
	42
}`
	err = os.WriteFile(filepath.Join(tempDir, "cached.ard"), []byte(moduleContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Load module first time
	program1, err := resolver.LoadModule("cache_test/cached")
	if err != nil {
		t.Fatalf("Failed to load module first time: %v", err)
	}

	// Load module second time (should come from cache)
	program2, err := resolver.LoadModule("cache_test/cached")
	if err != nil {
		t.Fatalf("Failed to load module second time: %v", err)
	}

	// Both should be the exact same pointer (cached)
	if program1 != program2 {
		t.Error("Expected cached AST to return same pointer, but got different pointers")
	}

	// Verify the content is correct
	if len(program1.Statements) != 1 {
		t.Errorf("Expected 1 statement, got %d", len(program1.Statements))
	}

	if len(program1.Imports) != 0 {
		t.Errorf("Expected 0 imports, got %d", len(program1.Imports))
	}

	// Test multiple calls return same pointer
	program3, err := resolver.LoadModule("cache_test/cached")
	if err != nil {
		t.Fatalf("Failed to load module third time: %v", err)
	}

	if program3 != program1 {
		t.Error("Expected third call to also return cached pointer")
	}
}

func TestCircularDependencyDetection(t *testing.T) {
	// Create a temporary project for testing
	tempDir, err := os.MkdirTemp("", "ard_circular_dep_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "circular_test"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module A that imports B
	moduleA := `use circular_test/module_b

pub fn func_a() Int {
	42
}`
	err = os.WriteFile(filepath.Join(tempDir, "module_a.ard"), []byte(moduleA), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module B that imports A (circular dependency)
	moduleB := `use circular_test/module_a

pub fn func_b() Int {
	24
}`
	err = os.WriteFile(filepath.Join(tempDir, "module_b.ard"), []byte(moduleB), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Try to load module A with dependencies, which should detect circular dependency
	_, err = resolver.LoadModuleWithDependencies("circular_test/module_a")
	if err == nil {
		t.Error("Expected circular dependency error, but got none")
		return
	}

	if !strings.Contains(err.Error(), "circular dependency detected") {
		t.Errorf("Expected circular dependency error, got: %v", err)
	}

	// Error message should show the dependency chain
	if !strings.Contains(err.Error(), "->") {
		t.Errorf("Expected dependency chain in error message, got: %v", err)
	}
}

func TestComplexCircularDependency(t *testing.T) {
	// Test A -> B -> C -> A circular dependency
	tempDir, err := os.MkdirTemp("", "ard_complex_circular_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "complex_circular"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create A -> B -> C -> A chain
	modules := map[string]string{
		"module_a": `use complex_circular/module_b
pub fn func_a() Int { 1 }`,
		"module_b": `use complex_circular/module_c
pub fn func_b() Int { 2 }`,
		"module_c": `use complex_circular/module_a
pub fn func_c() Int { 3 }`,
	}

	for name, content := range modules {
		err = os.WriteFile(filepath.Join(tempDir, name+".ard"), []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Try to load module A, should detect circular dependency
	_, err = resolver.LoadModuleWithDependencies("complex_circular/module_a")
	if err == nil {
		t.Error("Expected circular dependency error, but got none")
		return
	}

	if !strings.Contains(err.Error(), "circular dependency detected") {
		t.Errorf("Expected circular dependency error, got: %v", err)
	}
}

func TestNonCircularDependencies(t *testing.T) {
	// Test that valid dependency chains work fine
	tempDir, err := os.MkdirTemp("", "ard_valid_deps_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ard.toml
	tomlContent := `name = "valid_deps"`
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create valid dependency chain: A -> B -> C (no cycles)
	modules := map[string]string{
		"module_a": `use valid_deps/module_b
pub fn func_a() Int { 1 }`,
		"module_b": `use valid_deps/module_c
pub fn func_b() Int { 2 }`,
		"module_c": `pub fn func_c() Int { 3 }`, // No imports
	}

	for name, content := range modules {
		err = os.WriteFile(filepath.Join(tempDir, name+".ard"), []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create module resolver
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Load module A, should work fine
	program, err := resolver.LoadModuleWithDependencies("valid_deps/module_a")
	if err != nil {
		t.Fatalf("Expected valid dependency chain to work, got error: %v", err)
	}

	if program == nil {
		t.Fatal("Expected parsed program, got nil")
	}

	// Should have 1 import (module_b)
	if len(program.Imports) != 1 {
		t.Errorf("Expected 1 import, got %d", len(program.Imports))
	}
}

func TestSymbolExtraction(t *testing.T) {
	// Create test module with public and private symbols
	moduleContent := `
pub fn public_function() Int {
    42
}

fn private_function() Int {
    24
}

pub struct PublicStruct {
    field: Int
}

struct PrivateStruct {
    field: Str
}
`

	// Parse and check the module
	astTree, err := ast.Parse([]byte(moduleContent), "main.ard")
	if err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(".")
	if err != nil {
		t.Fatal(err)
	}

	module, diagnostics := checker.Check(astTree, resolver, "main.ard")
	if len(diagnostics) > 0 {
		t.Fatalf("Unexpected diagnostics: %v", diagnostics)
	}

	// Cast to UserModule for testing
	userModule, ok := module.(*checker.UserModule)
	if !ok {
		t.Fatal("Expected UserModule")
	}

	// Test public symbol access
	publicFunc := userModule.Get("public_function")
	if publicFunc == nil {
		t.Error("Expected to find public_function")
	}
	if funcDef, ok := publicFunc.(*checker.FunctionDef); ok {
		if !funcDef.Public {
			t.Error("Expected public_function to have Public=true")
		}
	} else {
		t.Error("Expected public_function to be a FunctionDef")
	}

	publicStruct := userModule.Get("PublicStruct")
	if publicStruct == nil {
		t.Error("Expected to find PublicStruct")
	}
	if structDef, ok := publicStruct.(*checker.StructDef); ok {
		if !structDef.Public {
			t.Error("Expected PublicStruct to have Public=true")
		}
	} else {
		t.Error("Expected PublicStruct to be a StructDef")
	}

	// Test private symbol access (should return nil)
	privateFunc := userModule.Get("private_function")
	if privateFunc != nil {
		t.Error("Expected private_function to be nil (not accessible)")
	}

	privateStruct := userModule.Get("PrivateStruct")
	if privateStruct != nil {
		t.Error("Expected PrivateStruct to be nil (not accessible)")
	}

	// Test non-existent symbol
	nonExistent := userModule.Get("nonexistent")
	if nonExistent != nil {
		t.Error("Expected nonexistent symbol to be nil")
	}
}
