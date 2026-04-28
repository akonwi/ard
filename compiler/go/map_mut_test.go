package ardgo

import "testing"

func TestMapSet(t *testing.T) {
	values := map[string]int{"a": 1}
	if ok := MapSet(values, "b", 2); !ok {
		t.Fatalf("expected map set to return true")
	}
	if got := values["b"]; got != 2 {
		t.Fatalf("expected key b to be set to 2, got %d", got)
	}
}

func TestMapDrop(t *testing.T) {
	values := map[string]int{"a": 1, "b": 2}
	MapDrop(values, "a")
	if _, exists := values["a"]; exists {
		t.Fatalf("expected key a to be removed")
	}
	if got := values["b"]; got != 2 {
		t.Fatalf("expected key b to remain 2, got %d", got)
	}
}
