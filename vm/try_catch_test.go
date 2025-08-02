package vm_test

import (
	"testing"
)

func TestTryEarlyReturn(t *testing.T) {
	input := `
	fn test_early_return() Str {
		try Result::err("test") -> err {
			"caught: {err}"
		}
		"should not reach here"
	}
	test_early_return()
	`
	result := run(t, input)
	expected := "caught: test"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestTrySuccess(t *testing.T) {
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
	result := run(t, input)
	expected := "success: 42"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestTryWithoutCatchPropagatesError(t *testing.T) {
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
	result := run(t, input)
	// Should return the error result as-is
	if result != "error" {
		t.Errorf("Expected error string, got %v", result)
	}
}

func TestTryWithoutCatchSuccess(t *testing.T) {
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
	result := run(t, input)
	expected := 42
	if result != expected {
		t.Errorf("Expected %d, got %v", expected, result)
	}
}

func TestTryErrorTransformation(t *testing.T) {
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
	result := run(t, input)
	expected := "Error processing: not a number"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestTryNestedEarlyReturns(t *testing.T) {
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
	result := run(t, input)
	expected := "Final catch: caught and re-wrapped: inner error"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestTryCatchWithFunction(t *testing.T) {
	input := `
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
	`
	result := run(t, input)
	expected := "Failed with code: something went wrong"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestTryCatchWithFunctionSuccess(t *testing.T) {
	input := `
	fn make_error_message(code: Int) Str {
		"Error: {code}"
	}

	fn foobar() Str!Int {
		Result::err(42)
	}

	fn do_thing() Str {
		let result = try foobar() -> make_error_message
		result
	}

	do_thing()
	`
	result := run(t, input)
	expected := "Error: 42"
	if result != expected {
		t.Errorf("Expected %s, got %v", expected, result)
	}
}

// Test simpler early return behavior
func TestTryEarlyReturnSkipsRestOfFunction(t *testing.T) {
	input := `
	fn test_func() Str {
		try Result::err("early") -> err {
			"caught: {err}"
		}
		"this should not execute"
	}

	test_func()
	`
	result := run(t, input)
	expected := "caught: early"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}
