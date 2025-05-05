package checker_v2_test

import (
	"fmt"
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker_v2"
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
				impl (self: Shape) {
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
				impl (self: Shape) {
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
			name: "A mutable impl block",
			input: fmt.Sprintf(
				`%s
				impl (mut self: Shape) {
				  fn resize(width: Int, height: Int) {
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
