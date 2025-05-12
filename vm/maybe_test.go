package vm_test

import "testing"

func TestMaybes(t *testing.T) {
	runTests(t, []test{
		{
			name: "Equality comparison returns false when each are different",
			input: `
				use ard/maybe
				maybe::some("hello") == maybe::none()
			`,
			want: false,
		},
		{
			name: "Equality comparison returns true when both are the same",
			input: `
				use ard/maybe
				maybe::some("hello") == maybe::some("hello")
			`,
			want: true,
		},
		{
			name: "Equality comparison returns true when both are none",
			input: `
				use ard/maybe
				maybe::none() == maybe::none()
			`,
			want: true,
		},
		{
			name: ".or() can be used to read and fallback to a default value",
			input: `
				use ard/maybe
				let a: Str? = maybe::none()
				a.or("foo")
			`,
			want: "foo",
		},
		{
			name: ".is_none() returns true for nones",
			input: `
				use ard/maybe
				maybe::none().is_none()
			`,
			want: true,
		},
		{
			name: ".is_some() returns true for somes",
			input: `
				use ard/maybe
				maybe::some(1).is_some()
			`,
			want: true,
		},
	})
}
