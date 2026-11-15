package gotarget

import (
	"strings"
	"testing"
)

// Match arms run in source order (ADR 0083): the first arm whose pattern
// matches runs, whatever kinds of pattern the arms use.
func TestGoTargetMatchArmsRunInSourceOrder(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

enum Light { red, yellow, green }

fn main() {
  let n = 5
  let overlap = match n {
    1..10 => "range",
    5 => "literal",
    _ => "other",
  }
  let reversed = match n {
    5 => "literal",
    1..10 => "range",
    _ => "other",
  }
  let ranges = match n {
    0..9 => "first",
    5..20 => "second",
    _ => "other",
  }
  let light = Light::yellow
  let catchall = match light {
    Light::red => "stop",
    _ => "slow",
    Light::yellow => "unreachable",
  }
  fmt::Println(overlap, reversed, ranges, catchall)
}
`)
	if want := "range literal first slow\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// A match evaluates its subject exactly once, whatever the subject's kind and
// however many arms test it.
func TestGoTargetMatchEvaluatesSubjectOnce(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

type Content = Str | Int

mut calls = 0

fn next() Int {
  calls =+ 1
  7
}

fn content() Content {
  calls =+ 1
  "text"
}

fn pair() (Int, Str) {
  calls =+ 1
  (2, "two")
}

fn main() {
  let a = match next() {
    1 => "one",
    2 => "two",
    3..5 => "few",
    _ => "many",
  }
  fmt::Println(a, calls)
  let b = match content() {
    Str(s) => s,
    Int(i) => i.to_str(),
  }
  fmt::Println(b, calls)
  let c = match pair() {
    (1, _) => "one",
    (2, word) => word,
    _ => "other",
  }
  fmt::Println(c, calls)
}
`)
	want := "many 1\ntext 2\ntwo 3\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// The elements of a tuple literal subject are evaluated once each, left to
