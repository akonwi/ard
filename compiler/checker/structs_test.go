package checker_test

import (
	"fmt"
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestStructs(t *testing.T) {
	personStructInput := strings.Join([]string{
		"struct Person {",
		"  name: Str,",
		"  age: Int,",
		"  employed: Bool",
		"}",
	}, "\n")
	run(t, []test{
		{
			name:  "Valid struct definition",
			input: personStructInput,
			output: &checker.Program{
				Statements: []checker.Statement{},
			},
		},
		{
			name: "A struct cannot have duplicate field names",
			input: strings.Join([]string{
				"struct Rect {",
				"  height: Str,",
				"  height: Int",
				"}",
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Duplicate field: height"},
			},
		},
		{
			name: "Using a struct",
			input: personStructInput + "\n" +
				`let alice = Person{ name: "Alice", age: 30, employed: true }` + "\n" +
				`alice.name`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.StructDef{
							Name: "Person",
							Fields: map[string]checker.Type{
								"name":     checker.Str,
								"age":      checker.Int,
								"employed": checker.Bool,
							},
							Methods: map[string]*checker.FunctionDef{},
						},
					},
					{
						Stmt: &checker.VariableDef{
							Name: "alice",
							Value: &checker.StructInstance{
								Name: "Person",
								Fields: map[string]checker.Expression{
									"name":     &checker.StrLiteral{"Alice"},
									"age":      &checker.IntLiteral{30},
									"employed": &checker.BoolLiteral{true},
								},
								FieldTypes: map[string]checker.Type{
									"name":     checker.Str,
									"age":      checker.Int,
									"employed": checker.Bool,
								},
							},
						},
					},
					{
						Expr: &checker.InstanceProperty{
							Subject:  &checker.Variable{},
							Property: "name",
							Kind:     checker.StructSubject,
						},
					},
				},
			},
		},
		{
			name: "Using a package struct",
			input: `use ard/http` + "\n" +
				`let req = http::Request{method:http::Method::Get, url:"google.com", headers: [:]}` + "\n" +
				`req.url`,
			// Note: We skip detailed checking of ModuleStructInstance FieldTypes
			// because it contains Type structs with unexported fields that are hard to compare
		},
		{
			name: "Cannot instantiate with incorrect fields",
			input: personStructInput + "\n" + strings.Join([]string{
				`Person{ name: "Alice", age: 30 }`,
				`Person{ color: "blue", name: "Alice", age: 30, employed: true }`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Missing field: employed"},
				{Kind: checker.Error, Message: "Unknown field: color"},
			},
		},
		{
			name: "Cannot use undefined fields",
			input: personStructInput + "\n" + strings.Join([]string{
				`let p = Person{ name: "Alice", age: 30, employed: true }`,
				`p.height`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined: p.height"},
			},
		},
		{
			name: "Can reassign to properties",
			input: fmt.Sprintf(`%s
				mut p = Person{name: "Alice", age: 30, employed: true}
				p.age = 31`, personStructInput),
			// Copy semantics allow this to be valid, so no errors
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Can't reassign to properties of immutable structs",
			input: fmt.Sprintf(`%s
						let p = Person{name: "Alice", age: 30, employed: true}
						p.age = 31`, personStructInput),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Immutable: p.age"},
			},
		},
		{
			name: "When an undefined variable is used as a value",
			input: fmt.Sprintf(`%s
						let p = Person{name: "Alice", age: 30, employed: is_employed}
						p.age = 31`, personStructInput),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: is_employed"},
				{Kind: checker.Error, Message: "Missing field: employed"},
			},
		},
	})
}

func TestCopySemantics(t *testing.T) {
	personStructInput := strings.Join([]string{
		"struct Person {",
		"  name: Str,",
		"  age: Int",
		"}",
	}, "\n")

	run(t, []test{
		{
			name: "mut assignment should accept copy from immutable struct",
			input: fmt.Sprintf(`%s
				let alice = Person{name: "Alice", age: 30}
				mut bob = alice`, personStructInput),
			// For now, just check that it compiles without errors
			diagnostics: []checker.Diagnostic{},
		},
	})
}

func TestMethods(t *testing.T) {
	shapeCode := strings.Join([]string{
		"struct Shape {",
		"  width: Int,",
		"  height: Int",
		"}",
	}, "\n")
	run(t, []test{
		{
			name: "Valid impl block",
			input: fmt.Sprintf(
				`%s
				impl Shape {
				  fn get_area() Int {
						self.width * self.height
					}
				}`, shapeCode),
			output: &checker.Program{
				Statements: []checker.Statement{},
			},
		},
		{
			name: "The instance can't be mutated in a non-mutable impl block",
			input: fmt.Sprintf(
				`%s
				impl Shape {
				  fn resize(h: Int, w: Int) {
						self.width = w
						self.height = h
					}
				}`, shapeCode),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Immutable: self.width"},
				{Kind: checker.Error, Message: "Immutable: self.height"},
			},
		},
		{
			name: "A mutable method can only mutate a mutable instance",
			input: fmt.Sprintf(
				`%s
				impl Shape {
				  fn mut resize(width: Int, height: Int) {
						self.width = width
						self.height = height
					}
				}

				let square = Shape{width: 5, height: 5}
				square.resize(8,8)`, shapeCode),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Cannot mutate immutable 'square' with '.resize()'"},
			},
		},
	})
}

func TestStructsWithMaybeFields(t *testing.T) {
	run(t, []test{
		{
			name: "Maybe fields can be omitted",
			input: `struct Message {
				kind: Str,
				stuff: Int?
			}
			Message{kind: "info"}
			`,
		},
	})
}

func TestStructsWithStaticFunctions(t *testing.T) {
	run(t, []test{
		{
			name: "Structs can have static functions",
			input: `use ard/maybe
			struct Message {
				kind: Str,
				stuff: Int?
			}
			fn Message::new(kind: Str, stuff: Int?) Message {
				Message{kind: kind, stuff: stuff}
			}
			Message::new("info", maybe::some(42))
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
