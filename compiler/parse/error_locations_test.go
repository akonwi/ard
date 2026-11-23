package parse

import "testing"

// Errors that stop parsing must still point at the offending source, not at
// the start of the file (#625).
func TestAbortingParseErrorsHaveLocations(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		start   Point
		message string
	}{
		{
			name:    "type declaration without a type",
			input:   "type Foo =\nfn main() {}\n",
			start:   Point{Row: 1, Col: 11},
			message: "Expected type definition after '='",
		},
		{
			name:    "try-catch arrow without a name",
			input:   "fn main() {\n  let x = try foo() -> 1\n}\n",
			start:   Point{Row: 2, Col: 24},
			message: "Expected identifier after '->' in try-catch",
		},
		{
			name:    "trait implementation member that is not a function",
			input:   "impl T for Int {\n  let x = 1\n}\n",
			start:   Point{Row: 2, Col: 3},
			message: "Expected function declaration in trait implementation block",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) != 1 {
				t.Fatalf("errors = %#v, want one", result.Errors)
			}
			got := result.Errors[0]
			if got.Location.Start != tt.start || got.Message != tt.message {
				t.Fatalf("error = %#v, want %q at %v", got, tt.message, tt.start)
			}
		})
	}
}

// A positional argument after a named one is reported on that argument, and
// parsing continues past it (#625).
func TestPositionalArgumentAfterNamedArgument(t *testing.T) {
	input := "fn main() {\n  let _ = div(a: \"x\", \"y\")\n  let _ = div(b: 1, 2 + 3, c: 4)\n}\n"
	result := Parse([]byte(input), "test.ard")
	want := []ParseError{
		{
			Location: Location{Start: Point{Row: 2, Col: 23}, End: Point{Row: 2, Col: 25}},
			Message:  "positional arguments cannot follow named arguments",
		},
		{
			Location: Location{Start: Point{Row: 3, Col: 21}, End: Point{Row: 3, Col: 25}},
			Message:  "positional arguments cannot follow named arguments",
		},
	}
	if len(result.Errors) != len(want) {
		t.Fatalf("errors = %#v, want %#v", result.Errors, want)
	}
	for i := range want {
		if result.Errors[i] != want[i] {
			t.Fatalf("error %d = %#v, want %#v", i, result.Errors[i], want[i])
		}
	}
	main := result.Program.Statements[0].(*FunctionDeclaration)
	if len(main.Body) != 2 {
		t.Fatalf("main body = %#v, want both statements", main.Body)
	}
}

// An error that stops parsing is reported even after recoverable errors.
func TestAbortingParseErrorFollowsRecoverableErrors(t *testing.T) {
	input := "fn a() {\n  foo(x: 1, 2)\n}\nimpl T for Int {\n  let x = 1\n}\n"
	result := Parse([]byte(input), "test.ard")
	want := []struct {
		start   Point
		message string
	}{
		{Point{Row: 2, Col: 13}, "positional arguments cannot follow named arguments"},
		{Point{Row: 5, Col: 3}, "Expected function declaration in trait implementation block"},
	}
	if len(result.Errors) != len(want) {
		t.Fatalf("errors = %#v, want %d", result.Errors, len(want))
	}
	for i, w := range want {
		if got := result.Errors[i]; got.Location.Start != w.start || got.Message != w.message {
			t.Fatalf("error %d = %#v, want %q at %v", i, got, w.message, w.start)
		}
	}
}
