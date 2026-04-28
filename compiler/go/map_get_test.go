package ardgo

import "testing"

func TestMapGetSome(t *testing.T) {
	value := MapGet(map[string]int{"a": 1}, "a")
	if !value.IsSome() {
		t.Fatalf("expected map get to return some value")
	}
	if got := value.Or(0); got != 1 {
		t.Fatalf("expected map get to return 1, got %d", got)
	}
}

func TestMapGetNone(t *testing.T) {
	value := MapGet(map[string]int{"a": 1}, "b")
	if !value.IsNone() {
		t.Fatalf("expected missing key to return none")
	}
	if got := value.Or(9); got != 9 {
		t.Fatalf("expected fallback 9, got %d", got)
	}
}
