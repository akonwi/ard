package vm_test

import "testing"

func TestHttpMethod(t *testing.T) {
	runTests(t, []test{
		{
			name: "Method implements Str::ToString()",
			input: `
				use ard/http
				let method = http::Method::Post
				"{method}"
			`,
			want: "POST",
		},
	})
}
