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
		{
			name: "omitting nullable parameters",
			input: `
				use ard/maybe

				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				add(1)`,
			want: 1,
		},
		{
			name: "omitting nullable parameters with explicit value",
			input: `
				use ard/maybe

				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				add(1, maybe::some(5))`,
			want: 6,
		},
		{
			name: "automatic wrapping of non-nullable values for nullable parameters",
			input: `
				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				add(1, 5)`,
			want: 6,
		},
		{
			name: "automatic wrapping works with omitted args",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(42)`,
			want: "42,0,default",
		},
		{
			name: "automatic wrapping with one wrapped argument",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(42, 7)`,
			want: "42,7,default",
		},
		{
			name: "automatic wrapping with all arguments provided",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(42, 7, "hello")`,
			want: "42,7,hello",
		},
		{
			name: "automatic wrapping of list literals for nullable parameters",
			input: `
				fn process(items: [Int]?) Bool {
					match items {
						lst => true
						_ => false
					}
				}
				process([10, 20, 30])`,
			want: true,
		},
		{
			name: "automatic wrapping of map literals for nullable parameters",
			input: `
				fn process(data: [Str:Int]?) Bool {
					match data {
						m => true
						_ => false
					}
				}
				process(["count": 42])`,
			want: true,
		},
		{
			name: "automatic wrapping with labeled arguments",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(a: 42, b: 7, c: "hello")`,
			want: "42,7,hello",
		},
		{
			name: "automatic wrapping with labeled arguments and omitted values",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(a: 42, c: "world")`,
			want: "42,0,world",
		},
		{
			name: "automatic wrapping with labeled arguments in different order",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(c: "reorder", b: 99, a: 5)`,
			want: "5,99,reorder",
		},
	}

	runTests(t, tests)
}

func TestInferringAnonymousFunctionTypes(t *testing.T) {
	tests := []test{
		{
			name: "callback with inferred Str parameter",
			input: `
			fn process(f: fn(Str) Bool) Bool {
			  f("hello")
			}

			process(fn(x) { x.size() > 0 })`,
			want: true,
		},
		{
			name: "callback with inferred Bool return type",
			input: `
			fn check(f: fn(Str) Bool) Bool {
			  f("test")
			}

			check(fn(s) { true })`,
			want: true,
		},
	}

	runTests(t, tests)
}
