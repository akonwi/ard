package checker

import "testing"

func TestStrAPI(t *testing.T) {
	size := Str{}.GetProperty("size")
	if size != (Num{}) {
		t.Fatalf("Str::size should be Num, got %s", size)
	}
}

func TestNumAPI(t *testing.T) {
	as_str := Num{}.GetProperty("as_str")
	if as_str != (Str{}) {
		t.Fatalf("Num::as_str should be Str, got %s", as_str)
	}
}

func TestBoolAPI(t *testing.T) {
	as_str := Bool{}.GetProperty("as_str")
	if as_str != (Str{}) {
		t.Fatalf("Bool::as_str should be Str, got %s", as_str)
	}
}
