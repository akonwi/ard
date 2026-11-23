package formatter

import (
	"fmt"
	"strings"
	"testing"

	"github.com/akonwi/ard/compiler/parse"
)

// typeShape renders a declared type with every grouping explicit, so two
// types have the same shape exactly when they have the same meaning. Source
// locations are ignored.
func typeShape(declared parse.DeclaredType) string {
	if declared == nil {
		return "<nil>"
	}
	shape := typeShapeInner(declared)
	if declared.IsNullable() {
		return "Maybe(" + shape + ")"
	}
	return shape
}

func typeShapeInner(declared parse.DeclaredType) string {
	shapes := func(types []parse.DeclaredType) string {
		parts := make([]string, 0, len(types))
		for _, item := range types {
			parts = append(parts, typeShape(item))
		}
		return strings.Join(parts, ", ")
	}
	switch node := declared.(type) {
	case *parse.StringType:
		return "Str"
	case *parse.IntType:
		return "Int"
	case *parse.FloatType:
		return "Float64"
	case *parse.BooleanType:
		return "Bool"
	case *parse.VoidType:
		return "Void"
	case *parse.GenericType:
		return "$" + node.Name
	case *parse.MutableType:
		kind := "Mut"
		if node.Pointer {
			kind = "RefMut"
			if node.ReadOnly {
				kind = "Ref"
			}
		}
		return kind + "(" + typeShape(node.Inner) + ")"
	case *parse.CustomType:
		name := node.Name
		if node.Type.Target != nil {
			name = node.Type.String()
		}
		return "Named(" + name + "<" + shapes(node.TypeArgs) + ">)"
	case *parse.List:
		return "List(" + typeShape(node.Element) + ")"
	case *parse.FixedArray:
		return fmt.Sprintf("Array(%s; %d)", typeShape(node.Element), node.Length)
	case *parse.TupleType:
		return "Tuple(" + shapes(node.Elements) + ")"
	case *parse.Map:
		return "Map(" + typeShape(node.Key) + ": " + typeShape(node.Value) + ")"
	case *parse.ResultType:
		return "Result(" + typeShape(node.Val) + ", " + typeShape(node.Err) + ")"
	case *parse.FunctionType:
		return fmt.Sprintf("Fn(%s; mut=%v; variadic=%v) -> %s", shapes(node.Params), node.ParamMutability, node.Variadic, typeShape(node.Return))
	default:
		return fmt.Sprintf("%T(%s)", declared, declared.GetName())
	}
}

// parseTypeAlias parses `type T = <source>` and returns the aliased types.
func parseTypeAlias(t *testing.T, source string) ([]parse.DeclaredType, []parse.ParseError) {
	t.Helper()
	result := parse.Parse([]byte("type T = "+source+"\n"), "types.ard")
	if len(result.Errors) > 0 {
		return nil, result.Errors
	}
	if len(result.Program.Statements) != 1 {
		t.Fatalf("expected one statement for %q, got %d", source, len(result.Program.Statements))
	}
	decl, ok := result.Program.Statements[0].(*parse.TypeDeclaration)
	if !ok {
		t.Fatalf("expected a type declaration for %q, got %T", source, result.Program.Statements[0])
	}
	return decl.Type, nil
}

func unionShape(types []parse.DeclaredType) string {
	parts := make([]string, 0, len(types))
	for _, item := range types {
		parts = append(parts, typeShape(item))
	}
	return strings.Join(parts, " | ")
}

