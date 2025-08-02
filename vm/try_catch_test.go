package vm_test

import (
	"testing"
)

func TestTryCatchBasic(t *testing.T) {
	input := `
	fn test_catch() Str {
		try Result::err("test") -> err {
			"caught"
		}
	}
	test_catch()
	`
	result := run(t, input)
	expected := "caught"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestTryCatchSuccess(t *testing.T) {
	input := `
	fn foobar() Int!Str {
		Result::ok(42)
	}

	fn do_thing() Int {
		try foobar() -> err {
			0
		}
	}
	
	do_thing()
	`
	result := run(t, input)
	expected := 42
	if result != expected {
		t.Errorf("Expected %d, got %v", expected, result)
	}
}

func TestTryCatchWithoutCatch(t *testing.T) {
	input := `
	fn foobar() Int!Str {
		Result::err("error")
	}

	fn do_thing() Int!Str {
		let res = try foobar()
		Result::ok(res)
	}

	do_thing()
	`
	result := run(t, input)
	// Should return the error result as-is
	if result != "error" {
		t.Errorf("Expected error string, got %v", result)
	}
}

func TestTryCatchErrorTransformation(t *testing.T) {
	input := `
	fn parse_number(s: Str) Int!Str {
		Result::err("not a number")
	}

	fn process_data() Str {
		try parse_number("abc") -> err {
			"Error processing: {err}"
		}
	}

	process_data()
	`
	result := run(t, input)
	expected := "Error processing: not a number"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestTryCatchNestedCalls(t *testing.T) {
	input := `
	fn inner() Int!Str {
		Result::err("inner error")
	}

	fn middle() Int!Str {
		try inner() -> err {
			Result::err("caught and re-wrapped: {err}")
		}
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
		try foobar() -> make_error_message
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
	fn make_error_message(code: Str) Int {
		0
	}

	fn foobar() Int!Str {
		Result::ok(42)
	}

	fn do_thing() Int {
		try foobar() -> make_error_message
	}

	do_thing()
	`
	result := run(t, input)
	expected := 42
	if result != expected {
		t.Errorf("Expected %d, got %v", expected, result)
	}
}
