package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestGenericTraitImplementationRequiresReceiverConstraints(t *testing.T) {
	want := []checker.Diagnostic{{
		Kind:    checker.Error,
		Message: "Trait implementation Contract for Box requires $T to be comparable, but Box does not provide that constraint",
	}}
	tests := []test{
		{
			name: "map key",
			input: `trait Contract {
  fn check() Bool
}

struct Box<$T> {
  value: $T,
}

impl Contract for Box {
  fn check() Bool {
    let value = self.value
    [value: true].has(value)
  }
}`,
			diagnostics: want,
		},
		{
			name: "equality",
			input: `trait Contract {
  fn check() Bool
}

struct Box<$T> {
  value: $T,
}

impl Contract for Box {
  fn check() Bool { self.value == self.value }
}`,
			diagnostics: want,
		},
		{
			name: "inequality",
			input: `trait Contract {
  fn check() Bool
}

struct Box<$T> {
  value: $T,
}

impl Contract for Box {
  fn check() Bool { self.value != self.value }
}`,
			diagnostics: want,
		},
		{
			name: "nullable equality",
			input: `trait Contract {
  fn check() Bool
}

struct Box<$T> {
  value: $T?,
}

impl Contract for Box {
  fn check() Bool { self.value == self.value }
}`,
			diagnostics: want,
		},
		{
			name: "retained closure",
			input: `trait Contract {
  fn check() Bool
}

struct Box<$T> {
  value: $T,
}

impl Contract for Box {
  fn check() Bool {
    let value = self.value
    let compare = fn() Bool { value == value }
    true
  }
}`,
			diagnostics: want,
		},
		{
			name: "retained named function",
			input: `trait Contract {
  fn check() Bool
}

struct Box<$T> {
  value: $T,
}

impl Contract for Box {
  fn check() Bool {
    fn compare(value: $T) Bool { value == value }
    true
  }
}`,
			diagnostics: want,
		},
		{
			name: "nested local type",
			input: `trait Contract {
  fn check() Bool
}

struct Keyed<$K> {
  values: [$K: Bool],
}

struct Box<$T> {
  value: $T,
}

impl Contract for Box {
  fn check() Bool {
    let absent = Maybe::new<Keyed<$T>>()
    absent.is_none()
  }
}`,
			diagnostics: want,
		},
		{
			name: "contextual local type",
			input: `trait Contract {
  fn check() Bool
}

struct Keyed<$K> {
  values: [$K: Bool],
}

struct Box<$T> {
  value: $T,
}

impl Contract for Box {
  fn check() Bool {
    let items: [Keyed<$T>] = []
    true
  }
}`,
			diagnostics: want,
		},
		{
			name: "transitive call declared later",
			input: `trait Contract {
  fn check() Bool
}

struct Box<$T> {
  value: $T,
}

impl Contract for Box {
  fn check() Bool { values_equal(self.value) }
}

fn values_equal(value: $V) Bool { value == value }
`,
			diagnostics: want,
		},
		{
			name: "transitive receiver method",
			input: `trait Contract {
  fn check() Bool
}

struct Box<$T> {
  value: $T,
}

impl Box {
  fn values_equal() Bool { self.value == self.value }
}

impl Contract for Box {
  fn check() Bool { self.values_equal() }
}`,
			diagnostics: want,
		},
		{
			name: "closure transitive call",
			input: `fn values_equal(value: $V) Bool { value == value }

trait Contract {
  fn check() Bool
}

struct Box<$T> {
  value: $T,
}

impl Contract for Box {
  fn check() Bool {
    let value = self.value
    let compare = fn() Bool { values_equal(value) }
    true
  }
}`,
			diagnostics: want,
		},
	}
	tests = append(tests, test{
		name: "transitive non-comparable binding",
		input: `fn values_equal(value: $V) Bool { value == value }

trait Contract {
  fn check() Bool
}

struct Box<$T> {
  values: [$T: Bool],
}

impl Contract for Box {
  fn check() Bool { values_equal(self.values) }
}`,
		diagnostics: []checker.Diagnostic{{
			Kind:    checker.Error,
			Message: "Trait implementation Contract for Box uses non-comparable type [$T: Bool] where comparable is required",
		}},
	})
	run(t, tests)
}

