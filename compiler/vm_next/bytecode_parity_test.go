package vm_next

import (
	"reflect"
	"testing"
)

type bytecodeParityCase struct {
	name  string
	input string
	want  any
}

func runBytecodeParityCases(t *testing.T, tests []bytecodeParityCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := runSourceGoValue(t, test.input)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Expected %#v, got %#v", test.want, got)
			}
		})
	}
}

func runSourceGoValue(t *testing.T, input string) any {
	t.Helper()

	return runSource(t, input).GoValue()
}

func TestVMNextBytecodeParityCoreExpressions(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "reassigning variables",
			input: `
				mut val = 1
				val = 2
				val = 3
				val
			`,
			want: 3,
		},
		{name: "unary not", input: `not true`, want: false},
		{name: "unary negative float", input: `-20.1`, want: -20.1},
		{name: "arithmetic precedence", input: `30 + (20 * 4)`, want: 110},
		{name: "chained comparisons", input: `200 <= 250 <= 300`, want: true},
		{
			name: "if/else-if/else",
			input: `
				let is_on = false
				mut result = ""
				if is_on { result = "then" }
				else if result.size() == 0 { result = "else if" }
				else { result = "else" }
				result
			`,
			want: "else if",
		},
		{
			name: "inline block expression",
			input: `
				let value = {
					let x = 10
					let y = 32
					x + y
				}
				value
			`,
			want: 42,
		},
	})
}

func TestVMNextBytecodeParityMultilineDotChaining(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "property access on next line",
			input: `
				let x = [1, 2, 3]
					.size()
				x
			`,
			want: 3,
		},
		{
			name: "chained methods across multiple lines",
			input: `
				let x = "hello"
					.size()
				x
			`,
			want: 5,
		},
		{
			name: "three-level chain across lines",
			input: `
				let x: Int!Str = Result::ok(42)
				let y = x
					.map(fn(v: Int) Str { "{v}!" })
					.or("fail")
				y
			`,
			want: "42!",
		},
		{
			name: "mixed single and multi-line chaining",
			input: `
				let a = [10, 20, 30].size()
				let b = [1, 2]
					.size()
				a + b
			`,
			want: 5,
		},
	})
}

func TestVMNextBytecodeParityNotGrouping(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{name: "grouped not binds before and", input: `(not false) and false`, want: false},
		{name: "ungrouped not keeps historical precedence", input: `not false and false`, want: true},
	})
}

