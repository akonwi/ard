package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestDuplicateImportHasStructuredLabels(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("use ard/list\nuse ard/list\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	original := result.Program.Imports[0].GetLocation()
	duplicate := result.Program.Imports[1].GetLocation()
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Kind != checker.Warn || diagnostic.Code != checker.DiagnosticCodeDuplicateImport {
		t.Fatalf("kind/code = %q/%q", diagnostic.Kind, diagnostic.Code)
	}
	if diagnostic.Primary.Span.Location != duplicate || diagnostic.Primary.Message != "`list` is imported again here" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != original || diagnostic.Secondary[0].Message != "first imported here" {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestDuplicateStructFieldHasStructuredLabels(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("struct User {\n  name: Str,\n  name: Int,\n}\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	definition := result.Program.Statements[0].(*parse.StructDefinition)
	original := definition.Fields[0].Name.GetLocation()
	duplicate := definition.Fields[1].Name.GetLocation()
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeDuplicateFieldDeclaration {
		t.Fatalf("code = %q, want duplicate field declaration", diagnostic.Code)
	}
	if diagnostic.Message != "Duplicate field: name" || diagnostic.Title != "Duplicate field declaration" {
		t.Fatalf("message/title = %q/%q", diagnostic.Message, diagnostic.Title)
	}
	if diagnostic.Primary.Span.Location != duplicate || diagnostic.Primary.Message != "field `name` is declared again here" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != original || diagnostic.Secondary[0].Message != "first declared here" {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestDuplicateTopLevelTypeDeclarationHasStructuredLabels(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("struct User {}\nenum User { guest }\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	original := result.Program.Statements[0].(*parse.StructDefinition).Name.GetLocation()
	duplicate := result.Program.Statements[1].(*parse.EnumDefinition).NameLocation
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeDuplicateDeclaration {
		t.Fatalf("code = %q, want duplicate declaration", diagnostic.Code)
	}
	if diagnostic.Message != "Duplicate declaration: User" {
		t.Fatalf("legacy message = %q", diagnostic.Message)
	}
	if diagnostic.Primary.Span.FilePath != filePath || diagnostic.Primary.Span.Location != duplicate {
		t.Fatalf("primary span = %#v, want second declaration at %v", diagnostic.Primary.Span, duplicate)
	}
	if diagnostic.Primary.Message != "`User` is declared again here" {
		t.Fatalf("primary label = %q", diagnostic.Primary.Message)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != original {
		t.Fatalf("secondary labels = %#v, want first declaration at %v", diagnostic.Secondary, original)
	}
	if diagnostic.Secondary[0].Message != "first declared here" {
		t.Fatalf("secondary label = %q", diagnostic.Secondary[0].Message)
	}
}

func TestDuplicateTopLevelTypesPointBackToFirstDeclaration(t *testing.T) {
	result := parse.Parse([]byte("struct User {}\nenum User { guest }\ntrait User {\n}\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	first := result.Program.Statements[0].(*parse.StructDefinition).Name.GetLocation()

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 2 {
		t.Fatalf("diagnostics = %#v, want two", c.Diagnostics())
	}
	for i, diagnostic := range c.Diagnostics() {
		if diagnostic.Primary.Span.Location.Start.Row != i+2 {
			t.Fatalf("diagnostic %d primary = %#v", i, diagnostic.Primary.Span)
		}
		if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != first {
			t.Fatalf("diagnostic %d secondary = %#v, want first declaration", i, diagnostic.Secondary)
		}
	}
}

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
