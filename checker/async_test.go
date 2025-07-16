package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestFibers(t *testing.T) {
	run(t, []test{
		{
			name: "Fibers cannot reference mutable variables in outer scopes",
			input: `
			use ard/async

			mut duration = 20
			async::start(fn() {
				duration + 1
			})
			`,
			diagnostics: []checker.Diagnostic{
				/* todo: need more specific in error message */
				{Kind: checker.Error, Message: "Undefined variable: duration"},
			},
		},
		{
			name: "Fibers can reference read-only variables in outer scopes",
			input: `
			use ard/async

			let duration = 20
			async::start(fn() {
				duration + 1
			})
			`,
			diagnostics: []checker.Diagnostic{},
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
		{
			name: "Module function calls within fibers are allowed",
			input: `
			use ard/async

			async::start(fn() {
				async::sleep(100)
			})
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
