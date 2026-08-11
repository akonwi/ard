package diagnostics_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/diagnostics"
	"github.com/akonwi/ard/parse"
)

func TestParseErrorsUseStructuredDiagnosticModel(t *testing.T) {
	converted := diagnostics.ParseErrors("main.ard", []parse.ParseError{{
		Location: parse.Location{Start: parse.Point{Row: 1, Col: 9}, End: parse.Point{Row: 1, Col: 9}},
		Message:  "Expected an expression",
	}})
	if len(converted) != 1 {
		t.Fatalf("diagnostics = %d, want 1", len(converted))
	}
	diagnostic := converted[0]
	if diagnostic.Kind != checker.Error || diagnostic.Code != checker.DiagnosticCodeParseError || diagnostic.Title != "Parse error" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if diagnostic.Primary.Span.FilePath != "main.ard" || diagnostic.Primary.Message != "Expected an expression" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
}

func TestRenderParseErrorsUsesSourceExcerpt(t *testing.T) {
	parseErrors := []parse.ParseError{{
		Location: parse.Location{Start: parse.Point{Row: 1, Col: 9}, End: parse.Point{Row: 1, Col: 9}},
		Message:  "Expected an expression",
	}}
	var output bytes.Buffer
	source := func(string) ([]byte, error) { return []byte("let x =\n"), nil }
	if err := diagnostics.RenderWithOptions(&output, diagnostics.ParseErrors("main.ard", parseErrors), source, diagnostics.RenderOptions{Color: diagnostics.ColorNever}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"error: Parse error",
		" --> main.ard:1:9",
		"1 | let x =",
		"^ Expected an expression",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, output.String())
		}
	}
}

func TestRenderRawStringMarginErrorUsesContentLine(t *testing.T) {
	sourceText := "`\n  first\n second\n  `\n"
	result := parse.Parse([]byte(sourceText), "main.ard")
	if len(result.Errors) != 1 {
		t.Fatalf("parse errors = %#v, want one", result.Errors)
	}
	var output bytes.Buffer
	source := func(string) ([]byte, error) { return []byte(sourceText), nil }
	if err := diagnostics.RenderWithOptions(&output, diagnostics.ParseErrors("main.ard", result.Errors), source, diagnostics.RenderOptions{Color: diagnostics.ColorNever}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		" --> main.ard:3:1",
		"3 |  second",
		"closing delimiter margin",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, output.String())
		}
	}
}

func TestParseErrorsNormalizeMissingLocation(t *testing.T) {
	converted := diagnostics.ParseErrors("main.ard", []parse.ParseError{{Message: "unexpected parser failure"}})
	location := converted[0].Primary.Span.Location
	if location.Start.Row != 1 || location.Start.Col != 1 || location.End != location.Start {
		t.Fatalf("location = %#v, want one-character fallback at 1:1", location)
	}
}