func TestVMNextBytecodeParityFunctions(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "noop function",
			input: `
				fn noop() {}
				noop()
			`,
			want: nil,
		},
		{
			name: "returning with no args",
			input: `
				fn get_msg() { "Hello" }
				get_msg()
			`,
			want: "Hello",
		},
		{
			name: "one arg",
			input: `
				fn greet(name: Str) { "Hello, {name}!" }
				greet("Alice")
			`,
			want: "Hello, Alice!",
		},
		{
			name: "multiple args",
			input: `
				fn add(a: Int, b: Int) { a + b }
				add(1, 2)
			`,
			want: 3,
		},
		{
			name: "first class functions",
			input: `
				let sub = fn(a: Int, b: Int) { a - b }
				sub(30, 8)
			`,
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
				addFive(10)
			`,
			want: 15,
		},
		{
			name: "named arguments on static function respect names",
			input: `
				fn Person::full(first: Str, last: Str) Str {
				  "{first} {last}"
				}

				Person::full(last: "Doe", first: "Jane")
			`,
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
				p.full(last: "Doe", first: "Jane")
			`,
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
				h.process(10)
			`,
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
				h.process(42)
			`,
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
				f.name
			`,
			want: "Test",
		},
	})
}

func TestVMNextBytecodeParityNullableArguments(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "omitting nullable parameters",
			input: `
				use ard/maybe

				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				add(1)
			`,
			want: 1,
		},
		{
			name: "omitting nullable parameters with explicit value",
			input: `
				use ard/maybe

				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				add(1, maybe::some(5))
			`,
			want: 6,
		},
		{
			name: "automatic wrapping of non-nullable values for nullable parameters",
			input: `
				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				add(1, 5)
			`,
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
				test(42)
			`,
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
				test(42, 7)
			`,
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
				test(42, 7, "hello")
			`,
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
				process([10, 20, 30])
			`,
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
				process(["count": 42])
			`,
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
				test(a: 42, b: 7, c: "hello")
			`,
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
				test(a: 42, c: "world")
			`,
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
				test(c: "reorder", b: 99, a: 5)
			`,
			want: "5,99,reorder",
		},
	})
}

func TestVMNextBytecodeParityInferringAnonymousFunctionTypes(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "callback with inferred Str parameter",
			input: `
				fn process(f: fn(Str) Bool) Bool {
				  f("hello")
				}

				process(fn(x) { x.size() > 0 })
			`,
			want: true,
		},
		{
			name: "callback with inferred Bool return type",
			input: `
				fn check(f: fn(Str) Bool) Bool {
				  f("test")
				}

				check(fn(s) { true })
			`,
			want: true,
		},
	})
}

func TestVMNextBytecodeParityTry(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "early return",
			input: `
				fn test_early_return() Str {
					try Result::err("test") -> err {
						"caught: {err}"
					}
					"should not reach here"
				}
				test_early_return()
			`,
			want: "caught: test",
		},
		{
			name: "success",
			input: `
				fn foobar() Int!Str {
					Result::ok(42)
				}

				fn do_thing() Str {
					let result = try foobar() -> err {
						"error: {err}"
					}
					"success: {result}"
				}

				do_thing()
			`,
			want: "success: 42",
		},
		{
			name: "without catch propagates error",
			input: `
				fn foobar() Int!Str {
					Result::err("error")
				}

				fn do_thing() Int!Str {
					let res = try foobar()
					Result::ok(res * 2)
				}

				do_thing()
			`,
			want: "error",
		},
		{
			name: "without catch success",
			input: `
				fn foobar() Int!Str {
					Result::ok(21)
				}

				fn do_thing() Int!Str {
					let res = try foobar()
					Result::ok(res * 2)
				}

				do_thing()
			`,
			want: 42,
		},
		{
			name: "error transformation",
			input: `
				fn parse_number(s: Str) Int!Str {
					Result::err("not a number")
				}

				fn process_data() Str {
					let num = try parse_number("abc") -> err {
						"Error processing: {err}"
					}
					"Got number: {num}"
				}

				process_data()
			`,
			want: "Error processing: not a number",
		},
		{
			name: "nested early returns",
			input: `
				fn inner() Int!Str {
					Result::err("inner error")
				}

				fn middle() Str!Str {
					let result = try inner() -> err {
						Result::err("caught and re-wrapped: {err}")
					}
					Result::ok("success: {result}")
				}

				fn outer() Str {
					try middle() -> err {
						"Final catch: {err}"
					}
				}

				outer()
			`,
			want: "Final catch: caught and re-wrapped: inner error",
		},
		{
			name: "catch with function name",
			input: `
				fn make_error_message(code: Str) Str {
					"Failed with code: {code}"
				}

				fn foobar() Int!Str {
					Result::err("something went wrong")
				}

				fn do_thing() Str {
					let result = try foobar() -> make_error_message
					"success: {result}"
				}

				do_thing()
			`,
			want: "Failed with code: something went wrong",
		},
		{
			name: "catch with qualified function name",
			input: `
				fn Foo::format(code: Str) Str {
					"Failed with code: {code}"
				}

				fn foobar() Int!Str {
					Result::err("something went wrong")
				}

				fn do_thing() Str {
					let result = try foobar() -> Foo::format
					"success: {result}"
				}

				do_thing()
			`,
			want: "Failed with code: something went wrong",
		},
		{
			name: "try in enum match success case",
			input: `
				enum Status { active, inactive }

				fn get_result() Int!Str {
					Result::ok(42)
				}

				fn process_status(status: Status) Int!Str {
					match status {
						Status::active => {
							let value = try get_result()
							Result::ok(value + 1)
						}
						Status::inactive => Result::err("inactive")
					}
				}

				process_status(Status::active).expect("")
			`,
			want: 43,
		},
		{
			name: "try in enum match error case",
			input: `
				enum Status { active, inactive }

				fn get_error() Int!Str {
					Result::err("failed")
				}

				fn process_status(status: Status) Int!Str {
					match status {
						Status::active => {
							let value = try get_error()
							Result::ok(value + 1)
						}
						Status::inactive => Result::err("inactive")
					}
				}

				process_status(Status::active).or(-1)
			`,
			want: -1,
		},
		{
			name: "try in maybe match success",
			input: `
				use ard/maybe

				fn get_result() Int!Str {
					Result::ok(100)
				}

				fn process_maybe(maybe_val: Int?) Int!Str {
					match maybe_val {
						val => {
							let result = try get_result()
							Result::ok(result + val)
						}
						_ => Result::err("no value")
					}
				}

				process_maybe(maybe::some(5)).expect("")
			`,
			want: 105,
		},
		{
			name: "try with catch in match blocks",
			input: `
				fn risky_operation() Str!Str {
					Result::err("operation failed")
				}

				fn process_with_catch(flag: Bool) Str {
					match flag {
						true => {
							try risky_operation() -> err {
								"caught: {err}"
							}
						}
						false => "no operation"
					}
				}

				process_with_catch(true)
			`,
			want: "caught: operation failed",
		},
		{
			name: "try in nested match blocks",
			input: `
				use ard/maybe
				enum Status { active, inactive }

				fn get_result() Int!Str {
					Result::ok(50)
				}

				fn process_nested(status: Status, maybe_val: Int?) Int!Str {
					match status {
						Status::active => {
							match maybe_val {
								val => {
									let result = try get_result()
									Result::ok(result + val)
								}
								_ => Result::err("no value")
							}
						}
						Status::inactive => Result::err("inactive")
					}
				}

				process_nested(Status::active, maybe::some(25)).expect("")
			`,
			want: 75,
		},
		{
			name: "trying with union err",
			input: `
				struct InvalidField { name: Str, message: Str }
				type Error = InvalidField | Str

				fn can_fail() Bool!Error {
					Result::err("Failed")
				}

				fn do_stuff() Bool!Error {
				  let ok = try can_fail()
					Result::ok(ok)
				}

				do_stuff()
			`,
			want: "Failed",
		},
		{
			name: "catching with union err",
			input: `
				struct InvalidField { name: Str, message: Str }
				type Error = InvalidField | Str

				fn can_fail() Bool!Int {
					Result::err(-1)
				}

				fn do_stuff() Bool!Error {
				  let ok = try can_fail() -> intErr {
						Result::err("Got a num")
					}
					Result::ok(ok)
				}

				do_stuff()
			`,
			want: "Got a num",
		},
	})
}

func TestVMNextBytecodeParityMaybes(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "equality comparison returns false when each are different",
			input: `
				use ard/maybe
				maybe::some("hello") == maybe::none()
			`,
			want: false,
		},
		{
			name: "equality comparison returns true when both are the same",
			input: `
				use ard/maybe
				maybe::some("hello") == maybe::some("hello")
			`,
			want: true,
		},
		{
			name: "equality comparison returns true when both are none",
			input: `
				use ard/maybe
				maybe::none() == maybe::none()
			`,
			want: true,
		},
		{
			name: ".or() can be used to read and fallback to a default value",
			input: `
				use ard/maybe
				let a: Str? = maybe::none()
				a.or("foo")
			`,
			want: "foo",
		},
		{
			name: ".is_none() returns true for nones",
			input: `
				use ard/maybe
				maybe::none().is_none()
			`,
			want: true,
		},
		{
			name: ".is_some() returns true for somes",
			input: `
				use ard/maybe
				maybe::some(1).is_some()
			`,
			want: true,
		},
		{
			name: "reassigning maybes",
			input: `
				use ard/maybe
				mut a: Str? = maybe::none()
				a = maybe::some("hello")
				match a {
					s => s,
					_ => "",
				}
			`,
			want: "hello",
		},
		{
			name: ".expect() returns value for some",
			input: `
				use ard/maybe
				maybe::some(42).expect("Should not panic")
			`,
			want: 42,
		},
		{
			name: ".map() transforms some values with inferred callback types",
			input: `
				use ard/maybe
				let result = maybe::some(41).map(fn(value) { value + 1 })
				result.or(0)
			`,
			want: 42,
		},
		{
			name: ".map() keeps none values with inferred callback types",
			input: `
				use ard/maybe
				let result: Int? = maybe::none()
				result.map(fn(value) { value + 1 }).is_none()
			`,
			want: true,
		},
		{
			name: ".and_then() transforms and flattens some values",
			input: `
				use ard/maybe
				let result = maybe::some(21).and_then(fn(value) { maybe::some(value * 2) })
				result.or(0)
			`,
			want: 42,
		},
		{
			name: ".and_then() keeps none values",
			input: `
				use ard/maybe
				let result: Int? = maybe::none()
				result.and_then(fn(value) { maybe::some(value + 1) }).is_none()
			`,
			want: true,
		},
		{
			name: ".and_then() supports explicit type args",
			input: `
				use ard/maybe
				let result = maybe::some(21).and_then<Str>(fn(value) { maybe::some("{value}") })
				result.or("")
			`,
			want: "21",
		},
	})
}

func TestVMNextBytecodeParityResults(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{name: "ok", input: `Result::ok(200)`, want: 200},
		{name: "err", input: `Result::err(404)`, want: 404},
		{
			name: "matching a result",
			input: `
				fn divide(a: Int, b: Int) Int!Str {
					match b == 0 {
					  true => Result::err("cannot divide by 0"),
					  false => Result::ok(a / b),
					}
				}
				match divide(100, 0) {
				  ok => ok,
					err => -1
				}
			`,
			want: -1,
		},
		{
			name: "Result.or()",
			input: `
				fn divide(a: Int, b: Int) Int!Str {
					match b == 0 {
					  true => Result::err("cannot divide by 0"),
					  false => Result::ok(a / b),
					}
				}
				let res = divide(100, 0)
				res.or(-1)
			`,
			want: -1,
		},
		{
			name: "Result.map() transforms ok values with inferred callback types",
			input: `
				let res: Int!Str = Result::ok(21)
				let mapped = res.map(fn(value) { value * 2 })
				mapped.or(0)
			`,
			want: 42,
		},
		{
			name: "Result.map() leaves err values unchanged with inferred callback types",
			input: `
				let res: Int!Str = Result::err("bad")
				let mapped = res.map(fn(value) { value * 2 })
				match mapped {
					err(msg) => msg,
					ok(value) => value.to_str(),
				}
			`,
			want: "bad",
		},
		{
			name: "Result.map_err() transforms err values with inferred callback types",
			input: `
				let res: Int!Str = Result::err("bad")
				let mapped = res.map_err(fn(err) { err.size() })
				match mapped {
					err(size) => size,
					ok(value) => value,
				}
			`,
			want: 3,
		},
		{
			name: "Result.map_err() leaves ok values unchanged with inferred callback types",
			input: `
				let res: Int!Str = Result::ok(42)
				let mapped = res.map_err(fn(err) { err.size() })
				mapped.or(0)
			`,
			want: 42,
		},
		{
			name: "Result.and_then() chains ok values",
			input: `
				let res: Int!Str = Result::ok(21)
				let chained = res.and_then(fn(value) { Result::ok(value * 2) })
				chained.or(0)
			`,
			want: 42,
		},
		{
			name: "Result.and_then() propagates callback errors",
			input: `
				let res: Int!Str = Result::ok(21)
				let chained = res.and_then(fn(value) { Result::err("bad") })
				chained.is_err()
			`,
			want: true,
		},
		{
			name: "Result.and_then() leaves err values unchanged",
			input: `
				let res: Int!Str = Result::err("bad")
				let chained = res.and_then(fn(value) { Result::ok(value * 2) })
				chained.is_err()
			`,
			want: true,
		},
		{
			name: "Result.and_then() supports explicit type args",
			input: `
				let res: Int!Str = Result::ok(21)
				let chained = res.and_then<Str>(fn(value) { Result::ok("{value}") })
				chained.or("")
			`,
			want: "21",
		},
		{
			name: "Result.and_then() explicit type args resolve err-only callbacks",
			input: `
				let res: Int!Str = Result::err("bad")
				let chained = res.and_then<Str>(fn(value) { Result::err("boom") })
				match chained {
					err(msg) => msg,
					ok(value) => value,
				}
			`,
			want: "bad",
		},
	})
}

func TestVMNextBytecodeParityLists(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{name: "list literal", input: `[1, 2, 3]`, want: []any{1, 2, 3}},
		{name: "List.size", input: `[1,2,3].size()`, want: 3},
		{
			name: "List::prepend",
			input: `
				mut list = [1,2,3]
				list.prepend(4)
				list.size()
			`,
			want: 4,
		},
		{
			name: "List::push",
			input: `
				mut list = [1,2,3]
				list.push(4)
				list.size()
			`,
			want: 4,
		},
		{
			name: "List::at",
			input: `
				mut list = [1,2,3]
				list.push(4)
				list.at(3)
			`,
			want: 4,
		},
		{
			name: "List::set updates the list at the specified index",
			input: `
				mut list = [1,2,3]
				list.set(1, 10)
				list.at(1)
			`,
			want: 10,
		},
		{
			name: "List.sort()",
			input: `
				mut list = [3,7,8,5,2,9,5,4]
				list.sort(fn(a: Int, b: Int) Bool { a < b })
				list.at(0) + list.at(7)
			`,
			want: 11,
		},
		{
			name: "List.swap swaps values at the given indexes",
			input: `
				mut list = [1,2,3]
				list.swap(0,2)
				list.at(0)
			`,
			want: 3,
		},
	})
}

func TestVMNextBytecodeParityMaps(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{name: "map literal", input: `["a": 1, "b": 2]`, want: map[any]any{"a": 1, "b": 2}},
		{
			name: "Map.size",
			input: `
				let items = ["a": 1, "b": 2]
				items.size()
			`,
			want: 2,
		},
		{
			name: "Map.has",
			input: `
				let items = ["a": 1, "b": 2]
				items.has("a")
			`,
			want: true,
		},
		{
			name: "Map.get",
			input: `
				let items = ["a": 1, "b": 2]
				items.get("a").or(0)
			`,
			want: 1,
		},
		{
			name: "Map.keys uses sorted keys",
			input: `
				let items = [3: "c", 1: "a", 2: "b"]
				let keys = items.keys()
				"{keys.at(0)}-{keys.at(1)}-{keys.at(2)}"
			`,
			want: "1-2-3",
		},
	})
}

func TestVMNextBytecodeParityLoops(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "basic for loop",
			input: `
				mut sum = 0
				for mut even = 0; even <= 10; even =+ 2 {
					sum =+ even
				}
				sum
			`,
			want: 30,
		},
		{
			name: "loop over numeric range",
			input: `
				mut sum = 0
				for i in 1..5 {
					sum = sum + i
				}
				sum
			`,
			want: 15,
		},
		{
			name: "loop over a number",
			input: `
				mut sum = 0
				for i in 5 {
					sum = sum + i
				}
				sum
			`,
			want: 15,
		},
		{
			name: "looping over a string",
			input: `
				mut res = ""
				for c in "hello" {
					res = "{c}{res}"
				}
				res
			`,
			want: "olleh",
		},
		{
			name: "looping over a list",
			input: `
				mut sum = 0
				for n in [1,2,3,4,5] {
					sum = sum + n
				}
				sum
			`,
			want: 15,
		},
		{
			name: "looping over a map",
			input: `
				mut sum = 0
				for k,count in ["key":3, "foobar":6] {
					sum =+ count
				}
				sum
			`,
			want: 9,
		},
		{
			name: "looping over a map uses sorted keys",
			input: `
				mut out = ""
				for key,val in [3:"c", 1:"a", 2:"b"] {
					out = out + "{key}:{val};"
				}
				out
			`,
			want: "1:a;2:b;3:c;",
		},
		{
			name: "while loop",
			input: `
				mut count = 5
				while count > 0 {
					count = count - 1
				}
				count == 0
			`,
			want: true,
		},
		{
			name: "break out of loop",
			input: `
				mut count = 5
				while count > 0 {
					count = count - 1
					if count == 3 {
						break
					}
				}
				count
			`,
			want: 3,
		},
	})
}

func TestVMNextBytecodeParityMatching(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "matching on booleans",
			input: `
				let is_on = true
				match is_on {
					true => "on",
					false => "off"
				}
			`,
			want: "on",
		},
		{
			name: "matching on enum",
			input: `
				enum Direction {
					Up, Down, Left, Right
				}
				let dir: Direction = Direction::Right
				match dir {
					Direction::Up => "North",
					Direction::Down => {
						"South"
					},
					Direction::Left => "West",
					Direction::Right => "East"
				}
			`,
			want: "East",
		},
		{
			name: "enum catch all",
			input: `
				enum Direction {
					Up, Down, Left, Right
				}
				let dir: Direction = Direction::Right
				match dir {
					Direction::Up => "North",
					Direction::Down => "South",
					_ => "skip"
				}
			`,
			want: "skip",
		},
		{
			name: "matching on an int",
			input: `
				let int = 0
				match int {
					-1 => "less",
					0 => "equal",
					1 => "greater",
					_ => "panic"
				}
			`,
			want: "equal",
		},
		{
			name: "matching with ranges",
			input: `
				let int = 80
				match int {
					-100..0 => "how?",
					0..60 => "F",
					_ => "pass"
				}
			`,
			want: "pass",
		},
		{
			name: "matching on int with enum variant patterns",
			input: `
				enum Status {
					active,
					inactive,
					pending
				}
				let code: Int = 0
				match code {
					Status::active => "Active",
					Status::inactive => "Inactive",
					Status::pending => "Pending",
					_ => "Unknown"
				}
			`,
			want: "Active",
		},
		{
			name: "matching on int with mixed enum and literal patterns",
			input: `
				enum HttpStatus {
					ok,
					created,
					notFound,
					serverError
				}
				let code: Int = 2
				match code {
					HttpStatus::ok => "200 OK",
					HttpStatus::created => "201 Created",
					HttpStatus::notFound => "404 Not Found",
					500..599 => "Server Error",
					_ => "Unknown"
				}
			`,
			want: "404 Not Found",
		},
		{
			name: "matching on int with custom enum values",
			input: `
				enum HttpStatus {
					Ok = 200,
					Created = 201,
					NotFound = 404,
					ServerError = 500
				}
				let code: Int = 404
				match code {
					HttpStatus::Ok => "Success",
					HttpStatus::Created => "Created",
					HttpStatus::NotFound => "Not Found",
					HttpStatus::ServerError => "Server Error",
					_ => "Unknown"
				}
			`,
			want: "Not Found",
		},
		{
			name: "matching on int with mixed custom enum values and ranges",
			input: `
				enum Status {
					Pending = 0,
					Active = 100,
					Inactive = 101,
					Deleted = 999
				}
				let code: Int = 150
				match code {
					Status::Pending => "Pending",
					Status::Active => "Active",
					Status::Inactive => "Inactive",
					100..199 => "In range 100-199",
					Status::Deleted => "Deleted",
					_ => "Unknown"
				}
			`,
			want: "In range 100-199",
		},
		{
			name: "conditional matching with catch-all",
			input: `
				let score = 85
				match {
					score >= 90 => "A",
					score >= 80 => "B",
					score >= 70 => "C",
					score >= 60 => "D",
					_ => "F"
				}
			`,
			want: "B",
		},
		{
			name: "conditional matching with complex conditions",
			input: `
				let age = 25
				let hasLicense = true
				match {
					age < 16 => "Too young",
					not hasLicense => "Need license",
					age >= 65 => "Senior driver",
					_ => "Regular driver"
				}
			`,
			want: "Regular driver",
		},
		{
			name: "conditional matching with boolean conditions",
			input: `
				let a = true
				let b = false
				match {
					a and b => "both true",
					a or b => "at least one true",
					_ => "both false"
				}
			`,
			want: "at least one true",
		},
	})
}

func TestVMNextBytecodeParityStructs(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "Struct usage",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}

				impl Point {
					fn print() Str {
						"{self.x.to_str()},{self.y.to_str()}"
					}
				}

				let p = Point { x: 10, y: 20 }
				p.print()
			`,
			want: "10,20",
		},
		{
			name: "Reassigning struct properties",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}
				mut p = Point { x: 10, y: 20 }
				p.x = 30
				p.x
			`,
			want: 30,
		},
		{
			name: "Nesting structs",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}
				struct Line {
					start: Point,
					end: Point,
				}
				let line = Line{
					start: Point { x: 10, y: 20 },
					end: Point { x: 10, y: 0 },
				}
				line.start.x + line.end.y
			`,
			want: 10,
		},
		{
			name: "copy semantics preserve original struct",
			input: `
				struct Person { name: Str, age: Int }
				let alice = Person { name: "Alice", age: 30 }
				mut bob = alice
				bob.age = 31
				"{alice.age} -- {bob.age}"
			`,
			want: "30 -- 31",
		},
		{
			name: "implicit wrapping of non-nullable value for nullable struct field",
			input: `
				struct Config {
					name: Str,
					timeout: Int?,
				}
				let c = Config{name: "app", timeout: 30}
				c.timeout.or(0)
			`,
			want: 30,
		},
		{
			name: "omitting nullable struct field still works",
			input: `
				struct Config {
					name: Str,
					timeout: Int?,
				}
				let c = Config{name: "app"}
				c.timeout.or(0)
			`,
			want: 0,
		},
		{
			name: "explicit maybe::some still works for struct fields",
			input: `
				use ard/maybe
				struct Config {
					name: Str,
					timeout: Int?,
				}
				let c = Config{name: "app", timeout: maybe::some(30)}
				c.timeout.or(0)
			`,
			want: 30,
		},
		{
			name: "implicit wrapping of list literal for nullable struct field",
			input: `
				struct Data {
					items: [Int]?,
				}
				let d = Data{items: [1, 2, 3]}
				match d.items {
					lst => lst.size()
					_ => 0
				}
			`,
			want: 3,
		},
		{
			name: "implicit wrapping of map literal for nullable struct field",
			input: `
				struct Data {
					meta: [Str:Int]?,
				}
				let d = Data{meta: ["count": 42]}
				match d.meta {
					m => true
					_ => false
				}
			`,
			want: true,
		},
		{
			name: "implicit wrapping with multiple nullable fields",
			input: `
				struct Options {
					a: Int?,
					b: Str?,
					c: Bool?,
				}
				let o = Options{a: 1, b: "hi", c: true}
				let av = o.a.or(0)
				let bv = o.b.or("")
				let cv = o.c.or(false)
				"{av},{bv},{cv}"
			`,
			want: "1,hi,true",
		},
	})
}

