package parse

import "testing"

// An empty match or select parses in both spellings so the checker can
// report the missing cases (issue #595). The braces' location is recorded
// for that diagnostic.
func TestEmptyMatchAndSelectBlocksParse(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		braces Location
	}{
		{"match on one line", "match x {}", Location{Start: Point{Row: 1, Col: 9}, End: Point{Row: 1, Col: 10}}},
		{"match on two lines", "match x {\n}", Location{Start: Point{Row: 1, Col: 9}, End: Point{Row: 2, Col: 1}}},
		{"conditional match on one line", "match {}", Location{Start: Point{Row: 1, Col: 7}, End: Point{Row: 1, Col: 8}}},
		{"conditional match on two lines", "match {\n}", Location{Start: Point{Row: 1, Col: 7}, End: Point{Row: 2, Col: 1}}},
		{"select on one line", "select {}", Location{Start: Point{Row: 1, Col: 8}, End: Point{Row: 1, Col: 9}}},
		{"select on two lines", "select {\n}", Location{Start: Point{Row: 1, Col: 8}, End: Point{Row: 2, Col: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("unexpected parse errors: %v", result.Errors)
			}
			if len(result.Program.Statements) != 1 {
				t.Fatalf("got %d statements, want 1", len(result.Program.Statements))
			}
			var cases int
			var braces Location
			switch expr := result.Program.Statements[0].(type) {
			case *MatchExpression:
				cases, braces = len(expr.Cases), expr.BracesLocation
			case *ConditionalMatchExpression:
				cases, braces = len(expr.Cases), expr.BracesLocation
			case *SelectExpression:
				cases, braces = len(expr.Cases), expr.BracesLocation
			default:
				t.Fatalf("unexpected statement %T", expr)
			}
			if cases != 0 {
				t.Fatalf("got %d cases, want 0", cases)
			}
			if braces != tt.braces {
				t.Fatalf("braces = %+v, want %+v", braces, tt.braces)
			}
		})
	}
}

// A case on the same line as the opening brace still needs a new line.
func TestMatchAndSelectCaseOnBraceLine(t *testing.T) {
	runTests(t, []test{
		{name: "match", input: "match x { _ => 1 }", wantErrs: []string{"Expected new line after '{'"}},
		{name: "conditional match", input: "match { _ => 1 }", wantErrs: []string{"Expected new line after '{'"}},
		{name: "select", input: "select { _ => 1 }", wantErrs: []string{"Expected new line after '{'"}},
	})
}
