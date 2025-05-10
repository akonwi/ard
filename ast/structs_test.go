package ast

import (
	"testing"
)

var personStructCode = `
struct Person {
	name: Str,
	age: Int,
	employed: Bool
}`

var personStruct = &StructDefinition{
	Name: Identifier{Name: "Person"},
	Fields: []StructField{
		{Identifier{Name: "name"}, &StringType{}},
		{Identifier{Name: "age"}, &IntType{}},
		{Identifier{Name: "employed"}, &BooleanType{}},
	},
}

func TestStructDefinitions(t *testing.T) {
	runTests(t, []test{
		{
			name: "An empty struct",
			input: `
					struct Box {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructDefinition{
						Name:   Identifier{Name: "Box"},
						Fields: []StructField{},
					},
				},
			},
		},
		{
			name:  "A struct with properties",
			input: personStructCode,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					personStruct,
				},
			},
		},
		{
			name: "Method definitions",
			input: `
					impl Shape {
						fn area() Int {
							@height * @width
						}

						fn mut set_height(h: Int) {}
					}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&ImplBlock{
						Target: Identifier{
							Name: "Shape",
						},
						Methods: []FunctionDeclaration{
							{
								Name:       "area",
								Parameters: []Parameter{},
								ReturnType: &IntType{},
								Body: []Statement{
									&BinaryExpression{
										Operator: Multiply,
										Left: &InstanceProperty{
											Target:   &Identifier{Name: "@"},
											Property: Identifier{Name: "height"},
										},
										Right: &InstanceProperty{
											Target:   &Identifier{Name: "@"},
											Property: Identifier{Name: "width"},
										},
									},
								},
							},
							{
								Name:    "set_height",
								Mutates: true,
								Parameters: []Parameter{
									{Name: "h", Type: &IntType{}},
								},
								Body: []Statement{},
							},
						},
					},
				},
			},
		},
	})
}

func TestUsingStructs(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Instantiating an empty struct",
			input: `Box{}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructInstance{
						Name:       Identifier{Name: "Box"},
						Properties: []StructValue{},
					},
				},
			},
		},
		{
			name:  "Instantiating with fields",
			input: `Person{ name: "John", age: age, employed: true }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructInstance{
						Name: Identifier{Name: "Person"},
						Properties: []StructValue{
							{Name: Identifier{Name: "name"},
								Value: &StrLiteral{Value: "John"}},
							{Name: Identifier{Name: "age"}, Value: &Identifier{Name: "age"}},
							{Name: Identifier{Name: "employed"}, Value: &BoolLiteral{Value: true}},
						},
					},
				},
			},
		},
		{
			name: "Referencing fields",
			input: `
					p.age
					p.employed = false`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&InstanceProperty{Target: &Identifier{Name: "p"}, Property: Identifier{Name: "age"}},
					&VariableAssignment{
						Target:   &InstanceProperty{Target: &Identifier{Name: "p"}, Property: Identifier{Name: "employed"}},
						Operator: Assign,
						Value:    &BoolLiteral{Value: false},
					},
				},
			},
		},
	})
}

func TestReferencingStructsFromPackage(t *testing.T) {
	runTests(t, []test{
		{
			name: `it can be used as a type`,
			input: `
				let req: http::Request? = maybe::none()
			`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name: "req",
						Type: &CustomType{
							Type: StaticProperty{
								Target:   Identifier{Name: "http"},
								Property: Identifier{Name: "Request"},
							},
						},
						Value: &StaticFunction{
							Target:   &Identifier{Name: "maybe"},
							Function: FunctionCall{Name: "none", Args: []Expression{}},
						},
					},
				},
			},
		},
	})
}
