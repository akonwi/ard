package formatter

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestFormatVariadicSpreadArguments(t *testing.T) {
	input := `fn main() {
call(reference...)
call((mut values)...)
call(
fixed,
reference...
)
}
`
	want := `fn main() {
  call(reference...)
  call((mut values)...)
  call(fixed, reference...)
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
