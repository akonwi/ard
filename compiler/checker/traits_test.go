package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

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
	})
}
