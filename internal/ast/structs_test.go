package ast

// func TestStructDefinitions(t *testing.T) {
// 	emptyStruct := StructType{
// 		Name:   "Box",
// 		Fields: map[string]Type{},
// 	}
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
// 			name: "An empty struct",
// 			input: `
// 				struct Box {}`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					StructDefinition{
// 						Type: emptyStruct,
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "A valid struct",
// 			input: `
// 				struct Person {
// 					name: Str,
// 					age: Num,
// 					employed: Bool
// 				}`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					StructDefinition{
// 						Type: personStruct,
// 					},
// 				},
// 			},
// 		},
// 	}

// 	runTests(t, tests)
// }

// func TestInstantiatingStructs(t *testing.T) {
// 	emptyStruct := StructType{
// 		Name:   "Box",
// 		Fields: map[string]Type{},
// 	}

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
// 			name: "Instantiating a field-less struct",
// 			input: `
// 				struct Box {}
// 				Box{}`,
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					StructDefinition{
// 						Type: emptyStruct,
// 					},
// 					StructInstance{
// 						Type:       emptyStruct,
// 						Properties: []StructValue{},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Instantiating with field errors",
// 			input: fmt.Sprintf(`%s
// 				Person { name: 23, employed: true, size: "xl"  }
// 			`, personStructCode),
// 			diagnostics: []Diagnostic{
// 				{Msg: "Type mismatch: expected Str, got Num"},
// 				{Msg: "'size' is not a field of 'Person'"},
// 				{Msg: "Missing field 'age' in struct 'Person'"},
// 			},
// 		},
// 		{
// 			name: "Correctly instantiating a struct with fields",
// 			input: fmt.Sprintf(`%s
// 				Person { name: "John", age: 23, employed: true }
// 			`, personStructCode),
// 			output: Program{
// 				Imports: []Import{},
// 				Statements: []Statement{
// 					StructDefinition{
// 						Type: personStruct,
// 					},
// 					StructInstance{
// 						Type: personStruct,
// 						Properties: []StructValue{
// 							{Name: "name", Value: StrLiteral{Value: `"John"`}},
// 							{Name: "age", Value: NumLiteral{Value: "23"}},
// 							{Name: "employed", Value: BoolLiteral{Value: true}},
// 						},
// 					},
// 				},
// 			},
// 		},
// 	}

// 	runTests(t, tests)
// }

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
