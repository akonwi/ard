package parse

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMaps(t *testing.T) {
	runTests(t, []test{
		{
			name: "Instantiating maps",
			input: `
				let empty: [Str:Int] = [:]
			  let num_to_str: [Int:Str] = [1: "one", 2: "two", 3: "three"]
				`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&VariableDeclaration{
						Mutable: false,
						Name:    "empty",
						Type:    &Map{Key: &StringType{}, Value: &IntType{}},
						Value: &MapLiteral{
							Entries: []MapEntry{},
						},
					},
					&VariableDeclaration{
						Mutable: false,
						Name:    "num_to_str",
						Type:    &Map{Key: &IntType{}, Value: &StringType{}},
						Value: &MapLiteral{
							Entries: []MapEntry{
								{Key: &NumLiteral{Value: "1"}, Value: &StrLiteral{Value: "one"}},
								{Key: &NumLiteral{Value: "2"}, Value: &StrLiteral{Value: "two"}},
								{Key: &NumLiteral{Value: "3"}, Value: &StrLiteral{Value: "three"}},
							},
						},
					},
				},
			},
		},
	})
}

// TestMapLiteralExpressionKeys pins that a map key is a full expression, so
// each key parses the same as its parenthesized form, in the first entry and
// in later ones (#581).
func TestMapLiteralExpressionKeys(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		keyType Expression
	}{
		{name: "call", key: `key()`, keyType: &FunctionCall{}},
		{name: "call with named argument", key: `key(x: 1)`, keyType: &FunctionCall{}},
		{name: "generic call", key: `key<Int>()`, keyType: &FunctionCall{}},
		{name: "member access", key: `user.id`, keyType: &InstanceProperty{}},
		{name: "method call", key: `user.name()`, keyType: &InstanceMethod{}},
		{name: "binary operator", key: `a + 1`, keyType: &BinaryExpression{}},
		{name: "comparison", key: `a == b`, keyType: &BinaryExpression{}},
		{name: "unary minus", key: `-1`, keyType: &UnaryExpression{}},
		{name: "not", key: `not done`, keyType: &UnaryExpression{}},
		{name: "static property", key: `Color::red`, keyType: &StaticProperty{}},
		{name: "static function", key: `Color::from(1)`, keyType: &StaticFunction{}},
		{name: "struct literal", key: `Point{x: 1, y: 2}`, keyType: &StructInstance{}},
		{name: "anonymous function", key: `fn() Int { 1 }`, keyType: &AnonymousFunction{}},
		{name: "nested map", key: `[a.b: 1]`, keyType: &MapLiteral{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forms := map[string]string{
				"first entry": "[%s: \"v\", 2: \"w\"]",
				"later entry": "[2: \"w\", %s: \"v\"]",
			}
			for formName, form := range forms {
				bare := parseOK(t, "let m = "+strings.Replace(form, "%s", tt.key, 1))
				parenthesized := parseOK(t, "let m = "+strings.Replace(form, "%s", "("+tt.key+")", 1))
				if diff := cmp.Diff(parenthesized, bare, compareOptions); diff != "" {
					t.Fatalf("%s: bare key parses differently from its parenthesized form (-paren +bare):\n%s", formName, diff)
				}
				m, ok := bare.Statements[0].(*VariableDeclaration).Value.(*MapLiteral)
				if !ok {
					t.Fatalf("%s: expected a map literal, got %T", formName, bare.Statements[0].(*VariableDeclaration).Value)
				}
				if len(m.Entries) != 2 {
					t.Fatalf("%s: expected 2 entries, got %d", formName, len(m.Entries))
				}
				index := 0
				if formName == "later entry" {
					index = 1
				}
				if got, want := reflect.TypeOf(m.Entries[index].Key), reflect.TypeOf(tt.keyType); got != want {
					t.Fatalf("%s: key is %v, want %v", formName, got, want)
				}
			}
		})
	}
}

