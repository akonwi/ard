package ast

import (
	"fmt"
	"testing"
)

var personStructCode = `
struct Person {
	name: Str,
	age: Num,
	employed: Bool
}`

var personStruct = StructDefinition{
	Name: Identifier{Name: "Person"},
	Fields: []StructField{
		{Identifier{Name: "name"}, StringType{}},
		{Identifier{Name: "age"}, NumberType{}},
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
	}

	runTests(t, tests)
}

func TestStructStuff(t *testing.T) {
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
				Person { name: "John", age: 23, employed: true }
			`, personStructCode),
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					personStruct,
					StructInstance{
						Name: Identifier{Name: "Person"},
						Properties: []StructValue{
							{Name: Identifier{Name: "name"},
								Value: StrLiteral{Value: `"John"`}},
							{Name: Identifier{Name: "age"}, Value: NumLiteral{Value: "23"}},
							{Name: Identifier{Name: "employed"}, Value: BoolLiteral{Value: true}},
						},
					},
				},
			},
		},
	}

	runTests(t, tests)
}

// func TestStructFieldAccess(t *testing.T) {
// 	personStructCode := `
// 		struct Person {
// 			name: Str,
// 			age: Num,
// 			employed: Bool
// 		}`
// 	personStruct := StructType{
// 		Name: "Person",
// 		Fields: map[string]Type{
// 			"name":     StrType,
// 			"age":      NumType,
// 			"employed": BoolType,
// 		},
// 	}
// 	tests := []test{
// 		{
// 			name: "Valid field access",
// 			input: fmt.Sprintf(`%s
// 				let person = Person { name: "Bobby", age: 12, employed: false }
// 				person.name
// 				person.age
// 				person.employed`, personStructCode),
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					StructDefinition{Type: personStruct},
// 					VariableDeclaration{
// 						Mutable: false,
// 						Name:    "person",
// 						Type:    personStruct,
// 						Value: StructInstance{
// 							Type: personStruct,
// 							Properties: []StructValue{
// 								{Name: "name", Value: StrLiteral{Value: `"Bobby"`}},
// 								{Name: "age", Value: NumLiteral{Value: "12"}},
// 								{Name: "employed", Value: BoolLiteral{Value: false}},
// 							},
// 						},
// 					},
// 					MemberAccess{
// 						Target:     Identifier{Name: "person", Type: personStruct},
// 						AccessType: Instance,
// 						Member:     Identifier{Name: "name", Type: StrType},
// 					},
// 					MemberAccess{
// 						Target:     Identifier{Name: "person", Type: personStruct},
// 						AccessType: Instance,
// 						Member:     Identifier{Name: "age", Type: NumType},
// 					},
// 					MemberAccess{
// 						Target:     Identifier{Name: "person", Type: personStruct},
// 						AccessType: Instance,
// 						Member:     Identifier{Name: "employed", Type: BoolType},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Accessing non-existent fields",
// 			input: fmt.Sprintf(`%s
// 				let person = Person { name: "Bobby", age: 12, employed: false }
// 				person.foobar`, personStructCode),
// 			diagnostics: []Diagnostic{
// 				{Msg: "No field 'foobar' in 'Person' struct"},
// 			},
// 		},
// 	}

// 	runTests(t, tests)
// }
