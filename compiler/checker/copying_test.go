package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

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