// TestMapLiteralColonContexts pins the places where ':' means something other
// than a map entry separator, around keys that are full expressions.
func TestMapLiteralColonContexts(t *testing.T) {
	t.Run("empty map and map type", func(t *testing.T) {
		program := parseOK(t, "let m: [Str:Int] = [:]")
		decl := program.Statements[0].(*VariableDeclaration)
		if _, ok := decl.Type.(*Map); !ok {
			t.Fatalf("expected a map type, got %T", decl.Type)
		}
		if m, ok := decl.Value.(*MapLiteral); !ok || len(m.Entries) != 0 {
			t.Fatalf("expected an empty map literal, got %#v", decl.Value)
		}
	})

	t.Run("static path keys keep their ::", func(t *testing.T) {
		m := parseMapLiteral(t, `[Color::red: 1, Color::blue: 2]`)
		for i, want := range []string{"red", "blue"} {
			key, ok := m.Entries[i].Key.(*StaticProperty)
			if !ok {
				t.Fatalf("entry %d key is %T, want *StaticProperty", i, m.Entries[i].Key)
			}
			if got := key.Property.(*Identifier).Name; got != want {
				t.Fatalf("entry %d property = %q, want %q", i, got, want)
			}
		}
	})

	t.Run("struct literals as keys and values", func(t *testing.T) {
		m := parseMapLiteral(t, `[Point{x: 1}: Point{x: 2}, Point{x: 3}: Point{x: 4}]`)
		if len(m.Entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(m.Entries))
		}
		for i, entry := range m.Entries {
			if _, ok := entry.Key.(*StructInstance); !ok {
				t.Fatalf("entry %d key is %T", i, entry.Key)
			}
			if _, ok := entry.Value.(*StructInstance); !ok {
				t.Fatalf("entry %d value is %T", i, entry.Value)
			}
		}
	})

	t.Run("struct literal key inside an if condition", func(t *testing.T) {
		parseOK(t, "if m == [Point{x: 1}: 1] {\n  1\n}")
	})

	t.Run("functions and blocks as keys and values", func(t *testing.T) {
		m := parseMapLiteral(t, `[fn(x: Int) Int { x }: { 1 }, { 2 }: fn(y: Int) Int { y }]`)
		if len(m.Entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(m.Entries))
		}
		if _, ok := m.Entries[0].Key.(*AnonymousFunction); !ok {
			t.Fatalf("entry 0 key is %T", m.Entries[0].Key)
		}
		if _, ok := m.Entries[0].Value.(*BlockExpression); !ok {
			t.Fatalf("entry 0 value is %T", m.Entries[0].Value)
		}
		if _, ok := m.Entries[1].Key.(*BlockExpression); !ok {
			t.Fatalf("entry 1 key is %T", m.Entries[1].Key)
		}
		if _, ok := m.Entries[1].Value.(*AnonymousFunction); !ok {
			t.Fatalf("entry 1 value is %T", m.Entries[1].Value)
		}
	})

	t.Run("comments between expression keys", func(t *testing.T) {
		m := parseMapLiteral(t, "[\n  // first\n  key(): 1, // after first\n  // second\n  user.id: 2,\n]")
		if len(m.Entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(m.Entries))
		}
		var got []string
		for _, comment := range m.Comments {
			got = append(got, comment.Value)
		}
		if want := []string{"first", "after first", "second"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("comments = %q, want %q", got, want)
		}
		if got := m.Location.Start; got != (Point{Row: 1, Col: 9}) {
			t.Fatalf("map starts at %v, want the '[' at 1:9", got)
		}
	})

	t.Run("a list stays a list", func(t *testing.T) {
		program := parseOK(t, "let l = [key(), user.id, a + 1]")
		list, ok := program.Statements[0].(*VariableDeclaration).Value.(*ListLiteral)
		if !ok || len(list.Items) != 3 {
			t.Fatalf("expected a 3-item list, got %#v", program.Statements[0].(*VariableDeclaration).Value)
		}
	})
}

// TestMapLiteralKeyErrors pins error reporting around map keys: an error in
// the first key is reported once, and mixing list items with map entries
// reports one error.
func TestMapLiteralKeyErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "error inside the first key is reported once",
			input: `let m = [(f(1 2)): 2]`,
			want:  []string{"Expected ',' between arguments"},
		},
		{
			name:  "error inside a bare first key is reported once",
			input: `let m = [f(1 2): 2]`,
			want:  []string{"Expected ',' between arguments"},
		},
		{
			name:  "map entry after a list item",
			input: `let m = [a, b: 1]`,
			want:  []string{"Unexpected ':' in list literal"},
		},
		{
			name:  "list item after a map entry",
			input: `let m = [a: 1, b]`,
			want:  []string{"Expected ':' after map key"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseErrors(t, tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("errors = %q, want %q", got, tt.want)
			}
		})
	}
}

func parseMapLiteral(t *testing.T, literal string) *MapLiteral {
	t.Helper()
	program := parseOK(t, "let m = "+literal)
	m, ok := program.Statements[0].(*VariableDeclaration).Value.(*MapLiteral)
	if !ok {
		t.Fatalf("expected a map literal, got %T", program.Statements[0].(*VariableDeclaration).Value)
	}
	return m
}
