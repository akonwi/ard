package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// Go interface values do not support == or != in Ard (ADR 0085). Go's own
// interface comparison panics when the dynamic value is not comparable, so the
// checker rejects it and the diagnostic points at a match type pattern
// (ADR 0042), which tests the dynamic type without a panic path.
func TestGoInterfaceEqualityIsRejectedWithMatchHint(t *testing.T) {
	const genericHint = "Go interface values do not support `==`. To test the value inside, match on its dynamic type with a type pattern, like `match value { pkg::Type(v) => ..., _ => ... }`"
	tests := []struct {
		name   string
		source string
		legacy string
		text   string
	}{
		{
			name: "interface compared with a concrete implementing value",
			source: `use go:fmt
use go:time

fn is_january(value: fmt::Stringer) Bool {
  value == time::January
}
`,
			legacy: "Invalid: fmt::Stringer == time::Month",
			text:   "`fmt::Stringer` is a Go interface, which does not support `==`. To test for a `time::Month`, match on the dynamic type: `match value { time::Month(v) => v == other, _ => false }`",
		},
		{
			name: "concrete value on the left with !=",
			source: `use go:fmt
use go:time

fn not_january(value: fmt::Stringer) Bool {
  time::January != value
}
`,
			legacy: "Invalid: time::Month != fmt::Stringer",
			text:   "`fmt::Stringer` is a Go interface, which does not support `!=`. To test for a `time::Month`, match on the dynamic type: `match value { time::Month(v) => v != other, _ => true }`",
		},
		{
			// os::Interrupt is itself declared with the os::Signal interface type.
			name: "two interface values",
			source: `use go:os

fn is_interrupt(sig: os::Signal) Bool {
  sig == os::Interrupt
}
`,
			legacy: "Invalid: os::Signal == os::Signal",
			text:   genericHint,
		},
		{
			name: "interface compared with a pointer implementation",
			source: `use go:io
use go:os

fn is_stdout(w: io::Writer) Bool {
  w == os::Stdout
}
`,
			legacy: "Invalid: io::Writer == os::File",
			text:   genericHint,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := checker.New("main.ard", checker.WrapStatementRuns(result.Program), nil, checker.CheckOptions{GoResolver: standardLibraryGoResolver(t)})
			c.Check()
			diagnostics := c.Diagnostics()
			if len(diagnostics) != 1 {
				t.Fatalf("diagnostics = %#v, want exactly one", diagnostics)
			}
			diagnostic := requireDiagnosticCode(t, diagnostics, checker.DiagnosticCodeInvalidEqualityOperation)
			if diagnostic.Message != tt.legacy {
				t.Fatalf("message = %q, want %q", diagnostic.Message, tt.legacy)
			}
			if diagnostic.Text != tt.text {
				t.Fatalf("text = %q\nwant  %q", diagnostic.Text, tt.text)
			}
		})
	}
}

func TestNonInterfaceEqualityHasNoMatchHint(t *testing.T) {
	result := parse.Parse([]byte("[1] == [1]\n"), "main.ard")
	c := checker.New("main.ard", checker.WrapStatementRuns(result.Program), nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeInvalidEqualityOperation)
	if diagnostic.Text != "" {
		t.Fatalf("text = %q, want none", diagnostic.Text)
	}
}

// The match form the hint suggests is accepted by the checker.
func TestGoInterfaceMatchTypePatternComparesDynamicValue(t *testing.T) {
	run(t, []test{
		{
			name: "match type pattern compares the narrowed value",
			input: `use go:fmt
use go:time

fn is_january(value: fmt::Stringer) Bool {
  match value {
    time::Month(month) => month == time::January,
    _ => false,
  }
}`,
		},
	})
}
