package checker_test

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// checkSource type-checks source in a temp project and returns diagnostics.
func checkSource(t *testing.T, source string) []checker.Diagnostic {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	resolver, err := checker.NewModuleResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New("test.ard", result.Program, resolver)
	c.Check()
	return c.Diagnostics()
}

func wantError(t *testing.T, diags []checker.Diagnostic, contains string) {
	t.Helper()
	for _, d := range diags {
		if d.Kind == checker.Error && strings.Contains(d.Message, contains) {
			return
		}
	}
	t.Fatalf("expected error containing %q, got %v", contains, diags)
}

func wantClean(t *testing.T, diags []checker.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Kind == checker.Error {
			t.Fatalf("expected no errors, got %v", diags)
		}
	}
}

// Field assignment must type-check the assigned value against the field type
// for native structs, not only foreign (Go) fields. Regression test for the
// soundness hole found by LSP validation against examples/vaxis-demo.
func TestFieldAssignmentChecksValueType(t *testing.T) {
	t.Run("direct mismatch", func(t *testing.T) {
		wantError(t, checkSource(t, `struct S {
  n: Int,
}

fn main() {
  mut s = S{n: 1}
  s.n = "oops"
}
`), "Type mismatch: Expected Int, got Str")
	})

	t.Run("through mut parameter", func(t *testing.T) {
		wantError(t, checkSource(t, `struct S {
  n: Int,
}

fn f(mut s: S) {
  s.n = "oops"
}

fn main() {
  mut s = S{n: 1}
  f(s)
}
`), "Type mismatch: Expected Int, got Str")
	})

	t.Run("nested field", func(t *testing.T) {
		wantError(t, checkSource(t, `struct Inner {
  n: Int,
}

struct Outer {
  inner: Inner,
}

fn main() {
  mut o = Outer{inner: Inner{n: 1}}
  o.inner.n = "oops"
}
`), "Type mismatch: Expected Int, got Str")
	})

	t.Run("valid assignment stays clean", func(t *testing.T) {
		wantClean(t, checkSource(t, `struct S {
  n: Int,
}

fn main() {
  mut s = S{n: 1}
  s.n = 2
}
`))
	})
}

// Assigning a bare T into a T? field wraps into Maybe, matching struct
// literal and call-argument behavior — for literals, variables, and
// Maybe-typed values alike.
func TestFieldAssignmentMaybeWrapping(t *testing.T) {
	t.Run("literal into nullable field", func(t *testing.T) {
		wantClean(t, checkSource(t, `struct S {
  label: Str?,
}

fn main() {
  mut s = S{label: "one"}
  s.label = "two"
}
`))
	})

	t.Run("variable into nullable field", func(t *testing.T) {
		wantClean(t, checkSource(t, `struct S {
  label: Str?,
}

fn main() {
  mut s = S{label: "one"}
  let name = "two"
  s.label = name
}
`))
	})

	t.Run("maybe value into nullable field", func(t *testing.T) {
		wantClean(t, checkSource(t, `use ard/maybe

struct S {
  label: Str?,
}

fn main() {
  mut a = S{label: "one"}
  mut b = S{label: "two"}
  a.label = b.label
  a.label = maybe::none()
}
`))
	})

	t.Run("wrong inner type still rejected", func(t *testing.T) {
		wantError(t, checkSource(t, `struct S {
  label: Str?,
}

fn main() {
  mut s = S{label: "one"}
  s.label = 42
}
`), "Type mismatch")
	})
}
