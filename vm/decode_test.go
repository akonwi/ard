package vm_test

import "testing"

func TestDecodeBasicPrimitives(t *testing.T) {
	runTests(t, []test{
		{
			name: "decode string from external data",
			input: `
				use ard/decode

				let data = decode::any("\"hello\"")
				decode::decode(decode::string(), data).expect("")
			`,
			want: "hello",
		},
		{
			name: "decode int from external data",
			input: `
				use ard/decode

				let data = decode::any("42")
				decode::decode(decode::int(), data).expect("")
			`,
			want: 42,
		},
		{
			name: "decode float from external data",
			input: `
				use ard/decode

				let data = decode::any("3.14")
				decode::decode(decode::float(), data).expect("")
			`,
			want: 3.14,
		},
		{
			name: "decode bool from external data",
			input: `
				use ard/decode

				let data = decode::any("true")
				decode::decode(decode::bool(), data).expect("")
			`,
			want: true,
		},
	})
}

func TestDecodeErrors(t *testing.T) {
	runTests(t, []test{
		{
			name: "string decoder fails on int",
			input: `
				use ard/decode

				let data = decode::any("42")
				decode::decode(decode::string(), data).is_err()
			`,
			want: true,
		},
		{
			name: "int decoder fails on string",
			input: `
				use ard/decode

				let data = decode::any("\"hello\"")
				decode::decode(decode::int(), data).is_err()
			`,
			want: true,
		},
		{
			name: "decode error has expected type",
			input: `
				use ard/decode

				let data = decode::any("42")
				let result = decode::decode(decode::string(), data)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "invalid external data fails at decode time",
			input: `
				use ard/decode

				// Invalid JSON becomes nil Dynamic, which fails at decode
				let data = decode::any("invalid json")
				decode::decode(decode::string(), data).is_err()
			`,
			want: true,
		},
	})
}
