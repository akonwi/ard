package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestRemainingUnresolvedReferencesHaveStructuredDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		code    checker.DiagnosticCode
		message string
		span    func(*parse.Program) parse.Location
	}{
		{name: "type", source: "let value: Missing = 1", code: checker.DiagnosticCodeUndefinedType, message: "Unrecognized type: Missing"},
		{name: "struct field", source: "struct User { name: Str }\nUser{name: \"A\", missing: 1}", code: checker.DiagnosticCodeUnknownField, message: "Unknown field: missing"},
		{name: "assignment target", source: "missing = 1", code: checker.DiagnosticCodeUndefinedName, message: "Undefined: missing", span: func(program *parse.Program) parse.Location {
			return program.Statements[0].(*parse.VariableAssignment).Target.GetLocation()
		}},
		{name: "static root", source: "Missing::value", code: checker.DiagnosticCodeUndefinedName, message: "Undefined: Missing", span: func(program *parse.Program) parse.Location {
			return program.Statements[0].(*parse.StaticProperty).Target.GetLocation()
		}},
		{name: "enum variant", source: "enum Color { red }\nColor::missing", code: checker.DiagnosticCodeUndefinedEnumVariant, message: "Undefined: Color::missing", span: func(program *parse.Program) parse.Location {
			return program.Statements[1].(*parse.StaticProperty).Target.GetLocation()
		}},
		{name: "undefined struct", source: "Missing{}", code: checker.DiagnosticCodeUndefinedType, message: "Undefined: Missing", span: func(program *parse.Program) parse.Location {
			return program.Statements[0].(*parse.StructInstance).GetLocation()
		}},
		{name: "not a struct", source: "let value = 1\nvalue{}", code: checker.DiagnosticCodeNotAStruct, message: "Undefined: value", span: func(program *parse.Program) parse.Location {
			return program.Statements[1].(*parse.StructInstance).GetLocation()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := checker.New("main.ard", result.Program, nil)
			c.Check()
			for _, diagnostic := range c.Diagnostics() {
				if diagnostic.Message == tt.message {
					if diagnostic.Code != tt.code || diagnostic.Primary.Message == "" {
						t.Fatalf("diagnostic = %#v", diagnostic)
					}
					if tt.span != nil && diagnostic.Primary.Span.Location != tt.span(result.Program) {
						t.Fatalf("primary span = %v, want %v", diagnostic.Primary.Span.Location, tt.span(result.Program))
					}
					return
				}
			}
			t.Fatalf("diagnostics = %#v, missing %q", c.Diagnostics(), tt.message)
		})
	}
}

func TestUndefinedMembersInMaybeAccessorChainsHaveStructuredDiagnostics(t *testing.T) {
	prefix := "struct Profile { name: Str }\nfn test() {\n  let profile: Profile? = Maybe::new(Profile{name: \"A\"})\n  try "
	tests := []struct {
		name     string
		expr     string
		location func(parse.Statement) parse.Location
		title    string
		legacy   string
	}{
		{
			name: "field after maybe", expr: "profile.missing",
			location: func(stmt parse.Statement) parse.Location {
				return stmt.(*parse.InstanceProperty).Property.GetLocation()
			},
			title: "Undefined field", legacy: "Undefined: Profile.missing",
		},
		{
			name: "method after maybe", expr: "profile.missing()",
			location: func(stmt parse.Statement) parse.Location { return stmt.(*parse.InstanceMethod).Method.GetLocation() },
			title:    "Undefined method", legacy: "Undefined: Profile.missing",
		},
		{
			name: "later member in chain", expr: "profile.name.missing",
			location: func(stmt parse.Statement) parse.Location {
				return stmt.(*parse.InstanceProperty).Property.GetLocation()
			},
			title: "Undefined field", legacy: "Undefined: Str.missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(prefix+tt.expr+" -> _ {\n  }\n}\n"), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			function := result.Program.Statements[len(result.Program.Statements)-1].(*parse.FunctionDeclaration)
			tryExpr := function.Body[len(function.Body)-1].(*parse.Try)
			location := tt.location(tryExpr.Expression)
			c := checker.New("main.ard", result.Program, nil)
			c.Check()
			if len(c.Diagnostics()) != 1 {
				t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
			}
			diagnostic := c.Diagnostics()[0]
			if diagnostic.Code != checker.DiagnosticCodeUndefinedMember || diagnostic.Title != tt.title || diagnostic.Message != tt.legacy {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if diagnostic.Primary.Span.Location != location {
				t.Fatalf("primary = %#v, want location %v", diagnostic.Primary, location)
			}
		})
	}
}

func TestUndefinedNamesHaveStructuredDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		title   string
		legacy  string
		message string
	}{
		{name: "variable", source: "missing", title: "Undefined variable", legacy: "Undefined variable: missing", message: "`missing` is not defined in this scope"},
		{name: "function", source: "missing()", title: "Undefined function", legacy: "Undefined function: missing", message: "`missing` is not defined in this scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			location := result.Program.Statements[0].GetLocation()
			c := checker.New("main.ard", result.Program, nil)
			c.Check()
			if len(c.Diagnostics()) != 1 {
				t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
			}
			diagnostic := c.Diagnostics()[0]
			if diagnostic.Code != checker.DiagnosticCodeUndefinedName || diagnostic.Title != tt.title || diagnostic.Message != tt.legacy {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if diagnostic.Primary.Span.Location != location || diagnostic.Primary.Message != tt.message {
				t.Fatalf("primary = %#v, want location %v", diagnostic.Primary, location)
			}
		})
	}
}

func TestUndefinedInstanceMembersHaveStructuredDiagnostics(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("\"foo\".length\n\"foo\".save()\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	property := result.Program.Statements[0].(*parse.InstanceProperty)
	method := result.Program.Statements[1].(*parse.InstanceMethod)

	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 2 {
		t.Fatalf("diagnostics = %#v, want two", c.Diagnostics())
	}
	tests := []struct {
		diagnostic checker.Diagnostic
		location   parse.Location
		title      string
		legacy     string
	}{
		{diagnostic: c.Diagnostics()[0], location: property.Property.GetLocation(), title: "Undefined field", legacy: `Undefined: "foo".length`},
		{diagnostic: c.Diagnostics()[1], location: method.Method.GetLocation(), title: "Undefined method", legacy: `Undefined: "foo".save`},
	}
	for _, tt := range tests {
		if tt.diagnostic.Code != checker.DiagnosticCodeUndefinedMember || tt.diagnostic.Title != tt.title || tt.diagnostic.Message != tt.legacy {
			t.Fatalf("diagnostic = %#v", tt.diagnostic)
		}
		if tt.diagnostic.Primary.Span.Location != tt.location {
			t.Fatalf("primary location = %v, want %v", tt.diagnostic.Primary.Span.Location, tt.location)
		}
	}
}

func TestImmutableVariableAssignmentHasStructuredLabels(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("let count = 0\ncount = 1\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	declaration := result.Program.Statements[0].(*parse.VariableDeclaration).NameLocation
	assignment := result.Program.Statements[1].(*parse.VariableAssignment).Target.GetLocation()
	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	diagnostic := c.Diagnostics()[0]
	if diagnostic.Kind != checker.Error || diagnostic.Code != checker.DiagnosticCodeImmutableAssignment {
		t.Fatalf("kind/code = %q/%q", diagnostic.Kind, diagnostic.Code)
	}
	if diagnostic.Message != "Immutable variable: count" || diagnostic.Title != "Cannot assign to immutable variable" {
		t.Fatalf("message/title = %q/%q", diagnostic.Message, diagnostic.Title)
	}
	if diagnostic.Primary.Span != (checker.SourceSpan{FilePath: filePath, Location: assignment}) || diagnostic.Primary.Message != "cannot assign to `count`" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span != (checker.SourceSpan{FilePath: filePath, Location: declaration}) || diagnostic.Secondary[0].Message != "`count` was declared immutable here" {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestImmutableAssignmentUsesInnermostBindingProvenance(t *testing.T) {
	result := parse.Parse([]byte("let value = 1\nfn main() {\n  let value = 2\n  value = 3\n}\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}
	secondary := c.Diagnostics()[0].Secondary
	if len(secondary) != 1 || secondary[0].Span.Location.Start.Row != 3 {
		t.Fatalf("secondary = %#v, want inner declaration", secondary)
	}
}

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
