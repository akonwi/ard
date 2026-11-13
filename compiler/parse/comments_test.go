package parse

import (
	"testing"
)

func TestComments(t *testing.T) {
	runTests(t, []test{
		{
			name:  "Single line comment",
			input: "// this is a comment",
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&Comment{Value: "// this is a comment"},
				},
			},
		},
		{
			name:  "Inline comment",
			input: "let x = 200 // this is a comment",
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Name:  "x",
						Value: &NumLiteral{Value: "200"},
					},
					&Comment{Value: "// this is a comment", Trailing: true},
				},
			},
		},
	})
}
func TestCommentsInStructFields(t *testing.T) {
	runTests(t, []test{
		{
			name: "Comments between struct fields",
			input: `
				struct Person {
					name: Str,
					// Comment between fields
					age: Int,
					// Another comment
					employed: Bool
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructDefinition{
						Name: Identifier{Name: "Person"},
						Fields: []StructField{
							{Name: Identifier{Name: "name"}, Type: &StringType{}},
							{Name: Identifier{Name: "age"}, Type: &IntType{}},
							{Name: Identifier{Name: "employed"}, Type: &BooleanType{}},
						},
						Comments: []Comment{
							{Value: "Comment between fields"},
							{Value: "Another comment"},
						},
					},
				},
			},
		},
		{
			name: "Comment after last struct field",
			input: `
				struct Person {
					name: Str,
					age: Int
					// Comment at end
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructDefinition{
						Name: Identifier{Name: "Person"},
						Fields: []StructField{
							{Name: Identifier{Name: "name"}, Type: &StringType{}},
							{Name: Identifier{Name: "age"}, Type: &IntType{}},
						},
						Comments: []Comment{
							{Value: "Comment at end"},
						},
					},
				},
			},
		},
	})
}
func TestCommentsInEnumVariants(t *testing.T) {
	runTests(t, []test{
		{
			name: "Comments between enum variants",
			input: `
				enum Color {
					Red,
					// Comment between variants
					Green,
					Blue
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&EnumDefinition{
						Name: "Color",
						Variants: []EnumVariant{
							{Name: "Red"},
							{Name: "Green"},
							{Name: "Blue"},
						},
						Comments: []Comment{
							{Value: "Comment between variants"},
						},
					},
				},
			},
		},
	})
}
func TestCommentsInFunctionParameters(t *testing.T) {
	runTests(t, []test{
		{
			name: "Comments between function parameters",
			input: `fn add(x: Int,
					// Comment between parameters
					y: Int) Int {}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&FunctionDeclaration{
						Name: "add",
						Parameters: []Parameter{
							{Name: "x", Type: &IntType{}},
							{Name: "y", Type: &IntType{}},
						},
						ReturnType: &IntType{},
						Comments: []Comment{
							{Value: "Comment between parameters"},
						},
						Body: []Statement{},
					},
				},
			},
		},
	})
}
func TestCommentsInStructLiterals(t *testing.T) {
	runTests(t, []test{
		{
			name: "Comments between struct literal properties",
			input: `Person{
					name: "Alice",
					// Comment between properties
					age: 30,
					employed: true
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructInstance{
						Name: Identifier{Name: "Person"},
						Properties: []StructValue{
							{Name: Identifier{Name: "name"}, Value: &StrLiteral{Value: "Alice"}},
							{Name: Identifier{Name: "age"}, Value: &NumLiteral{Value: "30"}},
							{Name: Identifier{Name: "employed"}, Value: &BoolLiteral{Value: true}},
						},
						Comments: []Comment{
							{Value: "Comment between properties"},
						},
					},
				},
			},
		},
	})
}
func TestInlineCommentsInStructLiterals(t *testing.T) {
	runTests(t, []test{
		{
			name: "Inline comments after struct literal properties",
			input: `Person{
					name: "Alice",     // Person name
					age: 30,           // Age in years
					employed: true     // Job status
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&StructInstance{
						Name: Identifier{Name: "Person"},
						Properties: []StructValue{
							{Name: Identifier{Name: "name"}, Value: &StrLiteral{Value: "Alice"}},
							{Name: Identifier{Name: "age"}, Value: &NumLiteral{Value: "30"}},
							{Name: Identifier{Name: "employed"}, Value: &BoolLiteral{Value: true}},
						},
						Comments: []Comment{
							{Value: "Person name", Trailing: true},
							{Value: "Age in years", Trailing: true},
							{Value: "Job status", Trailing: true},
						},
					},
				},
			},
		},
	})
}

