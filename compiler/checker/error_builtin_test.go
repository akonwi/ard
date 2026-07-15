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
