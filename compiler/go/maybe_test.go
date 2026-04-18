package ardgo

import "testing"

func TestMaybeSomeAndNone(t *testing.T) {
	some := Some(42)
	if some.IsNone() {
		t.Fatalf("expected Some to not be none")
	}
	if !some.IsSome() {
		t.Fatalf("expected Some to be some")
	}
	if got := some.Or(100); got != 42 {
		t.Fatalf("expected Or to return wrapped value, got %d", got)
	}
	if got := some.Expect("boom"); got != 42 {
		t.Fatalf("expected Expect to return wrapped value, got %d", got)
	}

	none := None[int]()
	if !none.IsNone() {
		t.Fatalf("expected None to be none")
	}
	if none.IsSome() {
		t.Fatalf("expected None to not be some")
	}
	if got := none.Or(100); got != 100 {
		t.Fatalf("expected Or to return default value, got %d", got)
	}
}

func TestMaybeExpectPanicsOnNone(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic")
		}
		if recovered != "boom" {
			t.Fatalf("expected panic message %q, got %v", "boom", recovered)
		}
	}()

	None[int]().Expect("boom")
}

func TestMaybeMapHelpers(t *testing.T) {
	mapped := MaybeMap(Some(21), func(value int) int {
		return value * 2
	})
	if got := mapped.Or(0); got != 42 {
		t.Fatalf("expected mapped some value, got %d", got)
	}

	noneMapped := MaybeMap(None[int](), func(value int) int {
		return value * 2
	})
	if !noneMapped.IsNone() {
		t.Fatalf("expected mapped none to remain none")
	}

	chained := MaybeAndThen(Some(5), func(value int) Maybe[string] {
		return Some("done")
	})
	if got := chained.Or(""); got != "done" {
		t.Fatalf("expected chained some value %q, got %q", "done", got)
	}

	propagated := MaybeAndThen(None[int](), func(value int) Maybe[string] {
		return Some("nope")
	})
	if !propagated.IsNone() {
		t.Fatalf("expected none to propagate through and_then")
	}
}
