package checker

import "testing"

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
