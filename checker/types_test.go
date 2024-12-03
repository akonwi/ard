package checker

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestNumEquality(t *testing.T) {
	if NumType.Equals(StrType) {
		t.Errorf("Num != Str")
	}
	if NumType.Equals(BoolType) {
		t.Errorf("Num != Bool")
	}
	if NumType.Equals(MakeList(NumType)) {
		t.Errorf("Num != [Num]")
	}
	if NumType.Equals(MakeMap(NumType)) {
		t.Errorf("Num != [Str:Num]")
	}
	if !NumType.Equals(NumType) {
		t.Errorf("Num == Num")
	}
}

func TestStrEquality(t *testing.T) {
	if !StrType.Equals(StrType) {
		t.Errorf("Str == Str")
	}
	if StrType.Equals(NumType) {
		t.Errorf("Str != Num")
	}
	if StrType.Equals(BoolType) {
		t.Errorf("Str != Bool")
	}
	if StrType.Equals(MakeList(NumType)) {
		t.Errorf("Str != [Num]")
	}
	if StrType.Equals(MakeMap(NumType)) {
		t.Errorf("Str != [Str:Num]")
	}
}

func TestBoolEquality(t *testing.T) {
	if !BoolType.Equals(BoolType) {
		t.Errorf("Bool == Bool")
	}
	if BoolType.Equals(NumType) {
		t.Errorf("Bool != Num")
	}
	if BoolType.Equals(StrType) {
		t.Errorf("Bool != Str")
	}
	if BoolType.Equals(MakeList(NumType)) {
		t.Errorf("Bool != [Num]")
	}
	if BoolType.Equals(MakeMap(NumType)) {
		t.Errorf("Bool != [Str:Num]")
	}
}

func TestListEquality(t *testing.T) {
	strList := MakeList(StrType)
	numList := MakeList(NumType)
	if !strList.Equals(strList) {
		t.Errorf("[Str] == [Str]")
	}
	if !numList.Equals(MakeList(NumType)) {
		t.Errorf("[Num] == [Num]")
	}
	if strList.Equals(numList) {
		t.Errorf("[Str] != [Num]")
	}
	if !strList.Equals(MakeList(nil)) {
		t.Errorf("[Str] == [?]")
	}
	if strList.Equals(MakeList(BoolType)) {
		t.Errorf("[Str] != [Bool]")
	}
}

func TestMapEquality(t *testing.T) {
	strToNumMap := MakeMap(NumType)
	strToStrMap := MakeMap(StrType)
	emptyMap := MakeMap(nil)
	if !strToNumMap.Equals(strToNumMap) {
		t.Errorf("[Str:Num] == [Str:Num]")
	}
	if !strToNumMap.Equals(emptyMap) {
		t.Errorf("[Str:Num] == [Str:?]")
	}
	if strToNumMap.Equals(strToStrMap) {
		t.Errorf("[Str:Num] != [Str:Str]")
	}
}

func TestEnumEquality(t *testing.T) {
	colorEnum := EnumType{Name: "Color", Variants: map[string]int{"Red": 0, "Green": 1}}
	placeEnum := EnumType{Name: "Place", Variants: map[string]int{"Office": 0, "Home": 1}}
	if !colorEnum.Equals(colorEnum) {
		t.Errorf("%s == %s", colorEnum, colorEnum)
	}
	if colorEnum.Equals(placeEnum) {
		t.Errorf("%s != %s", colorEnum, placeEnum)
	}
}

func TestGenerics(t *testing.T) {
	Foo := GenericType{name: "T"}
	if !Foo.Equals(NumType) {
		t.Errorf("T? == Num")
	}

	Foo.Fill(StrType)
	if !Foo.Equals(StrType) {
		t.Errorf("Str == Str")
	}
}

func TestListApi(t *testing.T) {
	str_list := MakeList(StrType)

	map_method := str_list.GetProperty("map").(FunctionType)
	expectedMap := FunctionType{
		Name:       "map",
		Mutates:    false,
		Parameters: []Type{FunctionType{Name: "callback", Parameters: []Type{StrType}, ReturnType: GenericType{name: "Out"}}},
		ReturnType: MakeList(GenericType{name: "Out"}),
	}
	if diff := cmp.Diff(expectedMap, map_method, cmpopts.IgnoreUnexported(GenericType{})); diff != "" {
		t.Errorf("List.map signature does not match (-want +got):\n%s", diff)
	}

	pop_method := str_list.GetProperty("pop").(FunctionType)
	expectedPop := FunctionType{
		Name:       "pop",
		Mutates:    true,
		Parameters: []Type{},
		ReturnType: str_list.ItemType,
	}
	if diff := cmp.Diff(expectedPop, pop_method); diff != "" {
		t.Errorf("List.pop signature does not match (-want +got):\n%s", diff)
	}

	push_method := str_list.GetProperty("push").(FunctionType)
	expectedPush := FunctionType{
		Name:       "push",
		Mutates:    true,
		Parameters: []Type{str_list.ItemType},
		ReturnType: NumType,
	}
	if diff := cmp.Diff(expectedPush, push_method); diff != "" {
		t.Errorf("List.push signature does not match (-want +got):\n%s", diff)
	}

	if str_list.GetProperty("size") != NumType {
		t.Errorf("List.size should be Num")
	}
}
