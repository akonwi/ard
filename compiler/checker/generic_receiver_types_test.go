package checker_test

import (
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func checkGenericReceiverSource(t *testing.T, source string) []checker.Diagnostic {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checked := checker.New("test.ard", result.Program, nil)
	checked.Check()
	return checked.Diagnostics()
}

func requireNoGenericReceiverDiagnostics(t *testing.T, source string) {
	t.Helper()
	if diagnostics := checkGenericReceiverSource(t, source); len(diagnostics) > 0 {
		t.Fatalf("expected no diagnostics, got %#v", diagnostics)
	}
}

func diagnosticLabelMessages(diagnostic checker.Diagnostic) []string {
	messages := []string{diagnostic.Primary.Message}
	for _, label := range diagnostic.Secondary {
		messages = append(messages, label.Message)
	}
	return messages
}

func requireDiagnosticLabels(t *testing.T, source string, title string, want ...string) {
	t.Helper()
	diagnostics := checkGenericReceiverSource(t, source)
	for _, diagnostic := range diagnostics {
		if diagnostic.Title != title {
			continue
		}
		messages := diagnosticLabelMessages(diagnostic)
		if !containsAllStrings(messages, want) {
			t.Fatalf("%q labels = %q, want them to include %q", title, messages, want)
		}
		return
	}
	t.Fatalf("expected a %q diagnostic, got %#v", title, diagnostics)
}

func containsAllStrings(messages []string, want []string) bool {
	for _, expected := range want {
		found := false
		for _, message := range messages {
			if message == expected {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

const genericReceiverBox = `struct Box<$T> {
  items: [$T],
}
`

// Issue #618: inside a method on a generic struct, `self` is the struct
// applied to its receiver generics (`Box<$T>`, or `&mut Box<$T>` in a
// `fn mut` method), not the bare declaration `Box`.
func TestGenericReceiverSelfIsTheAppliedStruct(t *testing.T) {
	t.Run("fn mut self passes as &mut Box<$T>", func(t *testing.T) {
		requireNoGenericReceiverDiagnostics(t, genericReceiverBox+`
impl Box {
  fn mut forward(item: $T) {
    touch(self, item)
  }
}

fn touch(b: &mut Box<$T>, item: $T) {
  b.items.push(item)
}
`)
	})

	t.Run("fn mut self binds to an explicit &mut Box<$T>", func(t *testing.T) {
		requireNoGenericReceiverDiagnostics(t, genericReceiverBox+`
impl Box {
  fn mut forward() {
    let ns: &mut Box<$T> = self
    touch(ns)
  }
}

fn touch(b: &mut Box<$T>) {}
`)
	})

	t.Run("read-only self passes as Box<$T> and returns as Box<$T>", func(t *testing.T) {
		requireNoGenericReceiverDiagnostics(t, genericReceiverBox+`
impl Box {
  fn count() Int {
    size(self)
  }

  fn same() Box<$T> {
    self
  }
}

fn size(b: Box<$T>) Int {
  b.items.size()
}
`)
	})

	t.Run("trait implementation self passes as Box<$T>", func(t *testing.T) {
		requireNoGenericReceiverDiagnostics(t, `trait Describe {
  fn describe() Str
}
`+genericReceiverBox+`
impl Describe for Box {
  fn describe() Str {
    label(self)
  }
}

fn label(b: Box<$T>) Str {
  "box"
}
`)
	})

	t.Run("self is not a fixed specialization", func(t *testing.T) {
		requireDiagnosticLabels(t, genericReceiverBox+`
impl Box {
  fn mut forward() {
    ints(self)
  }
}

fn ints(b: &mut Box<Int>) {}
`, "Incorrect argument type",
			"this argument has type `&mut Box<$T>`",
			"parameter `b` requires `&mut Box<Int>`",
		)
	})

	t.Run("&mut self names the receiver's full type", func(t *testing.T) {
		diagnostics := checkGenericReceiverSource(t, genericReceiverBox+`
impl Box {
  fn mut forward() {
    let ns = &mut self
  }
}
`)
		for _, diagnostic := range diagnostics {
			if diagnostic.Title == "Pointer to a pointer" {
				if !strings.Contains(diagnostic.Primary.Message, "`&mut Box<$T>`") {
					t.Fatalf("primary label = %q, want it to name `&mut Box<$T>`", diagnostic.Primary.Message)
				}
				return
			}
		}
		t.Fatalf("expected a pointer-to-pointer diagnostic, got %#v", diagnostics)
	})
}

// A structural mismatch against a generic parameter labels the whole
// argument and the whole parameter, keeping `&mut` and type arguments
// (issue #618) instead of the inner component where unification failed.
func TestGenericArgumentMismatchLabelsWholeTypes(t *testing.T) {
	t.Run("reference argument", func(t *testing.T) {
		requireDiagnosticLabels(t, genericReceiverBox+`
struct Plain {
  n: Int,
}

impl Plain {
  fn mut forward() {
    touch(self)
  }
}

fn touch(b: &mut Box<$T>) {}
`, "Incorrect argument type",
			"this argument has type `&mut Plain`",
			"parameter `b` requires `&mut Box<$T>`",
		)
	})

	t.Run("list argument", func(t *testing.T) {
		requireDiagnosticLabels(t, genericReceiverBox+`
struct Plain {
  n: Int,
}

fn many(b: [Box<$T>]) {}

fn main() {
  let plains = [Plain{n: 1}]
  many(plains)
}
`, "Incorrect argument type",
			"this argument has type `[Plain]`",
			"parameter `b` requires `[Box<$T>]`",
		)
	})

	t.Run("earlier binding is still related", func(t *testing.T) {
		requireDiagnosticLabels(t, `fn pair(first: $T, rest: [$T]) {}

fn main() {
  let words = ["a"]
  pair(1, words)
}
`, "Incorrect argument type",
			"this argument has type `[Str]`",
			"earlier argument establishes `$T` as `Int`",
			"parameter `rest` requires `[Int]`",
		)
	})
}

// Issue #619: once a generic call's return type is fully determined, the
// expected type is a compatibility obligation, so the result flows into an
// `Any` or trait destination like a non-generic call result.
func TestDeterminedGenericReturnMeetsCompatibleExpectation(t *testing.T) {
	const prelude = `trait Describe {
  fn describe() Str
}

struct R {
  n: Int,
}

impl Describe for R {
  fn describe() Str {
    "r"
  }
}
` + genericReceiverBox + `
impl Box {
  fn reports() [R] {
    [R{n: 1}]
  }

  fn first_report() R {
    R{n: 1}
  }
}

fn reports_for(seed: $T) [R] {
  [R{n: 1}]
}

fn identity(value: $T) $T {
  value
}

fn take(v: Any) {}

fn show(v: Describe) {}
`

	t.Run("method result on generic receiver as Any argument", func(t *testing.T) {
		requireNoGenericReceiverDiagnostics(t, prelude+`
fn main() {
  let b = Box<Int>{items: []}
  take(b.reports())
}
`)
	})

	t.Run("generic function result as Any argument", func(t *testing.T) {
		requireNoGenericReceiverDiagnostics(t, prelude+`
fn main() {
  take(reports_for(1))
}
`)
	})

	t.Run("generic function result as Any annotation", func(t *testing.T) {
		requireNoGenericReceiverDiagnostics(t, prelude+`
fn main() {
  let value: Any = reports_for(1)
}
`)
	})

	t.Run("method result on generic receiver as trait argument", func(t *testing.T) {
		requireNoGenericReceiverDiagnostics(t, prelude+`
fn main() {
  let b = Box<Int>{items: []}
  show(b.first_report())
  show(identity(R{n: 2}))
}
`)
	})

	t.Run("incompatible determined return still conflicts", func(t *testing.T) {
		requireDiagnosticLabels(t, prelude+`
fn main() {
  let value: Str = identity(1)
}
`, "Type mismatch",
			"earlier argument establishes `$T` as `Int`",
		)
	})
}
