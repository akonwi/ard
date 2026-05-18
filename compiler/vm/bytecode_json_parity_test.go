package vm

import "testing"

func TestVMBytecodeParityParseJSON(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "parse struct",
			input: `
				use ard/json

				struct Todo {
					id: Int,
					title: Str,
				}

				let todo = json::parse<Todo>("\{\"id\":1,\"title\":\"ship\"}").expect("parse")
				"{todo.id}:{todo.title}"
			`,
			want: "1:ship",
		},
		{
			name: "parse nested list",
			input: `
				use ard/json

				struct Todo {
					id: Int,
					title: Str,
				}
				struct Payload {
					items: [Todo],
				}

				let payload = json::parse<Payload>("\{\"items\":[\{\"id\":1,\"title\":\"ship\"}]}").expect("parse")
				payload.items.at(0).title
			`,
			want: "ship",
		},
		{
			name: "parse nullable missing field",
			input: `
				use ard/json

				struct Todo {
					id: Int,
					title: Str?,
				}

				let todo = json::parse<Todo>("\{\"id\":1}").expect("parse")
				todo.title.or("missing")
			`,
			want: "missing",
		},
		{
			name: "parse map",
			input: `
				use ard/json

				let counts = json::parse<[Str: Int]>("\{\"a\":1,\"b\":2}").expect("parse")
				counts.get("b").or(0)
			`,
			want: 2,
		},
		{
			name: "parse error includes path in message",
			input: `
				use ard/json

				struct Todo {
					id: Int,
					title: Str,
				}

				match json::parse<Todo>("\{\"id\":\"bad\",\"title\":\"ship\"}") {
					ok(_) => "ok",
					err(e) => e,
				}
			`,
			want: "id: got Str, expected Int",
		},
	})
}

func TestVMBytecodeParityEncodeJSONPrimitives(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "encoding Str",
			input: `
				use ard/encode
				encode::json("hello").expect("encode failed")
			`,
			want: `"hello"`,
		},
		{
			name: "encoding Int",
			input: `
				use ard/encode
				encode::json(200).expect("encode failed")
			`,
			want: `200`,
		},
		{
			name: "encoding Float",
			input: `
				use ard/encode
				encode::json(98.6).expect("encode failed")
			`,
			want: `98.6`,
		},
		{
			name: "encoding Bool",
			input: `
				use ard/encode
				encode::json(true).expect("encode failed")
			`,
			want: `true`,
		},
		{
			name: "encoding struct with nullable field",
			input: `
				use ard/json
				use ard/maybe

				struct Person {
					name: Str,
					employed: Bool?,
				}

				json::encode(Person{name: "kit", employed: maybe::none()}).expect("encode failed")
			`,
			want: `{"employed":null,"name":"kit"}`,
		},
	})
}
