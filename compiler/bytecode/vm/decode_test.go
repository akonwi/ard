package vm

import "testing"

func TestBytecodeDecodeBasicPrimitives(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
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

func TestBytecodeDecodeErrors(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "string decoder fails on int - returns error list",
			input: `
				use ard/decode
				let data = Dynamic::from(42)
				let result = decode::run(data, decode::string)
				match result {
					err(errs) => {
						if not errs.size() == 1 { panic("Expected 1 error. Got {errs.size()}") }
						errs.at(0).to_str()
					},
					ok(_) => ""
				}
			`,
			want: "got 42, expected Str",
		},
		{
			name: "int decoder fails on string",
			input: `
				use ard/decode
				let data = Dynamic::from("hello")
				let result = decode::run(data, decode::int)
				result.is_err()
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
					ok(_) => ""
				}
			`,
			want: "[1]: got false, expected Int",
		},
		{
			name: "error path with nested field and list",
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
			name: "from_json invalid json panics via expect",
			input: `
				use ard/decode
				decode::from_json("invalid json").expect("Failed to parse json")
			`,
			panic: "Failed to parse json",
		},
	})
}

func TestBytecodeDecodeNullable(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "nullable string decoder with valid string returns some",
			input: `
				use ard/decode
				let data = Dynamic::from("hello")
				let maybe_str = decode::run(data, decode::nullable(decode::string)).expect("Decoding failed")
				match maybe_str {
					str => str == "hello",
					_ => false
				}
			`,
			want: true,
		},
		{
			name: "nullable string on null returns none",
			input: `
				use ard/decode
				let data = decode::from_json("null").expect("parse")
				let result = decode::run(data, decode::nullable(decode::string)).expect("decode")
				result.is_none()
			`,
			want: true,
		},
		{
			name: "nullable int decoder with valid int returns some",
			input: `
				use ard/decode
				let data = Dynamic::from(42)
				let result = decode::run(data, decode::nullable(decode::int)).expect("decode")
				result.or(0)
			`,
			want: 42,
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
	})
}

func TestBytecodeDecodeList(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "can decode a list",
			input: `
				use ard/decode
				let data = decode::from_json("[1, 2, 3, 4, 5]").expect("parse")
				let list = decode::run(data, decode::list(decode::int)).expect("decode")
				if not list.size() == 5 { panic("Expected 5 elements") }
				list.at(4)
			`,
			want: 5,
		},
		{
			name: "empty list returns size 0",
			input: `
				use ard/decode
				let data = decode::from_json("[]").expect("parse")
				let list = decode::run(data, decode::list(decode::string)).expect("decode")
				list.size()
			`,
			want: 0,
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
			name: "list with invalid element returns one error",
			input: `
				use ard/decode
				let data = decode::from_json("[\"hello\", 42, \"world\"]").expect("parse")
				let result = decode::run(data, decode::list(decode::string))
				match result {
					ok => false,
					err(errs) => errs.size() == 1,
				}
			`,
			want: true,
		},
	})
}

func TestBytecodeDecodeMap(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "empty map returns size 0",
			input: `
				use ard/decode
				let data = decode::from_json("\{\}").expect("parse")
				let decode_map = decode::map(decode::string, decode::string)
				let m = decode_map(data).expect("decode")
				m.size()
			`,
			want: 0,
		},
		{
			name: "map of string keys to integers",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"age\": 30, \"score\": 95\}").expect("parse")
				let decode_map = decode::map(decode::string, decode::int)
				let m = decode_map(data).expect("decode")
				m.get("age").or(0)
			`,
			want: 30,
		},
		{
			name: "map decoder with non-object data returns error",
			input: `
				use ard/decode
				let data = decode::from_json("\"not an object\"").expect("parse")
				let decode_map = decode::map(decode::string, decode::string)
				decode_map(data).is_err()
			`,
			want: true,
		},
		{
			name: "map with invalid values returns error",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"name\": \"Alice\", \"age\": 30\}").expect("parse")
				let decode_map = decode::map(decode::string, decode::string)
				let result = decode_map(data)
				match result {
					ok => false,
					err(errs) => errs.size() == 1,
				}
			`,
			want: true,
		},
	})
}

