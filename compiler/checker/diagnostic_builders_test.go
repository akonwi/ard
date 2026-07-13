package checker

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestUnresolvedReferenceDiagnostic(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard"}
	tests := []struct {
		kind    unresolvedReferenceKind
		name    string
		code    DiagnosticCode
		message string
		title   string
	}{
		{unrecognizedType, "Missing", DiagnosticCodeUndefinedType, "Unrecognized type: Missing", "Unrecognized type"},
		{unrecognizedReturnType, "Missing", DiagnosticCodeUndefinedType, "Unrecognized return type: Missing", "Unrecognized return type"},
		{undefinedType, "Missing", DiagnosticCodeUndefinedType, "Undefined type: Missing", "Undefined type"},
		{undefinedTrait, "Missing", DiagnosticCodeUndefinedTrait, "Undefined trait: Missing", "Undefined trait"},
		{unknownModule, "ard/missing", DiagnosticCodeUndefinedModule, "Unknown module: ard/missing", "Unknown module"},
		{undefinedModule, "missing", DiagnosticCodeUndefinedModule, "Undefined module: missing", "Undefined module"},
		{unknownGoNamespace, "missing", DiagnosticCodeUndefinedNamespace, "Unknown Go namespace: missing", "Unknown Go namespace"},
		{unknownStructField, "missing", DiagnosticCodeUnknownField, "Unknown field: missing", "Unknown field"},
		{undefinedAssignmentTarget, "missing", DiagnosticCodeUndefinedName, "Undefined: missing", "Undefined assignment target"},
		{undefinedQualifiedMember, "mod::missing", DiagnosticCodeUndefinedQualifiedMember, "Undefined: mod::missing", "Undefined qualified member"},
		{undefinedGoFunction, "fmt::Missing", DiagnosticCodeUndefinedGoFunction, "Undefined Go function: fmt::Missing", "Undefined Go function"},
		{undefinedGoType, "image::Missing", DiagnosticCodeUndefinedType, "Undefined Go type: image::Missing", "Undefined Go type"},
		{undefinedStaticRoot, "Missing", DiagnosticCodeUndefinedName, "Undefined: Missing", "Undefined name"},
		{undefinedEnumVariant, "Color::missing", DiagnosticCodeUndefinedEnumVariant, "Undefined: Color::missing", "Undefined enum variant"},
		{invalidStaticMember, "Str::missing", DiagnosticCodeInvalidStaticMember, "Undefined: Str::missing", "Invalid static member"},
		{undefinedStructType, "Missing", DiagnosticCodeUndefinedType, "Undefined: Missing", "Undefined struct type"},
		{notAStruct, "value", DiagnosticCodeNotAStruct, "Undefined: value", "Not a struct"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			diagnostic := (unresolvedReferenceDiagnostic{Kind: tt.kind, Name: tt.name, Span: span}).build()
			if diagnostic.Code != tt.code || diagnostic.Message != tt.message || diagnostic.Title != tt.title {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if diagnostic.Primary.Span != span || diagnostic.Primary.Message == "" {
				t.Fatalf("primary = %#v", diagnostic.Primary)
			}
		})
	}
}

func TestUndefinedNameDiagnostic(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 1}}}
	tests := []struct {
		name   string
		kind   undefinedNameKind
		title  string
		legacy string
	}{
		{name: "variable", kind: undefinedVariable, title: "Undefined variable", legacy: "Undefined variable: missing"},
		{name: "function", kind: undefinedFunction, title: "Undefined function", legacy: "Undefined function: missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostic := (undefinedNameDiagnostic{Kind: tt.kind, Name: "missing", Span: span}).build()
			if diagnostic.Code != DiagnosticCodeUndefinedName || diagnostic.Title != tt.title || diagnostic.Message != tt.legacy {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if diagnostic.Primary.Span != span || diagnostic.Primary.Message != "`missing` is not defined in this scope" {
				t.Fatalf("primary = %#v", diagnostic.Primary)
			}
		})
	}
}

