package vm_test

import "testing"

// NOTE: The JSON strings in these tests use escaped braces (e.g., "\{\"key\": \"value\"\}")
// This is required because Ard uses curly braces for string interpolation.
// To include literal braces in strings, they must be escaped with backslashes.
// Real-world usage works with this same escaping syntax.

func TestDecodeBasicPrimitives(t *testing.T) {
	runTests(t, []test{
		{
			name: "decode string from external data",
			input: `
				use ard/decode

				let data = decode::any("\"hello\"")
				decode::run(data, decode::string).expect("")
			`,
			want: "hello",
		},
		{
			name: "decode int from external data",
			input: `
				use ard/decode

				let data = decode::any("42")
				decode::run(data, decode::int).expect("")
			`,
			want: 42,
		},
		{
			name: "decode float from external data",
			input: `
				use ard/decode

				let data = decode::any("3.14")
				decode::run(data, decode::float).expect("")
			`,
			want: 3.14,
		},
		{
			name: "decode bool from external data",
			input: `
				use ard/decode

				let data = decode::any("true")
				decode::run(data, decode::bool).expect("")
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
				let result = decode::run(data, decode::string)
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
				let result = decode::run(data, decode::int)
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
				let result = decode::run(data, decode::bool)
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
				let result = decode::run(data, decode::float)
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
				let result = decode::run(data, decode::string)
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
				let result = decode::run(data, decode::string)
				match result {
					err => {
						let first_error = err.at(0)
						first_error.expected == "Str" && first_error.found == "42"
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
				let result = decode::run(data, decode::int)
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
				let result = decode::run(data, decode::int)
				match result {
					err => {
						let first_error = err.at(0)
						first_error.expected == "Int" && first_error.found == "null"
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
				let string_decoder = decode::string
				let nullable_decoder = decode::nullable(string_decoder)
				let result = decode::run(data, nullable_decoder)
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
				let int_decoder = decode::int
				let nullable_decoder = decode::nullable(int_decoder)
				let result = decode::run(data, nullable_decoder)
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
				let string_decoder = decode::string
				let nullable_decoder = decode::nullable(string_decoder)
				let result = decode::run(data, nullable_decoder)
				result.expect("").or("default_value")
			`,
			want: "default_value",
		},
		{
			name: "nullable decoder propagates inner decoder errors",
			input: `
				use ard/decode

				let data = decode::any("42")
				let string_decoder = decode::string
				let nullable_decoder = decode::nullable(string_decoder)
				let result = decode::run(data, nullable_decoder)
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
				let int_decoder = decode::int
				let nullable_decoder = decode::nullable(int_decoder)
				let result = decode::run(data, nullable_decoder)
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
				let int_decoder = decode::int
				let list_decoder = decode::list(int_decoder)
				let result = decode::run(data, list_decoder)
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
				let string_decoder = decode::string
				let list_decoder = decode::list(string_decoder)
				let result = decode::run(data, list_decoder)
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
				let string_decoder = decode::string
				let list_decoder = decode::list(string_decoder)
				let result = decode::run(data, list_decoder)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "list decoder with non-array data returns error",
			input: `
				use ard/decode

				let data = decode::any("\"not an array\"")
				let string_decoder = decode::string
				let list_decoder = decode::list(string_decoder)
				let result = decode::run(data, list_decoder)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "list with invalid elements returns error",
			input: `
				use ard/decode

				let data = decode::any("[\"hello\", 42, \"world\"]")
				let string_decoder = decode::string
				let list_decoder = decode::list(string_decoder)
				let result = decode::run(data, list_decoder)
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
				let nullable_string_decoder = decode::nullable(decode::string)
				let list_decoder = decode::list(nullable_string_decoder)
				let result = decode::run(data, list_decoder)
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
				let int_decoder = decode::int
				let list_decoder = decode::list(int_decoder)
				let nullable_list_decoder = decode::nullable(list_decoder)
				let result = decode::run(data, nullable_list_decoder)
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
				let int_decoder = decode::int
				let list_decoder = decode::list(int_decoder)
				let nullable_list_decoder = decode::nullable(list_decoder)
				let result = decode::run(data, nullable_list_decoder)
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

				let data = decode::any("\{\}")
				let string_decoder = decode::string
				let map_decoder = decode::map(string_decoder, string_decoder)
				let result = decode::run(data, map_decoder)
				let map = result.expect("")
				map.size()
			`,
			want: 0,
		},
		{
			name: "map of string keys to integers - get specific value",
			input: `
				use ard/decode

				let data = decode::any("\{\"age\": 30, \"score\": 95\}")
				let string_decoder = decode::string
				let int_decoder = decode::int
				let map_decoder = decode::map(string_decoder, int_decoder)
				let result = decode::run(data, map_decoder)
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
				let string_decoder = decode::string
				let map_decoder = decode::map(string_decoder, string_decoder)
				let result = decode::run(data, map_decoder)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "map decoder with non-object data returns error",
			input: `
				use ard/decode

				let data = decode::any("\"not an object\"")
				let string_decoder = decode::string
				let map_decoder = decode::map(string_decoder, string_decoder)
				let result = decode::run(data, map_decoder)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "map decoder with array data returns error",
			input: `
				use ard/decode

				let data = decode::any("[1, 2, 3]")
				let string_decoder = decode::string
				let map_decoder = decode::map(string_decoder, string_decoder)
				let result = decode::run(data, map_decoder)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "map with invalid values returns error",
			input: `
				use ard/decode

				let data = decode::any("\{\"name\": \"Alice\", \"age\": 30\}")
				let string_decoder = decode::string
				let map_decoder = decode::map(string_decoder, string_decoder)
				let result = decode::run(data, map_decoder)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "map of nullable strings with mixed content",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("\{\"name\": \"Alice\", \"nickname\": null, \"city\": \"Boston\"\}")
				let nullable_string_decoder = decode::nullable(decode::string)
				let map_decoder = decode::map(decode::string, nullable_string_decoder)
				let result = decode::run(data, map_decoder)
				let map = result.expect("")
				map.get("nickname").is_none()
			`,
			want: true,
		},
		{
			name: "nullable map with object data returns some",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::any("\{\"count\": 42\}")
				let int_decoder = decode::int
				let map_decoder = decode::map(decode::string, int_decoder)
				let nullable_map_decoder = decode::nullable(map_decoder)
				let result = decode::run(data, nullable_map_decoder)
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
				let int_decoder = decode::int
				let map_decoder = decode::map(decode::string, int_decoder)
				let nullable_map_decoder = decode::nullable(map_decoder)
				let result = decode::run(data, nullable_map_decoder)
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

func TestDecodeField(t *testing.T) {
	runTests(t, []test{
		{
			name: "field decoder with null data returns error",
			input: `
				use ard/decode

				let data = decode::any("null")
				let field_decoder = decode::field("name", decode::string)
				let result = decode::run(data, field_decoder)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "field decoder with non-object data returns error",
			input: `
				use ard/decode

				let data = decode::any("\"not an object\"")
				let field_decoder = decode::field("name", decode::string)
				let result = decode::run(data, field_decoder)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "field decoder with array data returns error",
			input: `
				use ard/decode

				let data = decode::any("[1, 2, 3]")
				let field_decoder = decode::field("name", decode::string)
				let result = decode::run(data, field_decoder)
				result.is_err()
			`,
			want: true,
		},
	})
}

func TestDecodeErrorToString(t *testing.T) {
	runTests(t, []test{
		{
			name: "decode error implements to_str for readable error messages",
			input: `
				use ard/decode

				let invalid_data = decode::any("\"hello\"")
				let result = decode::run(invalid_data, decode::int)
				match result {
					ok => "unexpected success",
					err => {
						let first_error = err.at(0)
						first_error.to_str()
					}
				}
			`,
			want: "Decode error: expected Int, found \"hello\"",
		},
		{
			name: "decode error to_str includes path information",
			input: `
				use ard/decode

				let data = decode::any("[\{\"value\": \"not_a_number\"\}]")
				let result = decode::run(data, decode::list(decode::field("value", decode::int)))
				match result {
					ok => "unexpected success",
					err => {
						let first_error = err.at(0)
						first_error.to_str()
					}
				}
			`,
			want: "Decode error: expected Int, found \"not_a_number\" at [0].value",
		},
		{
			name: "decode error shows enhanced array formatting for small arrays",
			input: `
				use ard/decode

				let array_data = decode::any("[1, 2, 3]")
				let result = decode::run(array_data, decode::string)
				match result {
					ok => "unexpected success",
					err => {
						let first_error = err.at(0)
						first_error.to_str()
					}
				}
			`,
			want: "Decode error: expected Str, found [1, 2, 3]",
		},

		{
			name: "decode error shows empty array formatting",
			input: `
				use ard/decode

				let empty_array_data = decode::any("[]")
				let result = decode::run(empty_array_data, decode::string)
				match result {
					ok => "unexpected success",
					err => {
						let first_error = err.at(0)
						first_error.to_str()
					}
				}
			`,
			want: "Decode error: expected Str, found []",
		},
		{
			name: "decode error handles complex values using premarshal consistently",
			input: `
				use ard/decode

				// Test that premarshal handles all types consistently
				let func_string_data = decode::any("\"Str -> Int\"")  // String that looks like a function type
				let result = decode::run(func_string_data, decode::int)
				match result {
					ok => "unexpected success",
					err => {
						let first_error = err.at(0)
						first_error.to_str()
					}
				}
			`,
			want: "Decode error: expected Int, found \"Str -> Int\"",
		},
		{
			name: "decode error shows proper dot notation for nested paths",
			input: `
				use ard/decode

				// Test more complex path with nested field access
				let data = decode::any("[\{\"user\": \{\"profile\": \{\"age\": \"not_a_number\"\}\}\}]")
				let result = decode::run(data, decode::list(decode::field("user", decode::field("profile", decode::field("age", decode::int)))))
				match result {
					ok => "unexpected success",
					err => {
						let first_error = err.at(0)
						first_error.to_str()
					}
				}
			`,
			want: "Decode error: expected Int, found \"not_a_number\" at [0].user.profile.age",
		},
	})
}

func TestDecodeOneOf(t *testing.T) {
	runTests(t, []test{
		{
			name: "one_of with only string decoder - basic functionality",
			input: `
				use ard/decode

				let data = decode::any("\"hello\"")
				let string_dec = decode::string
				let decoder = decode::one_of([string_dec])
				decode::run(data, decoder).expect("")
			`,
			want: "hello",
		},
		{
			name: "one_of returns error from first decoder when all fail",
			input: `
				use ard/decode

				let data = decode::any("true")
				let str_dec1 = decode::string
				let str_dec2 = decode::string
				let decoder = decode::one_of([str_dec1, str_dec2])
				let result = decode::run(data, decoder)
				match result {
					err => {
						let first_error = err.at(0)
						first_error.expected == "Str"
					}
					ok(_) => false
				}
			`,
			want: true,
		},
		{
			name: "one_of with different decoder types returning same type",
			input: `
				use ard/decode

				// Custom decoder that converts int to string
				fn int_to_string(data: decode::Dynamic) Str![decode::Error] {
					let int_result = decode::as_int(data)
					match int_result {
						ok(val) => Result::ok(val.to_str()),
						err(errors) => Result::err(errors)
					}
				}

				// Test string input - first decoder succeeds
				let data1 = decode::any("\"hello\"")
				let decoder = decode::one_of([decode::string, int_to_string])
				let result1 = decode::run(data1, decoder).expect("")

				// Test int input - second decoder succeeds
				let data2 = decode::any("42")
				let result2 = decode::run(data2, decoder).expect("")

				result1 + "," + result2
			`,
			want: "hello,42",
		},
		{
			name: "one_of with empty decoder list returns error",
			input: `
				use ard/decode

				let data = decode::any("\"hello\"")
				let empty_decoders: [fn(decode::Dynamic) Str![decode::Error]] = []
				let decoder = decode::one_of(empty_decoders)
				let result = decode::run(data, decoder)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "one_of empty decoder error message is descriptive",
			input: `
				use ard/decode

				let data = decode::any("\"hello\"")
				let empty_decoders: [fn(decode::Dynamic) Str![decode::Error]] = []
				let decoder = decode::one_of(empty_decoders)
				let result = decode::run(data, decoder)
				match result {
					err => {
						let first_error = err.at(0)
						first_error.expected == "At least one decoder" && first_error.found == "Empty decoder list"
					}
					ok(_) => false
				}
			`,
			want: true,
		},
	})
}

