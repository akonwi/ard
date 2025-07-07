package vm_test

import (
	"strings"
	"testing"
)

func TestJsonDecodePrimitives(t *testing.T) {
	runTests(t, []test{
		{
			name: "decoding Str",
			input: `
				use ard/json
				json::decode<Str>("\"hello\"")
			`,
			want: "hello",
		},
		{
			name: "decoding Int",
			input: `
				use ard/json
				json::decode<Int>("200")
			`,
			want: 200,
		},
		{
			name: "decoding Bool",
			input: `
				use ard/json
				json::decode<Bool>("true")
			`,
			want: true,
		},
		{
			name: "decoding Int?",
			input: `
				use ard/json
				json::decode<Int?>("null").expect("").is_none()
			`,
			want: true,
		},
		{
			name: "decoding Str?",
			input: `
				use ard/json
				json::decode<Str?>("null").expect("").is_none()
			`,
			want: true,
		},
		{
			name: "decoding Bool?",
			input: `
				use ard/json
				json::decode<Bool?>("null").expect("").is_none()
			`,
			want: true,
		},
	})
}

func TestJsonDecodeList(t *testing.T) {
	result := run(t, `
		use ard/json
		let empty: [Int] = []
		let nums = json::decode<[Int]>("[1,2,3]").expect("")
		nums.at(1)
	`)

	if result != 2 {
		t.Errorf("Expected 2, got %v", result)
	}
}

func TestJsonDecodeStruct(t *testing.T) {
	runTests(t, []test{
		{
			name: "simple decoding",
			input: `
				use ard/json
				struct Person {
					name: Str,
					age: Int,
		  		employed: Bool
				}
				let john_str = "\{\"name\": \"John\", \"age\": 30, \"employed\": true}"
				let john = json::decode<Person>(john_str).expect("")
				john.name == "John" and john.age == 30 and john.employed == true
			`,
			want: true,
		},
		{
			name: "unexpected fields are ignored",
			input: `
				use ard/json
				struct Person {
					name: Str,
					age: Int,
		  		employed: Bool
				}
				let john_str = "\{\"name\": \"John\", \"age\": 30, \"employed\": true, \"id\": 42}"
				let john = json::decode<Person>(john_str).expect("")
				json::encode(john).expect("")
			`,
			want: `{"age":30,"employed":true,"name":"John"}`,
		},
		{
			name: "non-nullable fields are required",
			input: `
				use ard/json
				struct Person {
					name: Str,
					age: Int,
		  		employed: Bool
				}
				let john_str = "\{\"name\": \"John\", \"age\": 30}"
				json::decode<Person>(john_str).expect("Missing 'employed' field")
			`,
			panic: "Missing field",
		},
	})
}

// func TestJsonDecodeStructsWithMaybes(t *testing.T) {
// 	result := run(t, `
// 		use ard/json
// 		struct Person {
// 			name: Str?,
// 			age: Int?,
// 		  employed: Bool?
// 		}
// 		let john_str = "\{\"name\": \"John\", \"age\": null}"
// 		let result = json::decode<Person>(john_str)
// 		match result {
// 		  ok => ok.name.or("") == "John" and ok.age.or(0) == 0 and ok.employed.or(false) == false,
// 			err => false
// 		}
// 	`)

// 	if result != true {
// 		t.Errorf("Wanted %v, got %v", true, result)
// 	}
// }

// func TestJsonDecodeNestedStructWithList(t *testing.T) {
// 	result := run(t, `
// 		use ard/json
// 		struct Person {
// 			name: Str,
// 			id: Int,
// 		}
// 		struct Payload {
// 		  people: [Person]
// 		}

// 		let input = "\{ \"people\": [ \{ \"name\": \"John\", \"id\": 1 } ] }"
// 		let result = json::decode<Payload>(input)
// 		match result {
// 		  ok => ok.people.at(0).name,
// 			err => panic(err)
// 		}
// 	`)

// 	if result != "John" {
// 		t.Errorf("Wanted %v, got %v", "John", result)
// 	}
// }

func TestJsonEncode(t *testing.T) {
	result := run(t, `
		use ard/json
		struct Person {
			name: Str,
			age: Int,
		  employed: Bool
		}
		let john = Person{name: "John", age: 30, employed: true}
		let result = json::encode(john)
		match result {
		  ok => ok,
			err => err
		}
	`)

	got := result.(string)
	if !strings.Contains(got, `"name":"John"`) {
		t.Errorf("Result json string does not contain 'name': %s", got)
	}
	if !strings.Contains(got, `"age":30`) {
		t.Errorf("Result json string does not contain 'age': %s", got)
	}
	if !strings.Contains(got, `"employed":true`) {
		t.Errorf("Result json string does not contain 'employed': %s", got)
	}
}
