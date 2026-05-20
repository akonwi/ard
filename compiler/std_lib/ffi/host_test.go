package ffi

import "testing"

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

func TestNewHostFunctionsInjectsOSArgs(t *testing.T) {
	injected := []string{"ard", "run", "main.ard", "one"}
	functions := NewHostFunctions(HostConfig{Args: injected})
	injected[0] = "mutated"
	osArgs, ok := functions["OsArgs"].(func() []string)
	if !ok {
		t.Fatalf("OsArgs has type %T, want func() []string", functions["OsArgs"])
	}

	args := osArgs()
	want := []string{"ard", "run", "main.ard", "one"}
	if len(args) != len(want) {
		t.Fatalf("OsArgs len = %d, want %d (%v)", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("OsArgs[%d] = %q, want %q (%v)", i, args[i], want[i], args)
		}
	}

	args[0] = "mutated"
	if got := osArgs()[0]; got != "ard" {
		t.Fatalf("OsArgs returned mutable backing storage; got first arg %q", got)
	}
}
