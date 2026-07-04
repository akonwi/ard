package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func checkWithSpans(t *testing.T, source string) *checker.SpanIndex {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	resolver, err := checker.NewModuleResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New("test.ard", result.Program, resolver, checker.CheckOptions{RecordSpans: true})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("check errors: %v", c.Diagnostics())
	}
	return c.Spans()
}

func TestSpansRecordLocalDefAndUses(t *testing.T) {
	spans := checkWithSpans(t, `fn main() {
  let count = 1
  let doubled = count + count
}
`)
	// find a use of `count` at 3:17
	uses := spans.At(parse.Point{Row: 3, Col: 17})
	if len(uses) == 0 {
		t.Fatal("no spans at count use position")
	}
	var key any
	for _, rec := range uses {
		if rec.Key != nil {
			key = rec.Key
			break
		}
	}
	if key == nil {
		t.Fatal("count use has no identity key")
	}
	group := spans.ByKey(key)
	defs := 0
	refs := 0
	for _, rec := range group {
		if rec.IsDef {
			defs++
		} else {
			refs++
		}
	}
	if defs != 1 {
		t.Fatalf("expected 1 def for count, got %d", defs)
	}
	if refs != 2 {
		t.Fatalf("expected 2 uses of count, got %d", refs)
	}
}

func TestSpansRecordParameterBindings(t *testing.T) {
	spans := checkWithSpans(t, `fn add(left: Int, right: Int) Int {
  left + right
}
`)
	uses := spans.At(parse.Point{Row: 2, Col: 4})
	var key any
	for _, rec := range uses {
		if rec.Key != nil {
			key = rec.Key
		}
	}
	if key == nil {
		t.Fatal("left use has no identity key")
	}
	def, ok := spans.Def(key)
	if !ok {
		t.Fatal("no def recorded for parameter left")
	}
	if def.Loc.Start.Row != 1 {
		t.Fatalf("param def on row %d, want 1", def.Loc.Start.Row)
	}
}

func TestSpansRecordFunctionDefAndCall(t *testing.T) {
	spans := checkWithSpans(t, `fn helper() Int {
  1
}

fn main() {
  let x = helper()
}
`)
	key := checker.FunctionKey("test.ard", "helper")
	group := spans.ByKey(key)
	var def, use bool
	for _, rec := range group {
		if rec.IsDef {
			def = true
		} else {
			use = true
		}
	}
	if !def {
		t.Fatal("no function def recorded")
	}
	if !use {
		t.Fatal("no function call recorded")
	}
}

func TestSpansRecordExpressionTypes(t *testing.T) {
	spans := checkWithSpans(t, `fn main() {
  let msg = "hello"
}
`)
	recs := spans.At(parse.Point{Row: 2, Col: 14})
	found := false
	for _, rec := range recs {
		if rec.Node != nil && rec.Node.Type() != nil && rec.Node.Type().String() == "Str" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no Str-typed expression span at string literal, got %d recs", len(recs))
	}
}

func TestSpansDisabledByDefault(t *testing.T) {
	result := parse.Parse([]byte("fn main() {}\n"), "test.ard")
	resolver, err := checker.NewModuleResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New("test.ard", result.Program, resolver)
	c.Check()
	if c.Spans().Len() != 0 {
		t.Fatal("spans recorded without RecordSpans option")
	}
}

func TestSpansForLoopCursorBinding(t *testing.T) {
	spans := checkWithSpans(t, `fn main() {
  for item in [1, 2, 3] {
    let x = item
  }
}
`)
	uses := spans.At(parse.Point{Row: 3, Col: 14})
	var key any
	for _, rec := range uses {
		if rec.Key != nil {
			key = rec.Key
		}
	}
	if key == nil {
		t.Fatal("loop cursor use has no key")
	}
	if _, ok := spans.Def(key); !ok {
		t.Fatal("no def recorded for loop cursor")
	}
}

func TestSpansShadowedBindingsHaveDistinctKeys(t *testing.T) {
	spans := checkWithSpans(t, `fn main() {
  let x = 1
  let a = x
  match true {
    true => {
      let x = "inner"
      let b = x
    },
    false => {},
  }
}
`)
	outerUse := keyAt(t, spans, parse.Point{Row: 3, Col: 11})
	innerUse := keyAt(t, spans, parse.Point{Row: 7, Col: 15})
	if outerUse == innerUse {
		t.Fatal("shadowed binding shares key with outer binding")
	}
}

func TestSpansStructLiteralFieldsNotDuplicated(t *testing.T) {
	spans := checkWithSpans(t, `struct Point {
  x: Int,
  y: Int,
}

fn main() {
  let n = 1
  let p = Point{x: n, y: n}
}
`)
	key := keyAt(t, spans, parse.Point{Row: 8, Col: 20})
	group := spans.ByKey(key)
	refs := 0
	for _, rec := range group {
		if !rec.IsDef {
			refs++
		}
	}
	if refs != 2 {
		t.Fatalf("expected exactly 2 uses of n, got %d (speculative checking may be polluting spans)", refs)
	}
}

func TestSpansNamespacedCallsHaveNoLocalKey(t *testing.T) {
	spans := checkWithSpans(t, `fn ok() Void!Str {
  Result::ok(())
}

fn main() {
  let r = ok()
}
`)
	// Result::ok inside fn ok's body must not be grouped with fn ok.
	key := checker.FunctionKey("test.ard", "ok")
	for _, rec := range spans.ByKey(key) {
		if rec.Loc.Start.Row == 2 {
			t.Fatal("namespaced call Result::ok keyed as local fn ok")
		}
	}
}

func keyAt(t *testing.T, spans *checker.SpanIndex, p parse.Point) any {
	t.Helper()
	for _, rec := range spans.At(p) {
		if rec.Key != nil {
			return rec.Key
		}
	}
	t.Fatalf("no keyed span at %v", p)
	return nil
}
