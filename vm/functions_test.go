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
	}

	runTests(t, tests)
}
