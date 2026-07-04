# 0042: Support Dynamic Foreign Type Tests

## Status

Proposed

## Context

Idiomatic Go libraries frequently deliver events and messages as interface values and dispatch on their dynamic type with a type switch. The vaxis UI demo is a representative case:

```go
OnEvent: func(ctx ui.EventContext, ev ui.Event) ui.EventResult {
	switch ev := ev.(type) {
	case term.EventNotify:
		ctx.Notify(ev.Title, ev.Body)
		return ui.EventHandled
	case term.EventTitle:
		ctx.SetTitle(string(ev))
		return ui.EventHandled
	}
	return ui.EventIgnored
}
```

The concrete types involved are ordinary Go named types: `term.EventTitle` is a string newtype and `term.EventNotify` is a plain struct. The interface is not method-bearing at all — `ui.Event` is an alias for `vaxis.Event`, which is `interface{}`. So this pattern is a dynamic type test over `any`, and Ard currently has no way to express it:

- `ui::Event` fails to resolve because Go type aliases are only supported over primitive underlyings; an alias of an empty interface is rejected.
- `unsafe::cast<T>` (ADR 0036/0037) only accepts Ard-representable target types reachable from `Any`; foreign Go named types are not supported as cast targets.
- Ard `match` has no type patterns; union matching is closed and checked, which is a different feature.

Without this, event-driven Go frameworks (vaxis, bubbletea, tcell, fsnotify, etc.) cannot be driven from pure Ard, and every event handler needs a Go shim whose only job is a type switch.

## Decision

Support dynamic type tests against foreign Go types in two layers: a primitive checked cast, and match sugar over it.

### Empty-interface aliases map to `Any`

A Go type alias whose underlying type is the empty interface maps to `Any`, the same as a direct `any`/`interface{}`. `ui::Event` therefore resolves and values of that type are ordinary `Any` values in Ard.

Named (non-alias) empty-interface types also map to `Any`. There is no method set to preserve, so an opaque foreign type would only add friction.

### Phase 1: `unsafe::cast` accepts foreign Go types

Extend `unsafe::cast<T>(value: Any) T?` to accept foreign Go named types as the target:

```ard
use ard/unsafe
use go:go.rockorager.dev/vaxis/widgets/term

let notify = unsafe::cast<term::EventNotify>(ev)   // (term::EventNotify)?
```

Rules:

- `cast<T>` where `T` is a foreign Go named type lowers to a Go type assertion `value.(pkg.T)`, returning `some` on success and `none` on failure.
- `cast<mut T>` lowers to a pointer assertion `value.(*pkg.T)`, with nil pointers returning `none`, consistent with ADR 0036.
- The source may be `Any` (including values typed by an empty-interface alias) or a foreign Go interface value; Go permits type assertions on any interface value, and the lowering follows Go semantics.
- No coercion is performed. A boxed `term.EventTitle` does not cast to `Str`; it casts to `term::EventTitle`, whose named-scalar underlying then converts by the existing foreign named-scalar rules.
- Casting between unrelated foreign types simply returns `none`; there is no panic path.

This stays in `ard/unsafe` because a partial assertion invites `.expect()` misuse — the caller can turn a failed test into a runtime failure.

### Phase 2: match type patterns over dynamic subjects

Add type patterns to `match` when the subject is dynamic — `Any` or a foreign Go interface type:

```ard
OnEvent: fn(ctx: ui::EventContext, ev: ui::Event) ui::EventResult {
  match ev {
    term::EventNotify(e) => {
      ctx.Notify(e.Title, e.Body)
      ui::EventHandled
    },
    term::EventTitle(t) => {
      ctx.SetTitle(t)
      ui::EventHandled
    },
    _ => ui::EventIgnored,
  }
}
```

Rules:

- Type patterns are only valid when the subject's static type is `Any` or a foreign Go interface type. Ard unions keep their existing closed, checked matching; this feature does not apply to them.
- A pattern names a foreign Go type (or `mut` foreign type for the pointer form) and binds the narrowed value: `term::EventNotify(e)` binds `e` with type `term::EventNotify`.
- The dynamic type set is open, so a catch-all `_` arm is required, mirroring the existing rule for imported Go enum-like constants.
- Exhaustiveness is not checked beyond the catch-all requirement. Duplicate type patterns are diagnosed as unreachable.
- The whole match lowers to a single Go type switch, with each arm becoming a `case pkg.T:` clause.

Match type patterns do not require an `unsafe` marker. The distinction is deliberate: `unsafe::cast` is a partial operation whose result can be forced, while a catch-all-required match is total — a failed test falls through to another arm, with no panic and no misrepresented type. The `match` form is dynamic but not unsafe.

### What this is not

- Not a general runtime reflection or introspection API. The only observable is "does this value have exactly this dynamic type."
- Not applicable to Ard-owned types. Boxing an Ard struct into `Any` and recovering it uses the existing ADR 0036 rules; type patterns over Ard types in `match` are not added by this decision.
- Not interface-to-interface narrowing. Patterns and cast targets are concrete named Go types (value or pointer form). Asserting from one Go interface to another Go interface is deferred until a use case demands it.

## Consequences

- Event-driven Go frameworks become usable from pure Ard: handlers receive interface values and dispatch on concrete event types without a Go shim.
- `ui::Event` and similar empty-interface aliases resolve as `Any`, removing a class of "Unrecognized type" failures.
- `unsafe::cast` gains foreign targets with Go type-assertion semantics, staying consistent with the existing nil and pointer rules.
- Match gains a dynamic form, but only for `Any`/foreign-interface subjects and only with a mandatory catch-all, preserving the closed semantics of union matching.
- The vaxis demo's `OnEvent` handler ports as written, completing another gap in the full-compatibility port.
- Backends other than Go must implement a dynamic type-test primitive to support this; targets without one would need boxing metadata.

## Related

- `docs/adrs/0036-define-any-casting-policy.md`
- `docs/adrs/0037-define-unsafe-nil-interop-policy.md`
- `docs/adrs/0039-support-explicit-go-interface-interop.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `compiler/checker/go_import.go`
