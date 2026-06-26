package ffi

import "testing"

type isNilTestValue struct{}

func TestIsNilDetectsTypedNilValues(t *testing.T) {
	var ptr *isNilTestValue
	if !IsNil(ptr) {
		t.Fatal("IsNil returned false for a typed nil pointer")
	}

	if IsNil(&isNilTestValue{}) {
		t.Fatal("IsNil returned true for a non-nil pointer")
	}
}

func TestDynamicToMapReturnsDynamicKeyMap(t *testing.T) {
	got, err := DynamicToMap(map[string]any{"name": "ard", "count": 2})
	if err != nil {
		t.Fatalf("DynamicToMap returned error: %v", err)
	}
	if got["name"] != "ard" || got["count"] != 2 {
		t.Fatalf("DynamicToMap = %#v, want string-keyed values preserved", got)
	}

	got, err = DynamicToMap(map[any]any{"ok": true})
	if err != nil {
		t.Fatalf("DynamicToMap map[any]any returned error: %v", err)
	}
	if got["ok"] != true {
		t.Fatalf("DynamicToMap map[any]any = %#v, want ok=true", got)
	}

	if _, err := DynamicToMap(map[any]any{1: "bad"}); err == nil {
		t.Fatalf("DynamicToMap accepted non-string dynamic map key")
	}
}
