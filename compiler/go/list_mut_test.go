package ardgo

import "testing"

func TestListPush(t *testing.T) {
	values := []int{1, 2}
	out := ListPush(&values, 3)
	if len(values) != 3 || values[2] != 3 {
		t.Fatalf("expected push to append value, got %v", values)
	}
	if len(out) != 3 || out[2] != 3 {
		t.Fatalf("expected push return to match updated list, got %v", out)
	}
}

func TestListPrepend(t *testing.T) {
	values := []int{2, 3}
	out := ListPrepend(&values, 1)
	if len(values) != 3 || values[0] != 1 {
		t.Fatalf("expected prepend to insert at front, got %v", values)
	}
	if len(out) != 3 || out[0] != 1 {
		t.Fatalf("expected prepend return to match updated list, got %v", out)
	}
}

func TestListSet(t *testing.T) {
	values := []int{1, 2, 3}
	if ok := ListSet(values, 1, 9); !ok {
		t.Fatalf("expected list set to return true")
	}
	if values[1] != 9 {
		t.Fatalf("expected values[1] to be 9, got %d", values[1])
	}
}

func TestListSetOutOfRange(t *testing.T) {
	values := []int{1, 2, 3}
	if ok := ListSet(values, 5, 9); ok {
		t.Fatalf("expected list set out of range to return false")
	}
	if values[1] != 2 {
		t.Fatalf("expected list contents to stay unchanged, got %v", values)
	}
}

func TestListSwap(t *testing.T) {
	values := []int{1, 2, 3}
	ListSwap(values, 0, 2)
	if values[0] != 3 || values[2] != 1 {
		t.Fatalf("expected values to be swapped, got %v", values)
	}
}

func TestListSort(t *testing.T) {
	values := []int{3, 1, 2}
	ListSort(values, func(a int, b int) bool { return a < b })
	if values[0] != 1 || values[1] != 2 || values[2] != 3 {
		t.Fatalf("expected values to be sorted, got %v", values)
	}
}
