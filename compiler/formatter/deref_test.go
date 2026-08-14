package formatter

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestFormatPostfixDereferenceExpressions(t *testing.T) {
	input := `fn main() {
let snapshot=reference.@
let selected=reference.@.field
let field=reference.field.@
let sum=reference.@+value
let same=not reference.@==value
let shallow=(mut value).@
let independent=mut reference.@
let nested=reference.@.@
let loaded=load().@
let invoked=reference.@()
reader.deref()
Type::deref(value)
}
`
	want := `fn main() {
  let snapshot = reference.@
  let selected = reference.@.field
  let field = reference.field.@
  let sum = reference.@ + value
  let same = not reference.@ == value
  let shallow = (mut value).@
  let independent = mut reference.@
  let nested = reference.@.@
  let loaded = load().@
  let invoked = reference.@()
  reader.deref()
  Type::deref(value)
}
`

	assertDerefFormat(t, input, want)
}

func TestFormatMigratesLegacyDerefSyntax(t *testing.T) {
	input := `fn main() {
let snapshot=deref reference
let field=deref reference.field
let selected=(deref reference).field
let sum=deref reference+value
let same=not deref reference==value
let shallow=deref mut value
let independent=mut deref reference
let nested=deref deref reference
let loaded=deref load()
let invoked=(deref reference)()
}
`
	want := `fn main() {
  let snapshot = reference.@
  let field = reference.field.@
  let selected = reference.@.field
  let sum = reference.@ + value
  let same = not reference.@ == value
  let shallow = (mut value).@
  let independent = mut reference.@
  let nested = reference.@.@
  let loaded = load().@
  let invoked = reference.@()
}
`

	assertDerefFormat(t, input, want)
}

func TestFormatPostfixDerefPreservesStructuredMutOperand(t *testing.T) {
	input := `fn main() {
let snapshot=(mut [
ui::TextSpan{Text:"  "},
ui::TextSpan{Text:"A very long value that keeps this structured mutable list on multiple lines"},
]).@
}
`
	want := `fn main() {
  let snapshot = (mut [
    ui::TextSpan{Text: "  "},
    ui::TextSpan{Text: "A very long value that keeps this structured mutable list on multiple lines"},
  ]).@
}
`

	assertDerefFormat(t, input, want)
}

func TestFormatPostfixDerefPreservesComplexOperandParentheses(t *testing.T) {
	input := `fn main() {
let snapshot=(match true {
true=>first,
false=>second,
}).@
let callable=(fn() Int {
1
}).@
let borrowed=mut (match true {
true=>first,
false=>second,
})
}
`
	want := `fn main() {
  let snapshot = (match true {
    true => first,
    false => second,
  }).@
  let callable = (fn() Int {
    1
  }).@
  let borrowed = mut (match true {
    true => first,
    false => second,
  })
}
`

	assertDerefFormat(t, input, want)
}

func TestFormatPostfixDerefPreservesCallableMutOperand(t *testing.T) {
	input := `fn main() {
let callable=(mut (fn() Int {
1
})).@
}
`
	want := `fn main() {
  let callable = (mut (fn() Int {
    1
  })).@
}
`

	assertDerefFormat(t, input, want)
}

func TestFormatPostfixDerefCallPreservesStructuredCallee(t *testing.T) {
	input := `fn main() {
let invoked=load_callback("A very long callback name or argument that forces the inner call to use its multiline layout").@()
}
`
	want := `fn main() {
  let invoked = load_callback(
    "A very long callback name or argument that forces the inner call to use its multiline layout",
  ).@()
}
`

	assertDerefFormat(t, input, want)
}

func assertDerefFormat(t *testing.T, input, want string) {
	t.Helper()
	formatted, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if string(formatted) != want {
		t.Fatalf("formatted:\n%s\nwant:\n%s", formatted, want)
	}
	if result := parse.Parse(formatted, "test.ard"); len(result.Errors) > 0 {
		t.Fatalf("formatted output does not parse: %v", result.Errors)
	}
	again, err := Format(formatted, "test.ard")
	if err != nil {
		t.Fatalf("second format: %v", err)
	}
	if string(again) != want {
		t.Fatalf("formatter is not idempotent:\n%s", again)
	}
}
