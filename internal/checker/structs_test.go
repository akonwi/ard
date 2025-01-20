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
		"  age: Num,",
		"  employed: Bool",
		"}",
	}, "\n")
	personStruct := Struct{
		Name: "Person",
		Fields: map[string]Type{
			"name":     Str{},
			"age":      Num{},
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
				"  height: Num",
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
								"age":      NumLiteral{Value: 30},
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
	})
}

func TestMethods(t *testing.T) {
	shapeCode := strings.Join([]string{
		"struct Shape {",
		"  width: Num,",
		"  height: Num",
		"}",
	}, "\n")
	run(t, []test{
		{
			name: "Valid impl block",
			input: fmt.Sprintf(
				`%s
				impl (self: Shape) {
				  fn get_area() Num {
						self.width * self.height
					}
				}`, shapeCode),
			output: Program{
				Statements: []Statement{
					Struct{
						Name: "Shape",
						Fields: map[string]Type{
							"width":  Num{},
							"height": Num{},
						},
					},
				},
			},
		},
	})
}
