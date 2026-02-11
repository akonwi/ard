package vm

import "testing"

func TestBytecodeTryEarlyReturn(t *testing.T) {
	input := `
	fn test_early_return() Str {
		try Result::err("test") -> err {
			"caught: {err}"
		}
		"should not reach here"
	}
	test_early_return()
	`
	result := runBytecode(t, input)
	expected := "caught: test"
	if result != expected {
		t.Fatalf("Expected %q, got %q", expected, result)
	}
}

func TestBytecodeTrySuccess(t *testing.T) {
	input := `
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
	`
	result := runBytecode(t, input)
	expected := "success: 42"
	if result != expected {
		t.Fatalf("Expected %q, got %q", expected, result)
	}
}

func TestBytecodeTryWithoutCatchPropagatesError(t *testing.T) {
	input := `
	fn foobar() Int!Str {
		Result::err("error")
	}

	fn do_thing() Int!Str {
		let res = try foobar()
		Result::ok(res * 2)
	}

	do_thing()
	`
	result := runBytecode(t, input)
	if result != "error" {
		t.Fatalf("Expected %q, got %v", "error", result)
	}
}

func TestBytecodeTryWithoutCatchSuccess(t *testing.T) {
	input := `
	fn foobar() Int!Str {
		Result::ok(21)
	}

	fn do_thing() Int!Str {
		let res = try foobar()
		Result::ok(res * 2)
	}

	do_thing()
	`
	result := runBytecode(t, input)
	if result != 42 {
		t.Fatalf("Expected %d, got %v", 42, result)
	}
}

func TestBytecodeTryErrorTransformation(t *testing.T) {
	input := `
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
	`
	result := runBytecode(t, input)
	expected := "Error processing: not a number"
	if result != expected {
		t.Fatalf("Expected %q, got %q", expected, result)
	}
}

func TestBytecodeTryNestedEarlyReturns(t *testing.T) {
	t.Skip("TODO(bytecode): nested try/catch re-wrap emits invalid jump target")

	input := `
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
	`
	result := runBytecode(t, input)
	expected := "Final catch: caught and re-wrapped: inner error"
	if result != expected {
		t.Fatalf("Expected %q, got %q", expected, result)
	}
}

func TestBytecodeTryCatchWithFunction(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
		skip  bool
	}{
		{
			name: "a simple function name",
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
			name: "a qualified function name",
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runBytecode(t, test.input)
			if result != test.want {
				t.Fatalf("Expected %v, got %v", test.want, result)
			}
		})
	}
}

func TestBytecodeTryEarlyReturnSkipsRestOfFunction(t *testing.T) {
	input := `
	fn test_func() Str {
		try Result::err("early") -> err {
			"caught: {err}"
		}
		"this should not execute"
	}

	test_func()
	`
	result := runBytecode(t, input)
	expected := "caught: early"
	if result != expected {
		t.Fatalf("Expected %q, got %q", expected, result)
	}
}

func TestBytecodeTryInMatchBlocks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
		skip  bool
	}{
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
			skip: true,
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.skip {
				t.Skip("TODO(bytecode): Result.or fallback parity in try/match error path")
			}
			result := runBytecode(t, test.input)
			if result != test.want {
				t.Fatalf("Expected %v, got %v", test.want, result)
			}
		})
	}
}

func TestBytecodeTryingWithUnionErr(t *testing.T) {
	got := runBytecode(t, `
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
	`)

	if got != "Failed" {
		t.Fatalf("Expected 'Failed', got '%v'", got)
	}
}

func TestBytecodeCatchingWithUnionErr(t *testing.T) {
	got := runBytecode(t, `
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
	`)

	if got != "Got a num" {
		t.Fatalf("Expected 'Got a num', got '%v'", got)
	}
}
