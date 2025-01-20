package checker

import "testing"

func TestTypeEquality(t *testing.T) {
	type test struct {
		a    Type
		b    Type
		want bool
	}

	Shape := Struct{Name: "Shape", Fields: map[string]Type{"height": Num{}, "width": Num{}}}

	tests := []test{
		{Num{}, Num{}, true},
		{Num{}, Str{}, false},
		{Num{}, Bool{}, false},
		{Num{}, List{Num{}}, false},
		{List{&Shape}, List{&Shape}, true},
		{Num{}, Option{Num{}}, false},
		{Option{}, Option{Num{}}, true},
		{Option{Num{}}, Option{}, true},
	}
	for _, tt := range tests {
		if res := tt.a.Is(tt.b); res != tt.want {
			t.Errorf("%s == %s: want %v, got %v", tt.a, tt.b, tt.want, res)
		}
	}
}

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

func TestListAPI(t *testing.T) {
	list := List{element: Num{}}
	size := list.GetProperty("size")
	if size != (Num{}) {
		t.Fatalf("List::size should be Num, got %s", size)
	}

	push := list.GetProperty("push")
	expected := function{name: "push", parameters: []variable{{name: "item", _type: list.element}}, returns: Num{}}
	if !push.Is(expected) {
		t.Fatalf("List::push should be %s, got %s", expected, push)
	}
}

func TestListType(t *testing.T) {
	strList := makeList(Str{})
	numList := makeList(Num{})
	if !strList.Is(strList) {
		t.Errorf("[Str] == [Str]")
	}
	if !numList.Is(makeList(Num{})) {
		t.Errorf("[Num] == [Num]")
	}
	if strList.Is(numList) {
		t.Errorf("[Str] != [Num]")
	}
	if !strList.Is(makeList(nil)) {
		t.Errorf("[Str] == [?]")
	}
	if strList.Is(makeList(Bool{})) {
		t.Errorf("[Str] != [Bool]")
	}
}

func TestEnumTypes(t *testing.T) {
	Kind := Enum{Name: "Kind", Variants: []string{"Good", "Bad"}}
	good, _ := Kind.GetVariant("Good")

	if good.GetType().String() != Kind.String() {
		t.Errorf("%s is %s, got %s", good, Kind, good.GetType())
	}
	if !Kind.Is(good.GetType()) {
		t.Errorf("%s allows %s", Kind, good.GetType())
	}
}
