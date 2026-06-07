package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestMutableReferenceFields(t *testing.T) {
	run(t, []test{
		{
			name: "immutable struct can expose mutable reference field",
			input: `
				struct Tree { count: Int }
				struct Context { tree: mut Tree }

				fn bump(mut tree: Tree) {
					tree.count =+ 1
				}

				mut tree = Tree{count: 0}
				let ctx = Context{tree: tree}
				bump(ctx.tree)
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "mutable reference field requires mutable value",
			input: `
				struct Tree { count: Int }
				struct Context { tree: mut Tree }

				let tree = Tree{count: 0}
				let ctx = Context{tree: tree}
			`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected a mutable Tree"}},
		},
		{
			name: "assignment to mutable reference field is allowed through immutable holder",
			input: `
				struct Tree { count: Int }
				struct Context { tree: mut Tree }

				mut tree = Tree{count: 0}
				mut other = Tree{count: 1}
				let ctx = Context{tree: tree}
				ctx.tree = other
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "nested field assignment through mutable reference field is allowed",
			input: `
				struct Tree { count: Int }
				struct Context { tree: mut Tree }

				mut tree = Tree{count: 0}
				let ctx = Context{tree: tree}
				ctx.tree.count = 2
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}

func TestMutableReferenceParameters(t *testing.T) {
	run(t, []test{
		{
			name: "mutable parameter accepts mutable binding without call-site mut",
			input: `
				struct Person { age: Int }

				fn grow(mut p: Person) {
					p.age =+ 1
				}
				mut joe = Person{age: 20}
				grow(joe)
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "immutable binding cannot be passed to mutable parameter",
			input: `
				struct Person { age: Int }

				fn grow(mut p: Person) {
					p.age =+ 1
				}
				let joe = Person{age: 20}
				grow(joe)
			`,
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Type mismatch: Expected a mutable Person",
				},
			},
		},
		{
			name: "call-site mut no longer copies immutable argument",
			input: `
				struct Person { age: Int }

				fn grow(mut p: Person) {
					p.age =+ 1
				}
				let joe = Person{age: 20}
				grow(mut joe)
			`,
			diagnostics: []checker.Diagnostic{
				{
					Kind:    checker.Error,
					Message: "Type mismatch: Expected a mutable Person",
				},
			},
		},
	})
}
