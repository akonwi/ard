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
