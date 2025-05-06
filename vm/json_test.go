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
		  str => str,
			_ => ""
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

func testJsonDecodeList(t *testing.T) {
	result := run(t, `
		use ard/json
		let nums: [Int]? = json.decode("[1,2,3]")
		match nums {
		  ns => ns.size(),
			_ => 0
		}
	`)

	if result != 3 {
		t.Errorf("Expected 3, got %v", result)
	}
}

func testJsonDecodeStruct(t *testing.T) {
	result := run(t, `
		use ard/json
		struct Person {
			name: Str,
			age: Int,
		  employed: Bool
		}
		let john_str = "{\"name\": \"John\", \"age\": 30, \"employed\": true}"
		let result: Person? = json.decode(john_str)
		match result {
		  john => john.name == "John" and john.age == 30 and john.employed == true,
			_ => false
		}
	`)

	if result == false {
		t.Errorf("Got wrong decoded result")
	}
}
