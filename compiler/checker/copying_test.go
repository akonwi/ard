package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestCallingMutatingFunctions(t *testing.T) {
	run(t, []test{
		{
			name: "providing a read-only reference as a mutable parameter",
			input: `
				struct Person { age: Int }

				fn grow(mut p: Person) {
					p.age =+ 1
				}
				let joe = Person{age: 20}
				grow(joe)
				joe.age == 20
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
