package gotarget

import "testing"

// A struct field declared `fn() $T` is a Go function returning one value, while
// a concrete function type returning a Maybe, a Result, or a tuple returns its
// ABI results (ADRs 0038 and 0077). Storing a concrete function in such a
// field, assigning to it, reading it, and calling through it reconcile the two
// layouts (#572).
func TestGoTargetGenericStructFunctionFields(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

struct Holder {
  f: fn() $T,
}

impl Holder {
  fn get() $T {
    self.f()
  }

  fn fun() fn() $T {
    self.f
  }
}

struct Outer {
  inner: Holder<$T>,
}

struct Fallible {
  f: fn() $T!Str,
}

fn hold(f: fn() $T) Holder<$T> {
  Holder{f: f}
}

fn hold_maybe(value: $T) Holder<$T?> {
  Holder<$T?>{f: fn() $T? { Maybe::new(value) }}
}

fn reset(holder: &mut Holder<Int?>) {
  holder.f = fn() Int? { Maybe::new(11) }
}

fn project(holder: Holder<Int?>?) Int? {
  let f = try holder.f
  f()
}

fn main() {
  let maybe = hold(fn() Int? { Maybe::new(1) })
  let got = maybe.f()
  let read = maybe.f
  fmt::Println(got.or(0), read().or(0), maybe.get().or(0), maybe.fun()().or(0))
  let empty = Holder<Int?>{f: fn() Int? { Maybe::new() }}
  fmt::Println(empty.f().or(9), empty.f().is_none())

  mut result = Holder<Int!Str>{f: fn() Int!Str { Result::ok(2) }}
  fmt::Println(result.f().or(0))
  result.f = fn() Int!Str { Result::err("bad") }
  let failed = result.f
  fmt::Println(result.f().or(9), failed().is_err(), result.get().or(8))

  let tuple = hold(fn() (Int, Int) { (3, 4) })
  let a, b = tuple.f()
  let c, d = tuple.get()
  fmt::Println(a, b, c, d)
  mut pair = Holder<(Int, Str)>{f: fn() (Int, Str) { (5, "five") }}
  pair.f = fn() (Int, Str) { (6, "six") }
  let read_pair = pair.f
  let n, s = read_pair()
  fmt::Println(n, s)

  let fallible = Fallible<(Int, Int)>{f: fn() (Int, Int)!Str { Result::ok((7, 8)) }}
  let e, f = fallible.f().expect("pair")
  fmt::Println(e, f)

  let outer = Outer<Int?>{inner: Holder<Int?>{f: fn() Int? { Maybe::new(9) }}}
  fmt::Println(outer.inner.f().or(0), hold_maybe(10).f().or(0))

  mut target = hold(fn() Int? { Maybe::new(0) })
  reset(&mut target)
  let present: Holder<Int?>? = Maybe::new(target)
  match present.f {
    f => fmt::Println(f().or(0), project(present).or(0)),
    _ => fmt::Println("none"),
  }
}
`)
	want := "1 1 1 1\n" +
		"9 true\n" +
		"2\n" +
		"9 true 8\n" +
		"3 4 3 4\n" +
		"6 six\n" +
		"7 8\n" +
		"9 10\n" +
		"11 11\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// A generic function returning a function typed with a type parameter, or a
// Maybe, a Result, or a tuple holding one, hands back Go functions laid out by
// that declaration. Callers see them with the concrete layout (#572).
func TestGoTargetGenericFunctionReturnsFunctions(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

fn later(value: $T) fn() $T {
  fn() $T { value }
}

fn maybe_later(value: $T) (fn() $T)? {
  Maybe::new(fn() $T { value })
}

fn result_later(value: $T) (fn() $T)!Str {
  Result::ok(fn() $T { value })
}

fn pair_later(value: $T) (fn() $T, Int) {
  (fn() $T { value }, 7)
}

fn result_pair(value: $T) (fn(Int) $T, Int)!Str {
  Result::ok((fn(n: Int) $T { value }, 8))
}

fn maybe_with_arg(value: $T) (fn(Int) $T)? {
  Maybe::new(fn(n: Int) $T { value })
}

fn tried(value: $T) (fn() $T)!Str {
  let f = try result_later(value)
  Result::ok(f)
}

fn main() {
  let one: Int? = Maybe::new(1)
  let two: Int!Str = Result::ok(2)
  let three: (Int, Int) = (3, 4)

  let f1 = later(one)
  let f2 = later(two)
  let f3 = later(three)
  let a, b = f3()
  fmt::Println(f1().or(0), f2().or(0), a, b, later(one)().or(0))

  let m = maybe_later(two).expect("some")
  let r = result_later(one).expect("ok")
  let p, n = pair_later(two)
  let t1, t2 = result_later(three).expect("ok")()
  fmt::Println(m().or(0), r().or(0), p().or(0), n, t1, t2)

  let tf = tried(one).expect("ok")
  let rp, rn = result_pair(two).expect("ok")
  let wa = maybe_with_arg(one).expect("some")
  fmt::Println(tf().or(0), rp(0).or(0), rn, wa(0).or(0))
}
`)
	want := "1 2 3 4 1\n" +
		"2 1 2 7 3 4\n" +
		"1 2 8 1\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// Function types nested inside a type-parameter-typed function type (a
