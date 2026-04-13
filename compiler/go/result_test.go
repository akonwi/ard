package ardgo

import "testing"

func TestResultOkAndErr(t *testing.T) {
	ok := Ok[int, string](42)
	if !ok.IsOk() {
		t.Fatalf("expected ok result to be ok")
	}
	if ok.IsErr() {
		t.Fatalf("expected ok result to not be err")
	}
	if got := ok.Or(0); got != 42 {
		t.Fatalf("expected Or to return ok value, got %d", got)
	}
	if got := ok.UnwrapOk(); got != 42 {
		t.Fatalf("expected UnwrapOk to return ok value, got %d", got)
	}

	errRes := Err[int, string]("boom")
	if errRes.IsOk() {
		t.Fatalf("expected err result to not be ok")
	}
	if !errRes.IsErr() {
		t.Fatalf("expected err result to be err")
	}
	if got := errRes.Or(7); got != 7 {
		t.Fatalf("expected Or to return fallback, got %d", got)
	}
	if got := errRes.UnwrapErr(); got != "boom" {
		t.Fatalf("expected UnwrapErr to return err value, got %q", got)
	}
}

func TestResultExpectPanicsOnErr(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic")
		}
		if recovered != "boom" {
			t.Fatalf("expected panic message %q, got %v", "boom", recovered)
		}
	}()

	Err[int, string]("bad").Expect("boom")
}

func TestResultMapHelpers(t *testing.T) {
	mapped := ResultMap(Ok[int, string](21), func(value int) int {
		return value * 2
	})
	if got := mapped.Or(0); got != 42 {
		t.Fatalf("expected mapped ok value, got %d", got)
	}

	mappedErr := ResultMapErr(Err[int, string]("bad"), func(err string) int {
		return len(err)
	})
	if got := mappedErr.UnwrapErr(); got != 3 {
		t.Fatalf("expected mapped err value 3, got %d", got)
	}

	chained := ResultAndThen(Ok[int, string](5), func(value int) Result[string, string] {
		return Ok[string, string]("done")
	})
	if got := chained.Or(""); got != "done" {
		t.Fatalf("expected chained ok value %q, got %q", "done", got)
	}

	propagated := ResultAndThen(Err[int, string]("boom"), func(value int) Result[string, string] {
		return Ok[string, string]("nope")
	})
	if got := propagated.UnwrapErr(); got != "boom" {
		t.Fatalf("expected propagated err %q, got %q", "boom", got)
	}
}
