package parse

import (
	"testing"
)

func TestVariables(t *testing.T) {
	tests := []test{
		{
			name: "Declaring variables",
			input: `
				let name: Str = "Alice"
    		mut age: Int = 30
        mut temp: Float64 = 98.6
      	let is_student: Bool = true`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name:    "name",
						Mutable: false,
						Type:    &StringType{},
						Value: &StrLiteral{
							Value: "Alice",
						},
					},
					&VariableDeclaration{
						Name:    "age",
						Mutable: true,
						Type:    &IntType{},
						Value: &NumLiteral{
							Value: "30",
						},
					},
					&VariableDeclaration{
						Name:    "temp",
						Mutable: true,
						Type:    &FloatType{},
						Value: &NumLiteral{
							Value: "98.6",
						},
					},
					&VariableDeclaration{
						Name:    "is_student",
						Mutable: false,
						Type:    &BooleanType{},
						Value: &BoolLiteral{
							Value: true,
						},
					},
				},
			},
		},
		{
			name: "Reassigning variables",
			input: `
				name = "Bob"
				age =+ 1
				age =- 2
				bob.age = 30`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableAssignment{
						Target:   &Identifier{Name: "name"},
						Operator: Assign,
						Value: &StrLiteral{
							Value: "Bob",
						},
					},
					&VariableAssignment{
						Target:   &Identifier{Name: "age"},
						Operator: Assign,
						Value: &BinaryExpression{
							Operator: Plus,
							Left:     &Identifier{Name: "age"},
							Right: &NumLiteral{
								Value: "1",
							},
						},
					},
					&VariableAssignment{
						Target:   &Identifier{Name: "age"},
						Operator: Assign,
						Value: &BinaryExpression{
							Operator: Minus,
							Left:     &Identifier{Name: "age"},
							Right: &NumLiteral{
								Value: "2",
							},
						},
					},
					&VariableAssignment{
						Target: &InstanceProperty{
							Target:   &Identifier{Name: "bob"},
							Property: Identifier{Name: "age"},
						},
						Operator: Assign,
						Value: &NumLiteral{
							Value: "30",
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}
func TestFunctionTypes(t *testing.T) {
	runTests(t, []test{
		// Function type error cases
		{
			name:     "Missing opening paren in function type",
			input:    "let f: fn Int) String = test",
			wantErrs: []string{"Expected '(' after 'fn' in function type"},
		},
		{
			name:     "Missing closing paren in function type",
			input:    "let f: fn(Int String = test",
			wantErrs: []string{"Expected ')' after function parameters"},
		},
		{
			name:     "Valid function type",
			input:    "let f: fn(Int) String = test",
			wantErrs: []string{},
		},
		{
			name:     "Valid function type with no params",
			input:    "let f: fn() String = test",
			wantErrs: []string{},
		},
		{
			name:     "Valid function type with multiple params",
			input:    "let f: fn(Int, String) Bool = test",
			wantErrs: []string{},
		},
		{
			name:  "Grouped nullable function type",
			input: "let f: (fn(Int) Void)? = test",
			output: Program{
				Statements: []Statement{
					&VariableDeclaration{
						Name: "f",
						Type: &FunctionType{
							Params:   []DeclaredType{&IntType{}},
							Return:   &VoidType{},
							Nullable: true,
						},
						Value: &Identifier{Name: "test"},
					},
				},
			},
		},
		{
			name:     "Function type returning nullable void is rejected",
			input:    "let f: fn(Int) Void? = test",
			wantErrs: []string{"Function type return cannot be Void?; use (fn(...) Void)? for a nullable function"},
		},
		{
			name:     "Empty grouped type is rejected",
			input:    "let f: ()? = test",
			wantErrs: []string{"Expected a type"},
		},
		{
			name:  "Grouped nullable mutable type",
			input: "let f: (mut Int)? = test",
			output: Program{
				Statements: []Statement{
					&VariableDeclaration{
						Name:  "f",
						Type:  &MutableType{Inner: &IntType{}, nullable: true},
						Value: &Identifier{Name: "test"},
					},
				},
			},
		},
		{
			name:  "Grouped mutable value result type",
			input: "let f: (mut File)!Str = test",
			output: Program{
				Statements: []Statement{
					&VariableDeclaration{
						Name: "f",
						Type: &ResultType{
							Val: &MutableType{Inner: &CustomType{Name: "File"}},
							Err: &StringType{},
						},
						Value: &Identifier{Name: "test"},
					},
				},
			},
		},
		{
			name:     "Already nullable grouped type is rejected",
			input:    "let f: (Int?)? = test",
			wantErrs: []string{"Grouped type is already nullable"},
		},
		{
			name:     "Malformed grouped type recovers at closing paren",
			input:    "let f: (Int String) = test",
			wantErrs: []string{"Expected ')' after grouped type"},
		},
		{
			name:     "Malformed grouped type recovers past comma",
			input:    "let f: (Int, String) = test",
			wantErrs: []string{"Expected ')' after grouped type"},
		},
		{
			name:     "Malformed grouped nullable type consumes question mark during recovery",
			input:    "let f: (Int, String)? = test",
			wantErrs: []string{"Expected ')' after grouped type"},
		},
		{
			name:     "Invalid grouped type recovers at closing paren",
			input:    "let f: (@) = test",
			wantErrs: []string{"Expected a type"},
		},
	})
}
func TestGenericCallTypeArgumentDiagnostics(t *testing.T) {
	runTests(t, []test{
		{
			name:     "Invalid generic function type argument keeps type diagnostic",
			input:    "foo<fn(Int) Void?>()",
			wantErrs: []string{"Function type return cannot be Void?; use (fn(...) Void)? for a nullable function"},
		},
		{
			name:     "Missing generic function type argument keeps type diagnostic",
			input:    "foo<()>()",
			wantErrs: []string{"Expected a type"},
		},
		{
			name:     "Empty static generic function type argument is rejected",
			input:    "maybe::none<>()",
			wantErrs: []string{"Expected type argument"},
		},
		{
			name:     "Invalid static generic function type argument keeps type diagnostic",
			input:    "maybe::none<()>()",
			wantErrs: []string{"Expected a type"},
		},
		{
			name:     "Static generic function call missing greater than is diagnosed",
			input:    `json::parse<Int("1")`,
			wantErrs: []string{"Expected '>' after type arguments"},
		},
		{
			name:     "Generic function call with implicit void function type argument missing greater than is diagnosed",
			input:    "foo<fn(Int)()",
			wantErrs: []string{"Expected '>' after type arguments"},
		},
		{
			name:     "Static generic function call with implicit void function type argument missing greater than is diagnosed",
			input:    "maybe::none<fn(Int)()",
			wantErrs: []string{"Expected '>' after type arguments"},
		},
		{
			name:     "Generic function call with implicit void function type argument and arguments missing greater than is diagnosed",
			input:    "foo<fn(Int)(1)",
			wantErrs: []string{"Expected '>' after type arguments"},
		},
		{
			name:     "Static generic function call with implicit void function type argument and arguments missing greater than is diagnosed",
			input:    "maybe::none<fn(Int)(1)",
			wantErrs: []string{"Expected '>' after type arguments"},
		},
	})
}
func TestArrayMapTypes(t *testing.T) {
	runTests(t, []test{
		// Array/Map type error cases
		{
			name:     "Missing closing bracket in map type",
			input:    "let m: [String: Int = test",
			wantErrs: []string{"Expected ']'"},
		},
		{
			name:     "Missing closing bracket in array type",
			input:    "let arr: [String = test",
			wantErrs: []string{"Expected ']'"},
		},
		{
			name:     "Valid array type",
			input:    "let arr: [String] = test",
			wantErrs: []string{},
		},
		{
			name:     "Valid map type",
			input:    "let m: [String: Int] = test",
			wantErrs: []string{},
		},
		{
			name:     "Valid nested array type",
			input:    "let nested: [[String]] = test",
			wantErrs: []string{},
		},
		{
			name:     "Valid complex nested type",
			input:    "let complex: [String: [Int]] = test",
			wantErrs: []string{},
		},
	})
}
func TestGenericTypeParameters(t *testing.T) {
	runTests(t, []test{
		{
			name:     "Variable with single generic type parameter",
			input:    "let fiber: async::Fiber<Void> = test",
			wantErrs: []string{},
		},
		{
			name:     "Variable with single type parameter",
			input:    "let box: Box<Int> = test",
			wantErrs: []string{},
		},
		{
			name:     "Variable with multiple type parameters",
			input:    "let map: Map<String, Int> = test",
			wantErrs: []string{},
		},
		{
			name:     "Missing closing bracket in generic type",
			input:    "let fiber: Fiber<Int = test",
			wantErrs: []string{"Expected '>' to close generic type arguments"},
		},
		{
			name:     "Nested generic types",
			input:    "let nested: Box<Maybe<Int>> = test",
			wantErrs: []string{},
		},
		{
			name:     "Generic type with function type argument",
			input:    "let handler: Handler<fn(Int) Bool> = test",
			wantErrs: []string{},
		},
	})
}

// TestStaticPropertyAccess - basic error recovery implemented
// Comprehensive testing skipped due to complex interaction with assignment parsing
