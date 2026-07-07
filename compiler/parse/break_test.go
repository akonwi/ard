package parse

import (
	"testing"
)

// parseOK asserts the input parses without errors and returns the program.
func parseOK(t *testing.T, input string) *Program {
	t.Helper()
	result := Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Expected no parse errors, got: %v", result.Errors)
	}
	return result.Program
}

func TestBreakStatementTerminators(t *testing.T) {
	t.Run("break followed by newline", func(t *testing.T) {
		parseOK(t, "for i in 1..3 {\n  break\n}\n")
	})
	t.Run("break followed by a trailing comment", func(t *testing.T) {
		// Regression for issue #259.
		parseOK(t, "for i in 1..3 {\n  break // stop early\n}\n")
	})
	t.Run("break followed by closing brace on the same line", func(t *testing.T) {
		parseOK(t, "for i in 1..3 {\n  if i == 2 { break }\n}\n")
	})
}

func TestBreakInMatchArms(t *testing.T) {
	t.Run("inline break arm in subject match", func(t *testing.T) {
		program := parseOK(t, `for i in 1..3 {
  match i {
    2 => break,
    _ => {},
  }
}
`)
		loop := program.Statements[0].(*RangeLoop)
		matchExpr := loop.Body[0].(*MatchExpression)
		if len(matchExpr.Cases) != 2 {
			t.Fatalf("expected 2 cases, got %d", len(matchExpr.Cases))
		}
		if _, ok := matchExpr.Cases[0].Body[0].(*Break); !ok {
			t.Fatalf("expected first arm body to be *Break, got %T", matchExpr.Cases[0].Body[0])
		}
	})
	t.Run("inline break arm in conditional match", func(t *testing.T) {
		program := parseOK(t, `for i in 1..3 {
  match {
    i == 2 => break,
    _ => {},
  }
}
`)
		loop := program.Statements[0].(*RangeLoop)
		matchExpr := loop.Body[0].(*ConditionalMatchExpression)
		if len(matchExpr.Cases) != 2 {
			t.Fatalf("expected 2 cases, got %d", len(matchExpr.Cases))
		}
		if _, ok := matchExpr.Cases[0].Body[0].(*Break); !ok {
			t.Fatalf("expected first arm body to be *Break, got %T", matchExpr.Cases[0].Body[0])
		}
	})
	t.Run("single-line block break arm", func(t *testing.T) {
		parseOK(t, `for i in 1..3 {
  match i {
    2 => { break },
    _ => {},
  }
}
`)
	})
}
