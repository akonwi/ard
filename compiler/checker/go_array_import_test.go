package checker_test

import "testing"

func TestGoFixedArraysMapToFixedArrayTypes(t *testing.T) {
	run(t, []test{
		{
			name: "go array return maps to fixed array",
			input: `use go:crypto/sha256
fn digest() [Byte; 32] {
  mut bytes = "hello".bytes()
  sha256::Sum256(mut bytes)
}`,
		},
	})
}
