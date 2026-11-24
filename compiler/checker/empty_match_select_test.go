package checker_test

import (
	"testing"

	"github.com/akonwi/ard/compiler/checker"
	"github.com/akonwi/ard/compiler/parse"
)

// An empty match or select reports one diagnostic that names the missing
// cases and points at the braces, whether the braces share a line or not
// (issue #595).
func TestEmptyMatchAndSelectReportMissingCases(t *testing.T) {
	const matchTitle = "A match needs at least one case"
	const selectTitle = "A select needs at least one case"
	tests := []struct {
		name  string
		body  string
		code  checker.DiagnosticCode
		title string
	}{
		{"match on one line", "  match x {}", checker.DiagnosticCodeEmptyMatch, matchTitle},
		{"match on two lines", "  match x {\n  }", checker.DiagnosticCodeEmptyMatch, matchTitle},
		{"match value", "  let y: Int = match x {}", checker.DiagnosticCodeEmptyMatch, matchTitle},
		{"match with only a comment", "  match x {\n    // todo\n  }", checker.DiagnosticCodeEmptyMatch, matchTitle},
		{"conditional match on one line", "  match {}", checker.DiagnosticCodeEmptyMatch, matchTitle},
		{"conditional match on two lines", "  match {\n  }", checker.DiagnosticCodeEmptyMatch, matchTitle},
		{"select on one line", "  select {}", checker.DiagnosticCodeEmptySelect, selectTitle},
		{"select on two lines", "  select {\n  }", checker.DiagnosticCodeEmptySelect, selectTitle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "fn main() {\n  let x = 1\n" + tt.body + "\n}\n"
			result := parse.Parse([]byte(source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := checker.New("main.ard", checker.WrapStatementRuns(result.Program), nil)
			c.Check()
			diagnostics := c.Diagnostics()
			if len(diagnostics) != 1 {
				t.Fatalf("got %d diagnostics, want 1: %v", len(diagnostics), diagnostics)
			}
			diagnostic := diagnostics[0]
			if diagnostic.Code != tt.code || diagnostic.Kind != checker.Error || diagnostic.Title != tt.title || diagnostic.Message != tt.title || diagnostic.Primary.Message == "" {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if want := emptyBlockBraces(t, result.Program); diagnostic.Primary.Span.Location != want {
				t.Fatalf("primary span = %+v, want the braces %+v", diagnostic.Primary.Span.Location, want)
			}
		})
	}
}

func emptyBlockBraces(t *testing.T, program *parse.Program) parse.Location {
	t.Helper()
	fn := program.Statements[0].(*parse.FunctionDeclaration)
	stmt := fn.Body[len(fn.Body)-1]
	if decl, ok := stmt.(*parse.VariableDeclaration); ok {
		stmt = decl.Value
	}
	switch expr := stmt.(type) {
	case *parse.MatchExpression:
		return expr.BracesLocation
	case *parse.ConditionalMatchExpression:
		return expr.BracesLocation
	case *parse.SelectExpression:
		return expr.BracesLocation
	}
	t.Fatalf("unexpected statement %T", stmt)
	return parse.Location{}
}
