package vm_test

import "testing"

func TestFunctions(t *testing.T) {
	tests := []test{
		{
			name: "noop function",
			input: `
					fn noop() {}
					noop()`,
			want: nil,
		},
		{
			name: "returning with no args",
			input: `
					fn get_msg() { "Hello" }
					get_msg()`,
			want: "Hello",
		},
		{
			name: "one arg",
			input: `
					fn greet(name: Str) { "Hello, {name}!" }
					greet("Alice")`,
			want: "Hello, Alice!",
		},
		{
			name: "multiple args",
			input: `
					fn add(a: Int, b: Int) { a + b }
					add(1, 2)`,
			want: 3,
		},
		{
			name: "first class functions",
			input: `
			let sub = fn(a: Int, b: Int) { a - b }
			sub(30, 8)`,
			want: 22,
		},
		{
			name: "closure lexical scoping",
			input: `
			fn createAdder(base: Int) fn(Int) Int {
			  fn(x: Int) Int {
			    base + x
			  }
			}

			let addFive = createAdder(5)
			addFive(10)`,
			want: 15,
		},
		{
			name: "named arguments on static function respect names",
			input: `
			fn Person::full(first: Str, last: Str) Str {
			  "{first} {last}"
			}

			Person::full(last: "Doe", first: "Jane")`,
			want: "Jane Doe",
		},
		{
			name: "named arguments on instance method respect names",
			input: `
			struct Person {
			  first: Str,
			  last: Str,
			}

			impl Person {
			  fn full(first: Str, last: Str) Str {
			    "{first} {last}"
			  }
			}

			let p = Person{first: "Ignored", last: "AlsoIgnored"}
			p.full(last: "Doe", first: "Jane")`,
			want: "Jane Doe",
		},
		{
			name: "referencing module-level fn within method of same name",
			input: `
			fn process(x: Int) Int {
			  x * 2
			}

			struct Handler { }

			impl Handler {
			  fn process(x: Int) Str {
			    let result = process(5)
			    "Result: {result}"
			  }
			}

			let h = Handler{}
			h.process(10)`,
			want: "Result: 10",
		},
		{
			name: "module-level fn and method with same name but different param types",
			input: `
			fn process(x: Str) Str {
			  "string: {x}"
			}

			struct Handler { }

			impl Handler {
			  fn process(x: Int) Str {
			    let result = process("hello")
			    "handler: {result}"
			  }
			}

			let h = Handler{}
			h.process(42)`,
			want: "handler: string: hello",
		},
		{
			name: "calling Type::function static method defined with double colon syntax",
			input: `
			struct Fixture {
			  id: Int,
			  name: Str,
			}

			fn Fixture::from_entry(data: Str) Fixture {
			  Fixture{id: 1, name: data}
			}

			let f = Fixture::from_entry("Test")
			f.name`,
			want: "Test",
		},
	}

	runTests(t, tests)
}
