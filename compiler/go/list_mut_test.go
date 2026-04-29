package ardgo

import "testing"

func TestAnySlice(t *testing.T) {
	values := []int{1, 2, 3}
	out := AnySlice(values)
	if len(out) != 3 {
		t.Fatalf("expected 3 items, got %d", len(out))
	}
	if out[0] != 1 || out[1] != 2 || out[2] != 3 {
		t.Fatalf("expected converted values to match input, got %v", out)
	}
}