func TestVMNextBytecodeParityEnumsUnions(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "enum to int comparison",
			input: `
				enum Status { active, inactive, pending }
				let status = Status::active
				status == 0
			`,
			want: true,
		},
		{
			name: "enum explicit value",
			input: `
				enum HttpStatus {
					Ok = 200,
					Created = 201,
					Not_Found = 404
				}
				HttpStatus::Ok
			`,
			want: 200,
		},
		{
			name: "enum equality",
			input: `
				enum Direction { Up, Down, Left, Right }
				let dir1 = Direction::Up
				let dir2 = Direction::Down
				dir1 == dir2
			`,
			want: false,
		},
		{
			name: "union matching",
			input: `
				type Printable = Str | Int | Bool
				fn print(p: Printable) Str {
				  match p {
					  Str(str) => str,
						Int(int) => int.to_str(),
						_ => "boolean value"
					}
				}
				print(20)
			`,
			want: "20",
		},
	})
}

func TestVMNextBytecodeParityGenericEquality(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "direct generic return compared with ==",
			input: `
				fn id<$T>(value: $T) $T {
				  value
				}

				id(3) == 3
			`,
			want: true,
		},
		{
			name: "inline maybe wrapping in generic context",
			input: `
				use ard/maybe
				mut found = maybe::none<Int>()
				let list = [1, 2, 3, 4, 5]
				for t in list {
				  if t == 3 {
				    found = maybe::some(t)
				    break
				  }
				}
				match found {
				  value => value == 3,
				  _ => false
				}
			`,
			want: true,
		},
	})
}