// parameter that is itself a function, or a function returning a function)
// are adapted at every level, in both directions (#572).
func TestGoTargetGenericNestedFunctionTypes(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

struct Holder {
  f: fn() $T,
}

fn apply(g: fn(fn() $T) Int, f: fn() $T) Int {
  g(f)
}

fn call_twice(f: fn() fn() $T) $T {
  f()()
}

fn first_of(f: fn() (fn() $T, Int)!Str) $T {
  let g, n = f().expect("ok")
  g()
}

fn consumer(witness: $T) fn(fn() $T) $T {
  fn(f: fn() $T) $T { f() }
}

fn main() {
  let n = apply(fn(f: fn() Int?) Int { f().or(0) }, fn() Int? { Maybe::new(1) })
  let m = call_twice(fn() fn() Int!Str { fn() Int!Str { Result::ok(2) } })
  let x, y = call_twice(fn() fn() (Int, Int) { fn() (Int, Int) { (3, 4) } })
  let witness: Int? = Maybe::new()
  let consume = consumer(witness)
  let z = consume(fn() Int? { Maybe::new(5) })
  let nested = Holder<fn() Int!Str>{f: fn() fn() Int!Str { fn() Int!Str { Result::ok(6) } }}
  let w = first_of(fn() (fn() Int?, Int)!Str { Result::ok((fn() Int? { Maybe::new(7) }, 1)) })
  fmt::Println(n, m.or(0), x, y, z.or(0), nested.f()().or(0), w.or(0))
}
`)
	want := "1 2 3 4 5 6 7\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// A type parameter instantiated with Void is one Go value in a generic
// declaration, while a concrete function returning Void has no Go results.
func TestGoTargetGenericFunctionSlotsReturningVoid(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

struct Holder {
  f: fn() $T,
}

fn call(f: fn() $T) $T {
  f()
}

fn main() {
  call(fn() Void { fmt::Println("called") })
  let holder = Holder<Void>{f: fn() Void { fmt::Println("held") }}
  holder.f()
  let read = holder.f
  read()
}
`)
	want := "called\nheld\nheld\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// A list or map of `fn() $T` built in generic code keeps the Go type generic
// code gave it. Concrete code can hold it, measure it, and hand it back to
// generic code, as long as it does not read or write the elements itself,
// which the checker rejects (#588).
func TestGoTargetGenericBuiltFunctionCollectionsPassThroughConcreteCode(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

struct Registry {
  handlers: [fn() $T],
}

impl Registry {
  fn run() [$T] {
    mut out: [$T] = []
    let sink = &mut out
    for handler in self.handlers {
      sink.push(handler())
    }
    out
  }

  fn all() [fn() $T] {
    self.handlers
  }
}

fn register(f: fn() $T, g: fn() $T) Registry<$T> {
  Registry{handlers: [f, g]}
}

fn first(fs: [fn() $T]) $T {
  fs.at(0).expect("empty")()
}

fn named(f: fn() $T) [Str: fn() $T] {
  ["only": f]
}

fn call_named(fs: [Str: fn() $T], name: Str) $T {
  fs.get(name).expect("missing")()
}

fn main() {
  let registry = register(fn() Int? { Maybe::new(4) }, fn() Int? { Maybe::new() })
  fmt::Println(registry.handlers.size(), first(registry.handlers).or(0))

  let handlers = registry.handlers
  let annotated: [fn() Int?] = registry.all()
  fmt::Println(first(handlers).or(0), first(annotated).or(0), handlers.size())

  let copy = Registry<Int?>{handlers: registry.handlers}
  for value in copy.run() {
    fmt::Println(value.or(-1))
  }

  mut count = 0
  for handler in registry.handlers {
    count = count + 1
  }
  fmt::Println(count)

  let by_name = named(fn() Int!Str { Result::ok(7) })
  mut names: [Str] = []
  for name, handler in by_name {
    names = [name]
  }
  fmt::Println(by_name.size(), names.at(0).or(""), call_named(by_name, "only").or(0))
}
`)
	want := "2 4\n4 4 2\n4\n-1\n2\n1 only 7\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