// right, before any arm is tested.
func TestGoTargetTupleLiteralSubjectEvaluatesElementsInOrder(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

mut log = ""

fn note(name: Str, value: Int) Int {
  log = log + name
  value
}

fn main() {
  let result = match (note("a", 1), note("b", 2)) {
    (2, _) => "first",
    (1, 2) => "second",
    _ => "other",
  }
  fmt::Println(result, log)
}
`)
	if want := "second ab\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// Tuple patterns match each element with the patterns valid for its type
// (#573).
func TestGoTargetTupleMatch(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

enum Dir { up, down }

type Content = Str | Int

fn fetch(code: Int) (Int, Str) {
  (code, "body {code}")
}

fn add(left: Int!Str, right: Int!Str) Str {
  match (left, right) {
    (ok(a), ok(b)) => (a + b).to_str(),
    (err(e), _) => "left {e}",
    (_, err(e)) => "right {e}",
  }
}

fn handle(code: Int) Str {
  match fetch(code) {
    (200, body) => body,
    (404, _) => "not found",
    (code, _) => "failed {code}",
  }
}

fn same(left: Int?, right: Int?) Bool {
  match (left, right) {
    (some(a), some(b)) => a == b,
    (none, none) => true,
    _ => false,
  }
}

fn classify(dir: Dir, flag: Bool, count: Int) Str {
  match (dir, flag, count) {
    (Dir::up, true, 0) => "up-true-zero",
    (Dir::up, _, 1..9) => "up-small",
    (Dir::down, false, _) => "down-false",
    (_, _, n) => "other {n}",
  }
}

fn nested(value: ((Int, Str), Content)) Str {
  match value {
    ((0, name), Str(text)) => "zero {name} {text}",
    ((n, _), Int(i)) => "int {n} {i}",
    (pair, _) => {
      let first, second = pair
      "pair {first} {second}"
    },
  }
}

fn item(n: Int, s: Str, c: Content) ((Int, Str), Content) {
  ((n, s), c)
}

fn status(result: Int!Str) Str {
  match result {
    ok(200) => "ok",
    ok(code) => "code {code}",
    err("timeout") => "retry",
    err(_) => "failed",
  }
}

fn main() {
  fmt::Println(add(Result::ok(1), Result::ok(2)), add(Result::err("x"), Result::err("y")), add(Result::ok(1), Result::err("z")))
  fmt::Println(handle(200), handle(404), handle(500))
  fmt::Println(same(Maybe::new(1), Maybe::new(1)), same(Maybe::new(1), Maybe::new(2)), same(Maybe::new(), Maybe::new()), same(Maybe::new(1), Maybe::new()))
  fmt::Println(classify(Dir::up, true, 0), classify(Dir::up, true, 3), classify(Dir::down, false, 9), classify(Dir::down, true, 9))
  fmt::Println(nested(item(0, "a", "b")), nested(item(1, "a", 2)), nested(item(3, "c", "d")))
  fmt::Println(status(Result::ok(200)), status(Result::ok(5)), status(Result::err("timeout")), status(Result::err("boom")))
}
`)
	want := strings.Join([]string{
		"3 left x right z",
		"body 200 not found failed 500",
		"true false true false",
		"up-true-zero up-small down-false other 9",
		"zero a b int 1 2 pair 3 c",
		"ok code 5 retry failed",
	}, "\n") + "\n"
	if got != want {
		t.Fatalf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// A tuple literal subject bound whole by an arm is built as a tuple.
func TestGoTargetTupleMatchBindsWholeLiteralSubject(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

fn main() {
  let a = 1
  let b = "b"
  let text = match (a, b) {
    (0, _) => "zero",
    pair => {
      let n, s = pair
      "{n}{s}"
    },
  }
  fmt::Println(text)
}
`)
	if want := "1b\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// The legacy top-level forms keep their meaning: a bare name binds the present
// value of a Maybe and `_` matches none, bare `ok`/`err` bind themselves, and
// a bare union member binds `it`. Qualified `Result::ok(x)` patterns work too.
func TestGoTargetLegacyMatchForms(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

type Content = Str | Int

fn maybe(value: Int?) Str {
  match value {
    _ => "none",
    n => "some {n}",
  }
}

fn result(value: Int!Str) Str {
  match value {
    err => "err {err}",
    ok => "ok {ok}",
  }
}

fn qualified(value: Int!Str) Str {
  match value {
    Result::ok(n) => "ok {n}",
    Result::err(e) => "err {e}",
  }
}

fn content(value: Content) Str {
  match value {
    Str => "str {it}",
    Int => "int {it}",
  }
}

fn explicit(value: Int?) Str {
  match value {
    some(n) => "some {n}",
    none => "none",
  }
}

fn main() {
  fmt::Println(maybe(Maybe::new(1)), maybe(Maybe::new()))
  fmt::Println(result(Result::ok(1)), result(Result::err("e")))
  fmt::Println(qualified(Result::ok(2)), qualified(Result::err("f")))
  fmt::Println(content("s"), content(3))
  fmt::Println(explicit(Maybe::new(4)), explicit(Maybe::new()))
}
`)
	want := strings.Join([]string{
		"some 1 none",
		"ok 1 err e",
		"ok 2 err f",
		"str s int 3",
		"some 4 none",
	}, "\n") + "\n"
	if got != want {
		t.Fatalf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// Arms that end several branches of the decision tree run once, and break
// inside an arm still exits the enclosing loop.
func TestGoTargetMatchSharedArmsAndBreak(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

fn main() {
  mut seen = ""
  for pair in [(true, 1), (false, 2), (true, 3), (true, 9), (false, 4)] {
    match pair {
      (true, 1) => { seen = seen + "a" },
      (_, 9) => break,
      _ => { seen = seen + "." },
    }
  }
  fmt::Println(seen)
}
`)
	if want := "a..\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// Dynamic foreign type tests (ADR 0042) nest inside tuple patterns.
func TestGoTargetTupleMatchWithForeignTypePatterns(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
use go:time

fn describe(value: fmt::Stringer, verbose: Bool) Str {
  match (value, verbose) {
    (time::Duration(d), true) => "duration {d.String()}",
    (time::Duration(_), false) => "duration",
    (time::Month(_), _) => "month",
    _ => "other",
  }
}

fn main() {
  fmt::Println(describe(time::Second, true), describe(time::Second, false), describe(time::January, true), describe(time::Monday, false))
}
`)
	if want := "duration 1s duration month other\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// Pattern bindings may reuse the names of the subject's locals: an arm that
// always matches binds in its own scope, and every binding reads the subject
// before any new name shadows it.
func TestGoTargetMatchBindingsShadowSubjectNames(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

fn swap(a: Int, b: Int) Str {
  match (a, b) {
    (b, a) => "{a} {b}",
  }
}

fn bump(x: Int?) Int {
  match x {
    some(x) => x + 1,
    none => 0,
  }
}

fn main() {
  fmt::Println(swap(1, 2), bump(Maybe::new(4)), bump(Maybe::new()))
}
`)
	if want := "2 1 5 0\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// A local element of a tuple literal subject is read when it is evaluated,
// before later elements run (ADR 0078).
func TestGoTargetTupleSubjectSnapshotsLocalsInOrder(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

fn main() {
  mut x = 1
  let change = fn() Int {
    x = 2
    0
  }
  let result = match (x, change()) {
    (1, 0) => "snapshot",
    (2, 0) => "mutated",
    _ => "other",
  }
  fmt::Println(result, x)
}
`)
	if want := "snapshot 2\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
