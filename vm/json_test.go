package vm_test

import (
	"strings"
	"testing"
)

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

func TestJsonDecodeList(t *testing.T) {
	result := run(t, `
		use ard/json
		let empty: [Int] = []
		let nums = json::decode<[Int]>("[1,2,3]")
		nums.or(empty).size()
	`)

	if result != 3 {
		t.Errorf("Expected 3, got %v", result)
	}
}

func TestJsonDecodeStruct(t *testing.T) {
	result := run(t, `
		use ard/json
		struct Person {
			name: Str,
			age: Int,
		  employed: Bool
		}
		let john_str = "\{\"name\": \"John\", \"age\": 30, \"employed\": true}"
		let result = json::decode<Person>(john_str)
		match result {
		  ok => ok.name == "John" and ok.age == 30 and ok.employed == true,
			err => false
		}
	`)

	if result != true {
		t.Errorf("Wanted %v, got %v", true, result)
	}
}

func TestJsonDecodeStructsWithMaybes(t *testing.T) {
	result := run(t, `
		use ard/json
		struct Person {
			name: Str?,
			age: Int?,
		  employed: Bool?
		}
		let john_str = "\{\"name\": \"John\", \"age\": null}"
		let result = json::decode<Person>(john_str)
		match result {
		  ok => ok.name.or("") == "John" and ok.age.or(0) == 0 and ok.employed.or(false) == false,
			err => false
		}
	`)

	if result != true {
		t.Errorf("Wanted %v, got %v", true, result)
	}
}

func TestJsonDecodeNestedStructWithList(t *testing.T) {
	result := run(t, `
		use ard/json
		struct Person {
			name: Str,
			id: Int,
		}
		struct Payload {
		  people: [Person]
		}

		let input = "\{ \"people\": [ \{ \"name\": \"John\", \"id\": 1 } ] }"
		let result = json::decode<Payload>(input)
		match result {
		  ok => ok.people.size(),
			err => 0
		}
	`)

	if result != 1 {
		t.Errorf("Wanted %v, got %v", 1, result)
	}
}
