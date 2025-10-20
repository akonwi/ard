package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestFibers(t *testing.T) {
	run(t, []test{
		{
			name: "Fibers cannot reference mutable variables in outer scope",
			input: `
			use ard/async

			mut duration = 20
			async::start(fn() {
				duration + 1
			})
			`,
			diagnostics: []checker.Diagnostic{
				/* todo: need to be more specific about how the mutable reference won't be shared */
				{Kind: checker.Error, Message: "Undefined variable: duration"},
			},
		},
		{
			name: "Fibers cannot reference read-only variables in outer scope",
			input: `
			use ard/async

			let duration = 20
			async::start(fn() {
				duration + 1
			})
			`,
			diagnostics: []checker.Diagnostic{
				/* todo: need to be more specific about how the mutable reference won't be shared */
				{Kind: checker.Error, Message: "Undefined variable: duration"},
			},
		},
		{
			name: "Valid fiber functions work",
			input: `
			use ard/async

			async::start(fn() {
				let x = 42
				x + 1
			})
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
