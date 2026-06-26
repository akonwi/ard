package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestSelfRecursiveStructFieldsThroughIndirection(t *testing.T) {
	run(t, []test{
		{
			name: "list self reference",
			input: `struct Node {
  children: [Node],
}
`,
		},
		{
			name: "map value self reference",
			input: `struct Node {
  children: [Str:Node],
}
`,
		},
		{
			name: "nullable self reference",
			input: `struct Node {
  parent: Node?,
}
`,
		},
		{
			name: "mutable reference self reference",
			input: `struct Node {
  parent: mut Node,
}
`,
		},
		{
			name: "direct self reference is rejected",
			input: `struct Node {
  child: Node,
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Recursive field Node.child has infinite size. Put the recursive reference behind mut, list, map, nullable, trait, extern, or function indirection."}},
		},
	})
}

func TestSameModuleRecursiveTypeGroups(t *testing.T) {
	run(t, []test{
		{
			name: "retained tree finite through list and trait object",
			input: `struct Context {
  tree: ViewTree,
  node_id: Int,
}

struct ViewTree {
  nodes: [TreeNode],
}

struct TreeNode {
  view: View,
  children: [Int],
}

trait View {
  fn init(ctx: Context)
}
`,
		},
		{
			name: "forward same module struct reference",
			input: `struct A {
  b: B,
}

struct B {
  value: Int,
}
`,
		},
		{
			name: "mut reference breaks recursive layout cycle",
			input: `struct A {
  b: mut B,
}

struct B {
  a: A,
}
`,
		},
		{
			name: "map value breaks recursive layout cycle",
			input: `struct A {
  values: [Str:B],
}

struct B {
  a: A,
}
`,
		},
		{
			name: "nullable breaks recursive layout cycle",
			input: `struct A {
  b: B?,
}

struct B {
  a: A,
}
`,
		},
		{
			name: "function type breaks recursive layout cycle",
			input: `struct A {
  make: fn() B,
}

struct B {
  a: A,
}
`,
		},
		{
			name: "forward generic struct reference with type arguments",
			input: `struct Holder {
  box: Box<Int>,
}

struct Box {
  value: $T,
}

fn accept(holder: Holder) {}
`,
		},
		{
			name: "generic self specialization is rejected before partial types escape",
			input: `struct Node {
  next: Node<$T>?,
  value: $T,
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Recursive generic self-reference Node is not supported yet"}},
		},
		{
			name: "forward generic struct reference without type arguments is rejected",
			input: `struct Holder {
  box: Box,
}

struct Box {
  value: $T,
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Generic type Box requires type arguments"}},
		},
		{
			name: "alias to later type is available to earlier fields",
			input: `struct Holder {
  value: Item,
}

type Item = Leaf

struct Leaf {
  value: Int,
}
`,
		},
		{
			name: "alias chain to later type is resolved before fields",
			input: `struct Holder {
  value: A,
}

type A = B
type B = Leaf

struct Leaf {
  value: Int,
}
`,
		},
		{
			name: "nested alias dependency is resolved before fields",
			input: `struct Holder {
  values: A,
}

type A = [B]
type B = Leaf

struct Leaf {
  value: Int,
}
`,
		},
		{
			name: "generic specialized inline cycle is rejected",
			input: `struct A {
  box: Box<A>,
}

struct Box {
  value: $T,
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Recursive field A.box has infinite size. Put the recursive reference behind mut, list, map, nullable, trait, extern, or function indirection."}},
		},
		{
			name: "map key recursive cycle is rejected",
			input: `struct A {
  values: [A:Int],
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Recursive field A.values has infinite size. Put the recursive reference behind mut, list, map, nullable, trait, extern, or function indirection."}},
		},
		{
			name: "nested map key recursive cycle is rejected",
			input: `struct A {
  values: [[A:Int]],
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Recursive field A.values has infinite size. Put the recursive reference behind mut, list, map, nullable, trait, extern, or function indirection."}},
		},
		{
			name: "mut indirection permits recursive map key",
			input: `struct A {
  box: mut Box,
}

struct Box {
  values: [A:Int],
}
`,
		},
		{
			name: "nullable nested in result does not break layout yet",
			input: `struct A {
  result: A?!Str,
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Recursive field A.result has infinite size. Put the recursive reference behind mut, list, map, nullable, trait, extern, or function indirection."}},
		},
		{
			name: "nullable nested in union does not break layout yet",
			input: `type U = A? | Int

struct A {
  value: U,
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Recursive field A.value has infinite size. Put the recursive reference behind mut, list, map, nullable, trait, extern, or function indirection."}},
		},
		{
			name: "direct mutual struct value cycle is rejected",
			input: `struct A {
  b: B,
}

struct B {
  a: A,
}
`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Recursive field A.b has infinite size. Put the recursive reference behind mut, list, map, nullable, trait, extern, or function indirection."}},
		},
	})
}

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

func TestRecursiveTraitChildManagementTypeDoesNotOverflow(t *testing.T) {
	run(t, []test{{
		name: "struct method can accept list of trait that accepts struct",
		input: `struct Children {}

trait View {
  fn init(mut children: Children)
}

impl Children {
  fn mut set(children: [View]) {}
}

struct Leaf {}

impl View for Leaf {
  fn init(mut children: Children) {}
}
`,
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
			use ard/io

			trait Speaks {
				fn say(s: Str)
			}
			struct Dog {}

			impl Speaks for Dog {
			  fn say(stuff: Str) {
					io::print("woof")
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
				fn poke(mut c: Counter)
			}
			struct Doubler {}
			impl Bumpable for Doubler {
				fn poke(mut c: Counter) { () }
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
				fn poke(mut c: Counter)
			}
			struct Doubler {}
			impl Bumpable for Doubler {
				fn poke(c: Counter) { () }
			}
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Trait method 'poke' parameter 'c' mutability mismatch"},
			},
		},
		{
			name: "An invalid implementation",
			input: `
					use ard/io

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

func TestUsingPackageTraits(t *testing.T) {
	run(t, []test{
		{
			name: "Implementing Str::ToString",
			input: `
			struct Person { name: Str }

			impl Str::ToString for Person {
			  fn to_str() Str {
					"Person: {self.name}"
				}
			}
			`,
		},
	})
}

func TestTraitsAsTypes(t *testing.T) {
	run(t, []test{
		{
			name: "functions with Trait params",
			input: `
			use ard/io
			struct Foo {}

			fn display(item: Str::ToString) {
			  io::print(item.to_str())
			}
			display(100)
			display(Foo{})
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected implementation of ToString, got Foo"},
			},
		},
		// {
		// 	name: "functions with Trait return",
		// 	input: `
		// 	struct Foo {}

		// 	fn valid() Str::ToString {
		// 	  100
		// 	}
		// 	fn invalid(item: Str::ToString) Str::ToString {
		// 	  Foo{}
		// 	}
		// 	`,
		// 	diagnostics: []checker.Diagnostic{
		// 		{Kind: checker.Error, Message: "Type mismatch: Expected ToString, got Foo"},
		// 	},
		// },
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
