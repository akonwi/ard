package parse

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

func TestStructNameLocation(t *testing.T) {
	result := Parse([]byte("struct Sender {\n  name: Str\n}\n"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	def, ok := result.Program.Statements[0].(*StructDefinition)
	if !ok {
		t.Fatalf("statement = %T, want *StructDefinition", result.Program.Statements[0])
	}
	if got, want := def.Name.Location.Start.Row, 1; got != want {
		t.Fatalf("name start row = %d, want %d", got, want)
	}
	if got, want := def.Name.Location.Start.Col, 8; got != want {
		t.Fatalf("name start col = %d, want %d", got, want)
	}
	if got, want := def.Name.Location.End.Col, 13; got != want {
		t.Fatalf("name end col = %d, want %d", got, want)
	}
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
			name:  "A private struct",
			input: `private struct Box {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructDefinition{
						Private: true,
						Name:    Identifier{Name: "Box"},
						Fields:  []StructField{},
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
			name:  "A struct with explicit generic parameters",
			input: `struct State<$T> { handle: StateHandle }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructDefinition{
						Name:       Identifier{Name: "State"},
						TypeParams: []string{"T"},
						Fields: []StructField{
							{Identifier{Name: "handle"}, &CustomType{Name: "StateHandle"}},
						},
					},
				},
			},
		},
		{
			name: "A struct with mutable reference field",
			input: `struct Context {
					tree: mut ViewTree,
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructDefinition{
						Name: Identifier{Name: "Context"},
						Fields: []StructField{
							{Identifier{Name: "tree"}, &MutableType{Inner: &CustomType{Name: "ViewTree"}}},
						},
					},
				},
			},
		},
		{
			name: "Method definitions",
			input: `
					impl Shape {
						fn area() Int {
							self.height * self.width
						}

						private fn mut set_height(h: Int) {}
					}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&ImplBlock{
						Target: Identifier{
							Name: "Shape",
						},
						Receiver: Identifier{Name: "self"},
						Methods: []FunctionDeclaration{
							{
								Name:       "area",
								Parameters: []Parameter{},
								ReturnType: &IntType{},
								Body: []Statement{
									&BinaryExpression{
										Operator: Multiply,
										Left: &InstanceProperty{
											Target:   &Identifier{Name: "self"},
											Property: Identifier{Name: "height"},
										},
										Right: &InstanceProperty{
											Target:   &Identifier{Name: "self"},
											Property: Identifier{Name: "width"},
										},
									},
								},
							},
							{
								Private: true,
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
		// Error cases
		{
			name:     "Missing struct name",
			input:    "struct { name: string }",
			wantErrs: []string{"Expected name after 'struct'"},
		},
		{
			name:     "Missing opening brace",
			input:    "struct Person name: string }",
			wantErrs: []string{"Expected '{'"},
		},
		{
			name:     "Missing colon after field name",
			input:    "struct Person { name string }",
			wantErrs: []string{"Expected ':' after field name", "Expected '}'"},
		},
		{
			name:     "Missing comma between fields",
			input:    "struct Person { name: string age: int }",
			wantErrs: []string{"Expected ',' or '}' after field type", "Expected '}'"},
		},
		{
			name:     "Empty struct works",
			input:    "struct Person { }",
			wantErrs: []string{},
		},
		{
			name:     "Trailing comma works",
			input:    "struct Person { name: string, }",
			wantErrs: []string{},
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
					p.employed = false
					p.speak()`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&InstanceProperty{Target: &Identifier{Name: "p"}, Property: Identifier{Name: "age"}},
					&VariableAssignment{
						Target:   &InstanceProperty{Target: &Identifier{Name: "p"}, Property: Identifier{Name: "employed"}},
						Operator: Assign,
						Value:    &BoolLiteral{Value: false},
					},
					&InstanceMethod{
						Target: &Identifier{Name: "p"},
						Method: FunctionCall{
							Name:     "speak",
							Args:     []Argument{},
							Comments: []Comment{},
						},
					},
				},
			},
		},
	})
}
func TestReferencingStructsFromPackage(t *testing.T) {
	runTests(t, []test{
		{
			name: "using static properties as types",
			input: `
				let req: http::Request? = maybe::none()
			`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name: "req",
						Type: &CustomType{
							Name: "http::Request",
							Type: StaticProperty{
								Target:   &Identifier{Name: "http"},
								Property: &Identifier{Name: "Request"},
							},
						},
						Value: &StaticFunction{
							Target:   &Identifier{Name: "maybe"},
							Function: FunctionCall{Name: "none", Args: []Argument{}, Comments: []Comment{}},
						},
					},
				},
			},
		},
		{
			name: "instantiating static structs",
			input: `http::Request{
			  url: "foobar.com"
			}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StaticProperty{
						Target: &Identifier{Name: "http"},
						Property: &StructInstance{
							Name: Identifier{Name: "Request"},
							Properties: []StructValue{
								{
									Name:  Identifier{Name: "url"},
									Value: &StrLiteral{Value: "foobar.com"},
								},
							},
						},
					},
				},
			},
		},
	})
}
func TestNestedStructInstantiation(t *testing.T) {
	runTests(t, []test{
		{
			name:     "Minimal nested struct instantiation",
			input:    `Line{ start: Point { x: 10 } }`,
			wantErrs: []string{},
		},
		{
			name:     "Multiple nested struct instantiations",
			input:    `Line{ start: Point { x: 10, y: 20 }, end: Point { x: 30, y: 40 } }`,
			wantErrs: []string{},
		},
		{
			name:     "Static nested struct instantiation as field value",
			input:    `Outer{ padding: types::Inner{ a: 0, b: 1 } }`,
			wantErrs: []string{},
		},
		{
			name:     "Deeply nested structs",
			input:    `Box{ item: Container { value: Point { x: 1 } } }`,
			wantErrs: []string{},
		},
	})
}

