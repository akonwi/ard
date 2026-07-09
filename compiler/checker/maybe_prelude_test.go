package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestMaybePreludeAndFormalType(t *testing.T) {
	run(t, []test{
		{
			name: "Maybe constructor is in the prelude",
			input: `let a: Int? = Maybe::new(42)
let b: Int? = Maybe::new<Int>()`,
		},
		{
			name: "formal Maybe type spelling aliases nullable suffix",
			input: `let a: Maybe<Int> = Maybe::new(42)
let b: Int? = a`,
		},
		{
			name: "new preserves an existing Maybe value",
			input: `let none: Int? = Maybe::new<Int>()
let a: Int? = Maybe::new(none)
let some: Int? = Maybe::new(1)
let b: Int? = Maybe::new(some)`,
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
		{
			name: "old Maybe some and none constructors are no longer available",
			input: `let a = Maybe::some(1)
let b = Maybe::none<Int>()`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined: Maybe::some"},
				{Kind: checker.Error, Message: "Undefined: Maybe::none"},
			},
		},
	})
}

func TestMaybeMutableMethods(t *testing.T) {
	run(t, []test{
		{
			name: "set and clear mutate a maybe",
			input: `mut m = Maybe::new<Int>()
m.set(42)
m.clear()
let done: Bool = m.is_none()`,
		},
		{
			name: "set requires an inner value",
			input: `mut m = Maybe::new<Int>()
m.set("nope")`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
			},
		},
		{
			name: "set requires mutable receiver",
			input: `let m = Maybe::new<Int>()
m.set(1)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Immutable: Maybe.set receiver"},
			},
		},
		{
			name: "clear requires mutable receiver",
			input: `let m = Maybe::new(1)
m.clear()`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Immutable: Maybe.clear receiver"},
			},
		},
	})
}
