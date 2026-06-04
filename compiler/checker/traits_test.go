package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

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
		{
			name: "encode::Encodable only accepts primitives",
			input: `
			use ard/encode

			encode::json(1)
			encode::json("ok")
			encode::json(false)
			encode::json([1, 2, 3])
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected implementation of Encodable, got [Int]"},
			},
		},
		{
			name: "json parse validates supported target type",
			input: `
			use ard/json

			json::parse<fn() Void>("null")
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "json::parse does not support fn <function>() Void"},
			},
		},
		{
			name: "json parse validates map keys",
			input: `
			use ard/json

			json::parse<[Int: Str]>("{}")
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "json::parse only supports Str map keys, got Int"},
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
