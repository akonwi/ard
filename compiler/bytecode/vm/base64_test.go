package vm

import "testing"

func TestBytecodeBase64(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "encode standard",
			input: `
				use ard/base64
				base64::encode("hello")
			`,
			want: "aGVsbG8=",
		},
		{
			name: "encode empty string",
			input: `
				use ard/base64
				base64::encode("")
			`,
			want: "",
		},
		{
			name: "decode standard roundtrip",
			input: `
				use ard/base64
				match base64::decode("aGVsbG8=") {
					ok(v) => v,
					err(_) => "error",
				}
			`,
			want: "hello",
		},
		{
			name: "decode invalid input returns err",
			input: `
				use ard/base64
				base64::decode("not!valid!base64").is_err()
			`,
			want: true,
		},
		{
			name: "encode_url uses urlsafe alphabet",
			input: `
				use ard/base64
				base64::encode_url("subjects?")
			`,
			// base64url uses '_' instead of '/' for the 63rd character.
			want: "c3ViamVjdHM_",
		},
		{
			name: "encode_url_no_pad strips padding",
			input: `
				use ard/base64
				base64::encode_url_no_pad("f")
			`,
			want: "Zg",
		},
		{
			name: "encode_url_no_pad with no padding needed",
			input: `
				use ard/base64
				base64::encode_url_no_pad("foo")
			`,
			want: "Zm9v",
		},
		{
			name: "decode_url roundtrip",
			input: `
				use ard/base64
				match base64::decode_url("aGVsbG8gd29ybGQ=") {
					ok(v) => v,
					err(_) => "error",
				}
			`,
			want: "hello world",
		},
		{
			name: "decode_url_no_pad roundtrip",
			input: `
				use ard/base64
				match base64::decode_url_no_pad("aGVsbG8gd29ybGQ") {
					ok(v) => v,
					err(_) => "error",
				}
			`,
			want: "hello world",
		},
		{
			name: "decode_url rejects invalid input",
			input: `
				use ard/base64
				base64::decode_url("$$$$").is_err()
			`,
			want: true,
		},
		{
			name: "encode and decode full roundtrip",
			input: `
				use ard/base64
				let original = "The quick brown fox jumps over the lazy dog"
				let encoded = base64::encode(original)
				match base64::decode(encoded) {
					ok(v) => v,
					err(_) => "error",
				}
			`,
			want: "The quick brown fox jumps over the lazy dog",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}
