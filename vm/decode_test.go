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

				let data = Dynamic::from("hello")
				decode::run(data, decode::string).expect("")
			`,
			want: "hello",
		},
		{
			name: "decode int from external data",
			input: `
				use ard/decode

				let data = Dynamic::from(42)
				decode::run(data, decode::int).expect("Failed to decode")
			`,
			want: 42,
		},
		{
			name: "decode float from external data",
			input: `
				use ard/decode

				let data = Dynamic::from(3.14)
				decode::run(data, decode::float).expect("")
			`,
			want: 3.14,
		},
		{
			name: "decode bool from external data",
			input: `
				use ard/decode

				let data = Dynamic::from(true)
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

				let data = Dynamic::from(42)
				let result = decode::run(data, decode::string)
				match result {
					err(errs) => {
						if not errs.size() == 1 { panic("Expected 1 error. Got {errs.size()}") }
						let first = errs.at(0)
						first.to_str()
					},
					ok(_) => ""
				}
			`,
			want: "got 42, expected Str",
		},
		{
			name: "int decoder fails on string - returns error list",
			input: `
				use ard/decode

				let data = Dynamic::from("hello")
				let result = decode::run(data, decode::int)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "bool decoder fails on string - returns error list",
			input: `
				use ard/decode

				let data = Dynamic::from("true")
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

				let data = Dynamic::from("3.14")
				let result = decode::run(data, decode::float)
				match result {
					err => err.size() == 1
					ok(_) => false
				}
			`,
			want: true,
		},
		{
			name: "from_json returns Result::err for invalid json",
			input: `
				use ard/decode

				let data = decode::from_json("invalid json").expect("Failed to parse json")
			`,
			panic: "Failed to parse json",
		},
		{
			name: "error path is empty for primitive decoders",
			input: `
				use ard/decode

				let data = Dynamic::from("not_a_number")
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
			name: "list item errors include the failed index",
			input: `
				use ard/decode

				let data = decode::from_json("[1, false, 3]").expect("Failed to parse json")
				let result = decode::run(data, decode::list(decode::int))
				match result {
					err(errs) => {
						if not errs.size() == 1 { panic("Expected 1 error: got {errs.size()}") }
						errs.at(0).to_str()
					}
					ok(_) => "",
				}
			`,
			want: "[1]: got false, expected Int",
		},
		{
			name: "null data produces null error message",
			input: `
				use ard/decode

				let data = Dynamic::from("invalid json")
				let result = decode::run(data, decode::int)
				match result {
					err => err.at(0).to_str(),
					ok(_) => false
				}
			`,
			want: `got "invalid json", expected Int`,
		},
		{
			name: "Error string includes path information",
			input: `
				use ard/decode

				let data = decode::from_json("[\{\"value\": \"not_a_number\"\}]").expect("Unable to parse json")
				let result = decode::run(data, decode::list(decode::field("value", decode::int)))
				match result {
					ok => "unexpected success",
					err(errs) => errs.at(0).to_str()
				}
			`,
			want: "[0].value: got \"not_a_number\", expected Int",
		},
		{
			name: "shows enhanced array formatting for small arrays",
			input: `
				use ard/decode

				let array_data = decode::from_json("[1, 2, 3]").expect("Unable to parse json")
				let result = decode::run(array_data, decode::string)
				match result {
					ok => "unexpected success",
					err(errs) => errs.at(0).to_str()
				}
			`,
			want: "got [1, 2, 3], expected Str",
		},
		{
			name: "shows empty array formatting",
			input: `
				use ard/decode

				let empty_array_data = decode::from_json("[]").expect("Unable to parse json")
				let result = decode::run(empty_array_data, decode::string)
				match result {
					ok => "unexpected success",
					err(errs) => errs.at(0).to_str()
				}
			`,
			want: "got [], expected Str",
		},
		{
			name: "shows proper dot notation for nested paths",
			input: `
				use ard/decode

				// Test more complex path with nested field access
				let data = decode::from_json("[\{\"user\": \{\"profile\": \{\"age\": \"not_a_number\"\}\}\}]").expect("Unable to parse json")
				let result = decode::run(data, decode::list(decode::field("user", decode::field("profile", decode::field("age", decode::int)))))
				match result {
					ok => "unexpected success",
					err(errs) => errs.at(0).to_str()
				}
			`,
			want: "[0].user.profile.age: got \"not_a_number\", expected Int",
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

				let data = Dynamic::from("hello")
				let maybe_str = decode::run(data, decode::nullable(decode::string)).expect("Decoding failed")
				match maybe_str {
					str => str == "hello",
					_ => panic("Decoded string is null")
				}
			`,
			want: true,
		},
		{
			name: "nullable int decoder with valid int returns some",
			input: `
				use ard/decode
				use ard/maybe

				let data = Dynamic::from(42)
				let result = decode::run(data, decode::nullable(decode::int)).expect("Decoding failed")
				result.or(0)
			`,
			want: 42,
		},
		{
			name: "nullable decoder with null data returns none - uses default",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::from_json("null").expect("Failed to parse json")
				let result = decode::run(data, decode::nullable(decode::string)).expect("Decoding failed")
				result.or("default_value")
			`,
			want: "default_value",
		},
		{
			name: "nullable decoder propagates inner decoder errors",
			input: `
				use ard/decode

				let data = decode::from_json("42").expect("Failed to parse json")
				let result = decode::run(data, decode::nullable(decode::string))
				result.is_err()
			`,
			want: true,
		},
		{
			name: "invalid JSON becomes null - nullable returns default",
			input: `
				use ard/decode

				decode::from_json("invalid json").expect("Failed to parse json")
			`,
			panic: "Failed to parse json",
		},
	})
}

func TestDecodeList(t *testing.T) {
	runTests(t, []test{
		{
			name: "can decode a list",
			input: `
				use ard/decode

				let data = decode::from_json("[1, 2, 3, 4, 5]").expect("Failed to parse json")
				let list = decode::run(data, decode::list(decode::int)).expect("")
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

				let data = decode::from_json("[]").expect("Failed to parse json")
				let list = decode::run(data, decode::list(decode::string)).expect("failed to decode")
				list.size()
			`,
			want: 0,
		},
		{
			name: "list decoder with null data returns error",
			input: `
				use ard/decode

				let data = decode::from_json("null").expect("Failed to parse json")
				let result = decode::run(data, decode::list(decode::string))
				result.is_err()
			`,
			want: true,
		},
		{
			name: "list decoder with non-list data returns error",
			input: `
				use ard/decode

				let data = Dynamic::from("not an array")
				let result = decode::run(data, decode::list(decode::string))
				result.is_err()
			`,
			want: true,
		},
		{
			name: "list with an element that can't be decoded returns error",
			input: `
				use ard/decode

				let data = decode::from_json("[\"hello\", 42, \"world\"]").expect("Failed to parse json")
				let result = decode::run(data, decode::list(decode::string))
				match result {
					ok => panic("Expected an error"),
					err(errs) => errs.size() == 1
				}
			`,
			want: true,
		},
		{
			name: "list of nullables",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::from_json("[\"hello\", null, \"world\"]").expect("Failed to parse json")
				let list_decoder = decode::list(decode::nullable(decode::string))
				let result = list_decoder(data).expect("Unexpected decoder error")
				result.at(1).is_none()
			`,
			want: true,
		},
		{
			name: "nullable list with array data returns some",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::from_json("[1, 2, 3]").expect("Failed to parse json")
				let maybe_list = decode::run(data, decode::nullable(decode::list(decode::int))).expect("Unexpected decoder error")
				match maybe_list {
				  list => list.size(),
					_ => 0
				}
			`,
			want: 3,
		},
		{
			name: "nullable list with null returns none",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::from_json("null").expect("Failed to parse json")
				let maybe_list = decode::run(data, decode::nullable(decode::list(decode::int))).expect("Unexpected decoder error")
				maybe_list.is_none()
			`,
			want: true,
		},
	})
}

