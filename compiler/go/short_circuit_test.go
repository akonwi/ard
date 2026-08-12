package gotarget

import "testing"

func TestBooleanOperatorsShortCircuitStatementProducingOperands(t *testing.T) {
	program := lowerParitySource(t, `
fn skipped_and() Bool {
  let values = [1]
  false and values.at(5).expect("and evaluated its right operand") == 1
}

fn skipped_or() Bool {
  let values = [1]
  true or values.at(5).expect("or evaluated its right operand") == 1
}

fn main() Bool {
  let and_result = skipped_and()
  let or_result = skipped_or()
  not and_result and or_result
}
`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}

func TestBooleanOperatorsEvaluateRequiredOperandsOnceInOrder(t *testing.T) {
	program := lowerParitySource(t, `
mut trace = ""

fn mark(label: Str, value: Bool) Bool {
  trace = "{trace}{label}"
  value
}

fn prepared(label: Str, value: Bool) Bool {
  [mark(label, value)].at(0).expect("present")
}

fn main() Bool {
  let needed_and = mark("a", true) and prepared("b", true)
  let needed_or = mark("c", false) or prepared("d", true)
  let skipped_and = mark("e", false) and prepared("x", true)
  let skipped_or = mark("f", true) or prepared("y", false)
  let nested = mark("g", true) and (mark("h", false) or prepared("i", true))
  needed_and and needed_or and not skipped_and and skipped_or and nested and trace == "abcdefghi"
}
`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}
