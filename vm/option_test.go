package vm_test

import "testing"

func TestOptionals(t *testing.T) {
	runTests(t, []test{
		{
			name: "Equality comparison returns false when each are different",
			input: `
				use ard/option
				option.some("hello") == option.none()
			`,
			want: false,
		},
		{
			name: "Equality comparison returns true when both are the same",
			input: `
				use ard/option
				option.some("hello") == option.some("hello")
			`,
			want: true,
		},
		{
			name: "Equality comparison returns true when both are none",
			input: `
				use ard/option
				option.none() == option.none()
			`,
			want: true,
		},
		{
			name: ".or() can be used to read and fallback to a default value",
			input: `
				use ard/option
				let a: Str? = option.none()
				a.or("foo")
			`,
			want: "foo",
		},
	})
}
