package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestTopLevelTypeDetailsAreHoistedBeforeBodies(t *testing.T) {
	run(t, []test{
		{
			name: "function before later struct can use fields",
			input: `fn make() B {
  B{value: 1}
}

struct B {
  value: Int,
}
`,
		},
		{
			name: "impl before later trait sees required methods",
			input: `struct S {}

impl T for S {
}

trait T {
  fn id() Int
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Missing method 'id' in trait 'T'"}},
		},
	})
}

func TestTraitConformanceIsHoistedBeforeBodies(t *testing.T) {
	run(t, []test{
		{
			name: "mutable return projects before implementation",
			input: `trait View {
  fn value() Int
}

private struct Item {
  n: Int,
}

fn make() mut View {
  (mut Item{n: 1})
}

impl View for Item {
  fn value() Int { self.n }
}`,
		},
		{
			name: "annotated mutable binding projects before implementation",
			input: `trait View {
  fn value() Int
}

struct Item {
  n: Int,
}

fn make() mut View {
  let item = mut Item{n: 1}
  let view: mut View = item
  view
}

impl View for Item {
  fn value() Int { self.n }
}`,
		},
		{
			name: "value return projects before implementation",
			input: `trait View {
  fn value() Int
}

struct Item {
  n: Int,
}

fn make() View {
  Item{n: 1}
}

impl View for Item {
  fn value() Int { self.n }
}`,
		},
		{
			name: "enum return projects before implementation",
			input: `trait View {
  fn value() Int
}

enum Item {
  one
}

fn make() View {
  Item::one
}

impl View for Item {
  fn value() Int { 1 }
}`,
		},
	})
}

func TestTraitConformancePrepassPreservesInvalidTargets(t *testing.T) {
	run(t, []test{
		{
			name: "specialized alias implementation does not widen generic declaration",
			input: `trait IntOnly {
  fn label() Str
}

struct Box<$T> {
  value: $T,
}

type IntBox = Box<Int>

impl IntOnly for IntBox {
  fn label() Str { "int" }
}

fn wrong() IntOnly {
  Box<Str>{value: "wrong"}
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected implementation of IntOnly, got Box<Str>"},
				{Kind: checker.Error, Message: "Type mismatch: Expected implementation of IntOnly, got Void"},
			},
		},
		{
			name: "enum cannot gain builtin Error conformance",
			input: `enum Failure {
  bad
}

fn make() Error {
  Failure::bad
}

impl Error for Failure {
  fn error() Str { "bad" }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected implementation of Error, got Failure"},
				{Kind: checker.Error, Message: "Type mismatch: Expected implementation of Error, got Void"},
				{Kind: checker.Error, Message: "Failure cannot implement Error"},
			},
		},
	})
}

func TestRecursiveTraitChildManagementTypeDoesNotOverflow(t *testing.T) {
	run(t, []test{{
		name: "struct method can accept list of trait that accepts struct",
		input: `struct Children {}

trait View {
  fn init(children: mut Children)
}

impl Children {
  fn mut set(children: [View]) {}
}

struct Leaf {}

impl View for Leaf {
  fn init(children: mut Children) {}
}
`,
	}})
}

func TestMutableTraitProjectionRequiresKnownImplementation(t *testing.T) {
	run(t, []test{
		{
			name: "generic body lacks mutable trait constraint",
			input: `trait Widget {
  fn mut event()
}

fn drive(root: mut Widget) {
  root.event()
}

fn forward(root: mut $W) {
  drive(root)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected mut Widget, got mut $W"}},
		},
		{
			name: "concrete implementation projects to mutable trait",
			input: `trait Widget {
  fn mut event()
}

struct Root {}

impl Widget for Root {
  fn mut event() {}
}

fn drive(root: mut Widget) {
  root.event()
}

fn main() {
  let root = mut Root{}
  drive(root)
}`,
		},
		{
			name: "generic reference forwards to generic parameter",
			input: `fn pass(value: mut $T) {}

fn forward(value: mut $W) {
  pass(value)
}`,
		},
	})
}

func TestNestedMutableTraitProjectionRequiresKnownImplementation(t *testing.T) {
	run(t, []test{
		{
			name: "nullable generic reference cannot project to nullable trait reference",
			input: `trait Widget {
  fn mut event()
}

fn take(value: (mut Widget)?) {}

fn forward(value: (mut $W)?) {
  take(value)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected (mut Widget)?, got (mut $W)?"}},
		},
		{
			name: "list of generic references cannot project to list of trait references",
			input: `trait Widget {
  fn mut event()
}

fn take(value: [mut Widget]) {}

fn forward(value: [mut $W]) {
  take(value)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected [mut Widget], got [mut $W]"}},
		},
		{
			name: "inferred list rejects mixed trait and generic references",
			input: `trait Widget {
  fn mut event()
}

fn combine(left: mut Widget, right: mut $W) {
  let values = [left, right]
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: A list can only contain values of single type"}},
		},
		{
			name: "inferred map rejects mixed trait and generic references",
			input: `trait Widget {
  fn mut event()
}

fn combine(left: mut Widget, right: mut $W) {
  let values = ["left": left, "right": right]
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Map value type mismatch: Expected mut Widget, got mut $W"}},
		},
		{
			name: "if branches reject nested generic and trait references",
			input: `trait Widget {
  fn mut event()
}

fn choose(condition: Bool, left: (mut Widget)?, right: (mut $W)?) (mut Widget)? {
  if condition == true {
    left
  } else {
    right
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "All branches must have the same result type"},
				{Kind: checker.Error, Message: "Type mismatch: Expected (mut Widget)?, got Void"},
			},
		},
		{
			name: "match branches reject nested generic and trait references",
			input: `trait Widget {
  fn mut event()
}

fn choose(condition: Bool, left: (mut Widget)?, right: (mut $W)?) {
  let chosen = match condition {
    true => left,
    false => right,
  }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected (mut Widget)?, got (mut $W)?"}},
		},
		{
			name: "nested reference forwards through the same inferred generic",
			input: `fn take(value: [(mut $T)?]) {}

fn forward(value: [(mut $W)?]) {
  take(value)
}`,
		},
	})
}

func TestGenericReferenceEqualityRequiresCompatibleReferents(t *testing.T) {
	run(t, []test{
		{
			name: "trait and unconstrained generic references cannot compare",
			input: `trait Widget {
  fn mut event()
}

fn compare(left: mut Widget, right: mut $W) {
  let equal = left == right
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Invalid: mut Widget == mut $W"}},
		},
		{
			name: "distinct generic references cannot compare",
			input: `fn compare(left: mut $T, right: mut $W) {
  let equal = left == right
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Invalid: mut $T == mut $W"}},
		},
		{
			name: "reference equality rejects unresolved literal inference",
			input: `struct Marker<$T> {
  value: Int,
}

fn compare() {
  let equal = mut Marker{value: 1} == mut Marker<Int>{value: 2}
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Invalid: mut Marker<$T> == mut Marker<Int>"}},
		},
		{
			name: "references to the same generic can compare",
			input: `fn same(left: mut $T, right: mut $T) Bool {
  left == right
}`,
		},
		{
			name: "concrete reference can compare with implemented trait reference",
			input: `trait Widget {
  fn mut event()
}

struct Root {}

impl Widget for Root {
  fn mut event() {}
}

fn same(left: mut Widget, right: mut Root) Bool {
  left == right
}`,
		},
	})
}

func TestGenericTraitImplementationSignatureRequiresExactTypes(t *testing.T) {
	run(t, []test{{
		name: "receiver generic cannot satisfy concrete trait parameter",
		input: `trait Consumer {
  fn consume(value: Int)
}

struct Box<$T> {}

impl Consumer for Box {
  fn consume(value: $T) {}
}`,
		diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected Int, got $T"}},
	}})
}

func TestGenericStructReferenceFieldInfersReferent(t *testing.T) {
	run(t, []test{
		{
			name: "mutable generic field infers from reference value",
			input: `struct Box<$T> {
  value: mut $T,
}

struct User {}

fn main() {
  let user = mut User{}
  let box = Box{value: user}
  let value: mut User = box.value
}`,
		},
		{
			name: "mutable generic field rejects interface-shaped type argument",
			input: `trait Widget {
  fn mut event()
}

struct Root {}
impl Widget for Root {
  fn mut event() {}
}

struct Holder<$T> {
  value: mut $T,
}

fn main() {
  let root = mut Root{}
  let holder = Holder<Widget>{value: root}
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type argument Widget cannot be used for $T in Holder: direct mutable generic fields cannot specialize to an Ard trait"}},
		},
		{
			name: "phantom generic still requires contextual or explicit type",
			input: `struct Marker<$T> {
  value: Int,
}

fn main() {
  let marker = Marker{value: 1}
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unresolved generic: $T"}},
		},
	})
}

func TestTraitImplementationCanProjectSelfToCurrentTrait(t *testing.T) {
	run(t, []test{
		{
			name: "mutating self projects to current trait reference",
			input: `trait View {
  fn mut attach(child: mut View)
}

struct Node {
  children: [mut View],
}

impl View for Node {
  fn mut attach(child: mut View) {
    let parent: mut View = self
    self.children.push(child)
  }
}`,
		},
		{
			name: "ordinary struct self projects to current trait value",
			input: `trait View {
  fn as_view() View
}

struct Node {}

impl View for Node {
  fn as_view() View { self }
}`,
		},
		{
			name: "enum self projects to current trait value",
			input: `trait View {
  fn as_view() View
}

enum Node {
  item
}

impl View for Node {
  fn as_view() View { self }
}`,
		},
	})
}

func TestGenericTraitImplementationCanProjectSelf(t *testing.T) {
	run(t, []test{{
		name: "generic self projects to current trait reference",
		input: `trait View {
  fn mut self_ref() mut View
}

struct Box<$T> {
  value: $T,
}

impl View for Box {
  fn mut self_ref() mut View { self }
}`,
	}})
}

func TestMatchAllowsConcreteTraitImplementationBranch(t *testing.T) {
	traitFixture := `trait View {
  fn render()
}

struct Screen {}

impl View for Screen {
  fn render() {}
}

fn make_view() View {
  Screen{}
}
`
	run(t, []test{
		{
			name: "bool match can return trait and implementing struct",
			input: traitFixture + `
fn main(flag: Bool) View {
  match flag {
    true => make_view(),
    false => Screen{},
  }
}`,
		},
		{
			name: "maybe match can return trait and implementing struct",
			input: traitFixture + `
fn main(flag: Bool?) View {
  match flag {
    value => make_view(),
    _ => Screen{},
  }
}`,
		},
		{
			name: "string match can return trait and implementing struct",
			input: traitFixture + `
fn main(name: Str) View {
  match name {
    "home" => Screen{},
    _ => make_view(),
  }
}`,
		},
		{
			name: "enum match can return trait and implementing struct",
			input: traitFixture + `
enum Route {
  home
  other
}

fn main(route: Route) View {
  match route {
    Route::home => Screen{},
    Route::other => make_view(),
  }
}`,
		},
		{
			name: "result match can return trait and implementing struct",
			input: traitFixture + `
fn main(res: Screen!Str) View {
  match res {
    ok(screen) => screen,
    err(_message) => make_view(),
  }
}`,
		},
		{
			name: "int match can return trait and implementing struct",
			input: traitFixture + `
fn main(n: Int) View {
  match n {
    1 => Screen{},
    _ => make_view(),
  }
}`,
		},
		{
			name: "conditional match can return trait and implementing struct",
			input: traitFixture + `
fn main(flag: Bool) View {
  match {
    flag => Screen{},
    _ => make_view(),
  }
}`,
		},
		{
			name: "union match can return trait and implementing struct",
			input: traitFixture + `
type ScreenOrInt = Screen | Int

fn main(value: ScreenOrInt) View {
  match value {
    Screen(screen) => screen,
    _ => make_view(),
  }
}`,
		},
		{
			name: "conditional match uses expected result union type for result constructors",
			input: traitFixture + `
struct OtherScreen {}

impl View for OtherScreen {
  fn render() {}
}

type AnyScreen = Screen | OtherScreen

fn main(flag: Bool) AnyScreen!Str {
  match {
    flag => Result::ok(Screen{}),
    _ => Result::ok(OtherScreen{}),
  }
}`,
		},
	})
}
func TestTraitDefinitions(t *testing.T) {
	run(t, []test{
		{
			name: "A valid definition",
			input: `trait Speaks {
				fn say(s: Str)
			}`,
			output: &checker.Program{
				Statements: []checker.Statement{},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "A valid implementation",
			input: `
			trait Speaks {
				fn say(s: Str)
			}
			struct Dog {}

			impl Speaks for Dog {
			  fn say(stuff: Str) {
					()
				}
			}
			`,
			output: &checker.Program{
				Statements: []checker.Statement{},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Trait methods can declare mutable parameters",
			input: `
			struct Counter { value: Int }
			trait Bumpable {
				fn poke(c: mut Counter)
			}
			struct Doubler {}
			impl Bumpable for Doubler {
				fn poke(c: mut Counter) { () }
			}
			`,
			output: &checker.Program{
				Statements: []checker.Statement{},
			},
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Trait impl parameter mutability must match",
			input: `
			struct Counter { value: Int }
			trait Bumpable {
				fn poke(c: mut Counter)
			}
			struct Doubler {}
			impl Bumpable for Doubler {
				fn poke(c: Counter) { () }
			}
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected mut Counter, got Counter"},
				{Kind: checker.Error, Message: "Trait method 'poke' parameter 'c' mutability mismatch"},
			},
		},
		{
			name: "An invalid implementation",
			input: `
					trait Speaks {
						fn say(s: Str)
					}
					struct Dog {}

					impl Speaks for Dog {
					  fn say(stuff: Int) Int {
							stuff
						}
					}
					`,
			output: &checker.Program{
				Statements: []checker.Statement{},
			},
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
				{Kind: checker.Error, Message: "Trait method 'say' has return type of Void"},
			},
		},
		{
			name: "All trait methods must be provided",
			input: `
					trait Speaks {
					  fn introduce() Str
						fn say(s: Str) Str
					}
					struct Dog {}

					impl Speaks for Dog {
					  fn say(stuff: Str) Str {
							"woof"
						}
					}
					`,
			output: &checker.Program{
				Statements: []checker.Statement{},
			},
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Missing method 'introduce' in trait 'Speaks'"},
			},
		},
	})
}

func TestTraitReceiverMutability(t *testing.T) {
	run(t, []test{
		{
			name: "mutating contract accepts mutating implementation through reference",
			input: `trait Counter {
  fn mut set(value: Int)
}
struct Box { value: Int }
impl Counter for Box {
  fn mut set(value: Int) { self.value = value }
}
fn mutate(counter: mut Counter) {
  counter.set(2)
}
let box = Box{value: 1}
mutate(mut box)
`,
		},
		{
			name: "mutating trait method rejects ordinary trait receiver",
			input: `trait Counter {
  fn mut set(value: Int)
}
struct Box { value: Int }
impl Counter for Box {
  fn mut set(value: Int) { self.value = value }
}
fn mutate(counter: Counter) {
  counter.set(2)
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot call mutating method 'counter.set': receiver is not a reference"}},
		},
		{
			name: "non-mutating contract rejects mutating implementation",
			input: `trait Counter {
  fn set(value: Int)
}
struct Box { value: Int }
impl Counter for Box {
  fn mut set(value: Int) { self.value = value }
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Trait method 'set' does not allow a mutating receiver"}},
		},
		{
			name: "mutating contract accepts non-mutating implementation",
			input: `trait Counter {
  fn mut set(value: Int)
}
struct Noop {}
impl Counter for Noop {
  fn set(value: Int) {}
}
fn mutate(counter: mut Counter) {
  counter.set(2)
}
let noop = Noop{}
mutate(mut noop)
`,
		},
		{
			name: "mutating trait interpolation rejects ordinary receiver",
			input: `trait Renderable {
  fn mut to_str() Str
}
fn render(value: Renderable) Str {
  "{value}"
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot call mutating method 'value.to_str': receiver is not a reference"}},
		},
		{
			name: "mutating trait interpolation accepts reference receiver",
			input: `trait Renderable {
  fn mut to_str() Str
}
fn render(value: mut Renderable) Str {
  "{value}"
}
`,
		},
	})
}

func TestConcreteTraitMethodLookupBeforeImplementation(t *testing.T) {
	run(t, []test{
		{
			name: "struct method is available before trait implementation",
			input: `trait View {
  fn value() Int
}
struct Item { n: Int }
fn read(item: Item) Int {
  item.value()
}
impl View for Item {
  fn value() Int { self.n }
}
`,
		},
		{
			name: "enum method is available before trait implementation",
			input: `trait View {
  fn value() Int
}
enum Item { one }
fn read(item: Item) Int {
  item.value()
}
impl View for Item {
  fn value() Int { 1 }
}
`,
		},
		{
			name: "concrete lookup uses implementation receiver mutability",
			input: `trait Reader {
  fn mut read() Int
}
struct Value {}
fn read(value: Value) Int {
  value.read()
}
impl Reader for Value {
  fn read() Int { 42 }
}
`,
		},
		{
			name: "earlier trait method can call later sibling",
			input: `trait Pair {
  fn first() Int
  fn second() Int
}
struct Value {}
impl Pair for Value {
  fn first() Int { self.second() }
  fn second() Int { 42 }
}
`,
		},
		{
			name: "generic struct method is available before trait implementation",
			input: `trait View {
  fn value() Int
}
struct Box<$T> { value: $T }
fn read(box: Box<Int>) Int {
  box.value()
}
impl View for Box {
  fn value() Int { 42 }
}
`,
		},
	})
}

func TestTraitMethodSignaturePrepassDoesNotDuplicateDiagnostics(t *testing.T) {
	run(t, []test{{
		name: "unresolved return type is reported once",
		input: `trait View {
  fn value() Int
}
struct Item {}
impl View for Item {
  fn value() Missing { self }
}
`,
		diagnostics: []checker.Diagnostic{
			{Kind: checker.Error, Message: "Unrecognized type: Missing"},
			{Kind: checker.Error, Message: "Trait method 'value' has return type of Int"},
		},
	}})
}

func TestTraitImplementationsPreserveSameNamedMethodsPerTrait(t *testing.T) {
	implementationsByOrder := []string{
		`impl Reader for Device {
  fn act() {}
}
impl Writer for Device {
  fn mut act() {}
}`,
		`impl Writer for Device {
  fn mut act() {}
}
impl Reader for Device {
  fn act() {}
}`,
	}
	for _, implementations := range implementationsByOrder {
		run(t, []test{{
			name: "trait dispatch preserves each implementation",
			input: `trait Reader {
  fn act()
}
trait Writer {
  fn mut act()
}
struct Device {}
` + implementations,
		}, {
			name: "direct concrete call is ambiguous",
			input: `trait Reader {
  fn act()
}
trait Writer {
  fn mut act()
}
struct Device {}
` + implementations + `
let device = Device{}
device.act()
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Method 'act' is ambiguous on Device"}},
		}})
	}
}

func TestTraitInterpolationRejectsAmbiguousConcreteToString(t *testing.T) {
	implementations := []string{
		`impl First for Value {
  fn to_str() Str { "first" }
}
impl Second for Value {
  fn to_str() Str { "second" }
}`,
		`impl Second for Value {
  fn to_str() Str { "second" }
}
impl First for Value {
  fn to_str() Str { "first" }
}`,
	}
	for _, impls := range implementations {
		run(t, []test{{
			name: "implicit concrete to_str requires unambiguous trait",
			input: `trait First {
  fn to_str() Str
}
trait Second {
  fn to_str() Str
}
struct Value {}
` + impls + `
let value = Value{}
let rendered = "{value}"
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Method 'to_str' is ambiguous on Value"}},
		}})
	}
}

func TestTraitsAsTypes(t *testing.T) {
	run(t, []test{
		{
			name: "let binding with explicit trait type (success)",
			input: `
			trait Drawable {
			  fn draw() Str
			}

			struct Box { w: Int }

			impl Drawable for Box {
			  fn draw() Str { "box" }
			}

			fn main() {
			  let d: Drawable = Box{w: 5}
			}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "let binding with explicit trait type (failure)",
			input: `
			trait Drawable {
			  fn draw() Str
			}

			struct Circle {}

			fn main() {
			  let d: Drawable = Circle{}
			}
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected implementation of Drawable, got Circle"},
			},
		},
		{
			name: "struct field with trait type",
			input: `
			trait Drawable {
			  fn draw() Str
			}

			struct Box { w: Int }

			impl Drawable for Box {
			  fn draw() Str { "box" }
			}

			struct Container { child: Drawable }

			fn main() {
			  let c = Container{child: Box{w: 5}}
			}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "list with trait element type",
			input: `
			trait Drawable {
			  fn draw() Str
			}

			struct Box { w: Int }

			impl Drawable for Box {
			  fn draw() Str { "box" }
			}

			fn main() {
			  let items: [Drawable] = [Box{w: 5}, Box{w: 10}]
			}
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
