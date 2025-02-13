package checker

import (
	"fmt"
	"strings"
	"testing"
)

func TestStructs(t *testing.T) {
	personStructInput := strings.Join([]string{
		"struct Person {",
		"  name: Str,",
		"  age: Int,",
		"  employed: Bool",
		"}",
	}, "\n")
	personStruct := &Struct{
		Name: "Person",
		Fields: map[string]Type{
			"name":     Str{},
			"age":      Int{},
			"employed": Bool{},
		},
	}

	run(t, []test{
		{
			name:  "Valid struct definition",
			input: personStructInput,
			output: Program{
				Statements: []Statement{
					personStruct,
				},
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
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Duplicate field: height"},
			},
		},
		{
			name: "Using a struct",
			input: personStructInput + "\n" +
				`let alice = Person{ name: "Alice", age: 30, employed: true }` + "\n" +
				`alice.name`,
			output: Program{
				Statements: []Statement{
					personStruct,
					VariableBinding{
						Mut:  false,
						Name: "alice",
						Value: StructInstance{
							Name: "Person",
							Fields: map[string]Expression{
								"name":     StrLiteral{Value: "Alice"},
								"age":      IntLiteral{Value: 30},
								"employed": BoolLiteral{Value: true},
							},
						},
					},
					InstanceProperty{
						Subject:  Identifier{Name: "alice"},
						Property: Identifier{Name: "name"},
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
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Missing field: employed"},
				{Kind: Error, Message: "Unknown field: color"},
			},
		},
		{
			name: "Cannot use undefined fields",
			input: personStructInput + "\n" + strings.Join([]string{
				`let p = Person{ name: "Alice", age: 30, employed: true }`,
				`p.height`,
			}, "\n"),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Undefined: p.height"},
			},
		},
		{
			name: "Can reassign to properties",
			input: fmt.Sprintf(`%s
				mut p = Person{name: "Alice", age: 30, employed: true}
				p.age = 31`, personStructInput),
			output: Program{
				Statements: []Statement{
					personStruct,
					VariableBinding{
						Mut:  true,
						Name: "p",
						Value: StructInstance{
							Name: "Person",
							Fields: map[string]Expression{
								"name":     StrLiteral{Value: "Alice"},
								"age":      IntLiteral{Value: 30},
								"employed": BoolLiteral{Value: true},
							},
						},
					},
					VariableAssignment{
						Target: InstanceProperty{Subject: Identifier{Name: "p"}, Property: Identifier{Name: "age"}},
						Value:  IntLiteral{Value: 31},
					},
				},
			},
		},
		{
			name: "Can't reassign to properties of immutable structs",
			input: fmt.Sprintf(`%s
						let p = Person{name: "Alice", age: 30, employed: true}
						p.age = 31`, personStructInput),
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Cannot reassign in immutables"},
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
			output: Program{
				Statements: []Statement{
					&Struct{
						Name: "Shape",
						Fields: map[string]Type{
							"width":  Int{},
							"height": Int{},
						},
					},
				},
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
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Cannot reassign in immutables"},
				{Kind: Error, Message: "Cannot reassign in immutables"},
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
			diagnostics: []Diagnostic{
				{Kind: Error, Message: "Cannot mutate immutable 'square' with '.resize()'"},
			},
		},
	})
}