func TestVMNextBytecodeParityTypeAPIs(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{name: "Int.to_str", input: `100.to_str()`, want: "100"},
		{name: "Bool.to_str", input: `true.to_str()`, want: "true"},
		{name: "Str.replace_all", input: `"hello world hello world".replace_all("world", "universe")`, want: "hello universe hello universe"},
		{name: "Str.contains", input: `"hello".contains("ell")`, want: true},
	})
}

func TestVMNextBytecodeParityTryOnMaybe(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "try on Maybe::some returns unwrapped value",
			input: `
				use ard/maybe

				fn get_value() Int? {
					maybe::some(42)
				}

				fn test() Int? {
					let value = try get_value()
					maybe::some(value + 1)
				}

				let result = test()
				match result {
					value => value,
					_ => -1
				}
			`,
			want: 43,
		},
		{
			name: "try on Maybe::none propagates none",
			input: `
				use ard/maybe

				fn get_value() Int? {
					maybe::none()
				}

				fn test() Int? {
					let value = try get_value()
					maybe::some(value + 1)
				}

				let result = test()
				match result {
					value => value,
					_ => -999
				}
			`,
			want: -999,
		},
		{
			name: "try on Maybe with catch block transforms none",
			input: `
				use ard/maybe

				fn get_value() Int? {
					maybe::none()
				}

				fn test() Int {
					let value = try get_value() -> _ { 42 }
					value + 1
				}

				test()
			`,
			want: 42,
		},
		{
			name: "try on Maybe with catch block - some case",
			input: `
				use ard/maybe

				fn get_value() Int? {
					maybe::some(10)
				}

				fn test() Int {
					let value = try get_value() -> _ { 42 }
					value + 1
				}

				test()
			`,
			want: 11,
		},
		{
			name: "try on Maybe with different inner types - none case",
			input: `
				use ard/maybe

				fn get_value() Int? {
					maybe::none()
				}

				fn test() Str? {
					let value = try get_value()
					maybe::some("should not reach")
				}

				let result = test()
				match result {
					value => value,
					_ => "got none as expected"
				}
			`,
			want: "got none as expected",
		},
		{
			name: "try on chained maybes",
			input: `
				use ard/maybe

				struct Profile {
			  		name: Str?
				}

				struct User {
			  		profile: Profile?
				}

				fn get_user() User? {
					let profile = maybe::some(Profile{name: maybe::none() })
					maybe::some(User{ profile: profile })
			 	}

				fn get_name() Str {
					let name = try get_user().profile.name -> _ { "Sample" }
					name
				}

				get_name()
			`,
			want: "Sample",
		},
	})
}
