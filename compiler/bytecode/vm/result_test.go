package vm

import "testing"

func TestBytecodeResults(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name:  "Ok",
			input: `Result::ok(200)`,
			want:  200,
		},
		{
			name:  "Err",
			input: `Result::err(404)`,
			want:  404,
		},
		{
			name: "Matching a result",
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBytecodeResultOrThenMaybeCombinators(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "map after or(maybe::none()) on ok result",
			input: `
				use ard/maybe
				let res: Str?!Str = Result::ok(maybe::some("hello"))
				let val = res
					.or(maybe::none())
					.map(fn(s: Str) Int { s.size() })
					.or(0)
				val
			`,
			want: 5,
		},
		{
			name: "map after or(maybe::none()) on err result",
			input: `
				use ard/maybe
				let res: Str?!Str = Result::err("bad")
				let val = res
					.or(maybe::none())
					.map(fn(s: Str) Int { s.size() })
					.or(0)
				val
			`,
			want: 0,
		},
		{
			name: "and_then after or(maybe::none()) on ok result",
			input: `
				use ard/maybe
				let res: Str?!Str = Result::ok(maybe::some("world"))
				let val = res
					.or(maybe::none())
					.and_then(fn(s: Str) Int? { maybe::some(s.size()) })
					.or(0)
				val
			`,
			want: 5,
		},
		{
			name: "and_then after or(maybe::none()) on err result",
			input: `
				use ard/maybe
				let res: Str?!Str = Result::err("bad")
				let val = res
					.or(maybe::none())
					.and_then(fn(s: Str) Int? { maybe::some(s.size()) })
					.or(0)
				val
			`,
			want: 0,
		},
		{
			name: "or(fallback) after or(maybe::none())",
			input: `
				use ard/maybe
				let res: Str?!Str = Result::err("bad")
				let val = res
					.or(maybe::none())
					.or("default")
				val
			`,
			want: "default",
		},
		{
			name: "intermediate let binding preserves Maybe type",
			input: `
				use ard/maybe
				let res: Str?!Str = Result::ok(maybe::some("test"))
				let maybe_val: Str? = res.or(maybe::none())
				let mapped = maybe_val.map(fn(s: Str) Int { s.size() })
				mapped.or(0)
			`,
			want: 4,
		},
		{
			name: "map after or(maybe::none()) on function return value",
			input: `
				use ard/maybe

				fn returns_result() Dynamic?!Str {
					Result::ok(maybe::some(Dynamic::from("hello")))
				}

				let val = returns_result()
					.or(maybe::none())
					.map(fn(d: Dynamic) Str { "got it" })
					.or("nope")
				val
			`,
			want: "got it",
		},
		{
			name: "and_then after or(maybe::none()) on function return value",
			input: `
				use ard/maybe

				fn returns_result() Dynamic?!Str {
					Result::ok(maybe::some(Dynamic::from("hello")))
				}

				let val = returns_result()
					.or(maybe::none())
					.and_then(fn(d: Dynamic) Str? { maybe::some("chained") })
					.or("nope")
				val
			`,
			want: "chained",
		},
		{
			name: "map with intermediate binding from function return",
			input: `
				use ard/maybe

				fn returns_result() Dynamic?!Str {
					Result::ok(maybe::some(Dynamic::from("test")))
				}

				let res = returns_result()
				let maybe_val = res.or(maybe::none())
				let mapped = maybe_val.map(fn(d: Dynamic) Str { "mapped" })
				mapped.or("nope")
			`,
			want: "mapped",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBytecodeTryResultParity(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "trying an ok result",
			input: `
				fn divide(a: Int, b: Int) Int!Str {
					match b == 0 {
					  true => Result::err("cannot divide by 0"),
					  false => Result::ok(a / b),
					}
				}
				fn divide_plus_10(a: Int, b: Int) Int!Str {
					let res = try divide(a, b)
					Result::ok(res + 10)
				}

				divide_plus_10(100, 4)
			`,
			want: 35,
		},
		{
			name: "trying an error result",
			input: `
				fn divide(a: Int, b: Int) Int!Str {
					match b == 0 {
					  true => Result::err("cannot divide by 0"),
					  false => Result::ok(a / b),
					}
				}
				fn divide_plus_10(a: Int, b: Int) Int!Str {
					let res = try divide(a, b)
					Result::ok(res + 10)
				}

				divide_plus_10(100, 0)
			`,
			want: "cannot divide by 0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBytecodeTry(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "trying an ok result",
			input: `
				fn divide(a: Int, b: Int) Int!Str {
					match b == 0 {
					  true => Result::err("cannot divide by 0"),
					  false => Result::ok(a / b),
					}
				}
				fn divide_plus_10(a: Int, b: Int) Int!Str {
					let res = try divide(a, b)
					Result::ok(res + 10)
				}
				divide_plus_10(100, 4)
			`,
			want: 35,
		},
		{
			name: "trying an error result",
			input: `
				fn divide(a: Int, b: Int) Int!Str {
					match b == 0 {
					  true => Result::err("cannot divide by 0"),
					  false => Result::ok(a / b),
					}
				}
				fn divide_plus_10(a: Int, b: Int) Int!Str {
					let res = try divide(a, b)
					Result::ok(res + 10)
				}
				divide_plus_10(100, 0)
			`,
			want: "cannot divide by 0",
		},
	})
}