func TestDecodeMap(t *testing.T) {
	runTests(t, []test{
		{
			name: "empty map - returns size 0",
			input: `
				use ard/decode

				let data = decode::from_json("\{\}").expect("Failed to parse json")
				let decode_map = decode::map(decode::string, decode::string)
				let map = decode_map(data).expect("Unexpected decoder error")
				map.size()
			`,
			want: 0,
		},
		{
			name: "map of string keys to integers - get specific value",
			input: `
				use ard/decode

				let data = decode::from_json("\{\"age\": 30, \"score\": 95\}").expect("Failed to parse json")
				let decode_map = decode::map(decode::string, decode::int)
				let map = decode_map(data).expect("Unexpected decoder error")
				map.get("age").or(0)
			`,
			want: 30,
		},
		{
			name: "map decoder with null data returns error",
			input: `
				use ard/decode

				let data = decode::from_json("null").expect("Failed to parse json")
				let decode_map = decode::map(decode::string, decode::string)
				let result = decode_map(data)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "map decoder with non-object data returns error",
			input: `
				use ard/decode

				let data = decode::from_json("\"not an object\"").expect("Failed to parse json")
				let decode_map = decode::map(decode::string, decode::string)
				let result = decode_map(data)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "map with invalid values returns error",
			input: `
				use ard/decode

				let data = decode::from_json("\{\"name\": \"Alice\", \"age\": 30\}").expect("Failed to parse json")
				let decode_map = decode::map(decode::string, decode::string)
				let result = decode_map(data)
				match result {
					ok => false,
					err(errs) => errs.size() == 1
				}
			`,
			want: true,
		},
		{
			name: "map of nullable strings with mixed content",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::from_json("\{\"name\": \"Alice\", \"nickname\": null, \"city\": \"Boston\"\}").expect("Failed to parse json")
				let decode_map = decode::map(decode::string, decode::nullable(decode::string))
				let map = decode_map(data).expect("Unexpected decode error")
				map.get("nickname").is_none()
			`,
			want: true,
		},
		{
			name: "nullable map with object data returns maybe::some",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::from_json("\{\"count\": 42\}").expect("Failed to parse json")
				let decode_map = decode::nullable(decode::map(decode::string, decode::int))
				let maybe_map = decode_map(data).expect("Unexpected decode error")
				match maybe_map {
					map => map.size(),
					_ => 0
				}
			`,
			want: 1,
		},
		{
			name: "nullable map with null data returns maybe::none",
			input: `
				use ard/decode
				use ard/maybe

				let data = decode::from_json("null").expect("Failed to parse json")
				let decode_map = decode::nullable(decode::map(decode::string, decode::int))
				let maybe_map = decode_map(data).expect("Unexpected decode error")
				maybe_map.is_none()
			`,
			want: true,
		},
	})
}