// A comment that follows code on its line is marked Trailing, so the formatter
// can keep it on that line; a comment on a line of its own is not (#583).
func TestCommentsRecordWhetherTheyTrailCode(t *testing.T) {
	type mark struct {
		value    string
		trailing bool
	}
	tests := []struct {
		name     string
		input    string
		comments func(program *Program) []Comment
		want     []mark
	}{
		{
			name:  "list literal",
			input: "let l = [ // open\n  1, // one\n  // own\n  2 // two\n]\n",
			comments: func(program *Program) []Comment {
				return program.Statements[0].(*VariableDeclaration).Value.(*ListLiteral).Comments
			},
			want: []mark{{"open", true}, {"one", true}, {"own", false}, {"two", true}},
		},
		{
			name:  "map literal",
			input: "let m = [ // open\n  1: 2, // one\n  // own\n  3: 4\n]\n",
			comments: func(program *Program) []Comment {
				return program.Statements[0].(*VariableDeclaration).Value.(*MapLiteral).Comments
			},
			want: []mark{{"open", true}, {"one", true}, {"own", false}},
		},
		{
			name:  "tuple literal",
			input: "let t = (\n  1, // one\n  // own\n  2,\n)\n",
			comments: func(program *Program) []Comment {
				return program.Statements[0].(*VariableDeclaration).Value.(*TupleLiteral).Comments
			},
			want: []mark{{"one", true}, {"own", false}},
		},
		{
			name:  "call arguments",
			input: "foo( // open\n  1, // one\n  // own\n  2,\n)\n",
			comments: func(program *Program) []Comment {
				return program.Statements[0].(*FunctionCall).Comments
			},
			want: []mark{{"open", true}, {"one", true}, {"own", false}},
		},
		{
			name:  "struct literal",
			input: "Point{\n  x: 1, // ex\n  // own\n  y: 2,\n}\n",
			comments: func(program *Program) []Comment {
				return program.Statements[0].(*StructInstance).Comments
			},
			want: []mark{{"ex", true}, {"own", false}},
		},
		{
			name:  "function parameters",
			input: "fn add(\n  a: Int, // first\n  // own\n  b: Int,\n) Int {\n  a + b\n}\n",
			comments: func(program *Program) []Comment {
				return program.Statements[0].(*FunctionDeclaration).Comments
			},
			want: []mark{{"first", true}, {"own", false}},
		},
		{
			name:  "anonymous function parameters",
			input: "let f = fn(\n  a: Int, // first\n  // own\n  b: Int,\n) Int {\n  a + b\n}\n",
			comments: func(program *Program) []Comment {
				return program.Statements[0].(*VariableDeclaration).Value.(*AnonymousFunction).Comments
			},
			want: []mark{{"first", true}, {"own", false}},
		},
		{
			name:  "statements",
			input: "// own\nlet x = 1 // trailing\n",
			comments: func(program *Program) []Comment {
				return []Comment{*program.Statements[0].(*Comment), *program.Statements[2].(*Comment)}
			},
			want: []mark{{"// own", false}, {"// trailing", true}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Parse([]byte(test.input), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			comments := test.comments(result.Program)
			got := make([]mark, 0, len(comments))
			for _, comment := range comments {
				got = append(got, mark{comment.Value, comment.Trailing})
			}
			if len(got) != len(test.want) {
				t.Fatalf("comments = %v, want %v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("comments = %v, want %v", got, test.want)
				}
			}
		})
	}
}

// Enum variants record where they are, so the formatter can keep comments
// next to the variant they describe (#583).
func TestEnumVariantsRecordTheirLocation(t *testing.T) {
	result := Parse([]byte("enum Code {\n  Ok = 0,\n  // the rest\n  Bad,\n}\n"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	variants := result.Program.Statements[0].(*EnumDefinition).Variants
	want := []Location{
		{Start: Point{Row: 2, Col: 3}, End: Point{Row: 2, Col: 8}},
		{Start: Point{Row: 4, Col: 3}, End: Point{Row: 4, Col: 5}},
	}
	for i, variant := range variants {
		if variant.Location != want[i] {
			t.Fatalf("variant %s location = %+v, want %+v", variant.Name, variant.Location, want[i])
		}
	}
}
