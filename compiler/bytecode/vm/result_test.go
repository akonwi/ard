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
