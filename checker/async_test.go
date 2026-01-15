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
			name: "Fibers can reference read-only variables in outer scope",
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
			name: "The fiber created by async::start() is strictly Fiber<Void>",
			input: `
				use ard/async

				let fiber: async::Fiber<Int> = async::start(fn() { 2 })
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Fiber<Int>, got Fiber<Void>"},
			},
		},
	})
}

func TestAsyncEvalIsolation(t *testing.T) {
	run(t, []test{
		{
			name: "async::eval cannot reference mutable variables in outer scope",
			input: `
			use ard/async

			mut value = 10
			async::eval(fn() {
				value + 1
			})
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: value"},
			},
		},
		{
			name: "async::eval can reference read-only variables in outer scope",
			input: `
			use ard/async

			let value = 10
			async::eval(fn() {
				value + 1
			})
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "async::eval returns Fiber with result and join methods",
			input: `
			use ard/async

			let fiber = async::eval(fn() { 42 })
			fiber.join()
			let result = fiber.get()
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
