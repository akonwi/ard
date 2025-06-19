package vm_test

import (
	"testing"
)

func TestResults(t *testing.T) {
	runTests(t, []test{
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
				res.or(-1)`,
			want: -1,
		},
	})
}

func TestTry(t *testing.T) {
	runTests(t, []test{
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
