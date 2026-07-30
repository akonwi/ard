package formatter

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestFormatReferenceUnaryExpressions(t *testing.T) {
	input := `fn main() {
let snapshot=deref reference
let field=deref reference.field
let selected=(deref reference).field
let sum=deref reference+value
let same=not deref reference==value
let shallow=deref mut value
let independent=mut deref reference
reader.deref()
Type::deref(value)
}
`
	want := `fn main() {
  let snapshot = deref reference
  let field = deref reference.field
  let selected = (deref reference).field
  let sum = deref reference + value
  let same = not deref reference == value
  let shallow = deref mut value
  let independent = mut deref reference
  reader.deref()
  Type::deref(value)
}
`

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
