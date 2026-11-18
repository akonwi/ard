package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

const explicitTypeArgsEventPrelude = `struct Shutdown {}
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
fn listy(items: [$T], extra: $T) [$T] { items }
fn take_maybe(event: Event?) Bool { event.is_some() }
fn empty() $T? { Maybe::new() }
`

// Each case re-annotates or reassigns the call's result so the test fails
// unless the call produces exactly the requested type: Maybe, lists, and Result
// are invariant, and a Shutdown variable cannot be reassigned a Tick.
func TestExplicitTypeArgsFixGenericCallTypes(t *testing.T) {
	run(t, []test{
		{
			name: "Maybe::new keeps an explicit union type argument",
			input: explicitTypeArgsEventPrelude + `
let value = Maybe::new<Event>(Shutdown{})
let typed: Event? = value`,
		},
		{
			name: "Maybe::new keeps an expected union type",
			input: explicitTypeArgsEventPrelude + `
let value: Event? = Maybe::new(Shutdown{})
let passed = take_maybe(Maybe::new(Tick{n: 1}))`,
		},
		{
			name: "Maybe::new checks a nested Maybe against the explicit type",
			input: explicitTypeArgsEventPrelude + `
let value = Maybe::new<Event>(Maybe::new(Shutdown{}))
let typed: Event? = value`,
		},
		{
			name: "Maybe::new resolves a nested return-only generic from the explicit type",
			input: explicitTypeArgsEventPrelude + `
let value = Maybe::new<Event>(empty())
let typed: Event? = value
let contextual: Event? = Maybe::new(empty())`,
		},
		{
			name: "Maybe::new without context keeps the argument type",
			input: explicitTypeArgsEventPrelude + `
let value = Maybe::new(Shutdown{})
let typed: Shutdown? = value`,
		},
		{
			name: "Result constructors accept members of explicit union types",
			input: explicitTypeArgsEventPrelude + `
let ok = Result::ok<Event, Str>(Shutdown{})
let ok_typed: Event!Str = ok
let err = Result::err<Int, Event>(Tick{n: 1})
let err_typed: Int!Event = err`,
		},
		{
			name: "user generic functions accept members of explicit union types",
			input: explicitTypeArgsEventPrelude + `
mut value = id<Event>(Shutdown{})
value = Tick{n: 2}
let maybe = id<Event?>(Shutdown{})
let maybe_typed: Event? = maybe
let items = listy<Event>([], Tick{n: 1})
let items_typed: [Event] = items`,
		},
		{
			name: "earlier arguments, receivers, and concrete parameters accept union members",
			input: explicitTypeArgsEventPrelude + `
let first: Event = Tick{n: 1}
let both = pair(first, Shutdown{})
let both_typed: [Event] = both
let count = mixed(Shutdown{}, 1)
let holder: Holder<Event> = Holder{item: first}
let next = holder.with(Shutdown{})
let next_typed: Holder<Event> = next`,
		},
		{
			name: "or keeps a resolved union value type for a member fallback",
			input: explicitTypeArgsEventPrelude + `
let missing: Event? = Maybe::new()
mut value = missing.or(Shutdown{})
value = Tick{n: 2}
let result: Event!Str = Result::err("nope")
mut recovered = result.or(Tick{n: 1})
recovered = Shutdown{}
let open = Maybe::new().or(Shutdown{})
let open_typed: Shutdown = open`,
		},
	})
}

func TestExplicitTypeArgsStillRejectIncompatibleArguments(t *testing.T) {
	run(t, []test{
		{
			name: "Maybe::new rejects a value outside the explicit type",
			input: explicitTypeArgsEventPrelude + `
let value = Maybe::new<Event>(1)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Event, got Int"},
			},
		},
		{
			name: "Maybe::new reports an incompatible nested Maybe once",
			input: explicitTypeArgsEventPrelude + `
let value = Maybe::new<Event>(Maybe::new(1))`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Event?, got Int?"},
			},
		},
		{
			name: "user generic function rejects a value outside the explicit type",
			input: explicitTypeArgsEventPrelude + `
let value = id<Int>("text")`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "type mismatch: expected Int, got Str"},
			},
		},
		{
			name: "a later union argument does not widen an earlier member binding",
			input: explicitTypeArgsEventPrelude + `
let first: Event = Tick{n: 1}
let both = pair(Shutdown{}, first)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "type mismatch: expected Shutdown, got Event"},
			},
		},
		{
			name: "collections of members stay invariant under an explicit union type",
			input: explicitTypeArgsEventPrelude + `
let members: [Shutdown] = [Shutdown{}]
let items = listy<Event>(members, Tick{n: 1})`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "type mismatch: expected Event, got Shutdown"},
			},
		},
	})
}

// The nested Maybe::new is checked once without context and again against the
// explicit Event?; only the contextual attempt may remain in the span index,
// so hover reports Event? rather than the first attempt's Shutdown?.
func TestExplicitTypeArgsNestedMaybeSpanUsesContextualType(t *testing.T) {
	spans := checkWithSpans(t, `struct Shutdown {}
struct Tick { n: Int }
type Event = Shutdown | Tick

fn main() {
  let value = Maybe::new<Event>(Maybe::new(Shutdown{}))
}
`)
	// Only the inner call's own span (it starts at column 33) is inspected;
	// the outer Maybe::new<Event> span also covers this point.
	var types []string
	for _, rec := range spans.At(parse.Point{Row: 6, Col: 35}) {
		if _, ok := rec.Node.(*checker.ModuleFunctionCall); ok && rec.Loc.Start.Col == 33 {
			types = append(types, rec.Node.Type().String())
		}
	}
	if len(types) != 1 || types[0] != "Event?" {
		t.Fatalf("nested Maybe::new call span types = %v, want exactly [Event?]", types)
	}
}