func TestDecodeField(t *testing.T) {
	runTests(t, []test{
		{
			name: "field decoder with null data returns error",
			input: `
				use ard/decode

				let data = decode::from_json("null").expect("Failed to parse json")
				let result = decode::run(data, decode::field("name", decode::string))
				result.is_err()
			`,
			want: true,
		},
		{
			name: "field decoder with non-object data returns error",
			input: `
				use ard/decode

				let data = decode::from_json("\"not an object\"").expect("Failed to parse json")
				let result = decode::run(data, decode::field("name", decode::string))
				result.is_err()
			`,
			want: true,
		},
		{
			name: "valid field decoding",
			input: `
				use ard/decode

				let data = decode::from_json("\{\"name\": \"John Doe\"\}").expect("Failed to parse json")
				let result = decode::run(data, decode::field("name", decode::string))
				result.expect("Failed to decode name")
			`,
			want: "John Doe",
		},
	})
}

func TestDecodePath(t *testing.T) {
	runTests(t, []test{
		{
			name: "valid field decoding",
			input: `
				use ard/decode
				use ard/json

				struct Qux {
					item: Int
				}

				struct Bar {
					qux: Qux
				}

				struct Foo {
					bar: Bar
				}

				let qux = Qux{item: 42}
				let bar = Bar{qux: qux}
				let foo = Foo{bar: bar}

				let json_str = json::encode(foo).expect("Failed to json encode")

				let data = decode::from_json(json_str).expect("Failed to parse json")
				let result = decode::run(data, decode::path(["bar", "qux", "item"], decode::int))
				result.expect("Failed to decode nested path")
			`,
			want: 42,
		},
	})
}