func TestGenericStructInstantiation(t *testing.T) {
	runTests(t, []test{
		{
			name:  "explicit type args on a struct literal",
			input: `Radio<Str>{ value: "compact" }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructInstance{
						Name:     Identifier{Name: "Radio"},
						TypeArgs: []DeclaredType{&StringType{}},
						Properties: []StructValue{
							{Name: Identifier{Name: "value"}, Value: &StrLiteral{Value: "compact"}},
						},
					},
				},
			},
		},
		{
			name:  "explicit custom type args on a static struct literal",
			input: `ui::Provider<ui::Theme>{ value: active }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StaticProperty{
						Target: &Identifier{Name: "ui"},
						Property: &StructInstance{
							Name: Identifier{Name: "Provider"},
							TypeArgs: []DeclaredType{
								&CustomType{
									Name: "ui::Theme",
									Type: StaticProperty{
										Target:   &Identifier{Name: "ui"},
										Property: &Identifier{Name: "Theme"},
									},
								},
							},
							Properties: []StructValue{
								{Name: Identifier{Name: "value"}, Value: &Identifier{Name: "active"}},
							},
						},
					},
				},
			},
		},
		{
			name:  "empty generic struct literal",
			input: `Box<Int>{}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructInstance{
						Name:       Identifier{Name: "Box"},
						TypeArgs:   []DeclaredType{&IntType{}},
						Properties: []StructValue{},
					},
				},
			},
		},
		{
			name:  "comparison chain is not a generic struct literal",
			input: `let x = a < b`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name: "x",
						Value: &BinaryExpression{
							Operator: LessThan,
							Left:     &Identifier{Name: "a"},
							Right:    &Identifier{Name: "b"},
						},
					},
				},
			},
		},
	})
}

func TestGenericStructInstantiationAmbiguity(t *testing.T) {
	runTests(t, []test{
		{
			name:  "unspaced comparison is not a generic struct literal",
			input: `let x = a<b`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name: "x",
						Value: &BinaryExpression{
							Operator: LessThan,
							Left:     &Identifier{Name: "a"},
							Right:    &Identifier{Name: "b"},
						},
					},
				},
			},
		},
		{
			name:  "unspaced comparison in a larger expression leaves no errors",
			input: `let ok = a<b and c`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name: "ok",
						Value: &BinaryExpression{
							Operator: And,
							Left: &BinaryExpression{
								Operator: LessThan,
								Left:     &Identifier{Name: "a"},
								Right:    &Identifier{Name: "b"},
							},
							Right: &Identifier{Name: "c"},
						},
					},
				},
			},
		},
		{
			name:  "non-adjacent angle bracket is not a generic struct literal",
			input: `let x = a < b`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name: "x",
						Value: &BinaryExpression{
							Operator: LessThan,
							Left:     &Identifier{Name: "a"},
							Right:    &Identifier{Name: "b"},
						},
					},
				},
			},
		},
		{
			name:  "nested type args on a struct literal",
			input: `Box<List<Str>>{}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructInstance{
						Name: Identifier{Name: "Box"},
						TypeArgs: []DeclaredType{
							&CustomType{
								Name:     "List",
								TypeArgs: []DeclaredType{&StringType{}},
							},
						},
						Properties: []StructValue{},
					},
				},
			},
		},
		{
			name:  "multiple type args on a struct literal",
			input: `Pair<Str, Int>{ left: "a", right: 1 }`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructInstance{
						Name:     Identifier{Name: "Pair"},
						TypeArgs: []DeclaredType{&StringType{}, &IntType{}},
						Properties: []StructValue{
							{Name: Identifier{Name: "left"}, Value: &StrLiteral{Value: "a"}},
							{Name: Identifier{Name: "right"}, Value: &NumLiteral{Value: "1"}},
						},
					},
				},
			},
		},
	})
}

func TestMutStructInstanceOperand(t *testing.T) {
	// #285: `mut <struct literal>` must parse for all struct forms,
	// including empty braces and module-qualified names, and as a
	// struct-literal field value (not only as a top-level operand).
	runTests(t, []test{
		{
			name:  "mut empty struct literal as field value",
			input: `Holder{r: mut Inner{}}`,
		},
		{
			name:  "mut qualified empty struct literal as field value",
			input: `Holder{r: mut pkg::Thing{}}`,
		},
		{
			name:  "mut empty struct literal in a binding",
			input: `let x = mut pkg::Thing{}`,
		},
		{
			name:  "mut populated struct literal still parses",
			input: `Holder{r: mut Inner{n: 1}}`,
		},
	})
}

func TestMutOperandInSubjectPositions(t *testing.T) {
	// #285 review: recognizing struct-literal operands of `mut` must not
	// leak into subject positions (if/while/for/match), where a following
	// `{ ... }` is a block, not a struct literal for the mut operand.
	runTests(t, []test{
		{
			name:  "for-in over a mut iterable keeps the loop body",
			input: "for n in mut xs {\n  work(n)\n}",
		},
		{
			name:  "if over a mut condition keeps the body",
			input: "if mut flag {\n  work()\n}",
		},
		{
			name:  "while over a mut condition keeps the body",
			input: "while mut flag {\n  work()\n}",
		},
		{
			name:  "match on a mut subject keeps the arms block",
			input: "match mut subject {\n  _ => {}\n}",
		},
		{
			name:  "mut struct operand still works as a call argument",
			input: `take(mut pkg::Thing{})`,
		},
	})
}