func TestBytecodeDecodeField(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "field decoder with null data returns error",
			input: `
				use ard/decode
				let data = decode::from_json("null").expect("parse")
				let result = decode::run(data, decode::field("name", decode::string))
				result.is_err()
			`,
			want: true,
		},
		{
			name: "field decoder with non-object data returns error",
			input: `
				use ard/decode
				let data = decode::from_json("\"not an object\"").expect("parse")
				let result = decode::run(data, decode::field("name", decode::string))
				result.is_err()
			`,
			want: true,
		},
		{
			name: "decode field string",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"name\": \"John Doe\"\}").expect("parse")
				decode::run(data, decode::field("name", decode::string)).expect("decode")
			`,
			want: "John Doe",
		},
	})
}

func TestBytecodeDecodePath(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "path with only string segments",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"foo\": \{\"bar\": 42\}\}").expect("parse")
				let result = decode::run(data, decode::path(["foo", "bar"], decode::int))
				result.expect("decode")
			`,
			want: 42,
		},
		{
			name: "path with array index",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"items\": [10, 20, 30]\}").expect("parse")
				let result = decode::run(data, decode::path(["items", 1], decode::int))
				result.expect("decode")
			`,
			want: 20,
		},
		{
			name: "decode nested path with mixed segments",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"response\": [\{\"name\": \"Alice\"\}, \{\"name\": \"Bob\"\}]\}").expect("parse")
				decode::run(data, decode::path(["response", 0, "name"], decode::string)).expect("decode")
			`,
			want: "Alice",
		},
		{
			name: "path error includes array index",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"items\": [\"a\", \"b\"]\}").expect("parse")
				let result = decode::run(data, decode::path(["items", 1], decode::int))
				match result {
					ok => "unexpected success",
					err(errs) => errs.at(0).to_str(),
				}
			`,
			want: "items[1]: got \"b\", expected Int",
		},
	})
}

func TestBytecodeDecodePathWithTry(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "path with try in function",
			input: `
				use ard/decode
				fn extract_id(data: Dynamic) Int![decode::Error] {
					let id = try decode::run(data, decode::path(["fixture", "id"], decode::int))
					Result::ok(id)
				}
				let json = "\{\"fixture\": \{\"id\": 42\}\}"
				let data = decode::from_json(json).expect("parse")
				extract_id(data).expect("extract")
			`,
			want: 42,
		},
		{
			name: "path with try in a function - multiple calls",
			input: `
				use ard/decode
				fn extract_fixture(data: Dynamic) [Int]![decode::Error] {
					let id = try decode::run(data, decode::path(["fixture", "id"], decode::int))
					let timestamp = try decode::run(data, decode::path(["fixture", "timestamp"], decode::int))
					Result::ok([id, timestamp])
				}
				let json = "\{\"fixture\": \{\"id\": 1446667, \"timestamp\": 12345\}\}"
				let data = decode::from_json(json).expect("parse")
				let result = extract_fixture(data).expect("extract")
				result.at(0)
			`,
			want: 1446667,
		},
	})
}

func TestBytecodeDecodeOneOf(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "one_of falls back to alternate decoder",
			input: `
				use ard/decode
				fn int_to_string(data: Dynamic) Str![decode::Error] {
					let int = try decode::int(data)
					Result::ok(int.to_str())
				}
				let data = decode::from_json("20").expect("parse")
				let take_string = decode::one_of(decode::string, [int_to_string])
				take_string(data).expect("decode")
			`,
			want: "20",
		},
		{
			name: "returns first decoder errors when all fail",
			input: `
				use ard/decode
				fn int_to_string(data: Dynamic) Str![decode::Error] {
					let int = try decode::int(data)
					Result::ok(int.to_str())
				}
				let data = decode::from_json("true").expect("parse")
				let take_string = decode::one_of(decode::string, [int_to_string])
				match take_string(data) {
					ok => "unexpected success",
					err(errs) => errs.at(0).to_str(),
				}
			`,
			want: "got true, expected Str",
		},
	})
}

func TestBytecodeDecodeCustomFunctions(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "custom decoder function works",
			input: `
				use ard/decode
				fn custom_decoder(data: Dynamic) Str![decode::Error] {
					Result::ok("always works")
				}
				let data = decode::from_json("\"hello\"").expect("parse")
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
					Result::ok(Person{id: person_id, name: person_name})
				}
				let data = decode::from_json("\{\"id\": 123, \"name\": \"Alice\"\}").expect("parse")
				let maybe_person = decode::run(data, decode::nullable(decode_person)).expect("decode")
				match maybe_person {
					person => person.name,
					_ => "none",
				}
			`,
			want: "Alice",
		},
	})
}

func TestBytecodeDecodeFlatten(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "flatten single error",
			input: `
				use ard/decode
				let errors = [decode::Error{expected: "Int", found: "false", path: []}]
				decode::flatten(errors)
			`,
			want: "got false, expected Int",
		},
		{
			name: "flatten multiple errors with newlines",
			input: `
				use ard/decode
				let errors = [
					decode::Error{expected: "Int", found: "false", path: ["[1]"]},
					decode::Error{expected: "Str", found: "42", path: ["[2]"]},
				]
				decode::flatten(errors)
			`,
			want: "[1]: got false, expected Int\n[2]: got 42, expected Str",
		},
		{
			name: "flatten empty error list",
			input: `
				use ard/decode
				let errors: [decode::Error] = []
				decode::flatten(errors)
			`,
			want: "",
		},
	})
}

func TestBytecodeDeepCustomDecoders(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "deep custom decoder through fixtures",
			input: `
				use ard/decode
				use ard/fs

				let text = fs::read("./fixtures/json_data.json").or("")
				if text.is_empty() { panic("Empty json file") }

				fn first(as: fn(Dynamic) $T![decode::Error]) fn(Dynamic) $T![decode::Error] {
					fn(data: Dynamic) $T![decode::Error] {
						let list = try decode::run(data, decode::list(as))
						Result::ok(list.at(0))
					}
				}

				let data = decode::from_json(text).expect("Unable to parse json")
				let res = decode::run(data,
					decode::field("response",
						first(decode::field("bookmakers", first(decode::field("bets", first(decode::field("name", decode::string))))))
					)
				)

				match res {
					ok(name) => name,
					err(errs) => errs.at(0).to_str()
				}
			`,
			want: "Match Winner",
		},
	})
}
