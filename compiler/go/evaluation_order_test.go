package gotarget

import "testing"

// The tests in this file print from every operand of a composite expression.
// An operand written with `later` lowers to setup statements (the call returns
// a Maybe, which is unpacked from Go's `(T, bool)` results into a temporary),
// and those statements must not run before the operands written to its left.

const evaluationOrderHelpers = `
fn now(label: Str, value: Int) Int {
  fmt::Println(label)
  value
}

fn now_str(label: Str, value: Str) Str {
  fmt::Println(label)
  value
}

fn later(label: Str, value: Int) Int? {
  fmt::Println(label)
  Maybe::new(value)
}

fn later_str(label: Str, value: Str) Str? {
  fmt::Println(label)
  Maybe::new(value)
}
`

func TestGoTargetAggregateOperandsEvaluateInSourceOrder(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
use go:net/http
`+evaluationOrderHelpers+`
fn later_float(label: Str) Float64? {
  fmt::Println(label)
  Maybe::new(2.5)
}

fn silent(label: Str) {
  fmt::Println(label)
}

struct Trio {
  a: Int,
  b: Int?,
  c: Int,
}

fn main() {
  let list = [now("list a", 1), later("list b", 2).or(0), now("list c", 3)]
  let fixed: [Int; 3] = [now("fixed a", 1), later("fixed b", 2).or(0), now("fixed c", 3)]
  let trio = Trio{a: now("struct a", 1), b: later("struct b", 2), c: now("struct c", 3)}
  let keyed = [(now("map key", 1)): later("map value", 2).or(0)]
  let entries = [1: now("map entry a", 1), 2: later("map entry b", 2).or(0)]
  let ratios = [-1.5, later_float("constant b").or(0.0)]
  let voids = [silent("void a"), silent("void b")]
  let request = http::Request{Method: now_str("foreign struct a", "GET"), Proto: later_str("foreign struct b", "HTTP/1.1").or("")}
  fmt::Println(list, fixed, trio.a, trio.b.or(0), trio.c, keyed, entries, ratios, voids.size(), request.Method, request.Proto)
}
`)
	want := `list a
list b
list c
fixed a
fixed b
fixed c
struct a
struct b
struct c
map key
map value
map entry a
map entry b
constant b
void a
void b
foreign struct a
foreign struct b
[1 2 3] [1 2 3] 1 2 3 map[1:2] map[1:1 2:2] [-1.5 2.5] 2 GET HTTP/1.1
`
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestGoTargetCallOperandsEvaluateInSourceOrder(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
use go:strings
`+evaluationOrderHelpers+`
fn silent(label: Str) {
  fmt::Println(label)
}

fn take(a: Int, b: Int?, c: Int) Int {
  a + b.or(0) + c
}

fn take_void(a: Int, b: Void) Int {
  a
}

fn apply(a: Int, f: fn(Int) $T) $T {
  f(a)
}

fn half(label: Str) fn(Int) Int? {
  fmt::Println(label)
  fn(n: Int) Int? { Maybe::new(n / 2) }
}

struct Counter {
  base: Int,
}

impl Counter {
  fn add(n: Int) Int {
    self.base + n
  }
}

trait Adder {
  fn add_to(n: Int) Int
}

impl Adder for Counter {
  fn add_to(n: Int) Int {
    self.base + n
  }
}

fn counter(label: Str) Counter {
  fmt::Println(label)
  Counter{base: 10}
}

fn adder(label: Str) Adder {
  fmt::Println(label)
  Counter{base: 20}
}

fn closure(label: Str) fn(Int) Int {
  fmt::Println(label)
  fn(n: Int) Int { n + 30 }
}

fn replacer(label: Str) &mut strings::Replacer {
  fmt::Println(label)
  strings::NewReplacer("a", "b")
}

fn main() {
  let sum = take(now("call a", 1), later("call b", 2), now("call c", 3))
  let voided = take_void(now("void a", 1), silent("void b"))
  let applied = apply(now("adapted a", 8), half("adapted b")).or(0)
  let method = counter("method receiver").add(later("method arg", 1).or(0))
  let traited = adder("trait receiver").add_to(later("trait arg", 2).or(0))
  let called = closure("closure callee")(later("closure arg", 3).or(0))
  let pair = fn(a: Int, b: Int) Int { a + b }
  let paired = pair(now("closure args a", 1), later("closure args b", 2).or(0))
  let foreign = fmt::Sprint(now("foreign a", 1), later("foreign b", 2).or(0))
  let replaced = replacer("foreign receiver").Replace(later_str("foreign arg", "abc").or(""))
  fmt::Println(sum, voided, applied, method, traited, called, paired, foreign, replaced)
}
`)
	want := `call a
call b
call c
void a
void b
adapted a
adapted b
method receiver
method arg
trait receiver
trait arg
closure callee
closure arg
closure args a
closure args b
foreign a
foreign b
foreign receiver
foreign arg
6 1 4 11 22 33 3 1 2 bbc
`
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestGoTargetOperatorOperandsEvaluateInSourceOrder(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
`+evaluationOrderHelpers+`
fn main() {
  let sum = now("add left", 1) + later("add right", 2).or(0)
  let product = now("multiply left", 3) * later("multiply right", 4).or(0)
  let less = now("less left", 1) < later("less right", 2).or(0)
  let equal = now("equal left", 1) == later("equal right", 2).or(0)
  let joined = now_str("concat left", "a") + later_str("concat right", "b").or("")
  let interpolated = "{now("interpolate a", 1)}-{later("interpolate b", 2).or(0)}"
  fmt::Println(sum, product, less, equal, joined, interpolated)
}
`)
	want := `add left
add right
multiply left
multiply right
less left
less right
equal left
equal right
concat left
concat right
interpolate a
interpolate b
3 12 true false ab 1-2
`
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestGoTargetBuiltinMethodOperandsEvaluateInSourceOrder(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
`+evaluationOrderHelpers+`
fn list(label: Str, items: &mut [Int]) &mut [Int] {
  fmt::Println(label)
  items
}

fn dict(label: Str, entries: &mut [Int: Int]) &mut [Int: Int] {
  fmt::Println(label)
  entries
}

fn channel(label: Str, ch: Chan<Int>) Chan<Int> {
  fmt::Println(label)
  ch
}

fn main() {
  let contains = now_str("contains target", "abc").contains(later_str("contains arg", "b").or(""))
  let starts = now_str("starts target", "abc").starts_with(later_str("starts arg", "a").or(""))
  let ends = now_str("ends target", "abc").ends_with(later_str("ends arg", "c").or(""))
  let replaced = now_str("replace target", "abc").replace(now_str("replace from", "b"), later_str("replace to", "x").or(""))
  let replaced_all = now_str("replace_all target", "abb").replace_all(now_str("replace_all from", "b"), later_str("replace_all to", "y").or(""))
  let items = &mut [1, 2, 3]
  let at = list("list at target", items).at(later("list at index", 1).or(0)).or(0)
  list("swap target", items).swap(now("swap left", 0), later("swap right", 2).or(0))
  let entries = &mut [1: 10, 2: 20]
  let got = dict("map get target", entries).get(later("map get key", 1).or(0)).or(0)
  let has = dict("map has target", entries).has(later("map has key", 2).or(0))
  dict("map set target", entries).set(now("map set key", 3), later("map set value", 30).or(0))
  dict("map delete target", entries).delete(later("map delete key", 2).or(0))
  let expected = Maybe::new(now("expect target", 7)).expect(later_str("expect message", "missing").or(""))
  let ch = Chan::new<Int>(1)
  channel("send channel", ch).send(later("send value", 9).or(0))
  let empty = Chan::new<Int>(1)
  let buffered = Chan::new<Int>(1)
  let selected = select {
    channel("select recv channel", empty).recv() => 0,
    channel("select send channel", buffered).send(later("select send value", 4).or(0)) => buffered.recv().or(0),
  }
  fmt::Println(contains, starts, ends, replaced, replaced_all, at, items, got, has, entries, expected, ch.recv().or(0), selected)
}
`)
	want := `contains target
contains arg
starts target
starts arg
ends target
ends arg
replace target
replace from
replace to
replace_all target
replace_all from
replace_all to
list at target
list at index
swap target
swap left
swap right
map get target
map get key
map has target
map has key
map set target
map set key
map set value
map delete target
map delete key
expect target
expect message
send channel
send value
select recv channel
select send channel
select send value
true true true axc ayy 2 &[3 2 1] 10 true &map[1:10 3:30] 7 9 4
`
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// Assignment and mutation targets are places: the storage they designate is
// fixed before the value is evaluated, and they are evaluated once.
func TestGoTargetAssignmentTargetsEvaluateBeforeValues(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
use go:net/http
`+evaluationOrderHelpers+`
struct Point {
  x: Int,
  y: Int,
}

struct Holder {
  point: Point,
  items: [Int],
  label: Str?,
}

fn point(label: Str, p: &mut Point) &mut Point {
  fmt::Println(label)
  p
}

fn holder(label: Str, h: &mut Holder) &mut Holder {
  fmt::Println(label)
  h
}

fn number(label: Str, n: &mut Int) &mut Int {
  fmt::Println(label)
  n
}

fn request(label: Str, r: &mut http::Request) &mut http::Request {
  fmt::Println(label)
  r
}

fn main() {
  let p = &mut Point{x: 1, y: 2}
  point("field target", p).x = later("field value", 10).or(0)
  let h = &mut Holder{point: Point{x: 0, y: 0}, items: [1], label: Maybe::new()}
  holder("nested field target", h).point.y = later("nested field value", 20).or(0)
  holder("push target", h).items.push(later("push value", 2).or(0))
  holder("maybe set target", h).label.set(later_str("maybe set value", "set").or(""))
  mut n = 1
  number("pointee target", &mut n).* = later("pointee value", 5).or(0)
  let req = &mut http::Request{Method: "GET"}
  request("foreign field target", req).Method = later_str("foreign field value", "POST").or("")
  fmt::Println(p.x, h.point.y, h.items, h.label.or(""), n, req.Method)
}
`)
	want := `field target
field value
nested field target
nested field value
push target
push value
maybe set target
maybe set value
pointee target
pointee value
foreign field target
foreign field value
10 20 [1 2] set 5 POST
`
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
