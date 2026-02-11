package vm

import "testing"

func TestBytecodeHttpMethod(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "Method implements ToString",
			input: `
				use ard/http
				let method = http::Method::Post
				"{method}"
			`,
			want: "POST",
		},
	})
}
