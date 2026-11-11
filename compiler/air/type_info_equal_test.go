package air

import (
	"reflect"
	"testing"
)

// fullTypeInfo populates every TypeInfo field with a non-zero value so that
// per-field mutations below exercise each comparison branch.
func fullTypeInfo() TypeInfo {
	return TypeInfo{
		ID: 7, Kind: TypeStruct, Name: "User", ModulePath: "app/users", Private: true,
		Elem: 1, Length: 3, Key: 2, Value: 3, Error: 4,
		ForeignTarget: "go", ForeignNamespace: "net/http", ForeignQualifier: "http", ForeignSymbol: "Client",
		ForeignPointer: true, ForeignInterface: true,
		Fields: []FieldInfo{{
			Name: "name", Type: 5, Index: 0,
			JSON:   JSONFieldInfo{Name: "n", HasName: true, OmitNone: true, Skip: false},
			GoTags: []GoFieldTag{{Key: "db", Value: "name"}},
		}},
		Variants:   []VariantInfo{{Name: "A", Discriminant: 1}},
		EnumOpen:   true,
		Members:    []UnionMember{{Type: 6, Tag: 1, Name: "Int"}},
		Params:     []TypeID{1, 2},
		Return:     3,
		Variadic:   true,
		Trait:      9,
		TypeParams: []string{"T"}, ParamOwner: "List", ParamIndex: 1,
		Generic: 8, GenericArgs: []TypeID{4}, GenericComparable: []bool{true},
	}
}

// TestTypeInfoEqualMatchesDeepEqual guards the hand-written equality against
// drift from TypeInfo: it fails when a field is added without updating
// typeInfoEqual, and it checks that mutating any single field is detected.
func TestTypeInfoEqualMatchesDeepEqual(t *testing.T) {
	const knownFields = 30
	if n := reflect.TypeOf(TypeInfo{}).NumField(); n != knownFields {
		t.Fatalf("TypeInfo has %d fields, test knows %d: update typeInfoEqual and this test", n, knownFields)
	}

	base := fullTypeInfo()
	if !typeInfoEqual(base, fullTypeInfo()) {
		t.Fatal("identical TypeInfo values must compare equal")
	}

	baseValue := reflect.ValueOf(base)
	for i := 0; i < baseValue.NumField(); i++ {
		field := reflect.TypeOf(base).Field(i)
		mutated := fullTypeInfo()
		target := reflect.ValueOf(&mutated).Elem().Field(i)
		switch target.Kind() {
		case reflect.Slice:
			target.Set(reflect.Zero(target.Type()))
		case reflect.Bool:
			target.SetBool(!target.Bool())
		case reflect.String:
			target.SetString(target.String() + "x")
		case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint8, reflect.Uint32:
			target.Set(reflect.Zero(target.Type()))
		default:
			t.Fatalf("unhandled kind %s for field %s", target.Kind(), field.Name)
		}
		want := reflect.DeepEqual(base, mutated)
		if got := typeInfoEqual(base, mutated); got != want {
			t.Errorf("field %s: typeInfoEqual=%v, DeepEqual=%v", field.Name, got, want)
		}
		if want {
			t.Errorf("field %s: mutation did not change the value", field.Name)
		}
	}
}

func TestTypeInfoEqualDistinguishesNilAndEmptySlices(t *testing.T) {
	a := TypeInfo{Params: nil}
	b := TypeInfo{Params: []TypeID{}}
	if typeInfoEqual(a, b) != reflect.DeepEqual(a, b) {
		t.Fatal("nil and empty slices must compare like reflect.DeepEqual")
	}
	c := TypeInfo{Fields: []FieldInfo{{GoTags: nil}}}
	d := TypeInfo{Fields: []FieldInfo{{GoTags: []GoFieldTag{}}}}
	if typeInfoEqual(c, d) != reflect.DeepEqual(c, d) {
		t.Fatal("nested nil and empty slices must compare like reflect.DeepEqual")
	}
}

func TestTypeInfoEqualDetectsNestedFieldChanges(t *testing.T) {
	base := fullTypeInfo()
	cases := map[string]func(*TypeInfo){
		"field name":   func(ti *TypeInfo) { ti.Fields[0].Name = "other" },
		"field type":   func(ti *TypeInfo) { ti.Fields[0].Type = 99 },
		"field index":  func(ti *TypeInfo) { ti.Fields[0].Index = 5 },
		"field json":   func(ti *TypeInfo) { ti.Fields[0].JSON.Skip = true },
		"field go tag": func(ti *TypeInfo) { ti.Fields[0].GoTags[0].Value = "x" },
		"variant":      func(ti *TypeInfo) { ti.Variants[0].Discriminant = 2 },
		"member":       func(ti *TypeInfo) { ti.Members[0].Tag = 2 },
		"param":        func(ti *TypeInfo) { ti.Params[1] = 7 },
		"generic arg":  func(ti *TypeInfo) { ti.GenericArgs[0] = 1 },
		"generic cmp":  func(ti *TypeInfo) { ti.GenericComparable[0] = false },
		"type param":   func(ti *TypeInfo) { ti.TypeParams[0] = "U" },
		"slice length": func(ti *TypeInfo) { ti.Params = ti.Params[:1] },
	}
	for name, mutate := range cases {
		mutated := fullTypeInfo()
		mutate(&mutated)
		if typeInfoEqual(base, mutated) {
			t.Errorf("%s: change not detected", name)
		}
		if reflect.DeepEqual(base, mutated) {
			t.Errorf("%s: test mutation is a no-op", name)
		}
	}
}
