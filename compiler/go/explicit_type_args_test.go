package gotarget

import "testing"

// An explicit or contextual type argument fixes the type of a generic call's
// parameter. A union member passed there is converted to the union, and the
// call's result keeps the requested type rather than the argument's own type.
// Likewise Maybe.or and Result.or with a member fallback yield the union.
func TestGoTargetExplicitTypeArgsAcceptUnionMembers(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

struct Shutdown {}
struct Tick { n: Int }
type Event = Shutdown | Tick

struct Holder<$T> {
  item: $T,
}

impl Holder {
  fn with(next: $T) Holder<$T> { Holder{item: next} }
}

fn id(value: $T) $T { value }
fn pair(first: $T, second: $T) [$T] { [first, second] }
fn mixed(event: Event, extra: $T) $T { extra }
fn forward(extra: $U) $U { mixed(Tick{n: 8}, extra) }

fn describe(event: Event) Str {
  match event {
    Shutdown => "shutdown",
    Tick => "tick {it.n}",
  }
}

fn describe_maybe(event: Event?) Str {
  match event {
    e => describe(e),
    _ => "none",
  }
}

fn main() {
  mut explicit = Maybe::new<Event>(Shutdown{})
  fmt::Println("explicit", describe_maybe(explicit))
  explicit = Maybe::new(Tick{n: 1})
  fmt::Println("reassigned", describe_maybe(explicit))

  let contextual: Event? = Maybe::new(Tick{n: 2})
  fmt::Println("contextual", describe_maybe(contextual))
  fmt::Println("argument", describe_maybe(Maybe::new(Shutdown{})))
  fmt::Println("nested", describe_maybe(Maybe::new<Event>(Maybe::new(Tick{n: 3}))))

  let ok = Result::ok<Event, Str>(Shutdown{})
  fmt::Println("ok", describe(ok.or(Tick{n: 0})))
  let err = Result::err<Int, Event>(Tick{n: 4})
  match err {
    ok(value) => fmt::Println("unexpected", value),
    err(event) => fmt::Println("err", describe(event)),
  }

  fmt::Println("id", describe(id<Event>(Shutdown{})))
  fmt::Println("id maybe", describe_maybe(id<Event?>(Tick{n: 5})))

  let first: Event = Tick{n: 6}
  let both = pair(first, Shutdown{})
  for item in both {
    fmt::Println("pair", describe(item))
  }
  fmt::Println("mixed", mixed(Shutdown{}, 7))
  fmt::Println("forward", forward("enclosing"))

  let missing: Event? = Maybe::new()
  fmt::Println("maybe or", describe(missing.or(Tick{n: 9})))

  let holder: Holder<Event> = Holder{item: first}
  fmt::Println("receiver", describe(holder.with(Shutdown{}).item))
}
`)
	want := "explicit shutdown\n" +
		"reassigned tick 1\n" +
		"contextual tick 2\n" +
		"argument shutdown\n" +
		"nested tick 3\n" +
		"ok shutdown\n" +
		"err tick 4\n" +
		"id shutdown\n" +
		"id maybe tick 5\n" +
		"pair tick 6\n" +
		"pair shutdown\n" +
		"mixed 7\n" +
		"forward enclosing\n" +
		"maybe or tick 9\n" +
		"receiver shutdown\n"
	if got != want {
		t.Fatalf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}
