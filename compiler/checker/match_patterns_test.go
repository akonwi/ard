package checker_test

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// Tuple patterns and nested patterns (ADR 0083, #573).
func TestTupleMatchPatterns(t *testing.T) {
	run(t, []test{
		{
			name: "tuple patterns match each element",
			input: strings.Join([]string{
				`fn add(left: Int!Str, right: Int!Str) Int {`,
				`  match (left, right) {`,
				`    (ok(a), ok(b)) => a + b,`,
				`    (err(e), _) => e.size(),`,
				`    (_, err(e)) => e.size(),`,
				`  }`,
				`}`,
			}, "\n"),
		},
		{
			name: "a missing combination is reported",
			input: strings.Join([]string{
				`fn add(left: Int!Str, right: Int!Str) Int {`,
				`  match (left, right) {`,
				`    (ok(a), ok(b)) => a + b,`,
				`    (err(e), _) => e.size(),`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Incomplete match: missing case for '(ok(_), err(_))'"},
			},
		},
		{
			name: "every missing combination is listed",
			input: strings.Join([]string{
				`fn pick(a: Bool, b: Bool) Int {`,
				`  match (a, b) {`,
				`    (true, true) => 1,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Incomplete match: missing cases for '(true, false)', '(false, _)'"},
			},
		},
		{
			name: "unbounded elements need a catch-all",
			input: strings.Join([]string{
				`fn code(pair: (Int, Str)) Str {`,
				`  match pair {`,
				`    (200, body) => body,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Incomplete match: missing case for '(_, _)'"},
			},
		},
		{
			name: "an arm after a covering arm is unreachable",
			input: strings.Join([]string{
				`fn pick(a: Bool, n: Int) Int {`,
				`  match (a, n) {`,
				`    (_, _) => 0,`,
				`    (true, 1) => 1,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Warn, Message: "Unreachable match arm: (true, 1)"},
			},
		},
		{
			name: "an arm covered by several earlier arms is unreachable",
			input: strings.Join([]string{
				`fn pick(a: Bool, n: Int) Int {`,
				`  match (a, n) {`,
				`    (true, _) => 0,`,
				`    (false, _) => 1,`,
				`    (_, 1) => 2,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Warn, Message: "Unreachable match arm: (_, 1)"},
			},
		},
		{
			name: "the pattern must have one element per tuple element",
			input: strings.Join([]string{
				`fn pick(pair: (Int, Str)) Int {`,
				`  match pair {`,
				`    (a, b, c) => a,`,
				`    _ => 0,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Tuple pattern has 3 elements, but (Int, Str) has 2"},
			},
		},
		{
			name: "each element is checked against its type",
			input: strings.Join([]string{
				`fn pick(pair: (Int, Str)) Int {`,
				`  match pair {`,
				`    ("a", _) => 1,`,
				`    _ => 0,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Invalid pattern for Int match: *parse.StrLiteral"},
			},
		},
		{
			name: "a tuple pattern needs a tuple subject",
			input: strings.Join([]string{
				`fn pick(n: Int) Int {`,
				`  match n {`,
				`    (a, b) => a,`,
				`    _ => 0,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Invalid pattern for Int match: *parse.TupleLiteral"},
			},
		},
		{
			name: "a name is bound once per pattern",
			input: strings.Join([]string{
				`fn pick(pair: (Int, Int)) Int {`,
				`  match pair {`,
				`    (a, a) => a,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "a is bound more than once in this pattern"},
			},
		},
		{
			name: "nested ok and err patterns need a value pattern",
			input: strings.Join([]string{
				`fn pick(pair: (Int!Str, Int)) Int {`,
				`  match pair {`,
				`    (ok, _) => 1,`,
				`    _ => 0,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "A nested ok pattern needs a pattern for its value"},
			},
		},
		{
			name: "nested union members need a value pattern",
			input: strings.Join([]string{
				`type Content = Str | Int`,
				`fn pick(pair: (Content, Int)) Int {`,
				`  match pair {`,
				`    (Str, _) => 1,`,
				`    _ => 0,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "A nested union member pattern needs a pattern for its value: Str(_)"},
			},
		},
		{
			name: "some needs a value pattern",
			input: strings.Join([]string{
				`fn pick(pair: (Int?, Int)) Int {`,
				`  match pair {`,
				`    (some, _) => 1,`,
				`    _ => 0,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "A some pattern needs a pattern for the present value"},
			},
		},
		{
			name: "values without patterns can only be bound or ignored",
			input: strings.Join([]string{
				`fn pick(pair: ([Int], Int)) Int {`,
				`  match pair {`,
				`    (list, 0) => list.size(),`,
				`    ([1], _) => 1,`,
				`    _ => 0,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Values of type [Int] can only be matched with '_' or a name"},
			},
		},
	})
}

// Nested patterns and source-order arms on the existing subject kinds.
func TestNestedAndOrderedMatchPatterns(t *testing.T) {
	run(t, []test{
		{
			name: "an ok pattern can test its value",
			input: strings.Join([]string{
				`fn status(result: Int!Str) Str {`,
				`  match result {`,
				`    ok(200) => "ok",`,
				`    err(e) => e,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Incomplete match: missing case for 'ok(_)'"},
			},
		},
		{
			name: "some and none patterns cover a Maybe",
			input: strings.Join([]string{
				`fn pick(value: Int?) Int {`,
				`  match value {`,
				`    some(n) => n,`,
				`    none => 0,`,
				`  }`,
				`}`,
			}, "\n"),
		},
		{
			name: "a legacy Maybe match still reports a missing absent arm",
			input: strings.Join([]string{
				`fn pick(value: Int?) Int {`,
				`  match value {`,
				`    n => n,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Match on a Maybe type must include a wildcard (_) case"},
			},
		},
		{
			name: "a lone catch-all matches every Maybe",
			input: strings.Join([]string{
				`fn pick(value: Int?) Int {`,
				`  match value {`,
				`    _ => 0,`,
				`  }`,
				`}`,
			}, "\n"),
		},
		{
			name: "a literal after an overlapping range is unreachable",
			input: strings.Join([]string{
				`fn pick(n: Int) Str {`,
				`  match n {`,
				`    1..10 => "range",`,
				`    5 => "five",`,
				`    _ => "other",`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Warn, Message: "Unreachable match arm: 5"},
			},
		},
		{
			name: "an arm after a catch-all is unreachable",
			input: strings.Join([]string{
				`enum Light { red, green }`,
				`fn pick(light: Light) Str {`,
				`  match light {`,
				`    _ => "any",`,
				`    Light::red => "red",`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Warn, Message: "Unreachable match arm: Light::red"},
			},
		},
		{
			name: "a repeated Maybe binding arm is unreachable",
			input: strings.Join([]string{
				`fn pick(value: Int?) Int {`,
				`  match value {`,
				`    a => a,`,
				`    b => b,`,
				`    _ => 0,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Warn, Message: "Unreachable match arm: some(b)"},
			},
		},
		{
			name: "a repeated Int literal is unreachable",
			input: strings.Join([]string{
				`fn pick(n: Int) Str {`,
				`  match n {`,
				`    5 => "first",`,
				`    5 => "second",`,
				`    _ => "other",`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Warn, Message: "Unreachable match arm: 5"},
			},
		},
		{
			name: "a Result match accepts a catch-all",
			input: strings.Join([]string{
				`fn pick(result: Int!Str) Int {`,
				`  match result {`,
				`    ok(n) => n,`,
				`    _ => 0,`,
				`  }`,
				`}`,
			}, "\n"),
		},
	})
}

// Names bound by a pattern are definitions the language server can resolve.
func TestMatchPatternBindingsAreIndexed(t *testing.T) {
	source := strings.Join([]string{
		`fn add(left: Int!Str, right: Int!Str) Int {`,
		`  match (left, right) {`,
		`    (ok(first), ok(second)) => first + second,`,
		`    _ => 0,`,
		`  }`,
		`}`,
	}, "\n")
	line := strings.Split(source, "\n")[2]
	spans := checkWithSpans(t, source)
	for _, name := range []string{"first", "second"} {
		def := strings.Index(line, name) + 1
		use := strings.LastIndex(line, name) + 1
		var key any
		for _, rec := range spans.At(parse.Point{Row: 3, Col: use}) {
			if rec.Key != nil {
				key = rec.Key
				break
			}
		}
		if key == nil {
			t.Fatalf("%s use at 3:%d has no identity key", name, use)
		}
		found := false
		for _, rec := range spans.ByKey(key) {
			if rec.IsDef && rec.Loc.Start == (parse.Point{Row: 3, Col: def}) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s has no definition at 3:%d: %#v", name, def, spans.ByKey(key))
		}
	}
}
