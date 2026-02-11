package vm

import "testing"

func TestBytecodeMaybes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
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
		{
			name: "reassigning maybes",
			input: `
				use ard/maybe
				mut a: Str? = maybe::none()
				a = maybe::some("hello")
				match a {
					s => s,
					_ => "",
				}
			`,
			want: "hello",
		},
		{
			name: ".expect() returns value for some",
			input: `
				use ard/maybe
				maybe::some(42).expect("Should not panic")
			`,
			want: 42,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}

	expectBytecodeRuntimeError(t, "Value was expected but got none", `
		use ard/maybe
		maybe::none().expect("Value was expected but got none")
	`)
}