func TestGenericTraitImplementationPropagatesImportedConstraints(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "ard.toml"), []byte("name = \"constraints\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "compare.ard"), []byte(`fn values_equal(value: $V) Bool {
  value == value
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver, err := checker.NewModuleResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	source := `use constraints/compare

trait Contract {
  fn check() Bool
}

struct Box<$T> {
  value: $T,
}

impl Contract for Box {
  fn check() Bool { compare::values_equal(self.value) }
}`
	result := parse.Parse([]byte(source), filepath.Join(project, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatal(result.Errors[0].Message)
	}
	checked := checker.New(filepath.Join(project, "main.ard"), result.Program, resolver)
	checked.Check()
	for _, diagnostic := range checked.Diagnostics() {
		if diagnostic.Code == checker.DiagnosticCodeImplReceiverConstraint {
			return
		}
	}
	t.Fatalf("diagnostics = %v, want imported generic receiver constraint", checked.Diagnostics())
}

func TestGenericTraitImplementationTracksForeignStructuralComparability(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "ard.toml"), []byte("name = \"repro\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.com/repro\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(project, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte(`package ffi

type Box[T any] struct {
	Value T
}

type ComparableBox[T comparable] struct {
	Value T
}

type Checker interface {
	Check() bool
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	check := func(t *testing.T, source string) []checker.Diagnostic {
		t.Helper()
		resolver, err := checker.NewModuleResolver(project)
		if err != nil {
			t.Fatal(err)
		}
		result := parse.Parse([]byte(source), filepath.Join(project, "main.ard"))
		if len(result.Errors) > 0 {
			t.Fatal(result.Errors[0].Message)
		}
		checked := checker.New(filepath.Join(project, "main.ard"), result.Program, resolver)
		checked.Check()
		return checked.Diagnostics()
	}

	t.Run("unconstrained receiver is rejected", func(t *testing.T) {
		diagnostics := check(t, `use go:example.com/repro/ffi

fn values_equal(value: $V) Bool { value == value }
trait Contract {
  fn check() Bool
}
struct Holder<$T> {
  value: ffi::Box<$T>,
}
impl Contract for Holder {
  fn check() Bool { values_equal(self.value) }
}`)
		for _, diagnostic := range diagnostics {
			if diagnostic.Code == checker.DiagnosticCodeImplReceiverConstraint {
				return
			}
		}
		t.Fatalf("diagnostics = %v, want receiver constraint error", diagnostics)
	})

	t.Run("unsupported symbolic foreign map key is rejected", func(t *testing.T) {
		diagnostics := check(t, `use go:example.com/repro/ffi

trait Contract {
  fn check() Bool
}
struct Holder<$T> {
  key: ffi::Box<$T>,
}
impl Contract for Holder {
  fn check() Bool {
    let key = self.key
    [key: true].has(key)
  }
}`)
		for _, diagnostic := range diagnostics {
			if diagnostic.Code == checker.DiagnosticCodeInvalidMapKeyType {
				return
			}
		}
		t.Fatalf("diagnostics = %v, want unsupported foreign map-key error", diagnostics)
	})

	t.Run("unsupported symbolic foreign map field is rejected", func(t *testing.T) {
		diagnostics := check(t, `use go:example.com/repro/ffi

struct Index<$T> {
  values: [ffi::Box<$T>: Bool],
}`)
		for _, diagnostic := range diagnostics {
			if diagnostic.Code == checker.DiagnosticCodeInvalidMapKeyType {
				return
			}
		}
		t.Fatalf("diagnostics = %v, want unsupported foreign map-field error", diagnostics)
	})

	t.Run("declared foreign comparable constraint supports symbolic map field", func(t *testing.T) {
		diagnostics := check(t, `use go:example.com/repro/ffi

struct Index<$T> {
  values: [ffi::ComparableBox<$T>: Bool],
}`)
		if len(diagnostics) > 0 {
			t.Fatalf("unexpected diagnostics: %v", diagnostics)
		}
	})

	t.Run("generic Go interface implementation is rejected", func(t *testing.T) {
		diagnostics := check(t, `use go:example.com/repro/ffi

struct NativeBox<$T> {
  value: $T,
}
impl ffi::Checker for NativeBox {
  fn check() Bool { self.value == self.value }
}`)
		for _, diagnostic := range diagnostics {
			if diagnostic.Code == checker.DiagnosticCodeImplReceiverConstraint {
				return
			}
		}
		t.Fatalf("diagnostics = %v, want Go interface receiver constraint error", diagnostics)
	})

	t.Run("receiver-provided constraint is accepted", func(t *testing.T) {
		diagnostics := check(t, `use go:example.com/repro/ffi

fn values_equal(value: $V) Bool { value == value }
trait Contract {
  fn check() Bool
}
struct Index<$T> {
  values: [$T: Bool],
  key: $T,
}
impl Contract for Index {
  fn check() Bool {
    let pair = ffi::Box<$T>{Value: self.key}
    values_equal(pair)
  }
}`)
		if len(diagnostics) > 0 {
			t.Fatalf("unexpected diagnostics: %v", diagnostics)
		}
	})

	t.Run("Go interface receiver-provided constraint is accepted", func(t *testing.T) {
		diagnostics := check(t, `use go:example.com/repro/ffi

struct NativeIndex<$T> {
  values: [$T: Bool],
  value: $T,
}
impl ffi::Checker for NativeIndex {
  fn check() Bool { self.value == self.value }
}`)
		if len(diagnostics) > 0 {
			t.Fatalf("unexpected diagnostics: %v", diagnostics)
		}
	})
}

func TestGenericTraitImplementationAcceptsReceiverConstraints(t *testing.T) {
	run(t, []test{
		{
			name: "map key field provides comparable receiver parameter",
			input: `trait Contains {
  fn contains() Bool
}

struct Index<$T> {
  values: [$T: Bool],
  key: $T,
}

impl Contains for Index {
  fn contains() Bool { self.values.has(self.key) }
}

fn main() Bool {
  let value: Contains = Index<Int>{values: [1: true], key: 1}
  value.contains()
}`,
		},
		{
			name: "comparable union binding",
			input: `fn values_equal(value: $V) Bool { value == value }

type Scalar = Int | Str

trait Contract {
  fn check() Bool
}

struct Box<$T> {
  value: $T,
  scalar: Scalar,
}

impl Contract for Box {
  fn check() Bool { values_equal(self.scalar) }
}`,
		},
	})
}
