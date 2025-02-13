package ast

import (
	"fmt"
	"testing"
)

var personStructCode = `
struct Person {
	name: Str,
	age: Int,
	employed: Bool
}`

var personStruct = StructDefinition{
	Name: Identifier{Name: "Person"},
	Fields: []StructField{
		{Identifier{Name: "name"}, StringType{}},
		{Identifier{Name: "age"}, IntType{}},
		{Identifier{Name: "employed"}, BooleanType{}},
	},
}

func TestStructDefinitions(t *testing.T) {
	tests := []test{
		{
			name: "An empty struct",
			input: `
				struct Box {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					StructDefinition{
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
				struct Shape {
					height: Int,
					width: Int
				}
				impl (s: Shape) {
					fn area() Int {
						s.height * s.width
					}
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					StructDefinition{
						Name: Identifier{Name: "Shape"},
						Fields: []StructField{
							{Identifier{Name: "height"}, IntType{}},
							{Identifier{Name: "width"}, IntType{}},
						},
					},
					ImplBlock{
						Self: Parameter{
							Name: "s",
							Type: CustomType{Name: "Shape"},
						},
						Methods: []FunctionDeclaration{
							{
								Name:       "area",
								Parameters: []Parameter{},
								ReturnType: IntType{},
								Body: []Statement{
									BinaryExpression{
										Operator: Multiply,
										Left: MemberAccess{
											Target:     Identifier{Name: "s"},
											AccessType: Instance,
											Member:     Identifier{Name: "height"},
										},
										Right: MemberAccess{
											Target:     Identifier{Name: "s"},
											AccessType: Instance,
											Member:     Identifier{Name: "width"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

func TestUsingStructs(t *testing.T) {
	tests := []test{
		{
			name: "Instantiating a field-less struct",
			input: `
				struct Box {}
				Box{}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					StructDefinition{
						Name:   Identifier{Name: "Box"},
						Fields: []StructField{},
					},
					StructInstance{
						Name:       Identifier{Name: "Box"},
						Properties: []StructValue{},
					},
				},
			},
		},
		{
			name: "Correctly instantiating a struct with fields",
			input: fmt.Sprintf(`%s
				let age = 23
				Person { name: "John", age: age, employed: true }
			`, personStructCode),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					personStruct,
					VariableDeclaration{
						Mutable: false,
						Name:    "age",
						Value:   IntLiteral{Value: "23"},
					},
					StructInstance{
						Name: Identifier{Name: "Person"},
						Properties: []StructValue{
							{Name: Identifier{Name: "name"},
								Value: StrLiteral{Value: `"John"`}},
							{Name: Identifier{Name: "age"}, Value: Identifier{Name: "age"}},
							{Name: Identifier{Name: "employed"}, Value: BoolLiteral{Value: true}},
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
					MemberAccess{Target: Identifier{Name: "p"}, AccessType: Instance, Member: Identifier{Name: "age"}},
					VariableAssignment{
						Target:   MemberAccess{Target: Identifier{Name: "p"}, AccessType: Instance, Member: Identifier{Name: "employed"}},
						Operator: Assign,
						Value:    BoolLiteral{Value: false},
					},
				},
			},
		},
	}

	runTests(t, tests)
}
