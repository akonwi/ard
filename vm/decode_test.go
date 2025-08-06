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

func TestDecodeList(t *testing.T) {
	runTests(t, []test{
		{
			name: "list of integers - returns size",
			input: `
				use ard/decode

				let data = decode::any("[1, 2, 3, 4, 5]")
				let int_decoder = decode::int()
				let list_decoder = decode::list(int_decoder)
				let result = decode::decode(list_decoder, data)
				let list = result.expect("")
				if not list.size() == 5 {
					panic("Expected 5 elements")
				}
				list.at(4)
			`,
			want: 5,
		},
		{
			name: "empty list - returns size 0",
			input: `
				use ard/decode

				let data = decode::any("[]")
				let string_decoder = decode::string()
				let list_decoder = decode::list(string_decoder)
				let result = decode::decode(list_decoder, data)
				let list = result.expect("")
				list.size()
			`,
			want: 0,
		},
		{
			name: "list decoder with null data returns error",
			input: `
				use ard/decode

				let data = decode::any("null")
				let string_decoder = decode::string()
				let list_decoder = decode::list(string_decoder)
				let result = decode::decode(list_decoder, data)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "list decoder with non-array data returns error",
			input: `
				use ard/decode

				let data = decode::any("\"not an array\"")
				let string_decoder = decode::string()
				let list_decoder = decode::list(string_decoder)
				let result = decode::decode(list_decoder, data)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "list with invalid elements returns error",
			input: `
				use ard/decode

				let data = decode::any("[\"hello\", 42, \"world\"]")
				let string_decoder = decode::string()
				let list_decoder = decode::list(string_decoder)
				let result = decode::decode(list_decoder, data)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "list of nullable strings with mixed content",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("[\"hello\", null, \"world\"]")
				let nullable_string_decoder = decode::nullable(decode::string())
				let list_decoder = decode::list(nullable_string_decoder)
				let result = decode::decode(list_decoder, data)
				result.expect("").at(1).is_none()
			`,
			want: true,
		},
		{
			name: "nullable list with array data returns some",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("[1, 2, 3]")
				let int_decoder = decode::int()
				let list_decoder = decode::list(int_decoder)
				let nullable_list_decoder = decode::nullable(list_decoder)
				let result = decode::decode(nullable_list_decoder, data)
				let maybe_list = result.expect("")
				match maybe_list {
				  list => list.size(),
					_ => 0
				}
			`,
			want: 3,
		},
		{
			name: "nullable list with null data returns none - uses empty default",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("null")
				let int_decoder = decode::int()
				let list_decoder = decode::list(int_decoder)
				let nullable_list_decoder = decode::nullable(list_decoder)
				let result = decode::decode(nullable_list_decoder, data)
				let maybe_list = result.expect("")
				let maybe_list = result.expect("")
				match maybe_list {
				  list => list.size(),
					_ => 0
				}
			`,
			want: 0,
		},
	})
}

func TestDecodeMap(t *testing.T) {
	runTests(t, []test{
		{
			name: "empty map - returns size 0", 
			input: `
				use ard/decode

				let data = decode::any("\\{\\}")
				let string_decoder = decode::string()
				let map_decoder = decode::map(string_decoder, string_decoder)
				let result = decode::decode(map_decoder, data)
				let map = result.expect("")
				map.size()
			`,
			want: 0,
		},
		{
			name: "map of string keys to integers - get specific value",
			input: `
				use ard/decode

				let data = decode::any("\\{\\\"age\\\": 30, \\\"score\\\": 95\\}")
				let string_decoder = decode::string()
				let int_decoder = decode::int()
				let map_decoder = decode::map(string_decoder, int_decoder)
				let result = decode::decode(map_decoder, data)
				let map = result.expect("")
				map.get("age").or(0)
			`,
			want: 30,
		},
		{
			name: "map decoder with null data returns error",
			input: `
				use ard/decode

				let data = decode::any("null")
				let string_decoder = decode::string()
				let map_decoder = decode::map(string_decoder, string_decoder)
				let result = decode::decode(map_decoder, data)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "map decoder with non-object data returns error",
			input: `
				use ard/decode

				let data = decode::any("\"not an object\"")
				let string_decoder = decode::string()
				let map_decoder = decode::map(string_decoder, string_decoder)
				let result = decode::decode(map_decoder, data)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "map decoder with array data returns error",
			input: `
				use ard/decode

				let data = decode::any("[1, 2, 3]")
				let string_decoder = decode::string()
				let map_decoder = decode::map(string_decoder, string_decoder)
				let result = decode::decode(map_decoder, data)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "map with invalid values returns error",
			input: `
				use ard/decode

				let data = decode::any("\\{\\\"name\\\": \\\"Alice\\\", \\\"age\\\": 30\\}")
				let string_decoder = decode::string()
				let map_decoder = decode::map(string_decoder, string_decoder)
				let result = decode::decode(map_decoder, data)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "map of nullable strings with mixed content",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("\\{\\\"name\\\": \\\"Alice\\\", \\\"nickname\\\": null, \\\"city\\\": \\\"Boston\\\"\\}")
				let nullable_string_decoder = decode::nullable(decode::string())
				let map_decoder = decode::map(decode::string(), nullable_string_decoder)
				let result = decode::decode(map_decoder, data)
				let map = result.expect("")
				map.get("nickname").or(maybe::some("default")).is_none()
			`,
			want: true,
		},
		{
			name: "nullable map with object data returns some",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("\\{\\\"count\\\": 42\\}")
				let int_decoder = decode::int()
				let map_decoder = decode::map(decode::string(), int_decoder)
				let nullable_map_decoder = decode::nullable(map_decoder)
				let result = decode::decode(nullable_map_decoder, data)
				let maybe_map = result.expect("")
				match maybe_map {
					map => map.size(),
					_ => 0
				}
			`,
			want: 1,
		},
		{
			name: "nullable map with null data returns none - uses default size",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("null")
				let int_decoder = decode::int()
				let map_decoder = decode::map(decode::string(), int_decoder)
				let nullable_map_decoder = decode::nullable(map_decoder)
				let result = decode::decode(nullable_map_decoder, data)
				let maybe_map = result.expect("")
				match maybe_map {
					map => map.size(),
					_ => 0
				}
			`,
			want: 0,
		},
	})
}
