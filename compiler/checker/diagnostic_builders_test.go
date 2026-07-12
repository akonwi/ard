package checker

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

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
