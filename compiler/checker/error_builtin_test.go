package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestBuiltinErrorTypeFactoryAndImplementation(t *testing.T) {
	source := `struct ValidationError {
  message: Str,
}

impl Error for ValidationError {
  fn error() Str {
    self.message
  }
}

struct BothStringAndError {
  label: Str,
}

impl BothStringAndError {
  fn to_str() Str { "string" }
}

impl Error for BothStringAndError {
  fn error() Str { "error" }
}

fn report(error: Error) Error {
  error
}

fn validate(fail: Bool) Int!Error {
  match fail {
    true => Result::err(Error::new("invalid")),
    false => Result::ok(1),
  }
}

let simple: Error = Error::new("boom")
let custom: Error = report(ValidationError{message: "bad"})
let simple_message = "error: {simple}"
let concrete_message = "error: {ValidationError{message: "bad"}}"
let preferred_string = "{BothStringAndError{label: "both"}}"
let widened_error: Error = BothStringAndError{label: "both"}
let error_fallback = "{widened_error}"
`
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
}

func TestBuiltinErrorRejectsMutatingImplementation(t *testing.T) {
	source := `struct CountingError {
  calls: Int,
}

impl Error for CountingError {
  fn mut error() Str {
    self.calls =+ 1
    "called"
  }
}

let error = CountingError{calls: 0}
let message = "{error}"
`
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeImplReceiverMutability)
	if diagnostic.Message != "Trait method 'error' does not allow a mutating receiver" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestBuiltinErrorRejectsGeneratedGoFieldCollision(t *testing.T) {
	source := `struct Problem {
  error: Str,
}

impl Error for Problem {
  fn error() Str { self.error }
}
`
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeGoMethodFieldCollision)
	if diagnostic.Message != "Ard property 'Problem.error' lowers to Go field 'Error', which conflicts with Go interface method 'Error'" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestErrorInterpolationRejectsUnionOfErrors(t *testing.T) {
	source := `struct FirstError { message: Str }
struct SecondError { message: Str }

impl Error for FirstError {
  fn error() Str { self.message }
}

impl Error for SecondError {
  fn error() Str { self.message }
}

type Failure = FirstError | SecondError
let failure: Failure = FirstError{message: "failed"}
let message = "{failure}"
`
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected union Error interpolation diagnostic")
	}
}

func TestBuiltinErrorRejectsNonStructImplementation(t *testing.T) {
	source := `enum Failure { invalid }

impl Error for Failure {
  fn error() Str { "invalid" }
}
`
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected Error implementation target diagnostic")
	}
}

func TestBuiltinErrorRequiresExplicitImplementation(t *testing.T) {
	source := `struct ValidationError {
  message: Str,
}

impl ValidationError {
  fn error() Str {
    self.message
  }
}

let error: Error = ValidationError{message: "bad"}
`
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected explicit Error implementation requirement")
	}
}
