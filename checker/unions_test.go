package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestTypeUnions(t *testing.T) {
	run(t, []test{
		{
			name: "As an alias",
			input: `
				type UserId = Int
				let id1: UserId = 1
				let id2: UserId = 2
				let ids: [UserId] = [id1, id2, 3]

				id1.to_str() // Int has a .to_str() method

				match id1 {
					-1 => "not found",
					_ => "found"
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Aliasing a function type",
			input: `
				type BinaryOp = fn(Int, Int) Int
				fn do(op: BinaryOp)  Int {
					op(1, 2)
				}

				fn add(a: Int, b: Int) Int {
					a + b
				}

				do(add)
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Compatible types to a union",
			input: `
			  type Printable = Int|Str
				let a: Printable = "foo"
				let list: [Printable] = [1, "two", 3]`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Using unions in return declarations",
			input: `
				struct InvalidField { name: Str, message: Str }
				type Error = InvalidField | Str

				fn do_stuff() Error {
					"unknown failure"
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Using unions as err result",
			input: `
				struct InvalidField { name: Str, message: Str }
				type Error = InvalidField | Str

				fn do_stuff() Bool!Error {
					Result::err(InvalidField{ name: "foo", message: "bar" })
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Using unions as ok result",
			input: `
				type Success = Bool | Int

				fn do_stuff() Success!Str {
					Result::ok(true)
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Errors for an incompatible type",
			input: `
					  type Printable = Int|Str
						fn print(p: Printable) {}
						print(true)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Printable, got Bool"},
			},
		},
		{
			name: "Matching behavior on type unions",
			input: `
				type Printable = Int|Str|Bool
				let a: Printable = "foo"
				match a {
				  Int(int) => "number",
					Str(str) => "string",
					_ => "other"
				}`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