func TestDecodeCustomFunctions(t *testing.T) {
	runTests(t, []test{
		{
			name: "simple test that custom user-defined decoder function works",
			input: `
				use ard/decode
				use ard/result

				fn custom_decoder(data: decode::Dynamic) Str![decode::Error] {
					Result::ok("always works")
				}

				let data = decode::any("\"hello\"")
				decode::run(data, custom_decoder).expect("")
			`,
			want: "always works",
		},
		{
			name: "nullable custom struct decoder with non-null data",
			input: `
				use ard/decode

				struct Person {
					id: Int,
					name: Str,
				}

				fn decode_person(data: decode::Dynamic) Person![decode::Error] {
					let person_id = try decode::run(data, decode::field("id", decode::int))
					let person_name = try decode::run(data, decode::field("name", decode::string))

					Result::ok(Person{
						id: person_id,
						name: person_name,
					})
				}

				let data = decode::any("\{\"id\": 123, \"name\": \"Alice\"\}")
				let result = decode::run(data, decode::nullable(decode_person))
				match result.expect("") {
					person => person.name,
					_ => "none"
				}
			`,
			want: "Alice",
		},
		{
			name: "nullable custom struct decoder with null data",
			input: `
				use ard/decode
				use ard/maybe

				struct Person {
					id: Int,
					name: Str,
				}

				fn decode_person(data: decode::Dynamic) Person![decode::Error] {
					let person_id = try decode::run(data, decode::field("id", decode::int))
					let person_name = try decode::run(data, decode::field("name", decode::string))

					Result::ok(Person{
						id: person_id,
						name: person_name,
					})
				}

				let data = decode::any("null")
				let result = decode::run(data, decode::nullable(decode_person))
				match result.expect("") {
					person => person.name,
					_ => "none"
				}
			`,
			want: "none",
		},
		{
			name: "nullable custom struct in container struct - mirrors predictions.ard pattern",
			input: `
				use ard/decode

				struct Winner {
					id: Int,
					name: Str,
					comment: Str,
				}

				struct Prediction {
					advice: Str?,
					winner: Winner?,
				}

				fn decode_winner(data: decode::Dynamic) Winner![decode::Error] {
					let winner_id = try decode::run(data, decode::field("id", decode::int))
					let winner_name = try decode::run(data, decode::field("name", decode::string))
					let winner_comment = try decode::run(data, decode::field("comment", decode::string))

					Result::ok(Winner{
						id: winner_id,
						name: winner_name,
						comment: winner_comment,
					})
				}

				fn decode_prediction(data: decode::Dynamic) Prediction![decode::Error] {
					let advice = try decode::run(data, decode::field("advice", decode::nullable(decode::string)))
					let winner = try decode::run(data, decode::field("winner", decode::nullable(decode_winner)))

					Result::ok(Prediction{
						advice: advice,
						winner: winner,
					})
				}

				let data = decode::any("\{\"advice\": \"Double chance\", \"winner\": \{\"id\": 1598, \"name\": \"Orlando City SC\", \"comment\": \"Win or draw\"\}\}")
				let result = decode::run(data, decode_prediction)
				let prediction = result.expect("")
				match prediction.winner {
					winner => winner.name,
					_ => "no winner"
				}
			`,
			want: "Orlando City SC",
		},
		{
			name: "a function that decodes the first item in a list",
			input: `
			use ard/decode

			fn first(as: fn(decode::Dynamic) $T![decode::Error]) fn(decode::Dynamic) $T![decode::Error] {
			  fn(data: decode::Dynamic) $T![decode::Error] {
			    let list = try decode::run(data, decode::list(as))
			    Result::ok(list.at(0))
			  }
			}
			let data = decode::any("[3,2,1]")
			match decode::run(data, first(decode::int)) {
				ok => ok,
				err(errs) => -1
			}
			`,
			want: 3,
		},
	})
}
