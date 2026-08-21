package checker_test

import (
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func assertGenericCompatibilityRejected(t *testing.T, source string, want string) {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checked := checker.New("test.ard", result.Program, nil)
	checked.Check()
	if !checked.HasErrors() {
		t.Fatal("expected checker rejection, got no diagnostics")
	}
	if want == "" {
		return
	}
	for _, diagnostic := range checked.Diagnostics() {
		if strings.Contains(diagnostic.Message, want) {
			return
		}
	}
	t.Fatalf("expected a diagnostic containing %q, got %#v", want, checked.Diagnostics())
}

func TestRigidDeclarationGenericCallInference(t *testing.T) {
	assertGenericCompatibilityRejected(t, `fn pair(left: $T, right: $T) {}

fn forward(value: $W) {
  pair(value, 1)
}`, "$W")
}

func TestRigidDeclarationGenericStructInference(t *testing.T) {
	assertGenericCompatibilityRejected(t, `struct Pair<$T> {
  left: $T,
  right: $T,
}

fn forward(value: $W) {
  let pair = Pair{left: value, right: 1}
}`, "$W")
}

func TestDeclarationGenericValidationPaths(t *testing.T) {
	t.Run("arithmetic", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `fn add(left: Int, right: $T) Int {
  left + right
}`, "Cannot add different types")
	})
	t.Run("result propagation", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `fn convert(result: Int!$E) Int!Str {
  let value = try result
  Result::ok(value)
}`, "$E")
	})
	t.Run("discarding function coercion", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `fn consume(callback: fn(Int)) {}

fn forward(callback: fn($T) Int) {
  consume(callback)
}`, "$T")
	})
}

func TestUnionReferenceRepresentationCompatibility(t *testing.T) {
	t.Run("union and member equality", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `struct Left {}
struct Right {}
type Choice = Left | Right

fn same(left: mut Choice, right: mut Left) Bool {
  left == right
}`, "mut Choice")
	})
	t.Run("union member reference assignment", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `struct Left {}
struct Right {}
type Choice = Left | Right

fn take(value: mut Choice) {}

fn forward(value: mut Left) {
  take(value)
}`, "mut Choice")
	})
	t.Run("union reference and trait equality", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `trait View {
  fn show() Int
}

struct Left {}
impl View for Left {
  fn show() Int { 1 }
}

struct Right {}
impl View for Right {
  fn show() Int { 2 }
}

type Choice = Left | Right

fn same(left: mut View, right: mut Choice) Bool {
  left == right
}`, "mut View")
	})
}

func TestUnionMemberWidening(t *testing.T) {
	prefix := `struct Left {}
struct Right {}
type Choice = Left | Right
`
	run(t, []test{
		{
			name: "match member widens to existing union branch",
			input: prefix + `
fn choose(condition: Bool, existing: Choice) Choice {
  match condition {
    true => existing,
    false => Left{},
  }
}`,
		},
		{
			name: "if member widens to existing union branch",
			input: prefix + `
fn choose(condition: Bool, existing: Choice) Choice {
  if condition {
    Left{}
  } else {
    existing
  }
}`,
		},
		{
			name: "try catch member widens to function union result",
			input: prefix + `
fn risky(value: Choice) Choice!Str {
  Result::ok(value)
}

fn recover(value: Choice) Choice {
  try risky(value) -> err {
    Left{}
  }
}`,
		},
		{
			name: "list member widens to existing union element",
			input: prefix + `
fn build(existing: Choice) {
  let values = [existing, Left{}]
}`,
		},
		{
			name: "map member widens to existing union value",
			input: prefix + `
fn build(existing: Choice) {
  let values = ["existing": existing, "left": Left{}]
}`,
		},
	})
}

func TestUnresolvedLiteralInferenceErasure(t *testing.T) {
	t.Run("Any conversion", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `struct Marker<$T> {
  value: Int,
}

fn take(value: Any) {}

fn main() {
  take(Marker{value: 1})
}`, "Unresolved generic")
	})
	t.Run("nullable Any parameter", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `struct Marker<$T> {
  value: Int,
}

fn take(value: Any?) {}

fn main() {
  take(Marker{value: 1})
}`, "Unresolved generic")
	})
	t.Run("explicit nullable Any construction", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `struct Marker<$T> {
  value: Int,
}

fn main() {
  let value = Maybe::new<Any>(Marker{value: 1})
}`, "Unresolved generic")
	})
	t.Run("union membership", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `struct Marker<$T> {
  value: Int,
}

type Value = Marker<Int> | Str

fn take(value: Value) {}

fn main() {
  take(Marker{value: 1})
}`, "Marker<$T>")
	})
	t.Run("trait reference projection", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `trait View {
  fn show() Int
}

struct Box<$T> {
  value: Int,
}
impl View for Box {
  fn show() Int { self.value }
}

fn take(value: mut View) {}

fn main() {
  take(mut Box{value: 1})
}`, "Unresolved generic")
	})
}

