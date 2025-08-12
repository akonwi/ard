package vm_test

import (
	"strings"
	"testing"
)







func TestJsonEncodePrimitives(t *testing.T) {
	runTests(t, []test{
		{
			name: "encoding Str",
			input: `
				use ard/json
				json::encode("hello")
			`,
			want: `"hello"`,
		},
		{
			name: "encoding Int",
			input: `
				use ard/json
				json::encode(200)
			`,
			want: `200`,
		},
		{
			name: "encoding Float",
			input: `
				use ard/json
				json::encode(98.6)
			`,
			want: `98.6`,
		},
		{
			name: "encoding Bool",
			input: `
				use ard/json
				json::encode(true)
			`,
			want: `true`,
		},
		{
			name: "encoding Str?",
			input: `
				use ard/json
				use ard/maybe
				let s: Str? = maybe::none()
				json::encode(s)
			`,
			want: `null`,
		},
		{
			name: "encoding Int?",
			input: `
				use ard/json
				use ard/maybe
				json::encode(maybe::some(200))
			`,
			want: `200`,
		},
		{
			name: "encoding Float?",
			input: `
				use ard/json
				use ard/maybe
				json::encode(maybe::some(98.6))
			`,
			want: `98.6`,
		},
		{
			name: "encoding Bool",
			input: `
				use ard/json
				use ard/maybe
				json::encode(maybe::some(true))
			`,
			want: `true`,
		},
	})
}

func TestJsonEncodeList(t *testing.T) {
	result := run(t, `
		use ard/json
		let list = [1,2,3]
		json::encode(list).expect("")
	`)

	jsonString, ok := result.(string)
	if !ok {
		t.Fatalf("Expected a json string, got %v", result)
	}

	expected := `[1,2,3]`
	if jsonString != expected {
		t.Fatalf("Expected json list of %v, got %v", expected, jsonString)
	}
}

func TestJsonEncodeStruct(t *testing.T) {
	result := run(t, `
		use ard/json
		use ard/maybe
		struct Person {
			name: Str,
			age: Int,
		  employed: Bool
		}
		let john = Person{name: "John", age: 30, employed: maybe::none()}
		json::encode(john).expect("")
	`)

	got := result.(string)
	if !strings.Contains(got, `"name":"John"`) {
		t.Errorf("Result json string does not contain 'name': %s", got)
	}
	if !strings.Contains(got, `"age":30`) {
		t.Errorf("Result json string does not contain 'age': %s", got)
	}
	if !strings.Contains(got, `"employed":null`) {
		t.Errorf("Result json string does not contain 'employed': %s", got)
	}
}

func TestJsonEncodeResult(t *testing.T) {
	runTests(t, []test{
		{
			name: "ok",
			input: `
				use ard/json
				let result: Int!Bool = Result::ok(200)
				json::encode(result).expect("")
			`,
			want: "200",
		},
		{
			name: "err",
			input: `
				use ard/json
				let result: Int!Bool = Result::err(false)
				json::encode(result).expect("")
			`,
			want: "false",
		},
	})
}

func TestJsonEncodeEnums(t *testing.T) {
	runTests(t, []test{
		{
			name: "simple",
			input: `
				use ard/json
				enum Color { blue, green, red }
				json::encode(Color::green)
			`,
			want: "1",
		},
	})
}

func TestJsonEncodeMap(t *testing.T) {
	runTests(t, []test{
		{
			name: "string to int map",
			input: `
				use ard/json
				mut m: [Str:Int] = [:]
				m.set("foo", 42)
				m.set("bar", 24)
				let result = json::encode(m).expect("")
				result.contains("foo") and result.contains("42") and result.contains("bar") and result.contains("24")
			`,
			want: true,
		},
		{
			name: "nested map with structs",
			input: `
				use ard/json
				struct Person {
					name: Str,
					age: Int
				}
				mut people: [Str:Person] = [:]
				people.set("john", Person{name: "John", age: 30})
				let result = json::encode(people).expect("")
				result.contains("john") and result.contains("John") and result.contains("30")
			`,
			want: true,
		},
	})
}
