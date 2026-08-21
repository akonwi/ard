package gotarget

import (
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/frontend"
	"github.com/akonwi/ard/parse"
)

func TestGenericStructTraitImplementationDispatchesForConcreteApplications(t *testing.T) {
	program := lowerParitySource(t, `
		trait View {
			fn label() Str
		}

		struct Box<$T> {
			value: $T,
			name: Str,
		}

		impl View for Box {
			fn label() Str { self.name }
		}

		fn main() Str {
			let int_view: View = Box<Int>{value: 1, name: "int"}
			let str_view: View = Box<Str>{value: "value", name: "str"}
			"{int_view.label()}:{str_view.label()}"
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `"int:str"` {
		t.Fatalf("result = %s, want generic trait dispatch for both applications", got)
	}
}

func TestGenericDirectTraitCallKeepsNativeInterfaceMethod(t *testing.T) {
	program := lowerParitySource(t, `
		trait View {
			fn label() Str
		}
		struct Box<$T> {
			value: $T,
		}
		impl View for Box {
			fn label() Str { "box" }
		}
		fn direct(box: Box<Int>) Str { box.label() }
		fn dispatch(view: View) Str { view.label() }
	`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	receiverMethods := 0
	fallbackMethods := 0
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			fn, ok := node.(*ast.FuncDecl)
			if !ok || fn.Name == nil || fn.Recv == nil {
				return true
			}
			switch fn.Name.Name {
			case "Label":
				receiverMethods++
			case "ArdTraitMethod_0_0":
				fallbackMethods++
			}
			return true
		})
	}
	if receiverMethods != 1 || fallbackMethods != 0 {
		t.Fatalf("receiver methods Label=%d fallback=%d, want one native method", receiverMethods, fallbackMethods)
	}
}

func TestGenericTraitImplementationCanProjectMutatingSelf(t *testing.T) {
	program := lowerParitySource(t, `
		trait View {
			fn mut self_ref() mut View
			fn mut set(value: Int)
			fn value() Int
		}

		struct Box<$T> {
			marker: $T,
			number: Int,
		}

		impl View for Box {
			fn mut self_ref() mut View { self }
			fn mut set(value: Int) { self.number = value }
			fn value() Int { self.number }
		}

		fn main() Bool {
			let box = Box<Str>{marker: "box", number: 1}
			let reference = mut box
			let projected = reference.self_ref()
			projected.set(7)
			projected == reference and projected.value() == 7 and box.number == 7
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `true` {
		t.Fatalf("result = %s, want projected generic self to preserve identity", got)
	}
}

func TestGenericTraitMethodRejectsConstraintsMissingFromReceiver(t *testing.T) {
	program := lowerGenericTraitConstraintSource(t, `
		trait Contains {
			fn contains() Bool
		}
		struct Box<$T> {
			value: $T,
		}
		impl Contains for Box {
			fn contains() Bool {
				let value = self.value
				[value: true].has(value)
			}
		}
	`)
	_, err := lowerProgram(program, Options{PackageName: "main"})
	if err == nil || !strings.Contains(err.Error(), "requires constraints not provided by receiver") {
		t.Fatalf("lower error = %v, want missing generic receiver constraint", err)
	}
}

func TestGenericTraitMethodRejectsEqualityConstraintsMissingFromReceiver(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "equality",
			source: `trait Same {
  fn same() Bool
}
struct Box<$T> {
  value: $T,
}
impl Same for Box {
  fn same() Bool { self.value == self.value }
}`,
		},
		{
			name: "inequality",
			source: `trait Same {
  fn same() Bool
}
struct Box<$T> {
  value: $T,
}
impl Same for Box {
  fn same() Bool { self.value != self.value }
}`,
		},
		{
			name: "closure equality",
			source: `trait Same {
  fn same() Bool
}
struct Box<$T> {
  value: $T,
}
impl Same for Box {
  fn same() Bool {
    let value = self.value
    let compare = fn() Bool { value == value }
    compare()
  }
}`,
		},
		{
			name: "transitive equality",
			source: `fn values_equal(value: $T) Bool { value == value }
trait Same {
  fn same() Bool
}
struct Box<$T> {
  value: $T,
}
impl Same for Box {
  fn same() Bool { values_equal(self.value) }
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := lowerGenericTraitConstraintSource(t, tt.source)
			_, err := lowerProgram(program, Options{PackageName: "main"})
			if err == nil || !strings.Contains(err.Error(), "requires constraints not provided by receiver") {
				t.Fatalf("lower error = %v, want missing generic receiver constraint", err)
			}
		})
	}
}

func lowerGenericTraitConstraintSource(t *testing.T, source string) *air.Program {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checked := checker.New("test.ard", result.Program, nil)
	checked.Check()
	found := false
	for _, diagnostic := range checked.Diagnostics() {
		if diagnostic.Code == checker.DiagnosticCodeImplReceiverConstraint {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("checker diagnostics = %v, want generic receiver constraint diagnostic", checked.Diagnostics())
	}
	program, err := air.Lower(checked.Module())
	if err != nil {
		t.Fatalf("AIR lower error: %v", err)
	}
	return program
}

func TestGenericTraitMethodAcceptsConstraintsProvidedByReceiver(t *testing.T) {
	program := lowerParitySource(t, `
		trait Contains {
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
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `true` {
		t.Fatalf("result = %s, want receiver-provided comparable constraint", got)
	}
}

func TestImportedGenericStructTraitImplementationDispatches(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"generictraits\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "views.ard"), []byte(`trait View {
  fn value() Int
}

fn read(view: View) Int { view.value() }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "models.ard"), []byte(`use generictraits/views

struct Box<$T> {
  marker: $T,
  number: Int,
}

impl views::View for Box {
  fn value() Int { self.number }
}

struct Marker<$T> {}
impl views::View for Marker {
  fn value() Int { 11 }
}

struct Failure<$T> {}
impl Error for Failure {
  fn error() Str {
    let current: Error = self
    "failure"
  }
}

fn make_int() views::View { Box<Int>{marker: 1, number: 7} }
fn make_marker() views::View { Marker<Str>{} }
fn make_error() Error { Failure<Int>{} }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use generictraits/views
use generictraits/models

fn main() {
  if not views::read(models::make_int()) == 7 { panic("imported generic result") }
  if not views::read(models::make_marker()) == 11 { panic("imported phantom generic result") }
  let failure = models::make_error()
  if not "{failure}" == "failure" { panic("imported phantom generic Error") }
  let direct: views::View = models::Box<Str>{marker: "box", number: 9}
  if not views::read(direct) == 9 { panic("imported generic projection") }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
}

func TestGenericRequiredGoMethodSeesOrdinaryTraitImplementation(t *testing.T) {
	projectDir := t.TempDir()
	files := map[string]string{
		"ard.toml": "name = \"genericrequired\"\nard = \">= 0.1.0\"\n",
		"go.mod":   "module genericrequired\n\ngo 1.26\n",
		"ffi/ffi.go": `package ffi

type Stringer interface {
	String() string
}

func CallString(value Stringer) string { return value.String() }
`,
		"main.ard": `use go:genericrequired/ffi

trait View {
  fn label() Str
}
struct Box<$T> {
  value: $T,
}
impl View for Box {
  fn label() Str { "view" }
}
impl ffi::Stringer for Box {
  fn string() Str {
    let current: View = self
    current.label()
  }
}
fn main() {
  let box = Box<Int>{value: 1}
  if not ffi::CallString(box) == "view" { panic("generic required method trait projection") }
}
`,
	}
	for name, content := range files {
		path := filepath.Join(projectDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
}

func TestDuplicateGenericTraitImplementationKeepsOneDispatchMethod(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "native interface",
			source: `trait View {
  fn label() Str
}
struct Box<$T> {
  value: $T,
}
impl View for Box {
  fn label() Str { "first" }
}
impl View for Box {
  fn label() Str { "second" }
}
fn main() Str {
  let value: View = Box<Int>{value: 1}
  value.label()
}`,
			want: `"second"`,
		},
		{
			name: "fallback interface",
			source: `trait View {
  fn label() Str
}
struct Box<$T> {
  value: $T,
  label: Str,
}
impl View for Box {
  fn label() Str { self.label }
}
impl View for Box {
  fn label() Str { self.label }
}
fn main() Str {
  let value: View = Box<Int>{value: 1, label: "field"}
  value.label()
}`,
			want: `"field"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := lowerParitySource(t, tt.source)
			if len(program.Impls) != 1 {
				t.Fatalf("impl count = %d, want one", len(program.Impls))
			}
			if got := runGoTargetParityJSON(t, program); got != tt.want {
				t.Fatalf("result = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestGenericSameNamedTraitMethodsKeepOwnBodies(t *testing.T) {
	implementations := []string{
		`impl Reader for Device {
			fn act() Str { "read" }
		}
		impl Writer for Device {
			fn act() Str { "write" }
		}`,
		`impl Writer for Device {
			fn act() Str { "write" }
		}
		impl Reader for Device {
			fn act() Str { "read" }
		}`,
	}
	for _, impls := range implementations {
		program := lowerParitySource(t, `
			trait Reader {
				fn act() Str
			}
			trait Writer {
				fn act() Str
			}
			struct Device<$T> {
				value: $T,
			}
		`+impls+`
			fn read(value: Reader) Str { value.act() }
			fn write(value: Writer) Str { value.act() }
			fn main() Str {
				let device = Device<Int>{value: 1}
				"{read(device)}:{write(device)}"
			}
		`)
		if got := runGoTargetParityJSON(t, program); got != `"read:write"` {
			t.Fatalf("result = %s, want per-trait generic method bodies", got)
		}
	}
}
