package vm

import "testing"

func TestValueNativeExternBindingFloatFloor(t *testing.T) {
	got := runBytecode(t, `
		let floored = Float::floor(3.75)
		floored + 0.25
	`)

	if got != 3.25 {
		t.Fatalf("expected 3.25, got %v", got)
	}
}
