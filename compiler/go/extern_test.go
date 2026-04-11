package ardgo

import "testing"

func TestExternRegistryCall(t *testing.T) {
	registry := NewExternRegistry()
	registry.Register("Echo", func(args ...any) (any, error) {
		if len(args) != 1 {
			t.Fatalf("expected one arg, got %d", len(args))
		}
		return args[0], nil
	})

	result, err := registry.Call("Echo", 42)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
	if result != 42 {
		t.Fatalf("expected 42, got %v", result)
	}
}

func TestExternRegistryMissingBinding(t *testing.T) {
	registry := NewExternRegistry()
	_, err := registry.Call("Missing")
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "extern function not found: Missing" {
		t.Fatalf("unexpected error: %v", err)
	}
}
