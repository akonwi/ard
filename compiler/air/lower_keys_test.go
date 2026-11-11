package air

import (
	"fmt"
	"testing"
)

// TestFunctionKeysMatchFormattedSpelling pins the hand-built function keys to
// the fmt spelling they replaced: keys are map identities for deduplicating
// declared functions, so their bytes must not drift.
func TestFunctionKeysMatchFormattedSpelling(t *testing.T) {
	signature := Signature{
		Params: []Param{{Name: "a", Type: 3, ABI: 1}, {Name: "b", Type: 12, ABI: 0}},
		Return: 7,
	}
	wantSignature := fmt.Sprintf("(%d:%d,%d:%d)->%d", 3, 1, 12, 0, 7)
	if got := signatureKey(signature); got != wantSignature {
		t.Fatalf("signatureKey = %q, want %q", got, wantSignature)
	}
	if got := signatureKey(Signature{Return: NoType}); got != fmt.Sprintf("()->%d", NoType) {
		t.Fatalf("empty signatureKey = %q", got)
	}
	if got, want := concreteFunctionKey(4, "mod::f", signature, ""), fmt.Sprintf("%d:%s:%s", 4, "mod::f", wantSignature); got != want {
		t.Fatalf("concreteFunctionKey = %q, want %q", got, want)
	}
	if got, want := concreteFunctionKey(4, "mod::f", signature, "T=9"), fmt.Sprintf("%d:%s:%s:%s", 4, "mod::f", wantSignature, "T=9"); got != want {
		t.Fatalf("generic concreteFunctionKey = %q, want %q", got, want)
	}
}