func TestDecodeOneOf(t *testing.T) {
	runTests(t, []test{
		{
			name: "when a decoder succeeds",
			input: `
				use ard/decode

				fn int_to_string(data: Dynamic) Str![decode::Error] {
					let int = try decode::int(data)
					Result::ok(int.to_str())
				}

				let data = decode::from_json("20").expect("Failed to parse json")
				let take_string = decode::one_of(decode::string, [int_to_string])
				take_string(data).expect("Unable to decode")
			`,
			want: "20",
		},
		{
			name: "returns the first decoder errors when all fail",
			input: `
				use ard/decode

				fn int_to_string(data: Dynamic) Str![decode::Error] {
					let int = try decode::int(data)
					Result::ok(int.to_str())
				}

				let data = decode::from_json("true").expect("Failed to parse json")
				let take_string = decode::one_of(decode::string, [int_to_string])
				match take_string(data) {
					ok => "unexpected success",
					err(errs) => errs.at(0).to_str()
				}
			`,
			want: "Decode error: expected Str, found true",
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

				fn custom_decoder(data: Dynamic) Str![decode::Error] {
					Result::ok("always works")
				}

				let data = decode::from_json("\"hello\"").expect("Failed to parse json")
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

				fn decode_person(data: Dynamic) Person![decode::Error] {
					let person_id = try decode::run(data, decode::field("id", decode::int))
					let person_name = try decode::run(data, decode::field("name", decode::string))

					Result::ok(Person{
						id: person_id,
						name: person_name,
					})
				}

				let data = decode::from_json("\{\"id\": 123, \"name\": \"Alice\"\}").expect("Failed to parse json")
				let maybe_person = decode::run(data, decode::nullable(decode_person)).expect("Failed to decode")
				match maybe_person {
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

				fn decode_person(data: Dynamic) Person![decode::Error] {
					let person_id = try decode::run(data, decode::field("id", decode::int))
					let person_name = try decode::run(data, decode::field("name", decode::string))

					Result::ok(Person{
						id: person_id,
						name: person_name,
					})
				}

				let data = decode::from_json("null").expect("Failed to parse json")
				let maybe_person = decode::run(data, decode::nullable(decode_person)).expect("Failed to decode")
				match maybe_person {
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

				fn decode_winner(data: Dynamic) Winner![decode::Error] {
					let winner_id = try decode::run(data, decode::field("id", decode::int))
					let winner_name = try decode::run(data, decode::field("name", decode::string))
					let winner_comment = try decode::run(data, decode::field("comment", decode::string))

					Result::ok(Winner{
						id: winner_id,
						name: winner_name,
						comment: winner_comment,
					})
				}

				fn decode_prediction(data: Dynamic) Prediction![decode::Error] {
					let advice = try decode::run(data, decode::field("advice", decode::nullable(decode::string)))
					let winner = try decode::run(data, decode::field("winner", decode::nullable(decode_winner)))

					Result::ok(Prediction{
						advice: advice,
						winner: winner,
					})
				}

				let data = decode::from_json(
					"\{\"advice\": \"Double chance\", \"winner\": \{\"id\": 1598, \"name\": \"Orlando City SC\", \"comment\": \"Win or draw\"\}\}"
				).expect("Failed to parse JSON")
				let prediction: Prediction = decode_prediction(data).expect("foo")
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

			fn first(as: fn(Dynamic) $T![decode::Error]) fn(Dynamic) $T![decode::Error] {
			  fn(data: Dynamic) $T![decode::Error] {
			    let list = try decode::run(data, decode::list(as))
			    Result::ok(list.at(0))
			  }
			}
			let data = decode::from_json("[3,2,1]").expect("Unable to parse json")
			match decode::run(data, first(decode::int)) {
				ok => ok,
				err(errs) => -1
			}
			`,
			want: 3,
		},
	})
}

func TestDeepCustomDecoders(t *testing.T) {
	input := `
				use ard/decode
				use ard/fs

				let text = fs::read("./fixtures/json_data.json").or("")
				if text.is_empty() { panic("Empty json file") }

				// create a decoder that takes the first in a list
				fn first(as: fn(Dynamic) $T![decode::Error]) fn(Dynamic) $T![decode::Error] {
					fn(data: Dynamic) $T![decode::Error] {
						let list = try decode::run(data, decode::list(as))
						Result::ok(list.at(0))
					}
				}

				let data = decode::from_json(text).expect("Unable to parse json")

				// Extract response[0].bookmakers[0].bets[0].name
				let res = decode::run(data,
					decode::field("response",
						first(decode::field("bookmakers", first(decode::field("bets", first(decode::field("name", decode::string))))))
					)
				)

				match res {
					ok(name) => name,
					err(errs) => errs.at(0).to_str()
				}
			`
	out := run(t, input)
	if out != "Match Winner" {
		t.Fatalf("Didn't get the expected string. Got '%s'", out)
	}
}