func TestUndefinedMemberDiagnostic(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 2, Col: 8}}}
	tests := []struct {
		name        string
		kind        undefinedMemberKind
		title       string
		legacy      string
		primaryText string
	}{
		{name: "field", kind: undefinedField, title: "Undefined field", legacy: "Undefined: user.height", primaryText: "`height` is not defined for `user`"},
		{name: "method", kind: undefinedMethod, title: "Undefined method", legacy: "Undefined: user.save", primaryText: "`save` is not defined for `user`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			member := "height"
			if tt.kind == undefinedMethod {
				member = "save"
			}
			diagnostic := (undefinedMemberDiagnostic{Kind: tt.kind, Receiver: "user", Member: member, Span: span}).build()
			if diagnostic.Code != DiagnosticCodeUndefinedMember || diagnostic.Title != tt.title || diagnostic.Message != tt.legacy {
				t.Fatalf("code/title/message = %q/%q/%q", diagnostic.Code, diagnostic.Title, diagnostic.Message)
			}
			if diagnostic.Primary.Span != span || diagnostic.Primary.Message != tt.primaryText {
				t.Fatalf("primary = %#v", diagnostic.Primary)
			}
			if len(diagnostic.Secondary) != 0 {
				t.Fatalf("secondary = %#v, want none", diagnostic.Secondary)
			}
		})
	}
}

func TestDuplicateDeclarationDiagnosticBuildsBothLabels(t *testing.T) {
	original := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 8}}}
	duplicate := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 2, Col: 6}}}
	diagnostic := (duplicateDeclarationDiagnostic{
		Name:          "User",
		DuplicateSpan: duplicate,
		OriginalSpan:  original,
	}).build()

	if diagnostic.Kind != Error || diagnostic.Code != DiagnosticCodeDuplicateDeclaration {
		t.Fatalf("kind/code = %q/%q", diagnostic.Kind, diagnostic.Code)
	}
	if diagnostic.Message != "Duplicate declaration: User" || diagnostic.Title != "Duplicate declaration" {
		t.Fatalf("message/title = %q/%q", diagnostic.Message, diagnostic.Title)
	}
	if diagnostic.Primary.Span != duplicate || diagnostic.Primary.Message != "`User` is declared again here" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span != original || diagnostic.Secondary[0].Message != "first declared here" {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestTypeMismatchDiagnosticWithoutExpectationLabelsPrimary(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 1}}}
	diagnostic := (typeMismatchDiagnostic{
		Expected:   Str,
		Actual:     Int,
		ActualSpan: span,
	}).build()

	if diagnostic.Kind != Error {
		t.Fatalf("kind = %q, want error", diagnostic.Kind)
	}
	if diagnostic.Code != DiagnosticCodeTypeMismatch {
		t.Fatalf("code = %q, want type mismatch", diagnostic.Code)
	}
	if diagnostic.Title != "Type mismatch" || diagnostic.Text != "" {
		t.Fatalf("title/text = %q/%q", diagnostic.Title, diagnostic.Text)
	}
	if diagnostic.Primary.Message != "expected `Str`, but this expression has type `Int`" {
		t.Fatalf("primary label = %q", diagnostic.Primary.Message)
	}
	if len(diagnostic.Secondary) != 0 {
		t.Fatalf("secondary labels = %#v, want none", diagnostic.Secondary)
	}
	if diagnostic.Message != "Type mismatch: Expected Str, got Int" {
		t.Fatalf("legacy message = %q", diagnostic.Message)
	}
	if diagnostic.FilePath() != span.FilePath || diagnostic.Location() != span.Location {
		t.Fatalf("compatibility span = %q %v, want %#v", diagnostic.FilePath(), diagnostic.Location(), span)
	}
}

func TestTypeMismatchDiagnosticUsesNeutralExpectationLabel(t *testing.T) {
	expectedSpan := SourceSpan{FilePath: "types.ard", Location: parse.Location{Start: parse.Point{Row: 2, Col: 3}}}
	diagnostic := (typeMismatchDiagnostic{
		Expected:   Str,
		Actual:     Int,
		ActualSpan: SourceSpan{FilePath: "main.ard"},
		Expectation: &typeExpectation{
			Span: expectedSpan,
			Kind: expectationUnknown,
		},
	}).build()

	if len(diagnostic.Secondary) != 1 {
		t.Fatalf("secondary labels = %#v, want one", diagnostic.Secondary)
	}
	if diagnostic.Secondary[0].Span != expectedSpan || diagnostic.Secondary[0].Message != "this requires `Str`" {
		t.Fatalf("secondary label = %#v", diagnostic.Secondary[0])
	}
}
