package checker

import "testing"

func TestCoherenceChecks(t *testing.T) {
	type test struct {
		a    Type
		b    Type
		want bool
	}

	Shape := Struct{Name: "Shape", Fields: map[string]Type{"height": Int{}, "width": Int{}}}
	NumOrStr := Union{name: "NumOrStr", types: []Type{Int{}, Str{}}}

	tests := []test{
		{Int{}, Int{}, true},
		{Int{}, Float{}, false},
		{Int{}, Str{}, false},
		{Int{}, Bool{}, false},
		{Int{}, List{Int{}}, false},
		{List{}, Str{}, false},
		{List{&Shape}, List{&Shape}, true},
		{List{Str{}}, List{Str{}}, true},
		{List{Int{}}, List{Int{}}, true},
		{List{Int{}}, List{Str{}}, false},
		{Int{}, Option{Int{}}, false},
		{Option{}, Option{Int{}}, true},
		{Option{Int{}}, Option{}, true},
		{NumOrStr, Int{}, true},
		{NumOrStr, Bool{}, false},
		{NumOrStr, NumOrStr, true},
		{List{NumOrStr}, List{NumOrStr}, true},
		{Any{}, Any{}, true},
		{Any{}, Int{}, true},
		{Int{}, Any{}, true},
		{Int{}, Any{Str{}}, false},
		{Any{Str{}}, Int{}, false},
	}
	for _, tt := range tests {
		if res := AreCoherent(tt.a, tt.b); res != tt.want {
			t.Errorf("%s == %s: want %v, got %v", tt.a, tt.b, tt.want, res)
		}
	}
}

func TestStrAPI(t *testing.T) {
	want := function{name: "size", parameters: []variable{}, returns: Int{}}
	size := Str{}.GetProperty("size")
	if !AreCoherent(want, size) {
		t.Fatalf("Str::size() should be %s, got %s", want, size)
	}
}

func TestNumAPI(t *testing.T) {
	want := function{name: "to_str", parameters: []variable{}, returns: Str{}}
	as_str := Int{}.GetProperty("to_str")
	if !AreCoherent(want, as_str) {
		t.Fatalf("Int::to_str() should be %s, got %s", want, as_str)
	}
}

func TestBoolAPI(t *testing.T) {
	want := function{name: "to_str", parameters: []variable{}, returns: Str{}}
	to_str := Bool{}.GetProperty("to_str")
	if !AreCoherent(want, to_str) {
		t.Fatalf("Bool::to_str() should be %s, got %s", want, to_str)
	}
}

func TestListAPI(t *testing.T) {
	want := function{name: "size", parameters: []variable{}, returns: Int{}}
	size := (List{element: Int{}}).GetProperty("size")
	if !AreCoherent(want, size) {
		t.Fatalf("List::size should be %s, got %s", want, size)
	}

	push := (List{element: Int{}}).GetProperty("push")
	want = function{name: "push", parameters: []variable{{name: "item", _type: Int{}}}, returns: Int{}}
	if !AreCoherent(want, push) {
		t.Fatalf("List::push should be %s, got %s", want, push)
	}
}

func TestEnumTypes(t *testing.T) {
	Kind := Enum{Name: "Kind", Variants: []string{"Good", "Bad"}}
	good, _ := Kind.GetVariant("Good")

	if good.GetType().String() != Kind.String() {
		t.Errorf("%s is %s, got %s", good, Kind, good.GetType())
	}
	if !AreCoherent(Kind, good.GetType()) {
		t.Errorf("%s should allow %s", Kind, good.GetType())
	}
}
