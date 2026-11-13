package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestGoTargetTupleValues(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

struct Span {
  bounds: (Int, Int),
  label: Str,
}

trait Shape {
  fn area() Int
}

struct Square {
  side: Int,
}

impl Shape for Square {
  fn area() Int {
    self.side * self.side
  }
}

fn swap(pair: (Int, Str)) (Str, Int) {
  let number, text = pair
  (text, number)
}

fn widest(spans: [Span]) (Int, Str) {
  mut width, label = (0, "")
  for span in spans {
    let start, end = span.bounds
    if end - start > width {
      width = end - start
      label = span.label
    }
  }
  (width, label)
}

fn nested() ((Int, Int), Str) {
  ((1, 2), "nested")
}

fn shapes() (Shape, Int) {
  (Square{side: 3}, 1)
}

fn loud(n: Int) (Int, Int) {
  fmt::Println("loud", n)
  (n, n + 1)
}

fn main() {
  let text, number = swap((1, "a"))
  fmt::Println(text, number)

  let width, label = widest([Span{bounds: (0, 3), label: "x"}, Span{bounds: (2, 9), label: "y"}])
  fmt::Println(width, label)

  let inner, name = nested()
  let one, two = inner
  fmt::Println(one, two, name)

  let shape, count = shapes()
  fmt::Println(shape.area(), count)

  // Each destructuring evaluates its value once, including when every
  // element is discarded.
  let _, _ = loud(1)
  let a, _ = loud(2)
  let a, b = (a + 1, "shadow")
  let b, a = (a, b)
  fmt::Println(a, b)

  mut counter, status = (0, "start")
  let bump = fn() {
    counter = counter + 1
    status = "bumped"
  }
  bump()
  fmt::Println(counter, status)

  mut target = (1, "one")
  let pointer = &mut target
  pointer.* = (2, "two")
  let t1, t2 = target
  fmt::Println(t1, t2)

  let lookup: [Str: (Int, Int)] = ["a": (1, 2)]
  let m1, m2 = lookup.get("a").expect("a")
  let pairs: [(Int, Str)] = [(1, "p"), (2, "q")]
  let p1, p2 = pairs.at(1).expect("second")
  fmt::Println(m1, m2, p1, p2)

  let any: Any = (1, "any")
  fmt::Println(any)
}
`)
	want := "a 1\n7 y\n1 2 nested\n9 1\nloud 1\nloud 2\nshadow 3\n1 bumped\n2 two\n1 2 2 q\n{1 any}\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// A function returning a tuple has one Go result per element, and a fallible
// tuple return adds the trailing error, so tuple-returning Ard functions have
// the signatures Go code expects (ADR 0077).
func TestGoTargetTupleReturnsUseGoMultiResults(t *testing.T) {
	program := lowerSource(t, `
		fn pair() (Int, Str) {
			(1, "a")
		}

		fn fallible() (Int, Str)!Str {
			Result::ok((1, "a"))
		}

		fn find_pair() (Int, Str)? {
			Maybe::new((1, "a"))
		}

		fn nested() ((Int, Int), Str) {
			((1, 2), "a")
		}

		fn main() {
			let a, b = pair()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	tests := []struct {
		function string
		results  int
	}{
		{function: "Pair", results: 2},
		{function: "Fallible", results: 3},
		// A nullable tuple keeps Maybe's (value, ok) shape; its value is the
		// tuple struct.
		{function: "FindPair", results: 2},
		// Only the returned tuple spreads; a nested tuple is one value.
		{function: "Nested", results: 2},
	}
	for _, tt := range tests {
		function := findGeneratedFunction(t, files, tt.function)
		got := 0
		if function.Type.Results != nil {
			got = len(function.Type.Results.List)
		}
		if got != tt.results {
			t.Errorf("generated %s has %d Go results, want %d", tt.function, got, tt.results)
		}
	}
}

func TestGoTargetTupleReturnPaths(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

enum Problem {
  missing,
  broken,
}

struct Counter {
  count: Int,
}

impl Counter {
  fn split() (Int, Int) {
    (self.count / 2, self.count - self.count / 2)
  }
}

trait Bounded {
  fn bounds() (Int, Int)
}

struct Box {
  w: Int,
  h: Int,
}

impl Bounded for Box {
  fn bounds() (Int, Int) {
    (self.w, self.h)
  }
}

fn id(value: $T) $T {
  value
}

fn via_var() (Int, Str) {
  let pair = (1, "var")
  pair
}

fn via_call() (Int, Str) {
  via_var()
}

fn via_generic() (Int, Str) {
  id((2, "generic"))
}

fn via_if(flag: Bool) (Int, Str) {
  if flag {
    (3, "yes")
  } else {
    via_var()
  }
}

fn via_match(n: Int) (Int, Str) {
  match n {
    0 => (0, "zero"),
    _ => (n, "other"),
  }
}

fn load(ok: Bool) (Int, Str)!Str {
  match ok {
    true => Result::ok((7, "loaded")),
    false => Result::err("nope"),
  }
}

fn load_twice(ok: Bool) (Int, Str)!Str {
  let n, s = try load(ok)
  Result::ok((n * 2, s))
}

fn load_tail(ok: Bool) (Int, Str)!Str {
  load(ok)
}

fn load_or(ok: Bool) (Int, Str) {
  let pair = try load(ok) -> _ {
    (0 - 1, "caught")
  }
  pair
}

fn find(ok: Bool) (Int, Str)? {
  match ok {
    true => Maybe::new((9, "found")),
    false => Maybe::new(),
  }
}

fn first_found(ok: Bool) Int? {
  let n, _ = try find(ok)
  Maybe::new(n)
}

fn custom(ok: Bool) (Int, Str)!Problem {
  match ok {
    true => Result::ok((1, "custom")),
    false => Result::err(Problem::broken),
  }
}

fn risky(n: Int) (Int, Str) {
  if n > 1 {
    panic("too big")
  }
  (n, "fine")
}

fn area(shape: Bounded) Int {
  let w, h = shape.bounds()
  w * h
}

fn main() {
  let counter = Counter{count: 7}
  let lo, hi = counter.split()
  fmt::Println(lo, hi, area(Box{w: 3, h: 4}))

  let n1, s1 = via_var()
  let n2, s2 = via_call()
  let n3, s3 = via_generic()
  let n4, s4 = via_if(true)
  let n5, s5 = via_if(false)
  let n6, s6 = via_match(4)
  fmt::Println(n1, s1, n2, s2, n3, s3, n4, s4, n5, s5, n6, s6)

  let l1, l2 = load_twice(true).expect("ok")
  fmt::Println(l1, l2, load_twice(false).is_err(), load_tail(true).is_ok())
  let c1, c2 = load_or(false)
  fmt::Println(c1, c2)

  let f1, f2 = find(true).expect("found")
  let d1, d2 = find(false).or((0, "default"))
  fmt::Println(f1, f2, d1, d2, first_found(true).or(0), first_found(false).is_none())

  let e1, e2 = custom(true).expect("custom")
  fmt::Println(e1, e2, custom(false).is_err())

  let u1, u2 = unsafe { risky(1) }.expect("safe")
  let failed = unsafe { risky(2) }
  fmt::Println(u1, u2, failed.is_err())
}
`)
	want := "3 4 12\n" +
		"1 var 1 var 2 generic 3 yes 1 var 4 other\n" +
		"14 loaded true true\n" +
		"-1 caught\n" +
		"9 found 0 default 9 true\n" +
		"1 custom true\n" +
		"1 fine true\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestGoTargetTupleCallbacksAndGenerics(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use ard/async
use ard/list
use go:fmt

struct Holder {
  item: $T,
  pair: ($T, Int),
}

impl Holder {
  fn swap() (Int, $T) {
    let item, n = self.pair
    (n, item)
  }

  fn map_pair(f: fn($T) ($T, $T)) ($T, $T) {
    f(self.item)
  }
}

fn pair_of(a: $A, b: $B) ($A, $B) {
  (a, b)
}

fn first(pair: ($A, $B)) $A {
  let a, _ = pair
  a
}

fn apply(f: fn(Int) (Int, Int), n: Int) (Int, Int) {
  f(n)
}

fn call(f: fn() $T) $T {
  f()
}

fn retry(f: fn() $T!Str) $T!Str {
  f()
}

fn funcs() (fn(Int) Int, fn() (Int, Int)) {
  (fn(n: Int) Int { n + 1 }, fn() (Int, Int) { (8, 9) })
}

fn label(n: Int) (Int, Str) {
  (n, "{n}")
}

fn fallible() (Int, Str)!Str {
  Result::ok((5, "five"))
}

fn main() {
  let made = pair_of("x", true)
  let x, y = made
  fmt::Println(x, y, first(made))

  let double = fn(n: Int) (Int, Int) { (n, n * 2) }
  let d1, d2 = double(4)
  let d3, d4 = apply(double, 5)
  let d5, d6 = apply(fn(n: Int) (Int, Int) { (n + 1, n + 2) }, 1)
  fmt::Println(d1, d2, d3, d4, d5, d6)

  let inc, pair = funcs()
  let p1, p2 = pair()
  fmt::Println(inc(1), p1, p2)

  // A tuple-returning function passed where the declaration returns a bare
  // generic adapts to the one-value Go signature of that declaration.
  let pairs = list::map([1, 2, 3], fn(n: Int) (Int, Str) { (n, "{n}") })
  let labels = list::map([4], label)
  let l1, l2 = labels.at(0).expect("label")
  let c1, c2 = call(fn() (Int, Str) { (6, "six") })
  let r1, r2 = retry(fallible).expect("retry")
  let r3, r4 = call(fallible).expect("call")
  fmt::Println(pairs.size(), l1, l2, c1, c2, r1, r2, r3, r4)

  let holder = Holder{item: "h", pair: ("p", 4)}
  let n, item = holder.swap()
  let h1, h2 = holder.map_pair(fn(s: Str) (Str, Str) { (s, s + s) })
  fmt::Println(n, item, h1, h2)

  let found: (Int, Str)? = Maybe::new((9, "found"))
  let ms, mn = found.map(fn(pair: (Int, Str)) (Str, Int) {
    let n, s = pair
    (s, n)
  }).or(("none", 0))
  let loaded: (Int, Str)!Str = Result::ok((1, "r"))
  let rs, rn = loaded.map(fn(pair: (Int, Str)) (Str, Int) {
    let n, s = pair
    (s, n)
  }).expect("mapped")
  fmt::Println(ms, mn, rs, rn)

  let results = Chan::new<(Int, Str)>()
  async::start(fn() {
    results.send((42, "async"))
  })
  let a1, a2 = results.recv().expect("result")
  fmt::Println(a1, a2)
}
`)
	want := "x true x\n" +
		"4 8 5 10 2 3\n" +
		"2 8 9\n" +
		"3 4 4 6 six 5 five 5 five\n" +
		"4 p h hh\n" +
		"found 9 r 1\n" +
		"42 async\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// Function types return their ABI results (ADR 0038), while a generic
// declaration returning a bare type parameter returns one value. These cases
// cross that boundary with Result and Maybe returns.
func TestGoTargetABIReturnsCrossGenericBoundaries(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use ard/list
use go:fmt

fn id(value: $T) $T {
  value
}

fn call(f: fn() $T) $T {
  f()
}

fn parse() Int!Str {
  Result::ok(4)
}

fn find() Int? {
  Maybe::new(3)
}

fn tail_result() Int!Str {
  id(parse())
}

fn tail_maybe() Int? {
  id(find())
}

fn main() {
  let parsed = call(parse)
  let found = call(find)
  let failed = call(fn() Int!Str { Result::err("bad") })
  fmt::Println(parsed.or(0), found.or(0), failed.or(9))

  let maybes = list::map([1, 2], fn(n: Int) Int? { Maybe::new(n) })
  fmt::Println(maybes.size(), maybes.at(1).expect("second").or(0))

  fmt::Println(tail_result().or(0), tail_maybe().or(0))

  let some: Int? = Maybe::new(1)
  let nested = some.map(fn(n: Int) Int? { Maybe::new(n + 1) })
  let ok: Int!Str = Result::ok(1)
  let mapped = ok.map(fn(n: Int) Int!Str { Result::ok(n + 2) })
  fmt::Println(nested.expect("outer").or(0), mapped.expect("outer").or(0))
}
`)
	want := "4 3 9\n2 2\n4 3\n2 3\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestGoTargetGoMultiResultCalls(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:context
use go:fmt
use go:math
use go:os
use go:strings
use go:time

fn split(value: Float64) (Float64, Float64) {
  math::Modf(value)
}

fn read_first(text: Str) (Int32, Int)!Error {
  strings::NewReader(text).ReadRune()
}

fn pipe_roundtrip() Int!Error {
  let reader, writer = try os::Pipe()
  try writer.WriteString("ping")
  try writer.Close()
  mut buffer: [Byte] = [0, 0, 0, 0]
  let n = try reader.Read(&mut buffer)
  try reader.Close()
  Result::ok(n)
}

fn main() {
  let ctx, cancel = context::WithCancel(context::Background())
  cancel()
  fmt::Println(ctx.Err().is_err())

  let whole, fraction = split(1.5)
  fmt::Println(whole, fraction)

  let before, after, found = strings::Cut("key=value", "=")
  fmt::Println(before, after, found)

  let rune, size = read_first("héllo").expect("rune")
  fmt::Println(rune, size, read_first("").is_err())

  let date = time::Date(2024, time::March, 9, 0, 0, 0, 0, time::UTC)
  let year, month, day = date.Date()
  fmt::Println(year, month, day)

  let modf: fn(Float64) (Float64, Float64) = math::Modf
  let w2, f2 = modf(2.25)
  let clock = date.Clock
  let h, m, s = clock()
  fmt::Println(w2, f2, h, m, s)

  fmt::Println(pipe_roundtrip().or(0))
}
`)
	want := "true\n1 0.5\nkey value true\n104 1 true\n2024 March 9\n2 0.25 0 0 0\n4\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// Tuple-returning Ard closures, functions, and methods have the Go signatures
// multi-result Go callbacks and interface methods require, so they cross the
// boundary without adapters (ADR 0077).
func TestGoTargetTuplesCrossGoCallbacksAndInterfaces(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"tupleffi\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module tupleffi\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureDir := filepath.Join(projectDir, "ffi", "shapes")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "shapes.go"), []byte(`package shapes

import (
	"errors"
	"strconv"
)

type Splitter func(n int) (int, int)

type RuneSource interface {
	ReadRune() (rune, int, error)
}

type Bounded interface {
	Bounds() (int, int)
}

func RunTuple(cb func(n int) (string, int)) string {
	s, n := cb(3)
	return s + ":" + strconv.Itoa(n)
}

func RunTupleErr(cb func(n int) (string, int, error)) string {
	s, n, err := cb(4)
	if err != nil {
		return "err:" + err.Error()
	}
	return s + ":" + strconv.Itoa(n)
}

func RunTupleOk(cb func(n int) (string, int, bool)) bool {
	_, _, ok := cb(5)
	return ok
}

func RunNamed(cb Splitter) int {
	a, b := cb(10)
	return a*100 + b
}

func MakeSplitter() Splitter {
	return func(n int) (int, int) { return n / 3, n % 3 }
}

func MakePair() func() (string, int) {
	return func() (string, int) { return "made", 7 }
}

func FirstRune(src RuneSource) (rune, int, error) { return src.ReadRune() }

func Area(b Bounded) int {
	w, h := b.Bounds()
	return w * h
}

func Pair[A, B any](a A, b B) (A, B) { return a, b }

func Triple[A any](a A, fail bool) (A, A, error) {
	if fail {
		var zero A
		return zero, zero, errors.New("triple failed")
	}
	return a, a, nil
}

func Echo[T any](value T) T { return value }

func MapPair[A, B any](a A, f func(A) (B, B)) (B, B) { return f(a) }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:tupleffi/ffi/shapes

struct Text {
  value: Str,
}

impl shapes::RuneSource for Text {
  fn read_rune() (Int32, Int)!Error {
    match self.value.size() == 0 {
      true => Result::err(Error::new("empty")),
      false => Result::ok((65, self.value.size())),
    }
  }
}

struct Rect {
  w: Int,
  h: Int,
}

impl shapes::Bounded for Rect {
  fn bounds() (Int, Int) {
    (self.w, self.h)
  }
}

fn label(n: Int) (Str, Int) {
  ("label", n)
}

fn expect(actual: Str, want: Str) {
  if actual != want {
    panic("got {actual}, want {want}")
  }
}

fn main() {
  expect(shapes::RunTuple(fn(n: Int) (Str, Int) { ("cb", n) }), "cb:3")
  expect(shapes::RunTuple(label), "label:3")
  expect(shapes::RunTupleErr(fn(n: Int) (Str, Int)!Error { Result::ok(("ok", n)) }), "ok:4")
  expect(shapes::RunTupleErr(fn(n: Int) (Str, Int)!Error { Result::err(Error::new("boom")) }), "err:boom")
  expect("{shapes::RunTupleOk(fn(n: Int) (Str, Int, Bool) { ("x", n, true) })}", "true")
  expect("{shapes::RunNamed(fn(n: Int) (Int, Int) { (n, n + 1) })}", "1011")

  let splitter = shapes::MakeSplitter()
  let q, r = splitter(10)
  let make = shapes::MakePair()
  let s, n = make()
  expect("{q} {r} {s} {n}", "3 1 made 7")

  let a, b = shapes::Pair("left", 2)
  let t1, t2 = shapes::Triple(9, false).expect("triple")
  expect("{a} {b} {t1} {t2} {shapes::Triple(9, true).is_err()}", "left 2 9 9 true")

  let e1, e2 = shapes::Echo((1, "echo"))
  let m1, m2 = shapes::MapPair<Int, Str>(4, fn(n: Int) (Str, Str) { ("{n}", "{n + 1}") })
  expect("{e1} {e2} {m1} {m2}", "1 echo 4 5")

  expect("{shapes::Area(Rect{w: 3, h: 5})}", "15")
  let rune, size = shapes::FirstRune(Text{value: "abc"}).expect("rune")
  expect("{rune} {size} {shapes::FirstRune(Text{value: ""}).is_err()}", "65 3 true")
  let source: shapes::RuneSource = Text{value: "hello"}
  let r2, s2 = source.ReadRune().expect("rune")
  let bounded: shapes::Bounded = Rect{w: 2, h: 2}
  let bw, bh = bounded.Bounds()
  expect("{r2} {s2} {bw} {bh}", "65 5 2 2")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

// Elements are evaluated left to right even when a later element lowers to
// setup statements (a multi-result call that is packed into a tuple value, or
// a Void element).
func TestGoTargetTupleElementsEvaluateInSourceOrder(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

fn loud(label: Str, value: Int) Int {
  fmt::Println(label)
  value
}

fn pair(label: Str) (Int, Int) {
  fmt::Println(label)
  (2, 3)
}

fn silent(label: Str) {
  fmt::Println(label)
}

fn returned() (Int, (Int, Int)) {
  (loud("return first", 1), pair("return second"))
}

fn main() {
  let literal = (loud("literal first", 1), -1.5, pair("literal second"), loud("literal third", 4))
  let a, ratio, inner, d = literal
  let b, c = inner
  fmt::Println(a, ratio, b, c, d)

  let e, nested = returned()
  let f, g = nested
  fmt::Println(e, f, g)

  let h, _, i = (loud("void first", 5), silent("void second"), loud("void third", 6))
  fmt::Println(h, i)
}
`)
	want := `literal first
literal second
literal third
1 -1.5 2 3 4
return first
return second
1 2 3
void first
void second
void third
5 6
`
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
