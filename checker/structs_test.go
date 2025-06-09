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
						Stmt: &checker.VariableDef{
							Name: "alice",
							Value: &checker.StructInstance{
								Name: "Person",
								Fields: map[string]checker.Expression{
									"name":     &checker.StrLiteral{"Alice"},
									"age":      &checker.IntLiteral{30},
									"employed": &checker.BoolLiteral{true},
								},
							},
						},
					},
					{
						Expr: &checker.InstanceProperty{
							Subject:  &checker.Variable{},
							Property: "name",
						},
					},
				},
			},
		},
		{
			name: "Using a package struct",
			input: `use ard/http` + "\n" +
				`let req = http::Request{method:"GET", url:"google.com", headers: [:]}` + "\n" +
				`req.url`,
			output: &checker.Program{
				Imports: map[string]checker.Module{
					"ard/http": checker.HttpPkg{},
				},
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Name: "req",
							Value: &checker.ModuleStructInstance{
								Module: "http",
								Property: &checker.StructInstance{
									Name: "Request",
									Fields: map[string]checker.Expression{
										"method":  &checker.StrLiteral{"GET"},
										"url":     &checker.StrLiteral{"google.com"},
										"headers": &checker.MapLiteral{Keys: []checker.Expression{}, Values: []checker.Expression{}},
									},
								},
							},
						},
					},
					{
						Expr: &checker.InstanceProperty{
							Subject:  &checker.Variable{},
							Property: "url",
						},
					},
				},
			},
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
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Stmt: &checker.VariableDef{
							Mutable: true,
							Name:    "p",
							Value: &checker.StructInstance{
								Name: "Person",
								Fields: map[string]checker.Expression{
									"name":     &checker.StrLiteral{"Alice"},
									"age":      &checker.IntLiteral{30},
									"employed": &checker.BoolLiteral{true},
								},
							},
						},
					},
					{
						Stmt: &checker.Reassignment{
							Target: &checker.InstanceProperty{
								Subject:  &checker.Variable{},
								Property: "age",
							},
							Value: &checker.IntLiteral{31},
						},
					},
				},
			},
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
						@width * @height
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
						@width = w
						@height = h
					}
				}`, shapeCode),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Immutable: @width"},
				{Kind: checker.Error, Message: "Immutable: @height"},
			},
		},
		{
			name: "A mutable method can only mutate a mutable instance",
			input: fmt.Sprintf(
				`%s
				impl Shape {
				  fn mut resize(width: Int, height: Int) {
						@width = width
						@height = height
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
