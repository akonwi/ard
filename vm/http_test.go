package vm_test

import "testing"

func TestHttpMethodEnum(t *testing.T) {
	runTests(t, []test{
		{
			name: "Matching on http::Method",
			input: `
				use ard/http
				let meth = http::Method::Post
				match meth {
					http::Method::Get => "Safe read operation",
					http::Method::Post => "Create new resource",
					http::Method::Put => "Update entire resource",
					http::Method::Del => "Remove resource",
					http::Method::Patch => "Partial update"
				}
			`,
			want: "Create new resource",
		},
	})
}