// assertTypeRoundTrip checks that printing source keeps its meaning and that
// printing the printed form again changes nothing.
func assertTypeRoundTrip(t *testing.T, source string) string {
	t.Helper()
	parsed, errs := parseTypeAlias(t, source)
	if errs != nil {
		t.Fatalf("test input %q does not parse: %v", source, errs[0].Message)
	}
	want := unionShape(parsed)

	p := newPrinter(100)
	render := func(types []parse.DeclaredType) string {
		return strings.Join(p.renderUnionMembers(types), " | ")
	}

	printed := render(parsed)
	reparsed, errs := parseTypeAlias(t, printed)
	if errs != nil {
		t.Errorf("%q printed as %q, which does not parse: %s", source, printed, errs[0].Message)
		return printed
	}
	if got := unionShape(reparsed); got != want {
		t.Errorf("%q printed as %q, which changes its meaning:\n  want %s\n  got  %s", source, printed, want, got)
		return printed
	}
	if again := render(reparsed); again != printed {
		t.Errorf("printing %q is not stable: %q then %q", source, printed, again)
	}
	return printed
}

// TestFormatTypePreservesMeaning pins the printed form of type syntax where a
// nested type needs parentheses to keep its meaning (#621).
func TestFormatTypePreservesMeaning(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		// A Result in the ok position of a Result: `!` is right-associative.
		{"($T!Str)!Str", "($T!Str)!Str"},
		{"(Int!Str)!Str", "(Int!Str)!Str"},
		{"((Int!Str)!Str)!Str", "((Int!Str)!Str)!Str"},
		{"Int!(Str!Bool)", "Int!Str!Bool"},
		{"Int!Str!Bool", "Int!Str!Bool"},
		// A function type in the ok position would swallow the `!` as its return.
		{"(fn() Int)!Str", "(fn() Int)!Str"},
		{"(fn())!Str", "(fn())!Str"},
		{"fn() Int!Str", "fn() Int!Str"},
		{"fn() (Int!Str)", "fn() Int!Str"},
		// Pointers extend as far right as possible.
		{"(&Int)!Str", "(&Int)!Str"},
		{"&(Int!Str)", "&Int!Str"},
		{"(&mut Int)?", "(&mut Int)?"},
		// Nullable ok values are grouped; only named types would parse bare.
		{"Int?!Str", "(Int?)!Str"},
		{"(Int?)!Str", "(Int?)!Str"},
		{"mod::Foo?!Str", "(mod::Foo?)!Str"},
		{"Box<Int>?!Str", "(Box<Int>?)!Str"},
		{"Int!Str?", "Int!Str?"},
		{"([Int]?)!Str", "([Int]?)!Str"},
		{"([Int: Str]?)!Str", "([Int: Str]?)!Str"},
		{"([Int; 2]?)!Str", "([Int; 2]?)!Str"},
		{"((Int, Str)?)!Str", "((Int, Str)?)!Str"},
		{"((Int!Str)?)!Str", "((Int!Str)?)!Str"},
		{"((fn() Int)?)!Str", "((fn() Int)?)!Str"},
		{"((&Int)?)!Str", "((&Int)?)!Str"},
		// Maybe of a Result or of a function type.
		{"(Int!Str)?", "(Int!Str)?"},
		{"(fn() Int)?", "(fn() Int)?"},
		{"fn()?", "fn()?"},
		{"fn() Int?", "fn() Int?"},
		// Function types returning function types.
		{"fn() (fn() Int)?", "fn() (fn() Int)?"},
		{"fn() fn() Int", "fn() fn() Int"},
		// Containers delimit their elements.
		{"[(Int!Str)!Str]", "[(Int!Str)!Str]"},
		{"[Int!Str: (Int!Str)!Str]", "[Int!Str: (Int!Str)!Str]"},
		{"((Int!Str)!Str, fn() Int)", "((Int!Str)!Str, fn() Int)"},
		{"Box<(Int!Str)!Str>", "Box<(Int!Str)!Str>"},
		// A function type with no return before `|` must stay grouped.
		{"(fn()) | Int", "(fn()) | Int"},
		{"fn() Int | Str", "fn() Int | Str"},
		{"Int!(fn()) | Str", "(Int!fn()) | Str"},
		{"Str | fn()", "Str | fn()"},
		{"[(fn()); 2]", "[(fn()); 2]"},
		{"[(Int!fn(Str)); 2]", "[(Int!fn(Str)); 2]"},
		{"[fn() Int; 2]", "[fn() Int; 2]"},
		{"[fn()?; 2]", "[fn()?; 2]"},
		{"Int!Str | (Int!Str)!Str", "Int!Str | (Int!Str)!Str"},
		// A legacy `mut T` keeps its grouping after `&` and in function parameters.
		{"&(mut Int)", "&(mut Int)"},
		{"fn((mut Int))", "fn((mut Int))"},
		{"fn(mut Int)", "fn(mut Int)"},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			if got := assertTypeRoundTrip(t, tt.source); got != tt.want {
				t.Errorf("printed %q as %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

// typeCase is a generated type source with every nested type grouped, so its
// meaning is unambiguous regardless of the printer's precedence rules.
type typeCase struct {
	source   string
	nullable bool
	void     bool
}

func generatedTypeCases() []typeCase {
	atoms := []typeCase{
		{source: "Int"},
		{source: "mod::Foo"},
		{source: "Void", void: true},
	}
	group := func(c typeCase) string { return "(" + c.source + ")" }

	unary := func(c typeCase) []typeCase {
		out := []typeCase{
			{source: "[" + group(c) + "]"},
			{source: "[" + group(c) + "; 2]"},
			{source: "&" + group(c)},
			{source: "&mut " + group(c)},
			{source: "mut " + group(c)},
			{source: "Box<" + group(c) + ">"},
			{source: "fn(" + group(c) + ")"},
			{source: "fn(mut " + group(c) + ")"},
			{source: "fn(..." + group(c) + ")"},
		}
		if !c.nullable {
			out = append(out, typeCase{source: group(c) + "?", nullable: true, void: c.void})
		}
		// `fn() Void?` is rejected by the parser.
		if !(c.void && c.nullable) {
			out = append(out, typeCase{source: "fn() " + group(c)})
		}
		return out
	}
	binary := func(a, b typeCase) []typeCase {
		out := []typeCase{
			{source: group(a) + "!" + group(b)},
			{source: "[" + group(a) + ": " + group(b) + "]"},
			{source: "(" + group(a) + ", " + group(b) + ")"},
			{source: "fn(" + group(a) + ", " + group(b) + ")"},
		}
		if !(b.void && b.nullable) {
			out = append(out, typeCase{source: "fn(" + group(a) + ") " + group(b)})
		}
		return out
	}

	level1 := append([]typeCase{}, atoms...)
	for _, a := range atoms {
		level1 = append(level1, unary(a)...)
	}
	for _, a := range atoms {
		for _, b := range atoms {
			level1 = append(level1, binary(a, b)...)
		}
	}

	level2 := append([]typeCase{}, level1...)
	for _, a := range level1 {
		level2 = append(level2, unary(a)...)
	}
	for _, a := range level1 {
		for _, b := range atoms {
			level2 = append(level2, binary(a, b)...)
			level2 = append(level2, binary(b, a)...)
		}
	}

	out := append([]typeCase{}, level2...)
	for _, a := range level2 {
		out = append(out, unary(a)...)
	}
	return out
}

// TestFormatTypeRoundTripProperty prints every generated nested type and checks
// that re-parsing the output yields the same type and that printing is stable.
func TestFormatTypeRoundTripProperty(t *testing.T) {
	t.Parallel()
	cases := generatedTypeCases()
	for _, c := range cases {
		assertTypeRoundTrip(t, c.source)
		if t.Failed() {
			// The first failure is enough to diagnose; the rest would repeat it.
			t.Fatalf("stopped at %q (of %d generated types)", c.source, len(cases))
		}
	}
	t.Logf("checked %d generated types", len(cases))
}

// TestFormatNestedResultProgram formats the program from #621 end to end.
func TestFormatNestedResultProgram(t *testing.T) {
	input := `fn flatten(r: ($T!Str)!Str) $T!Str {
  try r
}

fn main() {
  let a: (Int!Str)!Str = Result::ok(Result::ok(1))
  let _ = flatten(a)
}
`
	assertDerefFormat(t, input, input)
}
