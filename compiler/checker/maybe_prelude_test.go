package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestMaybePreludeAndFormalType(t *testing.T) {
	run(t, []test{
		{
			name: "Maybe constructors are in the prelude",
			input: `let a: Int? = Maybe::some(42)
let b: Int? = Maybe::none<Int>()`,
		},
		{
			name: "formal Maybe type spelling aliases nullable suffix",
			input: `let a: Maybe<Int> = Maybe::some(42)
let b: Int? = a`,
		},
		{
			name: "old lowercase maybe module is no longer available",
			input: `use ard/maybe
let a = maybe::some(1)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Unknown module: ard/maybe"},
				{Kind: checker.Error, Message: "Undefined module: maybe"},
			},
		},
	})
}

func TestMaybeMutableMethods(t *testing.T) {
	run(t, []test{
		{
			name: "set and clear mutate a maybe",
			input: `mut m = Maybe::none<Int>()
m.set(42)
m.clear()
let done: Bool = m.is_none()`,
		},
		{
			name: "set requires an inner value",
			input: `mut m = Maybe::none<Int>()
m.set("nope")`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
			},
		},
		{
			name: "set requires mutable receiver",
			input: `let m = Maybe::none<Int>()
m.set(1)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Immutable: Maybe.set receiver"},
			},
		},
		{
			name: "clear requires mutable receiver",
			input: `let m = Maybe::some(1)
m.clear()`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Immutable: Maybe.clear receiver"},
			},
		},
	})
}