func TestContextualPhantomStructInference(t *testing.T) {
	run(t, []test{{
		name: "return context resolves phantom generic",
		input: `struct Marker<$T> {
  value: Int,
}

fn make(seed: $U) Marker<$U> {
  Marker{value: 1}
}`,
	}})
}

func TestResolvedGenericFieldConversions(t *testing.T) {
	prefix := `trait Widget {
  fn render() Int
}

struct Root {}
impl Widget for Root {
  fn render() Int { 1 }
}
`
	run(t, []test{
		{
			name: "explicit trait argument converts concrete field",
			input: prefix + `
struct Holder<$T> {
  value: $T,
}

fn main() {
  let holder = Holder<Widget>{value: Root{}}
}`,
		},
		{
			name: "nullable trait argument converts before wrapping",
			input: prefix + `
struct Holder<$T> {
  value: $T?,
}

fn main() {
  let holder = Holder<Widget>{value: Root{}}
}`,
		},
	})
}

func TestMutableContainerGenericInterfaceArguments(t *testing.T) {
	run(t, []test{{
		name: "mutable list storage keeps interface element representation",
		input: `trait Widget {
  fn render() Int
}

struct Root {}
impl Widget for Root {
  fn render() Int { 1 }
}

struct Holder<$T> {
  values: mut [$T],
}

fn main() {
  let values: [Widget] = [Root{}]
  let holder = Holder<Widget>{values: mut values}
}`,
	}})
}

func TestMutableGenericFieldsRejectArdTraitArguments(t *testing.T) {
	assertGenericCompatibilityRejected(t, `trait Widget {
  fn render() Int
}

struct Root {}
impl Widget for Root {
  fn render() Int { 1 }
}

struct Holder<$T> {
  value: (mut $T)?,
}

fn main() {
  let root = mut Root{}
  let holder = Holder<Widget>{value: root}
}`, "mutable generic fields")

	t.Run("type annotation", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `trait Widget {
  fn render() Int
}

struct Holder<$T> {
  value: mut $T,
}

fn read(holder: Holder<Widget>) mut Widget {
  holder.value
}`, "mutable generic fields")
	})
}

func TestMutableGenericsPreserveForeignInterfaceArguments(t *testing.T) {
	run(t, []test{
		{
			name: "generic struct annotation keeps pointer to Go interface",
			input: `use go:io

struct Holder<$T> {
  value: mut $T,
}

fn read(holder: Holder<io::Reader>) mut io::Reader {
  holder.value
}`,
		},
		{
			name: "generic function keeps pointer to Go interface",
			input: `use go:io

fn identity(value: mut $T) mut $T {
  value
}

fn consume(value: mut io::Reader) {
  let echoed = identity<io::Reader>(value)
}`,
		},
	})
}

func TestMutableGenericFunctionsRejectArdTraitArguments(t *testing.T) {
	prefix := `trait Widget {
  fn render() Int
}

struct Root {}
impl Widget for Root {
  fn render() Int { 1 }
}

fn identity(value: mut $T) mut $T {
  value
}
`
	t.Run("explicit type argument", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, prefix+`
fn main() {
  let root = mut Root{}
  let view: mut Widget = root
  let echoed = identity<Widget>(view)
}`, "mutable generic parameters")
	})
	t.Run("inferred type argument", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, prefix+`
fn main() {
  let root = mut Root{}
  let view: mut Widget = root
  let echoed = identity(view)
}`, "mutable generic parameters")
	})
	t.Run("nested generic struct result", func(t *testing.T) {
		assertGenericCompatibilityRejected(t, `trait Widget {
  fn render() Int
}

struct Root {}
impl Widget for Root {
  fn render() Int { 1 }
}

struct Holder<$T> {
  value: mut $T,
}

fn make(value: mut $T) Holder<$T> {
  Holder{value: value}
}

fn main() {
  let root = mut Root{}
  let view: mut Widget = root
  let holder = make<Widget>(view)
}`, "mutable generic parameters")
	})
}
