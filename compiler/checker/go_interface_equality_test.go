package checker_test

import (
	"os"
	"path/filepath"
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
			// The pattern uses the file's import alias, not the Go package name.
			name: "concrete value from an aliased import",
			source: `use go:fmt
use go:time as t

fn is_january(value: fmt::Stringer) Bool {
  value == t::January
}
`,
			legacy: "Invalid: fmt::Stringer == time::Month",
			text:   "`fmt::Stringer` is a Go interface, which does not support `==`. To test for a `t::Month`, match on the dynamic type: `match value { t::Month(v) => v == other, _ => false }`",
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
		{
			name: "match type pattern through an import alias",
			input: `use go:fmt
use go:time as t

fn is_january(value: fmt::Stringer) Bool {
  match value {
    t::Month(month) => month == t::January,
    _ => false,
  }
}`,
		},
	})
}

// The exact hint names a type pattern only when this file can spell it: an
// imported package must export a name for the type, either its own name or a
// Go alias. Otherwise the general hint is used.
func TestGoInterfaceEqualityHintOnlyNamesWritablePatterns(t *testing.T) {
	root := t.TempDir()
	writeGoInterfaceEqualityPackage(t, root)
	resolver := checker.NewGoPackagesResolver(root, nil)
	if err := resolver.Prime([]string{"example.com/app/ffi/labels", "example.com/app/ffi/calendar"}); err != nil {
		t.Fatalf("prime resolver: %v", err)
	}
	const genericHint = "Go interface values do not support `==`. To test the value inside, match on its dynamic type with a type pattern, like `match value { pkg::Type(v) => ..., _ => ... }`"
	tests := []struct {
		name   string
		source string
		legacy string
		text   string
	}{
		{
			name: "exported type from an imported package",
			source: `use go:example.com/app/ffi/labels as lib

fn is_primary(value: lib::Labeler) Bool {
  value == lib::Primary
}
`,
			legacy: "Invalid: labels::Labeler == labels::Kind",
			text:   "`labels::Labeler` is a Go interface, which does not support `==`. To test for a `lib::Kind`, match on the dynamic type: `match value { lib::Kind(v) => v == other, _ => false }`",
		},
		{
			name: "unexported type returned by an exported function",
			source: `use go:example.com/app/ffi/labels

fn is_hidden(value: labels::Labeler) Bool {
  value == labels::Hidden()
}
`,
			legacy: "Invalid: labels::Labeler == labels::level",
			text:   genericHint,
		},
		{
			name: "type from a package this file does not import",
			source: `use go:example.com/app/ffi/labels

fn is_january(value: labels::Namer) Bool {
  value == labels::FirstMonth()
}
`,
			legacy: "Invalid: labels::Namer == time::Month",
			text:   genericHint,
		},
		{
			// An exported Go alias of an unexported type is a valid pattern.
			name: "unexported type reachable through an exported alias",
			source: `use go:example.com/app/ffi/labels

fn is_tagged(value: labels::Labeler) Bool {
  value == labels::Tagged()
}
`,
			legacy: "Invalid: labels::Labeler == labels::tag",
			text:   "`labels::Labeler` is a Go interface, which does not support `==`. To test for a `labels::Tag`, match on the dynamic type: `match value { labels::Tag(v) => v == other, _ => false }`",
		},
		{
			// A re-export lets the pattern name a type from a package the
			// module does not import directly.
			name: "type re-exported from another package",
			source: `use go:example.com/app/ffi/calendar
use go:example.com/app/ffi/labels

fn is_january(value: labels::Namer) Bool {
  value == calendar::First()
}
`,
			legacy: "Invalid: labels::Namer == time::Month",
			text:   "`labels::Namer` is a Go interface, which does not support `==`. To test for a `calendar::MonthOfYear`, match on the dynamic type: `match value { calendar::MonthOfYear(v) => v == other, _ => false }`",
		},
		{
			// Named empty interfaces keep their Go type but are outside the rule.
			name: "named empty interface gets no hint",
			source: `use go:example.com/app/ffi/labels

fn same(a: labels::Anything, b: labels::Anything) Bool {
  a == b
}
`,
			legacy: "Invalid: labels::Anything == labels::Anything",
			text:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := checker.New("main.ard", checker.WrapStatementRuns(result.Program), nil, checker.CheckOptions{GoResolver: resolver})
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

// The alias spellings the hint suggests are accepted as type patterns.
func TestGoInterfaceEqualityHintAliasPatternsCheck(t *testing.T) {
	root := t.TempDir()
	writeGoInterfaceEqualityPackage(t, root)
	resolver := checker.NewGoPackagesResolver(root, nil)
	if err := resolver.Prime([]string{"example.com/app/ffi/labels", "example.com/app/ffi/calendar"}); err != nil {
		t.Fatalf("prime resolver: %v", err)
	}
	source := `use go:example.com/app/ffi/calendar
use go:example.com/app/ffi/labels

fn is_tagged(value: labels::Labeler) Bool {
  match value {
    labels::Tag(v) => v == labels::Tagged(),
    _ => false,
  }
}

fn is_january(value: labels::Namer) Bool {
  match value {
    calendar::MonthOfYear(v) => v == calendar::First(),
    _ => false,
  }
}
`
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", checker.WrapStatementRuns(result.Program), nil, checker.CheckOptions{GoResolver: resolver})
	c.Check()
	if diagnostics := c.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
}

func writeGoInterfaceEqualityPackage(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "ffi", "labels")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := `package labels

import "time"

type Labeler interface {
	Label() string
}

type Anything interface{}

type Namer interface {
	String() string
}

type Kind int

const Primary Kind = 1

func (Kind) Label() string { return "kind" }

type level int

func (level) Label() string { return "level" }

func Hidden() level { return 1 }

func FirstMonth() time.Month { return time.January }

type tag int

func (tag) Label() string { return "tag" }

type Tag = tag

func Tagged() Tag { return 1 }
`
	if err := os.WriteFile(filepath.Join(dir, "labels.go"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	calendarDir := filepath.Join(root, "ffi", "calendar")
	if err := os.MkdirAll(calendarDir, 0o755); err != nil {
		t.Fatal(err)
	}
	calendar := `package calendar

import "time"

type MonthOfYear = time.Month

func First() MonthOfYear { return time.January }
`
	if err := os.WriteFile(filepath.Join(calendarDir, "calendar.go"), []byte(calendar), 0o644); err != nil {
		t.Fatal(err)
	}
}
