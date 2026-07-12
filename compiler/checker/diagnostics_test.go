package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestAnnotatedBindingTypeMismatchHasStructuredLabels(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte(`let name: Str = 42`), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	binding, ok := result.Program.Statements[0].(*parse.VariableDeclaration)
	if !ok {
		t.Fatalf("statement = %T, want *parse.VariableDeclaration", result.Program.Statements[0])
	}

	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	diagnostic := c.Diagnostics()[0]
	if diagnostic.Kind != checker.Error {
		t.Fatalf("kind = %q, want error", diagnostic.Kind)
	}
	if diagnostic.Code != checker.DiagnosticCodeTypeMismatch {
		t.Fatalf("code = %q, want %q", diagnostic.Code, checker.DiagnosticCodeTypeMismatch)
	}
	if diagnostic.Title != "Type mismatch" {
		t.Fatalf("title = %q, want Type mismatch", diagnostic.Title)
	}
	if diagnostic.Primary.Span.FilePath != filePath {
		t.Fatalf("primary file = %q, want %q", diagnostic.Primary.Span.FilePath, filePath)
	}
	if diagnostic.Primary.Span.Location != binding.Value.GetLocation() {
		t.Fatalf("primary location = %v, want initializer %v", diagnostic.Primary.Span.Location, binding.Value.GetLocation())
	}
	if diagnostic.Primary.Message != "this expression has type `Int`" {
		t.Fatalf("primary label = %q", diagnostic.Primary.Message)
	}
	if len(diagnostic.Secondary) != 1 {
		t.Fatalf("secondary labels = %#v, want one", diagnostic.Secondary)
	}
	related := diagnostic.Secondary[0]
	if related.Span.FilePath != filePath || related.Span.Location != binding.Type.GetLocation() {
		t.Fatalf("related span = %#v, want annotation in %s at %v", related.Span, filePath, binding.Type.GetLocation())
	}
	if related.Message != "this annotation requires `Str`" {
		t.Fatalf("related label = %q", related.Message)
	}
}
