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
			name: "string decoder fails on int - returns error list",
			input: `
				use ard/decode

				let data = decode::any("42")
				let result = decode::decode(decode::string(), data)
				match result {
					err => err.size() == 1
					ok(_) => false
				}
			`,
			want: true,
		},
		{
			name: "int decoder fails on string - returns error list",
			input: `
				use ard/decode

				let data = decode::any("\"hello\"")
				let result = decode::decode(decode::int(), data)
				match result {
					err => err.size() == 1
					ok(_) => false
				}
			`,
			want: true,
		},
		{
			name: "bool decoder fails on string - returns error list",
			input: `
				use ard/decode

				let data = decode::any("\"true\"")
				let result = decode::decode(decode::bool(), data)
				match result {
					err => err.size() == 1
					ok(_) => false
				}
			`,
			want: true,
		},
		{
			name: "float decoder fails on string - returns error list",
			input: `
				use ard/decode

				let data = decode::any("\"3.14\"")
				let result = decode::decode(decode::float(), data)
				match result {
					err => err.size() == 1
					ok(_) => false
				}
			`,
			want: true,
		},
		{
			name: "invalid external data fails at decode time",
			input: `
				use ard/decode

				// Invalid JSON becomes nil Dynamic, which fails at decode
				let data = decode::any("invalid json")
				let result = decode::decode(decode::string(), data)
				match result {
					err => err.size() == 1
					ok(_) => false
				}
			`,
			want: true,
		},
		{
			name: "error contains expected and found information",
			input: `
				use ard/decode

				let data = decode::any("42")
				let result = decode::decode(decode::string(), data)
				match result {
					err => {
						let first_error = err.at(0)
						first_error.expected == "Str" && first_error.found == "Dynamic"
					}
					ok(_) => false
				}
			`,
			want: true,
		},
		{
			name: "error path is empty for primitive decoders",
			input: `
				use ard/decode

				let data = decode::any("\"not_a_number\"")
				let result = decode::decode(decode::int(), data)
				match result {
					err => {
						let first_error = err.at(0)
						first_error.path.size() == 0
					}
					ok(_) => false
				}
			`,
			want: true,
		},
		{
			name: "null data produces null error message",
			input: `
				use ard/decode

				let data = decode::any("invalid json")  // becomes nil Dynamic
				let result = decode::decode(decode::int(), data)
				match result {
					err => {
						let first_error = err.at(0)
						first_error.expected == "Int" && first_error.found == "Void"
					}
					ok(_) => false
				}
			`,
			want: true,
		},
	})
}

func TestDecodeNullable(t *testing.T) {
	runTests(t, []test{
		{
			name: "nullable string decoder with valid string returns some",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("\"hello\"")
				let string_decoder = decode::string()
				let nullable_decoder = decode::nullable(string_decoder)
				let result = decode::decode(nullable_decoder, data)
				result.expect("").or("default")
			`,
			want: "hello",
		},
		{
			name: "nullable int decoder with valid int returns some",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("42")
				let int_decoder = decode::int()
				let nullable_decoder = decode::nullable(int_decoder)
				let result = decode::decode(nullable_decoder, data)
				result.expect("").or(0)
			`,
			want: 42,
		},
		{
			name: "nullable decoder with null data returns none - uses default",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("null")
				let string_decoder = decode::string()  
				let nullable_decoder = decode::nullable(string_decoder)
				let result = decode::decode(nullable_decoder, data)
				result.expect("").or("default_value")
			`,
			want: "default_value",
		},
		{
			name: "nullable decoder propagates inner decoder errors",
			input: `
				use ard/decode

				let data = decode::any("42")
				let string_decoder = decode::string()
				let nullable_decoder = decode::nullable(string_decoder) 
				let result = decode::decode(nullable_decoder, data)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "invalid JSON becomes null - nullable returns default",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("invalid json")  // becomes nil Dynamic
				let int_decoder = decode::int()
				let nullable_decoder = decode::nullable(int_decoder)
				let result = decode::decode(nullable_decoder, data)  
				result.expect("").or(999)
			`,
			want: 999,
		},
	})
}
